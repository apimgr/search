package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type previewRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip previewRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestServePeerTubePreviewUsesBoundedSameOriginRange(t *testing.T) {
	client := &http.Client{Transport: previewRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://peertube.tv/static/web-videos/clip.mp4" {
			t.Fatalf("upstream URL = %q", request.URL)
		}
		if request.Header.Get("Range") != fmt.Sprintf("bytes=0-%d", maxVideoPreviewRange-1) {
			t.Fatalf("range = %q", request.Header.Get("Range"))
		}
		return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Content-Range": []string{"bytes 0-4/10000000"}}, ContentLength: 5, Body: io.NopCloser(strings.NewReader("video")), Request: request}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "/media/preview/peertube/clip.mp4", nil)
	response := httptest.NewRecorder()
	servePeerTubePreview(response, request, "clip.mp4", client)
	if response.Code != http.StatusPartialContent || response.Header().Get("Content-Type") != "video/mp4" || response.Body.String() != "video" {
		t.Fatalf("response = %d %q %q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}

func TestServePeerTubePreviewRejectsInvalidTargetsAndMethods(t *testing.T) {
	for _, test := range []struct {
		name, filename, method string
		want                   int
	}{
		{name: "path", filename: "../clip.mp4", method: http.MethodGet, want: http.StatusBadRequest},
		{name: "extension", filename: "clip.m3u8", method: http.MethodGet, want: http.StatusBadRequest},
		{name: "method", filename: "clip.mp4", method: http.MethodPost, want: http.StatusMethodNotAllowed},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, "/media/preview/peertube/"+test.filename, nil)
			servePeerTubePreview(response, request, test.filename, &http.Client{})
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}

func TestParsePreviewRangeBoundsAndRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantStart int64
		wantEnd   int64
		wantValid bool
	}{
		{name: "open ended", input: "bytes=1024-", wantStart: 1024, wantEnd: 1024 + maxVideoPreviewRange - 1, wantValid: true},
		{name: "oversized range is capped", input: "bytes=0-99999999", wantStart: 0, wantEnd: maxVideoPreviewRange - 1, wantValid: true},
		{name: "large end is capped without overflow", input: "bytes=0-9223372036854775807", wantStart: 0, wantEnd: maxVideoPreviewRange - 1, wantValid: true},
		{name: "reversed range", input: "bytes=10-9"},
		{name: "multiple ranges", input: "bytes=0-1,3-4"},
		{name: "overflow start", input: "bytes=9223372036854775807-"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end, valid := parsePreviewRange(test.input)
			if start != test.wantStart || end != test.wantEnd || valid != test.wantValid {
				t.Fatalf("parsePreviewRange(%q) = (%d, %d, %t), want (%d, %d, %t)", test.input, start, end, valid, test.wantStart, test.wantEnd, test.wantValid)
			}
		})
	}
}

func TestPeerTubePreviewDoesNotReachClearnetFromTor(t *testing.T) {
	server := &Server{}
	server.torBackendPort.Store(9053)
	request := httptest.NewRequest(http.MethodGet, "/media/preview/peertube/clip.mp4", nil)
	request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9053}))
	response := httptest.NewRecorder()
	server.handlePeerTubePreview(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("Tor preview status = %d, want 404", response.Code)
	}
}
