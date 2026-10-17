package server

import (
	"bytes"
	"image"
	colorpkg "image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/apimgr/search/src/config"
)

const imageProxyTestKey = "test-only-image-proxy-signing-key"

func signedImageProxyRequest(t *testing.T) *http.Request {
	t.Helper()
	target := "https://images.example/image.png"
	query := "url=" + url.QueryEscape(target) + "&sig=" + signImageProxyURL(target, []byte(imageProxyTestKey))
	return httptest.NewRequest(http.MethodGet, "/image-proxy?"+query, nil)
}

type imageProxyRoundTripper func(*http.Request) (*http.Response, error)

func (f imageProxyRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func imageProxyTestPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, colorpkg.White)
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestValidateImageProxyURL(t *testing.T) {
	credentialURL := (&url.URL{Scheme: "https", User: url.UserPassword("test-user", "test-password"), Host: "images.example", Path: "/a.png"}).String()
	for _, raw := range []string{"", "http://images.example/a.png", "https://127.0.0.1/a.png", "https://[::1]/a.png", "https://192.0.2.1/a.png", credentialURL, "https://images.example:8443/a.png", "https://images.example/a.png#fragment"} {
		if _, err := validateImageProxyURL(raw); err == nil {
			t.Errorf("validateImageProxyURL(%q) unexpectedly succeeded", raw)
		}
	}
	if _, err := validateImageProxyURL("https://images.example/image.png"); err != nil {
		t.Fatalf("valid public HTTPS URL rejected: %v", err)
	}
}

func TestImageProxyHandlerServesValidatedImage(t *testing.T) {
	body := imageProxyTestPNG(t)
	h := newImageProxyHandler(config.ImageProxyConfig{Enabled: true, MaxSize: 1024, Timeout: 2}, imageProxyTestKey)
	h.client = &http.Client{Transport: imageProxyRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.Header.Get("Referer") != "" {
			t.Errorf("upstream request method=%q Referer=%q", r.Method, r.Header.Get("Referer"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
	})}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, signedImageProxyRequest(t))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" || !bytes.Equal(response.Body.Bytes(), body) {
		t.Fatalf("proxy response status=%d type=%q body length=%d", response.Code, response.Header().Get("Content-Type"), response.Body.Len())
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Cache-Control") == "" {
		t.Fatal("proxy omitted required response headers")
	}
}

func TestImageProxyHandlerRejectsOversizeAndNonImage(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       []byte
		limit      int64
		statusCode int
	}{
		{name: "oversize", body: bytes.Repeat([]byte("x"), 32), limit: 16, statusCode: http.StatusRequestEntityTooLarge},
		{name: "not an image", body: []byte("not an image"), limit: 1024, statusCode: http.StatusUnsupportedMediaType},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newImageProxyHandler(config.ImageProxyConfig{Enabled: true, MaxSize: tt.limit, Timeout: 2}, imageProxyTestKey)
			h.client = &http.Client{Transport: imageProxyRoundTripper(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(tt.body)), Request: r}, nil
			})}
			response := httptest.NewRecorder()
			h.ServeHTTP(response, signedImageProxyRequest(t))
			if response.Code != tt.statusCode {
				t.Fatalf("status = %d, want %d", response.Code, tt.statusCode)
			}
		})
	}
}

func TestImageProxyRewritesOnlyWikipediaImages(t *testing.T) {
	s := &Server{config: &config.Config{Server: config.ServerConfig{SecretKey: imageProxyTestKey, ImageProxy: config.ImageProxyConfig{Enabled: true}}}}
	content := `<img src="https://upload.wikimedia.org/wikipedia/commons/a.png"><img src="https://example.org/a.png">`
	got := s.proxyWikipediaImageSources(content)
	if !bytes.Contains([]byte(got), []byte(`/image-proxy?url=https%3A%2F%2Fupload.wikimedia.org`)) || !bytes.Contains([]byte(got), []byte("&amp;sig=")) {
		t.Fatalf("Wikipedia thumbnail was not proxied: %s", got)
	}
	if !bytes.Contains([]byte(got), []byte(`src="https://example.org`)) {
		t.Fatalf("non-Wikimedia image should remain unchanged: %s", got)
	}
}

func TestImageProxySignaturePreventsOpenRelay(t *testing.T) {
	target := "https://images.example/image.png"
	sig := signImageProxyURL(target, []byte(imageProxyTestKey))
	if !validImageProxySignature(target, sig, []byte(imageProxyTestKey)) {
		t.Fatal("valid image signature rejected")
	}
	if validImageProxySignature("https://other.example/image.png", sig, []byte(imageProxyTestKey)) {
		t.Fatal("signature accepted for a changed target")
	}
	if validImageProxySignature(target, sig, nil) {
		t.Fatal("signature accepted without a signing key")
	}
}

func TestImageProxyHandlerRejectsUnsignedRequests(t *testing.T) {
	h := newImageProxyHandler(config.ImageProxyConfig{Enabled: true}, imageProxyTestKey)
	request := httptest.NewRequest(http.MethodGet, "/image-proxy?url=https%3A%2F%2Fimages.example%2Fimage.png", nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unsigned image request status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
