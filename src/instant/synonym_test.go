package instant

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apimgr/search/src/cache"
)

func TestSynonymHandlerFetchesAndCachesDatamuseResults(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("rel_syn") != "happy" || r.URL.Query().Get("md") != "dp" {
			t.Errorf("unexpected provider query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("provider request is missing a User-Agent")
		}
		fmt.Fprint(w, `[{"word":"joyful","tags":["adj"],"defs":["adjective\tfeeling joy"]}]`)
	}))
	defer server.Close()

	handler := NewSynonymHandler()
	handler.endpoint = server.URL
	handler.client = server.Client()
	handler.cache = cache.NewMemoryCache(10, time.Hour)
	for i := 0; i < 2; i++ {
		answer, err := handler.HandleInstantQuery(context.Background(), "synonyms for happy")
		if err != nil {
			t.Fatalf("HandleInstantQuery() error = %v", err)
		}
		if answer == nil || !strings.Contains(answer.Content, "Adjectives — feeling joy") || !strings.Contains(answer.Content, "joyful") {
			t.Fatalf("unexpected answer: %#v", answer)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("provider requests = %d, want 1 due to response cache", got)
	}
}

func TestAntonymHandlerUsesProviderAndGroupsByPartOfSpeech(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("rel_ant") != "hot" || r.URL.Query().Get("md") != "dp" {
			t.Errorf("unexpected provider query: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `[{"word":"cold","tags":["adj"],"defs":["adjective\tlow in temperature"]}]`)
	}))
	defer server.Close()

	handler := NewAntonymHandler()
	handler.endpoint = server.URL
	handler.client = server.Client()
	answer, err := handler.HandleInstantQuery(context.Background(), "antonyms of hot")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil || !strings.Contains(answer.Content, "Adjectives — low in temperature") || !strings.Contains(answer.Content, "cold") {
		t.Fatalf("unexpected answer: %#v", answer)
	}
}

func TestRelatedWordsRejectsOversizedTermsAndProviderResponses(t *testing.T) {
	if _, err := fetchRelated(context.Background(), http.DefaultClient, "https://api.datamuse.com/words", nil, "rel_syn", strings.Repeat("x", 201), "dp"); err == nil {
		t.Fatal("fetchRelated() accepted a term longer than 200 characters")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	if _, err := fetchRelated(context.Background(), server.Client(), server.URL, nil, "rel_syn", "happy", "dp"); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("fetchRelated() error = %v, want HTTP 401", err)
	}

	largeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(" ", (1<<20)+1)))
	}))
	defer largeServer.Close()
	if _, err := fetchRelated(context.Background(), largeServer.Client(), largeServer.URL, nil, "rel_syn", "happy", "dp"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("fetchRelated() error = %v, want bounded-response error", err)
	}
}
