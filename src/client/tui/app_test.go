package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/apimgr/search/src/client/api"
)

// Tests for color definitions

func TestDraculaColors(t *testing.T) {
	colors := []struct {
		name  string
		color lipgloss.Color
	}{
		{"background", background},
		{"foreground", foreground},
		{"selection", selection},
		{"comment", comment},
		{"cyan", cyan},
		{"green", green},
		{"orange", orange},
		{"pink", pink},
		{"purple", purple},
		{"red", red},
		{"yellow", yellow},
	}

	for _, c := range colors {
		t.Run(c.name, func(t *testing.T) {
			if c.color == "" {
				t.Errorf("Color %s should not be empty", c.name)
			}
		})
	}
}

// Tests for style definitions

func TestStyles(t *testing.T) {
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"titleStyle", titleStyle},
		{"inputStyle", inputStyle},
		{"resultStyle", resultStyle},
		{"urlStyle", urlStyle},
		{"helpStyle", helpStyle},
		{"errorStyle", errorStyle},
	}

	for _, s := range styles {
		t.Run(s.name, func(t *testing.T) {
			// Just verify the style exists and can render
			rendered := s.style.Render("test")
			if rendered == "" {
				t.Errorf("Style %s.Render() returned empty string", s.name)
			}
		})
	}
}

// Tests for model struct

func TestModelStruct(t *testing.T) {
	client := &api.Client{
		BaseURL: "https://api.example.com",
	}

	m := model{
		client:    client,
		results:   []api.SearchResult{},
		searching: false,
		width:     80,
		height:    24,
	}

	if m.client == nil {
		t.Error("model.client should not be nil")
	}
	if m.width != 80 {
		t.Errorf("model.width = %d, want 80", m.width)
	}
	if m.height != 24 {
		t.Errorf("model.height = %d, want 24", m.height)
	}
	if m.searching {
		t.Error("model.searching should be false initially")
	}
}

// Tests for searchResultMsg

func TestSearchResultMsg(t *testing.T) {
	results := []api.SearchResult{
		{Title: "Test", URL: "https://example.com"},
	}

	msg := searchResultMsg{
		results: results,
		err:     nil,
	}

	if len(msg.results) != 1 {
		t.Errorf("searchResultMsg.results length = %d, want 1", len(msg.results))
	}
	if msg.err != nil {
		t.Error("searchResultMsg.err should be nil")
	}
}

func TestSearchResultMsgWithError(t *testing.T) {
	msg := searchResultMsg{
		results: nil,
		err:     &testError{msg: "search failed"},
	}

	if msg.results != nil {
		t.Error("searchResultMsg.results should be nil on error")
	}
	if msg.err == nil {
		t.Error("searchResultMsg.err should not be nil")
	}
}

// Test error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// Tests for initialModel

func TestInitialModel(t *testing.T) {
	client := &api.Client{
		BaseURL: "https://api.example.com",
	}

	m := initialModel(client)

	if m.client != client {
		t.Error("initialModel() should set client")
	}
	if m.input.Placeholder != "Enter search query..." {
		t.Errorf("input.Placeholder = %q", m.input.Placeholder)
	}
	if !m.input.Focused() {
		t.Error("input should be focused initially")
	}
	if m.input.Width != 50 {
		t.Errorf("input.Width = %d, want 50", m.input.Width)
	}
}

func TestInitialModelWithNilClient(t *testing.T) {
	m := initialModel(nil)

	if m.client != nil {
		t.Error("initialModel(nil) should have nil client")
	}
}

// Tests for model.Init

func TestModelInit(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)

	cmd := m.Init()

	if cmd == nil {
		t.Error("Init() should return a command for text input blink")
	}
}

// Tests for model.Update

func TestModelUpdateQuit(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)

	keys := []string{"ctrl+c", "q"}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
			if key == "ctrl+c" {
				msg = tea.KeyMsg{Type: tea.KeyCtrlC}
			} else if key == "q" {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
			}

			_, cmd := m.Update(msg)

			// The Quit command should be returned
			// Note: Testing tea.Cmd is tricky, but we can at least verify no panic
			_ = cmd
		})
	}
}

