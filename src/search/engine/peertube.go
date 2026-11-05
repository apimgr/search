package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

const peerTubeInstance = "peertube.tv"

var peerTubeUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var peerTubeVideoFilename = regexp.MustCompile(`^[A-Za-z0-9_-]+\.(mp4|webm)$`)

type PeerTube struct {
	*search.BaseEngine
	client *http.Client
}

func NewPeerTubeEngine() *PeerTube {
	config := model.NewEngineConfig("peertube")
	config.DisplayName = "PeerTube"
	config.Priority = 45
	config.Categories = []string{"videos"}
	config.SupportsTor = false
	return &PeerTube{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{Timeout: 10 * time.Second, Transport: SharedTransport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func (e *PeerTube) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	if query == nil || strings.TrimSpace(query.Text) == "" {
		return nil, fmt.Errorf("peertube query is required")
	}
	params := url.Values{}
	params.Set("search", strings.TrimSpace(query.Text))
	count := e.GetConfig().GetMaxResults()
	if count > 10 {
		count = 10
	}
	params.Set("count", strconv.Itoa(count))
	params.Set("hasWebVideoFiles", "true")
	params.Set("nsfw", "false")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+peerTubeInstance+"/api/v1/search/videos?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", UserAgent)
	response, err := e.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peertube returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return nil, fmt.Errorf("peertube response exceeds the 4 MiB limit")
	}
	var payload struct {
		Total int `json:"total"`
		Data  []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			UUID        string `json:"uuid"`
			URL         string `json:"url"`
			Duration    int    `json:"duration"`
			Views       int64  `json:"views"`
			IsLocal     bool   `json:"isLocal"`
			Thumbnail   string `json:"thumbnailPath"`
			Channel     struct {
				DisplayName string `json:"displayName"`
			} `json:"channel"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode peertube response: %w", err)
	}
	results := make([]model.Result, 0, len(payload.Data))
	for _, item := range payload.Data {
		videoURL := safePeerTubeVideoURL(item.URL)
		if item.Name == "" || videoURL == "" {
			continue
		}
		result := model.Result{
			Title: item.Name, URL: videoURL, Content: item.Description, Engine: e.Name(),
			Category: model.CategoryVideos, Duration: item.Duration, ViewCount: item.Views,
			Author: item.Channel.DisplayName, Thumbnail: safePeerTubeThumbnail(item.Thumbnail),
		}
		if item.IsLocal {
			if preview := e.fetchLocalPreview(ctx, item.UUID); preview != "" {
				result.PreviewURL = preview
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func (e *PeerTube) fetchLocalPreview(ctx context.Context, videoID string) string {
	if !peerTubeUUID.MatchString(videoID) {
		return ""
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+peerTubeInstance+"/api/v1/videos/"+url.PathEscape(videoID), nil)
	if err != nil {
		return ""
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", UserAgent)
	response, err := e.client.Do(request)
	if err != nil {
		return ""
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return ""
	}
	var payload struct {
		Files []struct {
			FileURL string `json:"fileUrl"`
		} `json:"files"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, file := range payload.Files {
		if preview := safePeerTubePreview(file.FileURL); preview != "" {
			return preview
		}
	}
	return ""
}

func safePeerTubeVideoURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func safePeerTubeThumbnail(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, peerTubeInstance) || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func safePeerTubePreview(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, peerTubeInstance) || parsed.User != nil {
		return ""
	}
	if !strings.HasPrefix(parsed.Path, "/static/web-videos/") {
		return ""
	}
	filename := path.Base(parsed.Path)
	if !peerTubeVideoFilename.MatchString(filename) {
		return ""
	}
	return "/media/preview/peertube/" + url.PathEscape(filename)
}
