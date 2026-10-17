package search

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/model"
)

func TestNormalizeBooleanQuery(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"golang AND concurrency", "golang concurrency"},
		{"golang OR rust AND concurrency", "golang OR rust concurrency"},
		{`golang AND NOT "unsafe code"`, `golang -"unsafe code"`},
		{"NOT (golang OR rust)", "-golang -rust"},
		{"(golang OR rust) AND concurrency", "(golang OR rust) concurrency"},
		{"golang rust", "golang rust"},
		{`"go OR rust" AND tutorial`, `"go OR rust" tutorial`},
		{"golang or rust", "golang OR rust"},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			got, err := normalizeBooleanQuery(test.query)
			if err != nil {
				t.Fatalf("normalizeBooleanQuery() error = %v", err)
			}
			if got != test.want {
				t.Errorf("normalizeBooleanQuery() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizeBooleanQueryRejectsMalformedExpressions(t *testing.T) {
	for _, query := range []string{"golang AND", "OR rust", "golang OR )", "(golang OR rust", `"unclosed phrase`} {
		t.Run(query, func(t *testing.T) {
			if _, err := normalizeBooleanQuery(query); err == nil {
				t.Fatal("normalizeBooleanQuery() returned nil error")
			}
		})
	}
}

func TestAggregatorNormalizesBooleanQueryForEngines(t *testing.T) {
	engine := newMockEngine("mock", model.CategoryGeneral, true)
	engine.SetResults([]model.Result{{URL: "https://example.com", Title: "Example"}})
	aggregator := NewAggregatorSimple([]Engine{engine}, time.Second)
	query := model.NewQuery("golang AND NOT rust")
	if _, err := aggregator.Search(context.Background(), query); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	engine.mu.RLock()
	got := engine.searchQuery
	engine.mu.RUnlock()
	if got != "golang -rust" {
		t.Errorf("engine received query %q, want %q", got, "golang -rust")
	}
	if query.Text != "golang AND NOT rust" {
		t.Errorf("Search() mutated original query text to %q", query.Text)
	}
}

func TestAggregatorRejectsMalformedBooleanQueryBeforeSearching(t *testing.T) {
	engine := newMockEngine("mock", model.CategoryGeneral, true)
	aggregator := NewAggregatorSimple([]Engine{engine}, time.Second)
	_, err := aggregator.Search(context.Background(), model.NewQuery("golang OR"))
	if err == nil || !strings.Contains(err.Error(), "Boolean search query") {
		t.Fatalf("Search() error = %v, want a Boolean query error", err)
	}
	if engine.Calls() != 0 {
		t.Errorf("engine calls = %d, want 0", engine.Calls())
	}
}