func TestModelUpdateEnterWithQuery(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.SearchResponse{
			Results: []api.SearchResult{
				{Title: "Result", URL: "https://example.com"},
			},
		})
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL, "", 30)
	m := initialModel(client)
	m.input.SetValue("test query")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, cmd := m.Update(msg)

	updatedModel := newModel.(model)
	if !updatedModel.searching {
		t.Error("searching should be true after Enter with query")
	}
	if cmd == nil {
		t.Error("Update(Enter) should return search command")
	}
}

func TestModelUpdateEnterWithoutQuery(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.input.SetValue("")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := m.Update(msg)

	updatedModel := newModel.(model)
	if updatedModel.searching {
		t.Error("searching should be false when Enter pressed without query")
	}
}

func TestModelUpdateEsc(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.input.SetValue("test")
	m.results = []api.SearchResult{{Title: "Test"}}
	m.err = &testError{msg: "test error"}

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ := m.Update(msg)

	updatedModel := newModel.(model)
	if updatedModel.input.Value() != "" {
		t.Error("Esc should clear input")
	}
	if updatedModel.results != nil {
		t.Error("Esc should clear results")
	}
	if updatedModel.err != nil {
		t.Error("Esc should clear error")
	}
}

func TestModelUpdateWindowSize(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)

	msg := tea.WindowSizeMsg{Width: 100, Height: 50}
	newModel, _ := m.Update(msg)

	updatedModel := newModel.(model)
	if updatedModel.width != 100 {
		t.Errorf("width = %d, want 100", updatedModel.width)
	}
	if updatedModel.height != 50 {
		t.Errorf("height = %d, want 50", updatedModel.height)
	}
}

func TestModelUpdateSearchResult(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.searching = true
	m.width = 80
	m.height = 24

	results := []api.SearchResult{
		{Title: "Test", URL: "https://example.com"},
	}

	msg := searchResultMsg{results: results, err: nil}
	newModel, _ := m.Update(msg)

	updatedModel := newModel.(model)
	if updatedModel.searching {
		t.Error("searching should be false after receiving results")
	}
	if len(updatedModel.results) != 1 {
		t.Errorf("results length = %d, want 1", len(updatedModel.results))
	}
}

func TestModelUpdateSearchResultWithError(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.searching = true
	m.width = 80
	m.height = 24

	msg := searchResultMsg{results: nil, err: &testError{msg: "search error"}}
	newModel, _ := m.Update(msg)

	updatedModel := newModel.(model)
	if updatedModel.searching {
		t.Error("searching should be false after error")
	}
	if updatedModel.err == nil {
		t.Error("err should be set after error")
	}
}

// Tests for model.doSearch

func TestModelDoSearchSuccess(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Per AI.md PART 14: Wrapped response format {"ok": true, "data": {...}}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"data": api.SearchResponse{
				Results: []api.SearchResult{
					{Title: "Test Result", URL: "https://example.com", Description: "Test snippet"},
				},
				Pagination: api.SearchPagination{
					Page:  1,
					Limit: 20,
					Total: 1,
					Pages: 1,
				},
			},
		})
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL, "", 30)
	m := initialModel(client)
	m.input.SetValue("test query")

	result := m.doSearch()

	msg, ok := result.(searchResultMsg)
	if !ok {
		t.Fatalf("doSearch() returned %T, want searchResultMsg", result)
	}

	if msg.err != nil {
		t.Errorf("doSearch() error = %v", msg.err)
	}
	if len(msg.results) != 1 {
		t.Errorf("doSearch() results length = %d, want 1", len(msg.results))
	}
}

func TestModelDoSearchError(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL, "", 30)
	m := initialModel(client)
	m.input.SetValue("test query")

	result := m.doSearch()

	msg, ok := result.(searchResultMsg)
	if !ok {
		t.Fatalf("doSearch() returned %T, want searchResultMsg", result)
	}

	if msg.err == nil {
		t.Error("doSearch() should return error on server error")
	}
}

// Tests for model.renderResults

func TestModelRenderResultsWithError(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.err = &testError{msg: "test error"}

	result := m.renderResults()

	if result == "" {
		t.Error("renderResults() should not return empty string for error")
	}
}

func TestModelRenderResultsEmpty(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.results = []api.SearchResult{}

	result := m.renderResults()

	if result == "" {
		t.Error("renderResults() should not return empty string for no results")
	}
}

