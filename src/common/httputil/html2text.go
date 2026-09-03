package httputil

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// skipTags is the set of HTML elements whose content is fully skipped in text output.
// Per AI.md PART 14: form, input, button, script, style are non-interactive — skip entirely.
var skipTags = map[string]bool{
	"form":   true,
	"input":  true,
	"button": true,
	"script": true,
	"style":  true,
	"head":   true,
	"nav":    true,
	"footer": true,
}

// HTML2TextConverter converts rendered HTML to terminal-friendly plain text.
// width controls line wrapping (0 or negative uses 80).
// Per AI.md PART 14: used by HTTP tools (curl, wget, httpie) rendering path.
func HTML2TextConverter(htmlStr string, width int) string {
	if width <= 0 {
		width = 80
	}

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return stripAllTags(htmlStr)
	}

	var buf strings.Builder
	convertNode(&buf, doc, width, 0)
	result := strings.TrimRight(buf.String(), "\n")
	return result + "\n"
}

// convertNode recursively converts an HTML node tree to formatted text.
func convertNode(buf *strings.Builder, n *html.Node, width, indent int) {
	switch n.Type {
	case html.ElementNode:
		if skipTags[n.Data] {
			return
		}
		convertElement(buf, n, width, indent)
	case html.TextNode:
		text := strings.TrimSpace(n.Data)
		if text != "" {
			buf.WriteString(text)
		}
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			convertNode(buf, c, width, indent)
		}
	}
}

// convertElement dispatches individual HTML element rendering.
func convertElement(buf *strings.Builder, n *html.Node, width, indent int) {
	switch n.Data {
	case "h1":
		text := getTextContent(n)
		line := strings.Repeat("═", width)
		buf.WriteString(line + "\n")
		buf.WriteString(centerText(strings.ToUpper(text), width) + "\n")
		buf.WriteString(line + "\n\n")
	case "h2":
		text := getTextContent(n)
		buf.WriteString("─── " + text + " ───\n\n")
	case "h3":
		text := getTextContent(n)
		buf.WriteString("► " + text + "\n\n")
	case "h4", "h5", "h6":
		text := getTextContent(n)
		buf.WriteString("  " + text + "\n\n")
	case "p":
		text := strings.TrimSpace(getTextContent(n))
		if text != "" {
			buf.WriteString(wordWrap(text, width-indent) + "\n\n")
		}
	case "ul":
		convertList(buf, n, width, indent, false)
		buf.WriteString("\n")
	case "ol":
		convertList(buf, n, width, indent, true)
		buf.WriteString("\n")
	case "li":
		// Handled by convertList — skip bare li encountered outside list
		text := strings.TrimSpace(getTextContent(n))
		if text != "" {
			buf.WriteString("  • " + text + "\n")
		}
	case "a":
		text := strings.TrimSpace(getTextContent(n))
		href := getAttr(n, "href")
		if href == "" || href == "#" {
			buf.WriteString(text)
		} else if text == "" {
			buf.WriteString("[" + href + "]")
		} else {
			buf.WriteString(text + " [" + href + "]")
		}
	case "strong", "b":
		buf.WriteString("*" + strings.TrimSpace(getTextContent(n)) + "*")
	case "em", "i":
		buf.WriteString("_" + strings.TrimSpace(getTextContent(n)) + "_")
	case "code":
		buf.WriteString("`" + getTextContent(n) + "`")
	case "pre":
		text := getTextContent(n)
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			buf.WriteString("    " + line + "\n")
		}
		buf.WriteString("\n")
	case "blockquote":
		text := strings.TrimSpace(getTextContent(n))
		for _, line := range strings.Split(text, "\n") {
			buf.WriteString("│ " + line + "\n")
		}
		buf.WriteString("\n")
	case "hr":
		buf.WriteString(strings.Repeat("─", width) + "\n\n")
	case "br":
		buf.WriteString("\n")
	case "table":
		convertTable(buf, n, width)
	case "tr", "thead", "tbody", "tfoot":
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			convertNode(buf, c, width, indent)
		}
	case "th", "td":
		text := strings.TrimSpace(getTextContent(n))
		buf.WriteString("│ " + text + " ")
	case "div", "section", "article", "main", "aside", "header":
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			convertNode(buf, c, width, indent)
		}
	case "span":
		text := strings.TrimSpace(getTextContent(n))
		if text != "" {
			buf.WriteString(text + " ")
		}
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			convertNode(buf, c, width, indent)
		}
	}
}

