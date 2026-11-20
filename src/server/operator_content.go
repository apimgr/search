package server

import (
	"fmt"
	stdhtml "html"
	"html/template"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
	xhtml "golang.org/x/net/html"
)

const maxOperatorContentBytes = 256 << 10

type operatorContentCacheEntry struct {
	found   bool
	content template.HTML
}

var operatorContentCache = struct {
	sync.RWMutex
	entries map[string]operatorContentCacheEntry
}{entries: make(map[string]operatorContentCacheEntry)}

func operatorHTMLExists(name, lang string) bool {
	if name != "header" && name != "about" && name != "privacy" && name != "terms" && name != "footer" {
		return false
	}
	root := filepath.Join(config.GetConfigDir(), "html")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return false
	}
	extensions := []string{".html", ".md", ".txt"}
	if name == "header" || name == "footer" {
		extensions = []string{".html"}
	}
	for _, extension := range extensions {
		candidates := []string{}
		if lang != "" {
			candidates = append(candidates, filepath.Join(root, name+"."+lang+extension))
		}
		candidates = append(candidates, filepath.Join(root, name+extension))
		for _, path := range candidates {
			if _, err := readOperatorContent(root, path); err == nil {
				return true
			} else if !os.IsNotExist(err) {
				if cached, ok := cachedOperatorContent(root, name, lang); ok && cached.found {
					return true
				}
			}
		}
	}
	return false
}

func loadOperatorHTML(name, lang string, page PageData) template.HTML {
	if name != "header" && name != "about" && name != "privacy" && name != "terms" && name != "footer" {
		return ""
	}
	root := filepath.Join(config.GetConfigDir(), "html")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return ""
	}
	extensions := []string{".html", ".md", ".txt"}
	if name == "header" || name == "footer" {
		extensions = []string{".html"}
	}
	for _, extension := range extensions {
		candidates := []string{}
		if lang != "" {
			candidates = append(candidates, filepath.Join(root, name+"."+lang+extension))
		}
		candidates = append(candidates, filepath.Join(root, name+extension))
		invalidFile := false
		for _, path := range candidates {
			contents, err := readOperatorContent(root, path)
			if err != nil {
				if !os.IsNotExist(err) {
					invalidFile = true
					slog.Warn("operator content file ignored", "file", filepath.Base(path), "error", err)
				}
				continue
			}
			contents = expandOperatorVariables(contents, page)
			switch extension {
			case ".md":
				contents = string(markdownToHTML(contents))
			case ".txt":
				contents = operatorTextHTML(contents)
			}
			result := sanitizeOperatorHTML(contents, name == "header" || name == "footer")
			cacheOperatorContent(root, name, lang, operatorContentCacheEntry{found: true, content: result})
			return result
		}
		if invalidFile {
			if cached, ok := cachedOperatorContent(root, name, lang); ok && cached.found {
				return cached.content
			}
		}
	}
	return ""
}

func operatorContentCacheKey(root, name, lang string) string {
	return root + "\x00" + name + "\x00" + lang
}

func cachedOperatorContent(root, name, lang string) (operatorContentCacheEntry, bool) {
	operatorContentCache.RLock()
	defer operatorContentCache.RUnlock()
	entry, ok := operatorContentCache.entries[operatorContentCacheKey(root, name, lang)]
	return entry, ok
}

func cacheOperatorContent(root, name, lang string, entry operatorContentCacheEntry) {
	operatorContentCache.Lock()
	defer operatorContentCache.Unlock()
	operatorContentCache.entries[operatorContentCacheKey(root, name, lang)] = entry
}