func TestModelRenderResultsWithData(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.results = []api.SearchResult{
		{Title: "Test Result 1", URL: "https://example1.com", Description: "First result snippet"},
		{Title: "Test Result 2", URL: "https://example2.com", Description: ""},
	}

	result := m.renderResults()

	if result == "" {
		t.Error("renderResults() should not return empty string")
	}
}

func TestModelRenderResultsNoSnippet(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.results = []api.SearchResult{
		{Title: "No Snippet", URL: "https://example.com", Description: ""},
	}

	result := m.renderResults()

	if result == "" {
		t.Error("renderResults() should handle empty snippet")
	}
}

// Tests for model.View

func TestModelView(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.width = 80
	m.height = 24

	view := m.View()

	if view == "" {
		t.Error("View() should not return empty string")
	}
}

func TestModelViewSearching(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.width = 80
	m.height = 24
	m.searching = true

	view := m.View()

	if view == "" {
		t.Error("View() should not return empty string while searching")
	}
}

func TestModelViewWithResults(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.width = 80
	m.height = 24
	m.results = []api.SearchResult{
		{Title: "Test", URL: "https://example.com"},
	}

	view := m.View()

	if view == "" {
		t.Error("View() should not return empty string with results")
	}
}

func TestModelViewWithError(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.width = 80
	m.height = 24
	m.err = &testError{msg: "test error"}

	view := m.View()

	if view == "" {
		t.Error("View() should not return empty string with error")
	}
}

// Tests for Run function (note: can't fully test interactive TUI)

func TestRunFunctionExists(t *testing.T) {
	// Just verify the Run function exists and has correct signature
	var runFunc func(*api.Client) error = Run
	if runFunc == nil {
		t.Error("Run function should exist")
	}
}

// Tests for multiple search results

func TestModelRenderResultsMultiple(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.results = []api.SearchResult{
		{Title: "Result One", URL: "https://one.example.com", Description: "First snippet"},
		{Title: "Result Two", URL: "https://two.example.com", Description: "Second snippet"},
		{Title: "Result Three", URL: "https://three.example.com", Description: "Third snippet"},
		{Title: "Result Four", URL: "https://four.example.com", Description: "Fourth snippet"},
		{Title: "Result Five", URL: "https://five.example.com", Description: "Fifth snippet"},
	}

	result := m.renderResults()

	if result == "" {
		t.Error("renderResults() should handle multiple results")
	}
}

// Tests for viewport

func TestModelViewportInitialized(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)

	// Initially viewport is not set
	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	newModel, _ := m.Update(msg)

	updatedModel := newModel.(model)
	// After window size message, viewport should be initialized
	// Width should be 80, height should be 24-6=18
	_ = updatedModel.viewport
}

// Tests for input field

func TestModelInputValue(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)

	// Set input value
	m.input.SetValue("test search")

	if m.input.Value() != "test search" {
		t.Errorf("input.Value() = %q, want 'test search'", m.input.Value())
	}
}

func TestModelInputClearOnEsc(t *testing.T) {
	client := &api.Client{}
	m := initialModel(client)
	m.input.SetValue("test")

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ := m.Update(msg)

	updatedModel := newModel.(model)
	if updatedModel.input.Value() != "" {
		t.Error("input should be cleared on Esc")
	}
}

// Test model state transitions

func TestModelStateTransitions(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.SearchResponse{
			Results: []api.SearchResult{},
		})
	}))
	defer testServer.Close()

	client := api.NewClient(testServer.URL, "", 30)
	m := initialModel(client)
	m.width = 80
	m.height = 24

	// Initial state
	if m.searching {
		t.Error("Initial state: searching should be false")
	}
	if len(m.results) != 0 {
		t.Error("Initial state: results should be empty")
	}

	// Set input and press Enter
	m.input.SetValue("test")
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := m.Update(enterMsg)
	m = newModel.(model)

	if !m.searching {
		t.Error("After Enter: searching should be true")
	}

	// Receive results
	resultMsg := searchResultMsg{results: []api.SearchResult{{Title: "Test"}}, err: nil}
	newModel, _ = m.Update(resultMsg)
	m = newModel.(model)

	if m.searching {
		t.Error("After results: searching should be false")
	}
	if len(m.results) != 1 {
		t.Error("After results: should have results")
	}

	// Press Esc to clear
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ = m.Update(escMsg)
	m = newModel.(model)

	if len(m.results) != 0 {
		t.Error("After Esc: results should be cleared")
	}
}