// convertList renders an ordered or unordered list.
func convertList(buf *strings.Builder, n *html.Node, width, indent int, ordered bool) {
	counter := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "li" {
			continue
		}
		counter++
		text := strings.TrimSpace(getTextContent(c))
		if text == "" {
			continue
		}
		var prefix string
		if ordered {
			prefix = fmt.Sprintf("  %d. ", counter)
		} else {
			prefix = "  • "
		}
		wrapped := wordWrap(text, width-indent-len(prefix))
		lines := strings.Split(wrapped, "\n")
		for i, line := range lines {
			if i == 0 {
				buf.WriteString(prefix + line + "\n")
			} else if line != "" {
				buf.WriteString(strings.Repeat(" ", len(prefix)) + line + "\n")
			}
		}
	}
}

// convertTable renders an HTML table as ASCII art with box-drawing characters.
func convertTable(buf *strings.Builder, n *html.Node, width int) {
	rows := extractTableRows(n)
	if len(rows) == 0 {
		return
	}

	// Calculate column widths
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return
	}

	colWidths := make([]int, cols)
	for _, row := range rows {
		for j, cell := range row {
			if utf8.RuneCountInString(cell) > colWidths[j] {
				colWidths[j] = utf8.RuneCountInString(cell)
			}
		}
	}

	// Limit total table width to page width; shrink proportionally if needed
	totalWidth := 1
	for _, w := range colWidths {
		totalWidth += w + 3
	}
	if totalWidth > width && cols > 0 {
		excess := totalWidth - width
		perCol := excess / cols
		for j := range colWidths {
			if colWidths[j] > perCol+3 {
				colWidths[j] -= perCol
			}
		}
	}

	separator := buildTableSeparator(colWidths)
	buf.WriteString(separator + "\n")
	for i, row := range rows {
		buf.WriteString("│")
		for j, cell := range row {
			cellWidth := colWidths[j]
			if j >= len(colWidths) {
				break
			}
			// Truncate long cells
			runes := []rune(cell)
			if len(runes) > cellWidth {
				runes = runes[:cellWidth-1]
				cell = string(runes) + "…"
			}
			padding := cellWidth - utf8.RuneCountInString(cell)
			buf.WriteString(" " + cell + strings.Repeat(" ", padding) + " │")
		}
		// Pad missing columns
		for j := len(row); j < cols; j++ {
			buf.WriteString(" " + strings.Repeat(" ", colWidths[j]) + " │")
		}
		buf.WriteString("\n")
		// Draw separator after header row
		if i == 0 && len(rows) > 1 {
			buf.WriteString(buildTableSeparator(colWidths) + "\n")
		}
	}
	buf.WriteString(separator + "\n\n")
}

// buildTableSeparator constructs a ├─┼─┤ style separator line.
func buildTableSeparator(colWidths []int) string {
	var sb strings.Builder
	sb.WriteString("├")
	for i, w := range colWidths {
		sb.WriteString(strings.Repeat("─", w+2))
		if i < len(colWidths)-1 {
			sb.WriteString("┼")
		}
	}
	sb.WriteString("┤")
	return sb.String()
}

// extractTableRows collects all cell text values from a table node.
func extractTableRows(n *html.Node) [][]string {
	var rows [][]string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" {
			var cells []string
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "th" || c.Data == "td") {
					cells = append(cells, strings.TrimSpace(getTextContent(c)))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return rows
}

// getTextContent returns the concatenated text content of a node and all descendants,
// skipping elements in skipTags.
func getTextContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && skipTags[node.Data] {
			return
		}
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// getAttr retrieves the value of an HTML attribute by name.
func getAttr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// centerText centers text within the given width, padding with spaces.
func centerText(text string, width int) string {
	textLen := utf8.RuneCountInString(text)
	if textLen >= width {
		return text
	}
	totalPad := width - textLen
	leftPad := totalPad / 2
	rightPad := totalPad - leftPad
	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}

// wordWrap wraps text to the given column width, preserving words.
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	// Normalize whitespace
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= width {
		return text
	}

	words := strings.Fields(text)
	var lines []string
	var current strings.Builder
	lineLen := 0

	for _, word := range words {
		wordLen := utf8.RuneCountInString(word)
		if lineLen > 0 && lineLen+1+wordLen > width {
			lines = append(lines, current.String())
			current.Reset()
			lineLen = 0
		}
		if lineLen > 0 {
			current.WriteByte(' ')
			lineLen++
		}
		current.WriteString(word)
		lineLen += wordLen
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return strings.Join(lines, "\n")
}

// tagPattern matches any HTML tag for the fallback plain-text stripper.
var tagPattern = regexp.MustCompile(`<[^>]+>`)

// stripAllTags removes all HTML tags, used only when the HTML parser fails.
func stripAllTags(htmlStr string) string {
	return strings.TrimSpace(tagPattern.ReplaceAllString(htmlStr, " "))
}
