package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"golang.org/x/image/webp"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/instant"
)

const maxImageProxyDimension = 8192
const maxImageProxyPixels = 16 * 1024 * 1024

var wikipediaImageSource = regexp.MustCompile(`(?i)(src=["'])((?:https://upload\.wikimedia\.org/)[^"']+)(["'])`)

type imageProxyHandler struct {
	client  *http.Client
	maxSize int64
	key     []byte
}

func newImageProxyHandler(cfg config.ImageProxyConfig, key string) *imageProxyHandler {
	maxSize := cfg.MaxSize
	if maxSize <= 0 || maxSize > 10<<20 {
		maxSize = 10 << 20
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid image host")
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, fmt.Errorf("image host could not be resolved")
			}
			for _, candidate := range ips {
				if !isPublicImageIP(candidate.IP) {
					return nil, fmt.Errorf("image host resolved to a non-public address")
				}
			}
			dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
			var dialErr error
			for _, candidate := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				dialErr = err
			}
			return nil, dialErr
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConnsPerHost:   2,
	}
	return &imageProxyHandler{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		maxSize: maxSize,
		key:     []byte(key),
	}
}

func (h *imageProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	u, err := validateImageProxyURL(r.URL.Query().Get("url"))
	if err != nil || !validImageProxySignature(r.URL.Query().Get("url"), r.URL.Query().Get("sig"), h.key) {
		http.Error(w, "invalid image URL", http.StatusBadRequest)
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		http.Error(w, "invalid image URL", http.StatusBadRequest)
		return
	}
	request.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif")
	response, err := h.client.Do(request)
	if err != nil {
		http.Error(w, "image source unavailable", http.StatusBadGateway)
		return
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		http.Error(w, "image source unavailable", http.StatusBadGateway)
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, h.maxSize+1))
	if err != nil {
		http.Error(w, "image source unavailable", http.StatusBadGateway)
		return
	}
	if int64(len(body)) > h.maxSize {
		http.Error(w, "image too large", http.StatusRequestEntityTooLarge)
		return
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil && strings.EqualFold(response.Header.Get("Content-Type"), "image/webp") {
		config, err = webp.DecodeConfig(bytes.NewReader(body))
		format = "webp"
	}
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > maxImageProxyDimension || config.Height > maxImageProxyDimension || uint64(config.Width)*uint64(config.Height) > maxImageProxyPixels {
		http.Error(w, "unsupported image", http.StatusUnsupportedMediaType)
		return
	}
	contentType := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}[format]
	if contentType == "" {
		http.Error(w, "unsupported image", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

func (s *Server) proxyImageURL(raw string) string {
	u, err := validateImageProxyURL(raw)
	if err != nil || len(s.config.Server.SecretKey) == 0 {
		return ""
	}
	return "/image-proxy?url=" + url.QueryEscape(u.String()) + "&sig=" + signImageProxyURL(u.String(), []byte(s.config.Server.SecretKey))
}

func signImageProxyURL(raw string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func validImageProxySignature(raw, signature string, key []byte) bool {
	if len(key) == 0 || len(signature) != sha256.Size*2 {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	expected, _ := hex.DecodeString(signImageProxyURL(raw, key))
	return hmac.Equal(provided, expected)
}

func (s *Server) proxyWikipediaImageSources(content string) string {
	if !s.config.Server.ImageProxy.Enabled {
		return content
	}
	return wikipediaImageSource.ReplaceAllStringFunc(content, func(match string) string {
		parts := wikipediaImageSource.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		proxyURL := s.proxyImageURL(html.UnescapeString(parts[2]))
		if proxyURL == "" {
			return `src=""`
		}
		return parts[1] + html.EscapeString(proxyURL) + parts[3]
	})
}

func (s *Server) prepareInstantWikipediaAnswer(answer *instant.Answer) {
	if answer == nil || answer.Type != instant.AnswerTypeWiki || !s.config.Server.ImageProxy.Enabled || answer.Data == nil || strings.Contains(answer.Content, `class="wiki-instant-thumbnail"`) {
		return
	}
	thumbnail, _ := answer.Data["thumbnail"].(string)
	proxyURL := s.proxyImageURL(thumbnail)
	if proxyURL == "" {
		return
	}
	answer.Content = `<figure class="wiki-instant-thumbnail"><img src="` + html.EscapeString(proxyURL) + `" alt="` + html.EscapeString(answer.Title) + `" loading="lazy"><figcaption>` + answer.Content + `</figcaption></figure>`
}

func validateImageProxyURL(raw string) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return nil, fmt.Errorf("invalid URL length")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("invalid image URL")
	}
	if port := u.Port(); port != "" && port != "443" {
		return nil, fmt.Errorf("invalid image URL port")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !isPublicImageIP(ip) {
		return nil, fmt.Errorf("non-public image address")
	}
	return u, nil
}

func isPublicImageIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}