func readOperatorContent(root, path string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("symlink target escapes html directory")
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > maxOperatorContentBytes {
		return "", fmt.Errorf("file must be a private regular file no larger than 256 KiB")
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func operatorTextHTML(contents string) string {
	paragraphs := strings.Split(strings.TrimSpace(contents), "\n\n")
	var output strings.Builder
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		output.WriteString("<p>")
		output.WriteString(strings.ReplaceAll(stdhtml.EscapeString(paragraph), "\n", "<br>"))
		output.WriteString("</p>")
	}
	return output.String()
}

func sanitizeOperatorHTML(source string, strict bool) template.HTML {
	document, err := xhtml.Parse(strings.NewReader(source))
	if err != nil {
		return template.HTML(stdhtml.EscapeString(source))
	}
	var output strings.Builder
	var render func(*xhtml.Node)
	render = func(node *xhtml.Node) {
		switch node.Type {
		case xhtml.TextNode:
			output.WriteString(stdhtml.EscapeString(node.Data))
		case xhtml.ElementNode:
			if node.Data == "script" || node.Data == "style" || node.Data == "iframe" || node.Data == "object" || node.Data == "svg" || node.Data == "math" || node.Data == "form" || node.Data == "input" || node.Data == "button" || node.Data == "textarea" || node.Data == "select" {
				return
			}
			if operatorAllowedElement(node.Data, strict) {
				output.WriteByte('<')
				output.WriteString(node.Data)
				for _, attribute := range node.Attr {
					value, keep := operatorAllowedAttribute(node.Data, attribute.Key, attribute.Val, strict)
					if keep {
						fmt.Fprintf(&output, " %s=\"%s\"", attribute.Key, stdhtml.EscapeString(value))
					}
				}
				if node.Data == "a" {
					output.WriteString(` rel="noopener noreferrer nofollow ugc"`)
				}
				output.WriteByte('>')
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				render(child)
			}
			if operatorAllowedElement(node.Data, strict) && node.Data != "br" && node.Data != "hr" && node.Data != "img" {
				output.WriteString("</" + node.Data + ">")
			}
		case xhtml.DocumentNode:
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				render(child)
			}
		}
	}
	render(document)
	return template.HTML(output.String())
}

func operatorAllowedElement(name string, strict bool) bool {
	allowed := map[string]bool{
		"a": true, "blockquote": true, "br": true, "code": true, "del": true,
		"em": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"hr": true, "li": true, "ol": true, "p": true, "pre": true, "strong": true,
		"table": true, "tbody": true, "td": true, "th": true, "thead": true, "tr": true,
		"ul": true, "div": true, "span": true, "img": true, "b": true, "i": true,
		"u": true, "s": true, "small": true, "dl": true, "dt": true, "dd": true, "sup": true, "sub": true,
	}
	if strict {
		return map[string]bool{
			"p": true, "br": true, "span": true, "div": true, "strong": true, "b": true,
			"em": true, "i": true, "u": true, "s": true, "small": true,
			"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
			"ul": true, "ol": true, "li": true, "a": true, "img": true,
		}[name]
	}
	return allowed[name]
}

func operatorAllowedAttribute(element, name, value string, strict bool) (string, bool) {
	switch name {
	case "class", "id":
		return value, true
	case "title":
		return value, element == "a" || element == "img"
	case "target":
		return value, element == "a" && (value == "_blank" || value == "_self")
	case "rel":
		return value, false
	case "alt", "width", "height":
		return value, element == "img"
	case "src":
		return value, element == "img" && safeOperatorImageURL(value)
	case "href":
		if element != "a" || !safeOperatorLinkURL(value) || strings.Contains(strings.ToLower(value), "/server/contact") {
			return "", false
		}
		return value, true
	default:
		return "", false
	}
}

func safeOperatorLinkURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil || parsed.Scheme == "javascript" || parsed.Scheme == "data" {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http" || parsed.Scheme == "" && !strings.HasPrefix(value, "//") || strings.HasPrefix(value, "#")
}

func safeOperatorImageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "data" || parsed.Scheme == "" && !strings.HasPrefix(value, "//")
}

func expandOperatorVariables(contents string, page PageData) string {
	values := map[string]string{
		"{current_year}":    fmt.Sprint(time.Now().Year()),
		"{project_name}":    "search",
		"{project_org}":     "apimgr",
		"{project_version}": version.Version,
		"{build_datetime}":  page.BuildDate,
		"{onion_address}":   page.OnionAddress,
		"{i2p_address}":     page.I2PAddress,
	}
	for variable, value := range values {
		contents = strings.ReplaceAll(contents, variable, value)
	}
	return contents
}
