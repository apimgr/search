package instant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// JSONHandler handles JSON formatting and validation
type JSONHandler struct {
	patterns []*regexp.Regexp
}

// NewJSONHandler creates a new JSON handler
func NewJSONHandler() *JSONHandler {
	return &JSONHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^json:(.+)$`),
			regexp.MustCompile(`(?i)^json[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^format\s+json[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^validate\s+json[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^prettify\s+json[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^minify\s+json[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^json\s+format[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^json\s+validate[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^json\s+minify[:\s]+(.+)$`),
		},
	}
}

func (h *JSONHandler) Name() string {
	return "json"
}

func (h *JSONHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *JSONHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *JSONHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Check for mode
	lowerQuery := strings.ToLower(query)
	minifyMode := strings.Contains(lowerQuery, "minify")

	// Extract JSON from query
	jsonStr := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			jsonStr = strings.TrimSpace(matches[1])
			break
		}
	}

	if jsonStr == "" {
		return nil, nil
	}

	// Check for mode prefix in data
	if strings.HasPrefix(strings.ToLower(jsonStr), "minify ") {
		minifyMode = true
		jsonStr = strings.TrimPrefix(jsonStr, "minify ")
		jsonStr = strings.TrimPrefix(jsonStr, "Minify ")
	}

	// Try to parse the JSON
	var parsed interface{}
	err := json.Unmarshal([]byte(jsonStr), &parsed)

	if err != nil {
		// Get error line and position
		lineNum, charPos := getJSONErrorLinePosition(jsonStr, err)
		errorMsg := fmt.Sprintf("%s (line %d, position %d)", err.Error(), lineNum, charPos)

		return &Answer{
			Type:    AnswerTypeJSON,
			Query:   query,
			Title:   "JSON Validator",
			Content: fmt.Sprintf(`<div class="json-result json-error">
<strong>Status:</strong> <span style="color: red;">Invalid JSON</span><br><br>
<strong>Error:</strong> %s<br><br>
<strong>Input:</strong><br>
<pre><code>%s</code></pre>
</div>`, escapeHTML(errorMsg), escapeHTML(addJSONLineNumbers(jsonStr))),
			Data: map[string]interface{}{
				"valid":    false,
				"error":    err.Error(),
				"line":     lineNum,
				"position": charPos,
				"input":    jsonStr,
			},
		}, nil
	}

	// Format the JSON (pretty print)
	var prettyBuf bytes.Buffer
	prettyEncoder := json.NewEncoder(&prettyBuf)
	prettyEncoder.SetIndent("", "  ")
	prettyEncoder.SetEscapeHTML(false)
	prettyEncoder.Encode(parsed)
	pretty := strings.TrimSpace(prettyBuf.String())

	// Minify the JSON
	var miniBuf bytes.Buffer
	miniEncoder := json.NewEncoder(&miniBuf)
	miniEncoder.SetEscapeHTML(false)
	miniEncoder.Encode(parsed)
	minified := strings.TrimSpace(miniBuf.String())

	// Get statistics and tree structure info
	stats := getJSONStats(parsed)
	depth := getJSONTreeDepth(parsed)
	totalKeys := countJSONTotalKeys(parsed)
	objectCount := countJSONObjects(parsed)
	arrayCount := countJSONArrays(parsed)

	// Add tree structure info to stats
	stats.ExtraInfo += fmt.Sprintf("<li>Depth: %d</li>", depth)
	stats.ExtraInfo += fmt.Sprintf("<li>Total keys: %d</li>", totalKeys)
	stats.ExtraInfo += fmt.Sprintf("<li>Objects: %d</li>", objectCount)
	stats.ExtraInfo += fmt.Sprintf("<li>Arrays: %d</li>", arrayCount)

	// Choose output based on mode
	var output string
	var title string
	if minifyMode {
		output = minified
		title = "JSON Minifier"
	} else {
		output = pretty
		title = "JSON Formatter"
	}

	content := fmt.Sprintf(`<div class="json-result">
<strong>Status:</strong> <span style="color: green;">Valid JSON</span><br><br>
<strong>Statistics:</strong><br>
<ul>
<li>Type: %s</li>
<li>Size (formatted): %d bytes</li>
<li>Size (minified): %d bytes</li>
%s
</ul>
<strong>Output:</strong><br>
<pre><code>%s</code></pre>
<button class="copy-btn" onclick="copyCode(this)">Copy</button>
</div>`,
		stats.Type,
		len(pretty),
		len(minified),
		stats.ExtraInfo,
		escapeHTML(output))

	return &Answer{
		Type:    AnswerTypeJSON,
		Query:   query,
		Title:   title,
		Content: content,
		Data: map[string]interface{}{
			"valid":       true,
			"input":       jsonStr,
			"pretty":      pretty,
			"minified":    minified,
			"type":        stats.Type,
			"depth":       depth,
			"keys":        totalKeys,
			"objects":     objectCount,
			"arrays":      arrayCount,
			"inputSize":   len(jsonStr),
			"outputSize":  len(output),
		},
	}, nil
}

// jsonStats holds statistics about parsed JSON
type jsonStats struct {
	Type      string
	ExtraInfo string
}

// getJSONStats analyzes JSON and returns statistics
func getJSONStats(v interface{}) jsonStats {
	stats := jsonStats{}

	switch val := v.(type) {
	case map[string]interface{}:
		stats.Type = "Object"
		stats.ExtraInfo = fmt.Sprintf("<li>Root keys: %d</li>", len(val))
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		if len(keys) > 0 && len(keys) <= 10 {
			stats.ExtraInfo += fmt.Sprintf("<li>Key names: %s</li>", strings.Join(keys, ", "))
		}
	case []interface{}:
		stats.Type = "Array"
		stats.ExtraInfo = fmt.Sprintf("<li>Elements: %d</li>", len(val))
		if len(val) > 0 {
			elemType := getJSONValueType(val[0])
			stats.ExtraInfo += fmt.Sprintf("<li>First element type: %s</li>", elemType)
		}
	case string:
		stats.Type = "String"
		stats.ExtraInfo = fmt.Sprintf("<li>Length: %d characters</li>", len(val))
	case float64:
		stats.Type = "Number"
		stats.ExtraInfo = fmt.Sprintf("<li>Value: %v</li>", val)
	case bool:
		stats.Type = "Boolean"
		stats.ExtraInfo = fmt.Sprintf("<li>Value: %v</li>", val)
	case nil:
		stats.Type = "Null"
		stats.ExtraInfo = ""
	default:
		stats.Type = "Unknown"
	}

	return stats
}

// getJSONValueType returns the type name of a JSON value
func getJSONValueType(v interface{}) string {
	switch v.(type) {
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

// getJSONTreeDepth calculates the maximum nesting depth
func getJSONTreeDepth(v interface{}) int {
	switch val := v.(type) {
	case map[string]interface{}:
		maxDepth := 0
		for _, v := range val {
			d := getJSONTreeDepth(v)
			if d > maxDepth {
				maxDepth = d
			}
		}
		return maxDepth + 1
	case []interface{}:
		maxDepth := 0
		for _, v := range val {
			d := getJSONTreeDepth(v)
			if d > maxDepth {
				maxDepth = d
			}
		}
		return maxDepth + 1
	default:
		return 0
	}
}

// countJSONTotalKeys counts total keys in JSON structure
func countJSONTotalKeys(v interface{}) int {
	count := 0
	switch val := v.(type) {
	case map[string]interface{}:
		count += len(val)
		for _, v := range val {
			count += countJSONTotalKeys(v)
		}
	case []interface{}:
		for _, v := range val {
			count += countJSONTotalKeys(v)
		}
	}
	return count
}

// countJSONObjects counts total objects in JSON structure
func countJSONObjects(v interface{}) int {
	count := 0
	switch val := v.(type) {
	case map[string]interface{}:
		count = 1
		for _, v := range val {
			count += countJSONObjects(v)
		}
	case []interface{}:
		for _, v := range val {
			count += countJSONObjects(v)
		}
	}
	return count
}

// countJSONArrays counts total arrays in JSON structure
func countJSONArrays(v interface{}) int {
	count := 0
	switch val := v.(type) {
	case map[string]interface{}:
		for _, v := range val {
			count += countJSONArrays(v)
		}
	case []interface{}:
		count = 1
		for _, v := range val {
			count += countJSONArrays(v)
		}
	}
	return count
}

// getJSONErrorLinePosition extracts line and character position from JSON parse error
func getJSONErrorLinePosition(jsonStr string, err error) (int, int) {
	// Try to extract offset from json.SyntaxError
	if syntaxErr, ok := err.(*json.SyntaxError); ok {
		offset := int(syntaxErr.Offset)
		return calculateJSONLineAndPosition(jsonStr, offset)
	}

	// Try json.UnmarshalTypeError
	if typeErr, ok := err.(*json.UnmarshalTypeError); ok {
		offset := int(typeErr.Offset)
		return calculateJSONLineAndPosition(jsonStr, offset)
	}

	return 1, 1
}

// calculateJSONLineAndPosition calculates line number and position from byte offset
func calculateJSONLineAndPosition(s string, offset int) (int, int) {
	if offset <= 0 {
		return 1, 1
	}
	if offset > len(s) {
		offset = len(s)
	}

	line := 1
	lastNewline := 0

	for i := 0; i < offset; i++ {
		if s[i] == '\n' {
			line++
			lastNewline = i + 1
		}
	}

	position := offset - lastNewline + 1
	return line, position
}

// addJSONLineNumbers adds line numbers to JSON string for error display
func addJSONLineNumbers(s string) string {
	lines := strings.Split(s, "\n")
	var result strings.Builder
	for i, line := range lines {
		result.WriteString(fmt.Sprintf("%3d | %s\n", i+1, line))
	}
	return result.String()
}

// Note: escapeHTML is defined in cert.go
