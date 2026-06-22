package instant

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ---- CaseHandler ----

func TestCaseHandlerNameAndPatterns(t *testing.T) {
	h := NewCaseHandler()
	if h.Name() != "case" {
		t.Errorf("Name() = %q, want %q", h.Name(), "case")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestCaseHandlerCanHandle(t *testing.T) {
	h := NewCaseHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"case: hello world", true},
		{"uppercase: foo", true},
		{"lowercase: FOO", true},
		{"titlecase: hello world", true},
		{"camelcase: hello world", true},
		{"snakecase: hello world", true},
		{"snake_case: hello world", true},
		{"kebabcase: hello world", true},
		{"kebab-case: hello world", true},
		{"convert case: foo bar", true},
		{"text case: foo bar", true},
		{"unrelated query", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestCaseHandlerHandleInstantQuery(t *testing.T) {
	h := NewCaseHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		query   string
		wantNil bool
		checks  []string
	}{
		{
			name:   "case conversion",
			query:  "case: hello world",
			checks: []string{"hello world", "HELLO WORLD", "helloWorld", "HelloWorld", "hello_world", "hello-world"},
		},
		{
			name:   "uppercase query",
			query:  "uppercase: foo bar",
			checks: []string{"FOO BAR", "foo bar"},
		},
		{
			name:    "empty text after prefix",
			query:   "case:   ",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ans != nil {
					t.Error("expected nil answer")
				}
				return
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			if ans.Type != AnswerTypeCase {
				t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeCase)
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Content, check) {
					t.Errorf("Content missing %q", check)
				}
			}
		})
	}
}

func TestCaseHelperFunctions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		fn    func(string) string
		want  string
	}{
		{"toTitleCase", "hello world", toTitleCase, "Hello World"},
		{"toCamelCase simple", "hello world", toCamelCase, "helloWorld"},
		{"toPascalCase", "hello world", toPascalCase, "HelloWorld"},
		{"toSnakeCase", "hello world", toSnakeCase, "hello_world"},
		{"toKebabCase", "hello world", toKebabCase, "hello-world"},
		{"toConstantCase", "hello world", toConstantCase, "HELLO_WORLD"},
		{"toSnakeCase from camel", "helloWorld", toSnakeCase, "hello_world"},
		{"toKebabCase with hyphens", "hello-world", toKebabCase, "hello-world"},
		{"splitIntoWords empty", "", func(s string) string {
			words := splitIntoWords(s)
			if len(words) == 0 {
				return ""
			}
			return words[0]
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.input)
			if got != tt.want {
				t.Errorf("%s(%q) = %q, want %q", tt.name, tt.input, got, tt.want)
			}
		})
	}
}

// ---- CalendarHandler ----

func TestCalendarHandlerNameAndPatterns(t *testing.T) {
	h := NewCalendarHandler()
	if h.Name() != "calendar" {
		t.Errorf("Name() = %q, want %q", h.Name(), "calendar")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestCalendarHandlerCanHandle(t *testing.T) {
	h := NewCalendarHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"days until Christmas", true},
		{"how many days until January 1 2030", true},
		{"days since 2020-01-01", true},
		{"days between 2020-01-01 and 2020-12-31", true},
		{"what day is 2024-07-04", true},
		{"30 days from now", true},
		{"30 days ago", true},
		{"2 weeks from now", true},
		{"3 months ago", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestCalendarHandlerDaysUntil(t *testing.T) {
	h := NewCalendarHandler()
	ctx := context.Background()

	ans, err := h.HandleInstantQuery(ctx, "days until 2099-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeCalendar && ans.Type != AnswerTypeDate {
		t.Errorf("Type = %q, unexpected", ans.Type)
	}
}

func TestCalendarHandlerDaysSince(t *testing.T) {
	h := NewCalendarHandler()
	ctx := context.Background()

	ans, err := h.HandleInstantQuery(ctx, "days since 2020-01-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer for days since")
	}
}

func TestCalendarHandlerDaysBetween(t *testing.T) {
	h := NewCalendarHandler()
	ctx := context.Background()

	ans, err := h.HandleInstantQuery(ctx, "days between 2020-01-01 and 2020-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer for days between")
	}
}

func TestCalendarHandlerWhatDay(t *testing.T) {
	h := NewCalendarHandler()
	ctx := context.Background()

	ans, err := h.HandleInstantQuery(ctx, "what day is 2024-07-04")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer for what day is")
	}
}

