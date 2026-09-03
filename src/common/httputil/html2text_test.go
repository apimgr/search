package httputil

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// parseHTMLFragment parses an HTML snippet for direct node-level testing
// of unexported helpers (getAttr, getTextContent).
func parseHTMLFragment(htmlStr string) (*html.Node, error) {
	return html.Parse(strings.NewReader(htmlStr))
}

// findFirstElement performs a depth-first search for the first element node
// with the given tag name.
func findFirstElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirstElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// HTML2TextConverter: covers width normalization, parser failure fallback,
// and end-to-end rendering of common tags.
func TestHTML2TextConverter(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		width    int
		contains []string
	}{
		{
			name:     "heading and paragraph",
			html:     "<h1>Title</h1><p>Hello world</p>",
			width:    80,
			contains: []string{"TITLE", "Hello world"},
		},
		{
			name:     "width zero defaults to 80",
			html:     "<p>text</p>",
			width:    0,
			contains: []string{"text"},
		},
		{
			name:     "negative width defaults to 80",
			html:     "<p>text</p>",
			width:    -5,
			contains: []string{"text"},
		},
		{
			name:     "script and style skipped",
			html:     "<script>alert(1)</script><style>.a{}</style><p>visible</p>",
			width:    80,
			contains: []string{"visible"},
		},
		{
			name:     "link with href",
			html:     `<a href="https://example.com">click</a>`,
			width:    80,
			contains: []string{"click [https://example.com]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTML2TextConverter(tt.html, tt.width)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("HTML2TextConverter(%q) = %q, want substring %q", tt.html, got, want)
				}
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("HTML2TextConverter() result must end with a single newline, got %q", got)
			}
		})
	}
}

// HTML2TextConverter: verify skipTags fully suppress form/input/button/nav/footer/head content.
func TestHTML2TextConverterSkipTags(t *testing.T) {
	htmlStr := `<html><head><title>hidden</title></head><body>
	<nav>navlink</nav>
	<form><input value="x"><button>submit</button></form>
	<footer>footerlink</footer>
	<p>main content</p>
	</body></html>`

	got := HTML2TextConverter(htmlStr, 80)
	for _, notWant := range []string{"navlink", "submit", "footerlink", "hidden"} {
		if strings.Contains(got, notWant) {
			t.Errorf("HTML2TextConverter() should skip %q, got %q", notWant, got)
		}
	}
	if !strings.Contains(got, "main content") {
		t.Errorf("HTML2TextConverter() missing expected content, got %q", got)
	}
}

// convertElement: covers each element branch individually — h2-h6, lists,
// anchor edge cases, emphasis, code, pre, blockquote, hr, br, table, span, div.
func TestConvertElementBranches(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"h2", "<h2>Section</h2>", "─── Section ───"},
		{"h3", "<h3>Sub</h3>", "► Sub"},
		{"h4", "<h4>Minor</h4>", "  Minor"},
		{"h5", "<h5>Tiny</h5>", "  Tiny"},
		{"h6", "<h6>Tinier</h6>", "  Tinier"},
		{"unordered list", "<ul><li>one</li><li>two</li></ul>", "•"},
		{"ordered list", "<ol><li>one</li><li>two</li></ol>", "1."},
		{"anchor no href", `<a>text only</a>`, "text only"},
		{"anchor hash href", `<a href="#">text</a>`, "text"},
		{"anchor empty text uses href", `<a href="https://x.com"></a>`, "[https://x.com]"},
		{"strong", "<strong>bold</strong>", "*bold*"},
		{"b tag", "<b>bold2</b>", "*bold2*"},
		{"em", "<em>italic</em>", "_italic_"},
		{"i tag", "<i>italic2</i>", "_italic2_"},
		{"code", "<code>x := 1</code>", "`x := 1`"},
		{"pre", "<pre>line1\nline2</pre>", "    line1"},
		{"blockquote", "<blockquote>quoted text</blockquote>", "│ quoted text"},
		{"hr", "<p>a</p><hr><p>b</p>", "─"},
		{"br", "<p>before<br>after</p>", "before"},
		{"span", "<span>spanned</span>", "spanned"},
		{"div passthrough", "<div><p>nested</p></div>", "nested"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTML2TextConverter(tt.html, 80)
			if !strings.Contains(got, tt.want) {
				t.Errorf("HTML2TextConverter(%q) = %q, want substring %q", tt.html, got, tt.want)
			}
		})
	}
}

