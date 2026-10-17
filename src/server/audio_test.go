package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type dictionaryAudioRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip dictionaryAudioRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestServeDictionaryAudio(t *testing.T) {
	const source = "https://api.dictionaryapi.dev/media/pronunciations/hello-us.mp3"
	client := &http.Client{Transport: dictionaryAudioRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != source {
			t.Fatalf("requested URL = %q, want %q", request.URL.String(), source)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"audio/mpeg"}}, Body: io.NopCloser(strings.NewReader("audio bytes")), Request: request}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "/direct/audio", nil)
	response := httptest.NewRecorder()
	serveDictionaryAudio(response, request, source, client)
	if response.Code != http.StatusOK || response.Body.String() != "audio bytes" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "audio/mpeg" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe or missing response headers: %v", response.Header())
	}
	if response.Header().Get("Content-Security-Policy") != "default-src 'none'" || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("missing isolation headers: %v", response.Header())
	}
}

func TestServeDictionaryAudioRejectsUnsafeResponses(t *testing.T) {
	for _, test := range []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{name: "redirect", status: http.StatusFound, contentType: "audio/mpeg"},
		{name: "html", status: http.StatusOK, contentType: "text/html", body: "<script>alert(1)</script>"},
		{name: "oversize", status: http.StatusOK, contentType: "audio/mpeg", body: strings.Repeat("x", maxDictionaryAudioBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: dictionaryAudioRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": []string{test.contentType}}, Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			})}
			response := httptest.NewRecorder()
			serveDictionaryAudio(response, httptest.NewRequest(http.MethodGet, "/direct/audio", nil), "https://api.dictionaryapi.dev/media/pronunciations/hello-us.mp3", client)
			if response.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
			}
		})
	}
}

func TestHandleDictionaryAudioRejectsUntrustedInputs(t *testing.T) {
	for _, target := range []string{"", "http://api.dictionaryapi.dev/media/pronunciations/a.mp3", "https://evil.example/media/a.mp3", "https://api.dictionaryapi.dev/media/pronunciations/a.mp3?x=1"} {
		request := httptest.NewRequest(http.MethodGet, "/direct/audio?url="+target, nil)
		response := httptest.NewRecorder()
		handleDictionaryAudio(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("target %q status = %d, want %d", target, response.Code, http.StatusBadRequest)
		}
	}
}
