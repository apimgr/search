package instant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLHandler handles YAML formatting and conversion
type YAMLHandler struct {
	patterns []*regexp.Regexp
}

// NewYAMLHandler creates a new YAML handler
func NewYAMLHandler() *YAMLHandler {
	return &YAMLHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^yaml:to-json\s+(.+)$`),
			regexp.MustCompile(`(?i)^yaml:from-json\s+(.+)$`),
			regexp.MustCompile(`(?i)^yaml:(.+)$`),
			regexp.MustCompile(`(?i)^yaml[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^format\s+yaml[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^validate\s+yaml[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^yaml\s+to\s+json[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^json\s+to\s+yaml[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^convert\s+yaml[:\s]+(.+)$`),
		},
	}
}

func (h *YAMLHandler) Name() string {
	return "yaml"
}

func (h *YAMLHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *YAMLHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *YAMLHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract data from query
	lowerQuery := strings.ToLower(query)

	// Determine mode
	mode := "format" // default
	if strings.Contains(lowerQuery, "yaml:to-json") || strings.Contains(lowerQuery, "yaml to json") {
		mode = "to-json"
	} else if strings.Contains(lowerQuery, "yaml:from-json") || strings.Contains(lowerQuery, "json to yaml") {
		mode = "from-json"
	} else if strings.Contains(lowerQuery, "validate") {
		mode = "validate"
	}

	data := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			data = strings.TrimSpace(matches[1])
			break
		}
	}

	if data == "" {
		return nil, nil
	}

	// Check for mode prefix in data
	if strings.HasPrefix(strings.ToLower(data), "to-json ") {
		mode = "to-json"
		data = strings.TrimPrefix(data, "to-json ")
		data = strings.TrimPrefix(data, "To-json ")
	} else if strings.HasPrefix(strings.ToLower(data), "from-json ") {
		mode = "from-json"
		data = strings.TrimPrefix(data, "from-json ")
		data = strings.TrimPrefix(data, "From-json ")
	}

	var parsed interface{}
	var inputFormat string
	var err error
	var yamlOutput, jsonOutput string

	switch mode {
	case "from-json":
		// Convert JSON to YAML
		err = json.Unmarshal([]byte(data), &parsed)
		if err != nil {
			lineNum, charPos := getJSONErrorLinePosition(data, err)
			errorMsg := fmt.Sprintf("%s (line %d, position %d)", err.Error(), lineNum, charPos)
			return &Answer{
				Type:    AnswerTypeYAML,
				Query:   query,
				Title:   "JSON to YAML Converter",
				Content: fmt.Sprintf(`<div class="yaml-result yaml-error">
<strong>Status:</strong> <span style="color: red;">Invalid JSON</span><br><br>
<strong>Error:</strong> %s<br><br>
<strong>Input:</strong><br>
<pre><code>%s</code></pre>
</div>`, escapeHTML(errorMsg), escapeHTML(addYAMLLineNumbers(data))),
				Data: map[string]interface{}{
					"valid":    false,
					"error":    err.Error(),
					"line":     lineNum,
					"position": charPos,
					"input":    data,
				},
			}, nil
		}
		inputFormat = "JSON"

		// Generate YAML output
		var yamlBuf bytes.Buffer
		yamlEncoder := yaml.NewEncoder(&yamlBuf)
		yamlEncoder.SetIndent(2)
		yamlEncoder.Encode(parsed)
		yamlOutput = strings.TrimSpace(yamlBuf.String())

	case "to-json":
		// Convert YAML to JSON
		err = yaml.Unmarshal([]byte(data), &parsed)
		if err != nil {
			lineNum, col := getYAMLErrorLineCol(err)
			errorMsg := formatYAMLError(err, lineNum, col)
			return &Answer{
				Type:    AnswerTypeYAML,
				Query:   query,
				Title:   "YAML to JSON Converter",
				Content: fmt.Sprintf(`<div class="yaml-result yaml-error">
<strong>Status:</strong> <span style="color: red;">Invalid YAML</span><br><br>
<strong>Error:</strong> %s<br><br>
<strong>Input:</strong><br>
<pre><code>%s</code></pre>
</div>`, escapeHTML(errorMsg), escapeHTML(addYAMLLineNumbers(data))),
				Data: map[string]interface{}{
					"valid":  false,
					"error":  err.Error(),
					"line":   lineNum,
					"column": col,
					"input":  data,
				},
			}, nil
		}
		inputFormat = "YAML"

		// Generate JSON output
		var jsonBuf bytes.Buffer
		jsonEncoder := json.NewEncoder(&jsonBuf)
		jsonEncoder.SetIndent("", "  ")
		jsonEncoder.SetEscapeHTML(false)
		jsonEncoder.Encode(parsed)
		jsonOutput = strings.TrimSpace(jsonBuf.String())

	default:
		// Format/validate YAML - try YAML first, then JSON
		err = yaml.Unmarshal([]byte(data), &parsed)
		if err != nil {
			// Try JSON as fallback
			err = json.Unmarshal([]byte(data), &parsed)
			if err != nil {
				lineNum, col := getYAMLErrorLineCol(err)
				errorMsg := formatYAMLError(err, lineNum, col)
				return &Answer{
					Type:    AnswerTypeYAML,
					Query:   query,
					Title:   "YAML Validator",
					Content: fmt.Sprintf(`<div class="yaml-result yaml-error">
<strong>Status:</strong> <span style="color: red;">Invalid YAML/JSON</span><br><br>
<strong>Error:</strong> %s<br><br>
<strong>Input:</strong><br>
<pre><code>%s</code></pre>
</div>`, escapeHTML(errorMsg), escapeHTML(addYAMLLineNumbers(data))),
					Data: map[string]interface{}{
						"valid":  false,
						"error":  err.Error(),
						"line":   lineNum,
						"column": col,
						"input":  data,
					},
				}, nil
			}
			inputFormat = "JSON"
		} else {
			inputFormat = "YAML"
		}

		// Generate both outputs
		var yamlBuf bytes.Buffer
		yamlEncoder := yaml.NewEncoder(&yamlBuf)
		yamlEncoder.SetIndent(2)
		yamlEncoder.Encode(parsed)
		yamlOutput = strings.TrimSpace(yamlBuf.String())

		var jsonBuf bytes.Buffer
		jsonEncoder := json.NewEncoder(&jsonBuf)
		jsonEncoder.SetIndent("", "  ")
		jsonEncoder.SetEscapeHTML(false)
		jsonEncoder.Encode(parsed)
		jsonOutput = strings.TrimSpace(jsonBuf.String())
	}

	// Get data type info and structure stats
	dataType := getYAMLDataType(parsed)
	depth := getYAMLTreeDepth(parsed)
	keys := countYAMLTotalKeys(parsed)
	listCount := countYAMLLists(parsed)
	mapCount := countYAMLMaps(parsed)

	// Build content based on mode
	var content string
	var title string

	switch mode {
	case "from-json":
		title = "JSON to YAML Converter"
		content = fmt.Sprintf(`<div class="yaml-result">
<strong>Status:</strong> <span style="color: green;">Valid JSON converted to YAML</span><br><br>
<strong>Statistics:</strong>
<ul>
<li>Data Type: %s</li>
<li>Depth: %d</li>
<li>Keys: %d</li>
<li>Maps: %d</li>
<li>Lists: %d</li>
</ul>
<strong>YAML Output:</strong><br>
<pre><code>%s</code></pre>
<button class="copy-btn" onclick="copyCode(this)">Copy</button>
</div>`,
			dataType, depth, keys, mapCount, listCount,
			escapeHTML(yamlOutput))

	case "to-json":
		title = "YAML to JSON Converter"
		content = fmt.Sprintf(`<div class="yaml-result">
<strong>Status:</strong> <span style="color: green;">Valid YAML converted to JSON</span><br><br>
<strong>Statistics:</strong>
<ul>
<li>Data Type: %s</li>
<li>Depth: %d</li>
<li>Keys: %d</li>
<li>Maps: %d</li>
<li>Lists: %d</li>
</ul>
<strong>JSON Output:</strong><br>
<pre><code>%s</code></pre>
<button class="copy-btn" onclick="copyCode(this)">Copy</button>
</div>`,
			dataType, depth, keys, mapCount, listCount,
			escapeHTML(jsonOutput))

	default:
		if mode == "validate" {
			title = "YAML Validator"
		} else {
			title = "YAML Formatter"
		}
		content = fmt.Sprintf(`<div class="yaml-result">
<strong>Status:</strong> <span style="color: green;">Valid %s</span><br><br>
<strong>Statistics:</strong>
<ul>
<li>Data Type: %s</li>
<li>Depth: %d</li>
<li>Keys: %d</li>
<li>Maps: %d</li>
<li>Lists: %d</li>
</ul>
<strong>YAML Output:</strong><br>
<pre><code>%s</code></pre>
<button class="copy-btn" onclick="copyCode(this)">Copy YAML</button>
<br><br>
<strong>JSON Output:</strong><br>
<pre><code>%s</code></pre>
<button class="copy-btn" onclick="copyCode(this)">Copy JSON</button>
</div>`,
			inputFormat, dataType, depth, keys, mapCount, listCount,
			escapeHTML(yamlOutput),
			escapeHTML(jsonOutput))
	}

	return &Answer{
		Type:    AnswerTypeYAML,
		Query:   query,
		Title:   title,
		Content: content,
		Data: map[string]interface{}{
			"valid":       true,
			"mode":        mode,
			"input":       data,
			"inputFormat": strings.ToLower(inputFormat),
			"yaml":        yamlOutput,
			"json":        jsonOutput,
			"type":        dataType,
			"depth":       depth,
			"keys":        keys,
			"maps":        mapCount,
			"lists":       listCount,
		},
	}, nil
}

// getYAMLDataType returns the type of YAML data
func getYAMLDataType(v interface{}) string {
	switch val := v.(type) {
	case map[string]interface{}:
		return fmt.Sprintf("Object (%d keys)", len(val))
	case []interface{}:
		return fmt.Sprintf("Array (%d items)", len(val))
	case string:
		return "String"
	case int, int64, float64:
		return "Number"
	case bool:
		return "Boolean"
	case nil:
		return "Null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// getYAMLErrorLineCol extracts line and column from YAML error
func getYAMLErrorLineCol(err error) (int, int) {
	errStr := err.Error()

	// Try to parse line number from error message
	// Format: "yaml: line X: ..."
	var line int
	if _, scanErr := fmt.Sscanf(errStr, "yaml: line %d:", &line); scanErr == nil {
		return line, 1
	}

	// Try another format
	var col int
	if _, scanErr := fmt.Sscanf(errStr, "yaml: unmarshal errors:\n  line %d: column %d:", &line, &col); scanErr == nil {
		return line, col
	}

	return 0, 0
}

// formatYAMLError formats a YAML error with line information
func formatYAMLError(err error, line, col int) string {
	if line > 0 {
		if col > 0 {
			return fmt.Sprintf("%s (line %d, column %d)", err.Error(), line, col)
		}
		return fmt.Sprintf("%s (line %d)", err.Error(), line)
	}
	return err.Error()
}

// addYAMLLineNumbers adds line numbers to text for error display
func addYAMLLineNumbers(s string) string {
	lines := strings.Split(s, "\n")
	var result strings.Builder
	for i, line := range lines {
		result.WriteString(fmt.Sprintf("%3d | %s\n", i+1, line))
	}
	return result.String()
}

// getYAMLTreeDepth calculates the maximum nesting depth
func getYAMLTreeDepth(v interface{}) int {
	switch val := v.(type) {
	case map[string]interface{}:
		maxDepth := 0
		for _, v := range val {
			d := getYAMLTreeDepth(v)
			if d > maxDepth {
				maxDepth = d
			}
		}
		return maxDepth + 1
	case []interface{}:
		maxDepth := 0
		for _, v := range val {
			d := getYAMLTreeDepth(v)
			if d > maxDepth {
				maxDepth = d
			}
		}
		return maxDepth + 1
	default:
		return 0
	}
}

// countYAMLTotalKeys counts total keys in YAML structure
func countYAMLTotalKeys(v interface{}) int {
	count := 0
	switch val := v.(type) {
	case map[string]interface{}:
		count += len(val)
		for _, v := range val {
			count += countYAMLTotalKeys(v)
		}
	case []interface{}:
		for _, v := range val {
			count += countYAMLTotalKeys(v)
		}
	}
	return count
}

// countYAMLLists counts total lists in YAML structure
func countYAMLLists(v interface{}) int {
	count := 0
	switch val := v.(type) {
	case map[string]interface{}:
		for _, v := range val {
			count += countYAMLLists(v)
		}
	case []interface{}:
		count = 1
		for _, v := range val {
			count += countYAMLLists(v)
		}
	}
	return count
}

// countYAMLMaps counts total maps in YAML structure
func countYAMLMaps(v interface{}) int {
	count := 0
	switch val := v.(type) {
	case map[string]interface{}:
		count = 1
		for _, v := range val {
			count += countYAMLMaps(v)
		}
	case []interface{}:
		for _, v := range val {
			count += countYAMLMaps(v)
		}
	}
	return count
}