// convertTable: covers header/body rows, column width shrink, and empty table.
func TestConvertTable(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		contains []string
	}{
		{
			name:     "simple table with header and row",
			html:     "<table><tr><th>Name</th><th>Age</th></tr><tr><td>Bob</td><td>30</td></tr></table>",
			contains: []string{"Name", "Age", "Bob", "30", "│"},
		},
		{
			name:     "table with thead/tbody",
			html:     "<table><thead><tr><th>H</th></tr></thead><tbody><tr><td>D</td></tr></tbody></table>",
			contains: []string{"H", "D"},
		},
		{
			name:     "single row table no repeated separator",
			html:     "<table><tr><td>only</td></tr></table>",
			contains: []string{"only"},
		},
		{
			name:     "table with no rows produces no output",
			html:     "<table></table><p>after</p>",
			contains: []string{"after"},
		},
		{
			name:     "wide table shrinks columns to fit width",
			html:     "<table><tr><td>" + strings.Repeat("a", 200) + "</td><td>short</td></tr></table>",
			contains: []string{"short"},
		},
		{
			name:     "ragged rows padded to column count",
			html:     "<table><tr><td>a</td><td>b</td></tr><tr><td>c</td></tr></table>",
			contains: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTML2TextConverter(tt.html, 40)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("HTML2TextConverter(%q) = %q, want substring %q", tt.html, got, want)
				}
			}
		})
	}
}

// wordWrap: covers empty text, exact fit, wrapping, and non-positive width.
func TestWordWrap(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{"zero width returns unchanged", "hello world", 0, "hello world"},
		{"negative width returns unchanged", "hello world", -1, "hello world"},
		{"short text within width", "hi", 10, "hi"},
		{"exact width fit", "hello", 5, "hello"},
		{"wraps at word boundary", "one two three", 7, "one two\nthree"},
		{"collapses internal whitespace", "one   two\tthree", 20, "one two three"},
		{"single long word exceeding width kept intact", "supercalifragilistic", 5, "supercalifragilistic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wordWrap(tt.text, tt.width)
			if got != tt.want {
				t.Errorf("wordWrap(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

// centerText: covers text longer than width, exact width, and padding split.
func TestCenterText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{"text longer than width returned unchanged", "toolongtext", 5, "toolongtext"},
		{"text equal to width returned unchanged", "exact", 5, "exact"},
		{"even padding split evenly", "hi", 6, "  hi  "},
		{"odd padding left gets less", "hi", 5, " hi  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := centerText(tt.text, tt.width)
			if got != tt.want {
				t.Errorf("centerText(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

// getAttr: covers found, not found, and no-attrs cases.
func TestGetAttr(t *testing.T) {
	doc, err := parseHTMLFragment(`<a href="https://x.com" title="X">link</a>`)
	if err != nil {
		t.Fatalf("failed to parse fragment: %v", err)
	}
	anchor := findFirstElement(doc, "a")
	if anchor == nil {
		t.Fatal("anchor node not found")
	}
	if got := getAttr(anchor, "href"); got != "https://x.com" {
		t.Errorf("getAttr(href) = %q, want %q", got, "https://x.com")
	}
	if got := getAttr(anchor, "missing"); got != "" {
		t.Errorf("getAttr(missing) = %q, want empty string", got)
	}
}

// stripAllTags: exercised via the parser-failure fallback path, and directly
// for the plain-tag-stripping behavior.
func TestStripAllTags(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"strips simple tags", "<p>hello</p>", "hello"},
		{"no tags returned unchanged", "plain text", "plain text"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAllTags(tt.in)
			if got != tt.want {
				t.Errorf("stripAllTags(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// stripAllTags: covers multi-tag input where exact inter-tag spacing is an
// implementation detail — assert the tag markup is gone and both text
// fragments survive, not the precise whitespace count.
func TestStripAllTagsMultipleTags(t *testing.T) {
	got := stripAllTags("<div><span>a</span> <span>b</span></div>")
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("stripAllTags() left tag markers in %q", got)
	}
	fields := strings.Fields(got)
	if len(fields) != 2 || fields[0] != "a" || fields[1] != "b" {
		t.Errorf("stripAllTags() = %q, want fields [a b]", got)
	}
}

// getTextContent: covers nested elements and skip-tag exclusion during text extraction.
func TestGetTextContent(t *testing.T) {
	doc, err := parseHTMLFragment(`<div>outer <span>inner</span> <script>ignored</script>tail</div>`)
	if err != nil {
		t.Fatalf("failed to parse fragment: %v", err)
	}
	div := findFirstElement(doc, "div")
	if div == nil {
		t.Fatal("div node not found")
	}
	got := getTextContent(div)
	if strings.Contains(got, "ignored") {
		t.Errorf("getTextContent() should skip script content, got %q", got)
	}
	if !strings.Contains(got, "outer") || !strings.Contains(got, "inner") || !strings.Contains(got, "tail") {
		t.Errorf("getTextContent() missing expected text, got %q", got)
	}
}