func TestCalendarHandlerRelativeDate(t *testing.T) {
	h := NewCalendarHandler()
	ctx := context.Background()

	tests := []string{
		"30 days from now",
		"30 days ago",
		"2 weeks from now",
		"3 months ago",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, q)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
		})
	}
}

func TestCalendarHandlerDateArithmetic(t *testing.T) {
	h := NewCalendarHandler()
	ctx := context.Background()

	ans, err := h.HandleInstantQuery(ctx, "2024-01-01 + 30 days")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer for date arithmetic")
	}
}

// ---- EscapeHandler ----

func TestEscapeHandlerNameAndPatterns(t *testing.T) {
	h := NewEscapeHandler()
	if h.Name() != "escape" {
		t.Errorf("Name() = %q, want %q", h.Name(), "escape")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestEscapeHandlerCanHandle(t *testing.T) {
	h := NewEscapeHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"escape: hello & world", true},
		{"html escape: <script>", true},
		{"js escape: hello", true},
		{"javascript escape: test", true},
		{"sql escape: test", true},
		{"regex escape: test.pattern", true},
		{"shell escape: test value", true},
		{"csv escape: hello, world", true},
		{"xml escape: <tag>", true},
		{"unescape: hello%20world", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestEscapeHandlerHandleInstantQuery(t *testing.T) {
	h := NewEscapeHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		query   string
		wantNil bool
		checks  []string
	}{
		{
			name:   "escape html special chars",
			query:  "escape: <hello & world>",
			checks: []string{"&lt;", "&amp;"},
		},
		{
			name:   "escape single quote in sql",
			query:  "sql escape: it's a test",
			checks: []string{"SQL:"},
		},
		{
			name:    "empty text",
			query:   "escape:   ",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ans != nil {
					t.Error("expected nil answer")
				}
				return
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			if ans.Type != AnswerTypeEscape {
				t.Errorf("Type = %q, want escape", ans.Type)
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Content, check) {
					t.Errorf("Content missing %q", check)
				}
			}
		})
	}
}

func TestEscapeHelperFunctions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		fn    func(string) string
		check func(string) bool
	}{
		{
			name:  "escapeJavaScript backslash",
			input: `hello\world`,
			fn:    escapeJavaScript,
			check: func(s string) bool { return strings.Contains(s, `\\`) },
		},
		{
			name:  "escapeJavaScript newline",
			input: "hello\nworld",
			fn:    escapeJavaScript,
			check: func(s string) bool { return strings.Contains(s, `\n`) },
		},
		{
			name:  "escapeJavaScript angle brackets",
			input: "<script>",
			fn:    escapeJavaScript,
			check: func(s string) bool { return strings.Contains(s, `\x3C`) },
		},
		{
			name:  "escapeSQL single quote",
			input: "it's",
			fn:    escapeSQL,
			check: func(s string) bool { return strings.Contains(s, "''") },
		},
		{
			name:  "escapeRegex dot",
			input: "hello.world",
			fn:    escapeRegex,
			check: func(s string) bool { return strings.Contains(s, `\.`) },
		},
		{
			name:  "escapeShell dollar",
			input: "$HOME",
			fn:    escapeShell,
			check: func(s string) bool { return strings.Contains(s, `\$`) },
		},
		{
			name:  "escapeCSV comma",
			input: "hello, world",
			fn:    escapeCSV,
			check: func(s string) bool { return strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) },
		},
		{
			name:  "escapeCSV no special chars",
			input: "helloworld",
			fn:    escapeCSV,
			check: func(s string) bool { return s == "helloworld" },
		},
		{
			name:  "escapeXML ampersand",
			input: "a & b",
			fn:    escapeXML,
			check: func(s string) bool { return strings.Contains(s, "&amp;") },
		},
		{
			name:  "escapeXML less than",
			input: "a < b",
			fn:    escapeXML,
			check: func(s string) bool { return strings.Contains(s, "&lt;") },
		},
		{
			name:  "escapeUnicode ascii passthrough",
			input: "hello",
			fn:    escapeUnicode,
			check: func(s string) bool { return s == "hello" },
		},
		{
			name:  "escapeHex",
			input: "AB",
			fn:    escapeHex,
			check: func(s string) bool { return s == "4142" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.input)
			if !tt.check(got) {
				t.Errorf("%s(%q) = %q, check failed", tt.name, tt.input, got)
			}
		})
	}
}

