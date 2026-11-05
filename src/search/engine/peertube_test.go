package engine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/apimgr/search/src/model"
)

type peerTubeRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip peerTubeRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestPeerTubeSearchBuildsSameOriginPreview(t *testing.T) {
	engine := NewPeerTubeEngine()
	engine.client.Transport = peerTubeRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != peerTubeInstance {
			t.Fatalf("unexpected PeerTube request: %s", request.URL)
		}
		body := `{"total":1,"data":[{"name":"Fixture clip","description":"A public video","uuid":"bd48085e-e07d-406f-995a-7176f11590ed","url":"https://peertube.tv/w/fixture","duration":120,"views":42,"isLocal":true,"thumbnailPath":"https://peertube.tv/lazy-static/previews/fixture.jpg","channel":{"displayName":"Fixture channel"}}]}`
		if request.URL.Path == "/api/v1/search/videos" && request.URL.Query().Get("hasWebVideoFiles") != "true" {
			t.Fatalf("search request missing web video file filter: %s", request.URL)
		}
		if request.URL.Path == "/api/v1/videos/bd48085e-e07d-406f-995a-7176f11590ed" {
			body = `{"files":[{"fileUrl":"https://peertube.tv/static/web-videos/fixture.mp4"}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	results, err := engine.Search(context.Background(), &model.Query{Text: "fixture", Category: model.CategoryVideos})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].PreviewURL != "/media/preview/peertube/fixture.mp4" {
		t.Fatalf("results = %#v", results)
	}
}

func TestPeerTubeSearchRejectsUntrustedMediaHosts(t *testing.T) {
	if got := safePeerTubePreview("https://evil.example/static/web-videos/clip.mp4"); got != "" {
		t.Fatalf("untrusted preview URL = %q", got)
	}
	if got := safePeerTubePreview("https://peertube.tv/api/v1/users/clip.mp4"); got != "" {
		t.Fatalf("unexpected path preview URL = %q", got)
	}
}
