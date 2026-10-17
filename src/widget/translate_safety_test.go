package widget

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type translationResponseTransport struct {
	responseCount int
	response      *http.Response
}

func (transport *translationResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.responseCount++
	response := *transport.response
	response.Request = request
	return &response, nil
}

func TestTranslateFetcherBoundsProviderResponses(t *testing.T) {
	transport := &translationResponseTransport{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"translation":"` + strings.Repeat("x", maxTranslationResponseBytes) + `"}`)),
	}}
	fetcher := NewTranslateFetcher()
	fetcher.httpClient.Transport = transport
	if _, err := fetcher.fetchFromLingva(context.Background(), "hello", "auto", "es"); err == nil {
		t.Fatal("oversized translation response unexpectedly succeeded")
	}
}

func TestTranslateFetcherDoesNotFollowProviderRedirects(t *testing.T) {
	transport := &translationResponseTransport{response: &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"http://127.0.0.1/private"}},
		Body:       io.NopCloser(strings.NewReader("")),
	}}
	fetcher := NewTranslateFetcher()
	fetcher.httpClient.Transport = transport
	result, err := fetcher.fetchFromLingva(context.Background(), "hello", "auto", "es")
	if err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Fatal("redirect response unexpectedly succeeded")
	}
	if transport.responseCount != 1 {
		t.Fatalf("request count = %d, want one request without following redirect", transport.responseCount)
	}
}