// ---- SlugHandler ----

func TestSlugHandlerNameAndPatterns(t *testing.T) {
	h := NewSlugHandler()
	if h.Name() != "slug" {
		t.Errorf("Name() = %q, want %q", h.Name(), "slug")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestSlugHandlerCanHandle(t *testing.T) {
	h := NewSlugHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"slug: Hello World", true},
		{"slugify: Hello World", true},
		{"url slug: test page", true},
		{"to slug: my page title", true},
		{"make slug: some text", true},
		{"unrelated query", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestSlugHandlerHandleInstantQuery(t *testing.T) {
	h := NewSlugHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		query   string
		wantNil bool
		checks  []string
	}{
		{
			name:   "basic slug",
			query:  "slug: Hello World",
			checks: []string{"hello-world"},
		},
		{
			name:   "slugify with special chars",
			query:  "slugify: My Page Title!",
			checks: []string{"my-page-title"},
		},
		{
			name:    "empty text",
			query:   "slug:   ",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ans != nil {
					t.Error("expected nil answer")
				}
				return
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			if ans.Type != AnswerTypeSlug {
				t.Errorf("Type = %q, want slug", ans.Type)
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Content, check) {
					t.Errorf("Content missing %q", check)
				}
			}
		})
	}
}

func TestSlugHelperFunctions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		fn    func(string) string
		want  string
	}{
		{"toBasicSlug simple", "Hello World", toBasicSlug, "hello-world"},
		{"toBasicSlug special chars", "My Page Title!", toBasicSlug, "my-page-title"},
		{"toBasicSlug double hyphen", "hello--world", toBasicSlug, "hello-world"},
		{"toUnderscoreSlug", "Hello World", toUnderscoreSlug, "hello_world"},
		{"toUnderscoreSlug double underscore", "hello  world", toUnderscoreSlug, "hello_world"},
		{"toSlugCamelCase", "Hello World", toSlugCamelCase, "helloWorld"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.input)
			if got != tt.want {
				t.Errorf("%s(%q) = %q, want %q", tt.name, tt.input, got, tt.want)
			}
		})
	}
}

func TestToTruncatedSlug(t *testing.T) {
	long := "this-is-a-very-long-title-that-should-be-truncated-at-some-point"
	result := toTruncatedSlug(long, 20)
	if len(result) > 20 {
		t.Errorf("truncated slug length %d > 20", len(result))
	}

	short := "short"
	result = toTruncatedSlug(short, 50)
	if result != "short" {
		t.Errorf("short slug should be unchanged, got %q", result)
	}
}

// ---- StopwatchHandler ----

func TestStopwatchHandlerNameAndPatterns(t *testing.T) {
	h := NewStopwatchHandler()
	if h.Name() != "stopwatch" {
		t.Errorf("Name() = %q, want %q", h.Name(), "stopwatch")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestStopwatchHandlerCanHandle(t *testing.T) {
	h := NewStopwatchHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"timer 5 minutes", true},
		{"set timer for 10 minutes", true},
		{"5 minute timer", true},
		{"countdown 30 seconds", true},
		{"stopwatch", true},
		{"start stopwatch", true},
		{"pomodoro", true},
		{"pomodoro timer", true},
		{"start pomodoro", true},
		{"work timer", true},
		{"break timer", true},
		{"alarm in 10 minutes", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestStopwatchHandlerHandleInstantQuery(t *testing.T) {
	h := NewStopwatchHandler()
	ctx := context.Background()

	tests := []struct {
		name   string
		query  string
		checks []string
	}{
		{
			name:   "stopwatch",
			query:  "stopwatch",
			checks: []string{"Stopwatch", "stopwatch"},
		},
		{
			name:   "start stopwatch",
			query:  "start stopwatch",
			checks: []string{"Stopwatch", "stopwatch"},
		},
		{
			name:   "pomodoro",
			query:  "pomodoro",
			checks: []string{"Pomodoro"},
		},
		{
			name:   "work timer",
			query:  "work timer",
			checks: []string{"Timer"},
		},
		{
			name:   "break timer",
			query:  "break timer",
			checks: []string{"Timer"},
		},
		{
			name:   "timer 5 minutes",
			query:  "timer 5 minutes",
			checks: []string{"Timer"},
		},
		{
			name:   "10 minute timer",
			query:  "10 minute timer",
			checks: []string{"Timer"},
		},
		{
			name:   "countdown 30 seconds",
			query:  "countdown 30 seconds",
			checks: []string{"Timer"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Title+ans.Content, check) {
					t.Errorf("Answer missing %q", check)
				}
			}
		})
	}
}

func TestStopwatchParseDuration(t *testing.T) {
	h := NewStopwatchHandler()

	tests := []struct {
		input   string
		wantErr bool
		check   func(d int64) bool
	}{
		{"5m", false, func(d int64) bool { return d == 300 }},
		{"1h", false, func(d int64) bool { return d == 3600 }},
		{"30s", false, func(d int64) bool { return d == 30 }},
		{"1 hour 30 minutes", false, func(d int64) bool { return d == 5400 }},
		{"2 hours", false, func(d int64) bool { return d == 7200 }},
		{"5 mins", false, func(d int64) bool { return d == 300 }},
		{"10", false, func(d int64) bool { return d == 600 }},
		{"5:30", false, func(d int64) bool { return d == 330 }},
		{"1:30:00", false, func(d int64) bool { return d == 5400 }},
		{"not a duration at all xyz abc", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := h.parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && tt.check != nil && !tt.check(int64(d.Seconds())) {
				t.Errorf("parseDuration(%q) = %v seconds, check failed", tt.input, d.Seconds())
			}
		})
	}
}

func TestStopwatchFormatDuration(t *testing.T) {
	h := NewStopwatchHandler()

	tests := []struct {
		seconds int
		want    string
	}{
		{0, "0 seconds"},
		{1, "1 second"},
		{60, "1 minute"},
		{3600, "1 hour"},
		{3661, "1 hour 1 minute 1 second"},
		{7200, "2 hours"},
		{120, "2 minutes"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			d := h.formatDuration(time.Duration(tt.seconds) * time.Second)
			if d != tt.want {
				t.Errorf("formatDuration(%d) = %q, want %q", tt.seconds, d, tt.want)
			}
		})
	}
}

// ---- UnicodeHandler ----

func TestUnicodeHandlerNameAndPatterns(t *testing.T) {
	h := NewUnicodeHandler()
	if h.Name() != "unicode" {
		t.Errorf("Name() = %q, want %q", h.Name(), "unicode")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestUnicodeHandlerCanHandle(t *testing.T) {
	h := NewUnicodeHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"unicode: A", true},
		{"character: A", true},
		{"char: A", true},
		{"codepoint: U+0041", true},
		{"U+0041", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestUnicodeHandlerHandleInstantQuery(t *testing.T) {
	h := NewUnicodeHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		query   string
		wantNil bool
		checks  []string
	}{
		{
			name:   "unicode by char A",
			query:  "unicode: A",
			checks: []string{"U+0041", "LATIN CAPITAL"},
		},
		{
			name:   "unicode by codepoint",
			query:  "U+0041",
			checks: []string{"U+0041"},
		},
		{
			name:   "character query",
			query:  "character: €",
			checks: []string{"U+20AC"},
		},
		{
			name:    "empty input",
			query:   "unicode:   ",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ans != nil {
					t.Error("expected nil answer")
				}
				return
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			if ans.Type != AnswerTypeUnicode {
				t.Errorf("Type = %q, want unicode", ans.Type)
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Content, check) {
					t.Errorf("Content missing %q; got: %s", check, ans.Content)
				}
			}
		})
	}
}

// ---- TimezoneHandler ----

func TestTimezoneHandlerNameAndPatterns(t *testing.T) {
	h := NewTimezoneHandler()
	if h.Name() != "timezone" {
		t.Errorf("Name() = %q, want %q", h.Name(), "timezone")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestTimezoneHandlerCanHandle(t *testing.T) {
	h := NewTimezoneHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"time in London", true},
		{"what time is it in Tokyo", true},
		{"current time in Paris", true},
		{"New York time", true},
		{"3pm EST to PST", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestTimezoneHandlerHandleInstantQuery(t *testing.T) {
	h := NewTimezoneHandler()
	ctx := context.Background()

	tests := []struct {
		name   string
		query  string
		checks []string
	}{
		{
			name:   "time in London",
			query:  "time in London",
			checks: []string{"London"},
		},
		{
			name:   "time in New York",
			query:  "time in New York",
			checks: []string{"New York"},
		},
		{
			name:   "Tokyo time",
			query:  "Tokyo time",
			checks: []string{"Tokyo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			if ans.Type != AnswerTypeTimezone && ans.Type != AnswerTypeTime {
				t.Errorf("Type = %q, unexpected type", ans.Type)
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Content+ans.Title, check) {
					t.Errorf("Answer missing %q", check)
				}
			}
		})
	}
}

// ---- JSONHandler ----

func TestJSONHandlerNameAndPatterns(t *testing.T) {
	h := NewJSONHandler()
	if h.Name() != "json" {
		t.Errorf("Name() = %q, want %q", h.Name(), "json")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestJSONHandlerCanHandle(t *testing.T) {
	h := NewJSONHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{`json: {"key":"value"}`, true},
		{`format json: {"key":"value"}`, true},
		{`validate json: {"key":"value"}`, true},
		{`prettify json: {"key":"value"}`, true},
		{`minify json: {"key":"value"}`, true},
		{`json format: {"key":"value"}`, true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestJSONHandlerHandleInstantQuery(t *testing.T) {
	h := NewJSONHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		query   string
		wantNil bool
		checks  []string
	}{
		{
			name:   "valid json format",
			query:  `json: {"name":"alice","age":30}`,
			checks: []string{"name", "alice"},
		},
		{
			name:   "invalid json",
			query:  `json: {invalid`,
			checks: []string{"Invalid JSON", "Error"},
		},
		{
			name:   "minify json",
			query:  `minify json: {"name": "alice"}`,
			checks: []string{"alice"},
		},
		{
			name:    "empty json string",
			query:   "json:   ",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ans != nil {
					t.Error("expected nil answer")
				}
				return
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			if ans.Type != AnswerTypeJSON {
				t.Errorf("Type = %q, want json", ans.Type)
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Content+ans.Title, check) {
					t.Errorf("Answer missing %q; content: %s", check, ans.Content[:min100(len(ans.Content))])
				}
			}
		})
	}
}

// ---- YAMLHandler ----

func TestYAMLHandlerNameAndPatterns(t *testing.T) {
	h := NewYAMLHandler()
	if h.Name() != "yaml" {
		t.Errorf("Name() = %q, want %q", h.Name(), "yaml")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestYAMLHandlerCanHandle(t *testing.T) {
	h := NewYAMLHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"yaml: name: alice", true},
		{"format yaml: name: alice", true},
		{"validate yaml: name: alice", true},
		{"yaml to json: name: alice", true},
		{"json to yaml: {\"name\":\"alice\"}", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestYAMLHandlerHandleInstantQuery(t *testing.T) {
	h := NewYAMLHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		query   string
		wantNil bool
		checks  []string
	}{
		{
			name:   "valid yaml format",
			query:  "yaml: name: alice",
			checks: []string{"alice"},
		},
		{
			name:   "yaml validate",
			query:  "validate yaml: name: alice",
			checks: []string{"alice"},
		},
		{
			name:   "yaml to json",
			query:  "yaml to json: name: alice",
			checks: []string{"alice"},
		},
		{
			name:   "json to yaml",
			query:  `json to yaml: {"name":"alice"}`,
			checks: []string{"alice"},
		},
		{
			name:   "invalid yaml",
			query:  "yaml: {invalid: [missing",
			checks: []string{"YAML"},
		},
		{
			name:    "empty data",
			query:   "yaml:   ",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ans != nil {
					t.Error("expected nil answer")
				}
				return
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			if ans.Type != AnswerTypeYAML {
				t.Errorf("Type = %q, want yaml", ans.Type)
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Content+ans.Title, check) {
					t.Errorf("Answer missing %q; content: %s", check, ans.Content[:min100(len(ans.Content))])
				}
			}
		})
	}
}

// ---- RegexHandler ----

func TestRegexHandlerNameAndPatterns(t *testing.T) {
	h := NewRegexHandler()
	if h.Name() != "regex" {
		t.Errorf("Name() = %q, want %q", h.Name(), "regex")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestRegexHandlerCanHandle(t *testing.T) {
	h := NewRegexHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"regex: [a-z]+", true},
		{"regexp: \\d+", true},
		{"explain regex: .+", true},
		{"regex explain: ^foo$", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestRegexHandlerHandleInstantQuery(t *testing.T) {
	h := NewRegexHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		query   string
		wantNil bool
		checks  []string
	}{
		{
			name:   "valid regex",
			query:  `regex: [a-z]+`,
			checks: []string{"Regex"},
		},
		{
			name:   "invalid regex",
			query:  `regex: [unclosed`,
			checks: []string{"Regex"},
		},
		{
			name:    "empty pattern",
			query:   "regex:   ",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if ans != nil {
					t.Error("expected nil answer")
				}
				return
			}
			if ans == nil {
				t.Fatal("expected non-nil answer")
			}
			if ans.Type != AnswerTypeRegex {
				t.Errorf("Type = %q, want regex", ans.Type)
			}
			for _, check := range tt.checks {
				if !strings.Contains(ans.Content+ans.Title, check) {
					t.Errorf("Answer missing %q", check)
				}
			}
		})
	}
}

// ---- HTMLEntity decodeHTMLEntities (via handler method) ----

func TestHTMLEntityHandlerDecodeMethod(t *testing.T) {
	h := NewHTMLEntityHandler()
	tests := []struct {
		input string
		want  string
	}{
		{"&amp;", "&"},
		{"&lt;", "<"},
		{"&gt;", ">"},
		{"&quot;", `"`},
		{"&#65;", "A"},
		{"&#x41;", "A"},
		{"hello", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := h.decodeHTMLEntities(tt.input)
			if got != tt.want {
				t.Errorf("decodeHTMLEntities(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---- BeautifyHandler minifySQL ----

func TestMinifySQL(t *testing.T) {
	input := `SELECT
    id,
    name
FROM
    users
WHERE
    active = 1`
	result := minifySQL(input)
	if strings.Contains(result, "\n") {
		t.Errorf("minifySQL result should not contain newlines: %q", result)
	}
	if !strings.Contains(result, "SELECT") {
		t.Errorf("minifySQL result missing SELECT: %q", result)
	}
}

// ---- ASNHandler Name/Patterns ----

func TestASNHandlerNameAndPatterns(t *testing.T) {
	h := NewASNHandler()
	if h.Name() != "asn" {
		t.Errorf("Name() = %q, want %q", h.Name(), "asn")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestASNHandlerCanHandle(t *testing.T) {
	h := NewASNHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"asn 15169", true},
		{"asn: 15169", true},
		{"AS15169", true},
		{"asn lookup 15169", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- WHOISHandler Name/Patterns ----

func TestWHOISHandlerNameAndPatterns(t *testing.T) {
	h := NewWHOISHandler()
	if h.Name() != "whois" {
		t.Errorf("Name() = %q, want %q", h.Name(), "whois")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestWHOISHandlerCanHandle(t *testing.T) {
	h := NewWHOISHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"whois example.com", true},
		{"whois google.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- DNSHandler Name/Patterns ----

func TestDNSHandlerNameAndPatterns(t *testing.T) {
	h := NewDNSHandler()
	if h.Name() != "dns" {
		t.Errorf("Name() = %q, want %q", h.Name(), "dns")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestDNSHandlerCanHandle(t *testing.T) {
	h := NewDNSHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"dns example.com", true},
		{"dns lookup example.com", true},
		{"nslookup example.com", true},
		{"dig example.com", true},
		{"dns: example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- CVEHandler Name/Patterns ----

func TestCVEHandlerNameAndPatterns(t *testing.T) {
	h := NewCVEHandler()
	if h.Name() != "cve" {
		t.Errorf("Name() = %q, want %q", h.Name(), "cve")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestCVEHandlerCanHandle(t *testing.T) {
	h := NewCVEHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"CVE-2021-44228", true},
		{"cve-2021-44228", true},
		{"cve: 2021-44228", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- CertHandler Name/Patterns ----

func TestCertHandlerNameAndPatterns(t *testing.T) {
	h := NewCertHandler()
	if h.Name() != "cert" {
		t.Errorf("Name() = %q, want %q", h.Name(), "cert")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestCertHandlerCanHandle(t *testing.T) {
	h := NewCertHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"cert: example.com", true},
		{"certificate: example.com", true},
		{"ssl cert: example.com", true},
		{"tls: example.com", true},
		{"ssl: example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- CheatHandler Name/Patterns ----

func TestCheatHandlerNameAndPatterns(t *testing.T) {
	h := NewCheatHandler()
	if h.Name() != "cheat" {
		t.Errorf("Name() = %q, want %q", h.Name(), "cheat")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestCheatHandlerCanHandle(t *testing.T) {
	h := NewCheatHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"cheat: curl", true},
		{"cheatsheet: curl", true},
		{"cht: git", true},
		{"tldr: curl", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- ManHandler Name/Patterns ----

func TestManHandlerNameAndPatterns(t *testing.T) {
	h := NewManHandler()
	if h.Name() != "man" {
		t.Errorf("Name() = %q, want %q", h.Name(), "man")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestManHandlerCanHandle(t *testing.T) {
	h := NewManHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"man curl", true},
		{"man ls", true},
		{"manpage curl", true},
		{"man: curl", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- PkgHandler Name/Patterns ----

func TestPkgHandlerNameAndPatterns(t *testing.T) {
	h := NewPkgHandler()
	if h.Name() != "pkg" {
		t.Errorf("Name() = %q, want %q", h.Name(), "pkg")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestPkgHandlerCanHandle(t *testing.T) {
	h := NewPkgHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"npm react", true},
		{"pypi requests", true},
		{"pkg github.com/go-chi/chi", true},
		{"npm: react", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- ExpandHandler Name/Patterns ----

func TestExpandHandlerNameAndPatterns(t *testing.T) {
	h := NewExpandHandler()
	if h.Name() != "expand" {
		t.Errorf("Name() = %q, want %q", h.Name(), "expand")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestExpandHandlerCanHandle(t *testing.T) {
	h := NewExpandHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"expand https://bit.ly/abc", true},
		{"unshorten https://t.co/abc", true},
		{"expand url https://example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- FeedHandler Name/Patterns ----

func TestFeedHandlerNameAndPatterns(t *testing.T) {
	h := NewFeedHandler()
	if h.Name() != "feed" {
		t.Errorf("Name() = %q, want %q", h.Name(), "feed")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestFeedHandlerCanHandle(t *testing.T) {
	h := NewFeedHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"feed example.com", true},
		{"rss example.com", true},
		{"feeds example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- HeadersHandler Name/Patterns ----

func TestHeadersHandlerNameAndPatterns(t *testing.T) {
	h := NewHeadersHandler()
	if h.Name() != "headers" {
		t.Errorf("Name() = %q, want %q", h.Name(), "headers")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestHeadersHandlerCanHandle(t *testing.T) {
	h := NewHeadersHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"headers example.com", true},
		{"http headers example.com", true},
		{"response headers example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- ResolveHandler Name/Patterns ----

func TestResolveHandlerNameAndPatterns(t *testing.T) {
	h := NewResolveHandler()
	if h.Name() != "resolve" {
		t.Errorf("Name() = %q, want %q", h.Name(), "resolve")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestResolveHandlerCanHandle(t *testing.T) {
	h := NewResolveHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"resolve example.com", true},
		{"lookup example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- RFCHandler Name/Patterns ----

func TestRFCHandlerNameAndPatterns(t *testing.T) {
	h := NewRFCHandler()
	if h.Name() != "rfc" {
		t.Errorf("Name() = %q, want %q", h.Name(), "rfc")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestRFCHandlerCanHandle(t *testing.T) {
	h := NewRFCHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"RFC 2616", true},
		{"rfc2616", true},
		{"rfc 7230", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- RobotsHandler Name/Patterns ----

func TestRobotsHandlerNameAndPatterns(t *testing.T) {
	h := NewRobotsHandler()
	if h.Name() != "robots" {
		t.Errorf("Name() = %q, want %q", h.Name(), "robots")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestRobotsHandlerCanHandle(t *testing.T) {
	h := NewRobotsHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"robots example.com", true},
		{"robots.txt example.com", true},
		{"check robots example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- SafeHandler Name/Patterns ----

func TestSafeHandlerNameAndPatterns(t *testing.T) {
	h := NewSafeHandler()
	if h.Name() != "safe" {
		t.Errorf("Name() = %q, want %q", h.Name(), "safe")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestSafeHandlerCanHandle(t *testing.T) {
	h := NewSafeHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"is example.com safe", true},
		{"check example.com", true},
		{"safe example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- SitemapHandler Name/Patterns ----

func TestSitemapHandlerNameAndPatterns(t *testing.T) {
	h := NewSitemapHandler()
	if h.Name() != "sitemap" {
		t.Errorf("Name() = %q, want %q", h.Name(), "sitemap")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestSitemapHandlerCanHandle(t *testing.T) {
	h := NewSitemapHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"sitemap example.com", true},
		{"urls example.com", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- TechHandler Name/Patterns ----

func TestTechHandlerNameAndPatterns(t *testing.T) {
	h := NewTechHandler()
	if h.Name() != "tech" {
		t.Errorf("Name() = %q, want %q", h.Name(), "tech")
	}
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestTechHandlerCanHandle(t *testing.T) {
	h := NewTechHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"tech example.com", true},
		{"technology stack example.com", true},
		{"what is example.com built with", true},
		{"unrelated query", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---- Direct.go: TLDRHandler Patterns, PortHandler Patterns ----

func TestTLDRHandlerPatterns(t *testing.T) {
	h := NewTLDRHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestPortHandlerPatterns(t *testing.T) {
	h := NewPortHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestCronHandlerPatterns(t *testing.T) {
	h := NewCronHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestChmodHandlerPatterns(t *testing.T) {
	h := NewChmodHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestTimestampHandlerPatterns(t *testing.T) {
	h := NewTimestampHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestSubnetHandlerPatterns(t *testing.T) {
	h := NewSubnetHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

func TestJWTHandlerPatterns(t *testing.T) {
	h := NewJWTHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() returned empty slice")
	}
}

// ---- Manager includes all handlers ----

func TestManagerIncludesAllHandlers(t *testing.T) {
	m := NewManager()
	handlers := m.GetHandlers()

	expectedNames := []string{
		"case", "slug", "escape", "stopwatch", "unicode", "timezone",
		"json", "yaml", "regex", "asn", "whois", "dns",
	}
	nameSet := make(map[string]bool)
	for _, h := range handlers {
		nameSet[h.Name()] = true
	}
	for _, name := range expectedNames {
		if !nameSet[name] {
			t.Errorf("Manager missing handler %q", name)
		}
	}
}

// ---- helper for test truncation ----
func min100(n int) int {
	if n < 100 {
		return n
	}
	return 100
}

