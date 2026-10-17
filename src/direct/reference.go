package direct

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/common/qr"
	"github.com/apimgr/search/src/version"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// UserAgentHandler handles useragent:{string} queries
type UserAgentHandler struct{}

// NewUserAgentHandler creates a new user agent parser handler
func NewUserAgentHandler() *UserAgentHandler {
	return &UserAgentHandler{}
}

func (h *UserAgentHandler) Type() AnswerType {
	return AnswerTypeUserAgent
}

func (h *UserAgentHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" || strings.ToLower(term) == "my" {
		// Return current UA
		term = version.BrowserUserAgent
	}

	// Parse the user agent
	parsed := parseUserAgent(term)
	lang := LangFromContext(ctx)

	data := map[string]interface{}{
		"raw":    term,
		"parsed": parsed,
	}

	return &Answer{
		Type:        AnswerTypeUserAgent,
		Term:        truncateString(term, 50),
		Title:       i18n.T(lang, "direct.user_agent_parser_title"),
		Description: fmt.Sprintf("%s on %s", parsed["browser"], parsed["os"]),
		Content:     formatUserAgentContent(term, parsed),
		Source:      "User Agent Parser",
		Data:        data,
	}, nil
}

func parseUserAgent(ua string) map[string]string {
	result := map[string]string{
		"browser":      "Unknown",
		"browserVer":   "",
		"engine":       "Unknown",
		"os":           "Unknown",
		"osVer":        "",
		"device":       "Desktop",
		"architecture": "",
		"isBot":        "No",
	}

	uaLower := strings.ToLower(ua)

	// Detect bots
	botPatterns := []string{"bot", "crawler", "spider", "scraper", "curl", "wget", "python-requests"}
	for _, pattern := range botPatterns {
		if strings.Contains(uaLower, pattern) {
			result["isBot"] = "Yes"
			result["device"] = "Bot"
			break
		}
	}

	// Browser detection
	browsers := []struct{ name, pattern, versionRe string }{
		{"Edge", "edg", `Edg[eA]?/(\d+[\.\d]*)`},
		{"Opera", "opr/", `OPR/(\d+[\.\d]*)`},
		{"Chrome", "chrome", `Chrome/(\d+[\.\d]*)`},
		{"Firefox", "firefox", `Firefox/(\d+[\.\d]*)`},
		{"Safari", "safari", `Version/(\d+[\.\d]*)`},
		{"IE", "msie", `MSIE (\d+[\.\d]*)`},
		{"IE", "trident", `rv:(\d+[\.\d]*)`},
	}

	for _, b := range browsers {
		if strings.Contains(uaLower, b.pattern) {
			result["browser"] = b.name
			if re := regexp.MustCompile(b.versionRe); re != nil {
				if matches := re.FindStringSubmatch(ua); len(matches) > 1 {
					result["browserVer"] = matches[1]
				}
			}
			break
		}
	}

	// Engine detection
	if strings.Contains(uaLower, "webkit") {
		result["engine"] = "WebKit"
		if strings.Contains(uaLower, "chrome") {
			result["engine"] = "Blink"
		}
	} else if strings.Contains(uaLower, "gecko") {
		result["engine"] = "Gecko"
	} else if strings.Contains(uaLower, "trident") {
		result["engine"] = "Trident"
	}

	// OS detection
	osPatterns := []struct{ name, pattern, versionRe string }{
		{"Windows 11", "windows nt 10.0", ""},
		{"Windows 10", "windows nt 10", ""},
		{"Windows 8.1", "windows nt 6.3", ""},
		{"Windows 8", "windows nt 6.2", ""},
		{"Windows 7", "windows nt 6.1", ""},
		{"macOS", "mac os x", `Mac OS X (\d+[_\.\d]*)`},
		{"iOS", "iphone", `OS (\d+[_\.\d]*)`},
		{"iOS", "ipad", `OS (\d+[_\.\d]*)`},
		{"Android", "android", `Android (\d+[\.\d]*)`},
		{"Linux", "linux", ""},
		{"Chrome OS", "cros", ""},
	}

	for _, o := range osPatterns {
		if strings.Contains(uaLower, o.pattern) {
			result["os"] = o.name
			if o.versionRe != "" {
				if re := regexp.MustCompile(o.versionRe); re != nil {
					if matches := re.FindStringSubmatch(ua); len(matches) > 1 {
						result["osVer"] = strings.ReplaceAll(matches[1], "_", ".")
					}
				}
			}
			break
		}
	}

	// Device detection
	if strings.Contains(uaLower, "mobile") || strings.Contains(uaLower, "android") && !strings.Contains(uaLower, "tablet") {
		result["device"] = "Mobile"
	} else if strings.Contains(uaLower, "tablet") || strings.Contains(uaLower, "ipad") {
		result["device"] = "Tablet"
	}

	// Architecture
	if strings.Contains(ua, "x64") || strings.Contains(ua, "Win64") || strings.Contains(ua, "x86_64") {
		result["architecture"] = "64-bit"
	} else if strings.Contains(ua, "x86") || strings.Contains(ua, "Win32") || strings.Contains(ua, "i686") {
		result["architecture"] = "32-bit"
	} else if strings.Contains(uaLower, "arm") {
		result["architecture"] = "ARM"
	}

	return result
}

func formatUserAgentContent(ua string, parsed map[string]string) string {
	var html strings.Builder
	html.WriteString("<div class=\"useragent-content\">")
	html.WriteString("<h1>User Agent Parser</h1>")

	html.WriteString("<h2>User Agent String</h2>")
	fmt.Fprintf(&html, "<pre class=\"ua-string\"><code>%s</code></pre>", escapeHTML(ua))

	html.WriteString("<h2>Parsed Information</h2>")
	html.WriteString("<table class=\"ua-table\">")
	html.WriteString("<tbody>")

	browserFull := parsed["browser"]
	if parsed["browserVer"] != "" {
		browserFull += " " + parsed["browserVer"]
	}
	fmt.Fprintf(&html, "<tr><td>Browser</td><td>%s</td></tr>", escapeHTML(browserFull))
	fmt.Fprintf(&html, "<tr><td>Rendering Engine</td><td>%s</td></tr>", escapeHTML(parsed["engine"]))

	osFull := parsed["os"]
	if parsed["osVer"] != "" {
		osFull += " " + parsed["osVer"]
	}
	fmt.Fprintf(&html, "<tr><td>Operating System</td><td>%s</td></tr>", escapeHTML(osFull))
	fmt.Fprintf(&html, "<tr><td>Device Type</td><td>%s</td></tr>", escapeHTML(parsed["device"]))

	if parsed["architecture"] != "" {
		fmt.Fprintf(&html, "<tr><td>Architecture</td><td>%s</td></tr>", escapeHTML(parsed["architecture"]))
	}

	fmt.Fprintf(&html, "<tr><td>Is Bot</td><td>%s</td></tr>", escapeHTML(parsed["isBot"]))

	html.WriteString("</tbody></table>")
	html.WriteString("</div>")
	return html.String()
}

// MIMEHandler handles mime:{type} queries
type MIMEHandler struct{}

// NewMIMEHandler creates a new MIME type handler
func NewMIMEHandler() *MIMEHandler {
	return &MIMEHandler{}
}

func (h *MIMEHandler) Type() AnswerType {
	return AnswerTypeMIME
}

// MIME types database
var mimeDB = map[string]struct {
	Type        string
	Extensions  []string
	Category    string
	Description string
	Binary      bool
}{
	// Text
	"text/plain":      {"text/plain", []string{".txt"}, "Text", "Plain text", false},
	"text/html":       {"text/html", []string{".html", ".htm"}, "Text", "HTML document", false},
	"text/css":        {"text/css", []string{".css"}, "Text", "CSS stylesheet", false},
	"text/javascript": {"text/javascript", []string{".js", ".mjs"}, "Text", "JavaScript", false},
	"text/csv":        {"text/csv", []string{".csv"}, "Text", "CSV data", false},
	"text/xml":        {"text/xml", []string{".xml"}, "Text", "XML document", false},
	"text/markdown":   {"text/markdown", []string{".md", ".markdown"}, "Text", "Markdown", false},

	// Application
	"application/json":             {"application/json", []string{".json"}, "Application", "JSON data", false},
	"application/xml":              {"application/xml", []string{".xml"}, "Application", "XML document", false},
	"application/pdf":              {"application/pdf", []string{".pdf"}, "Application", "PDF document", true},
	"application/zip":              {"application/zip", []string{".zip"}, "Application", "ZIP archive", true},
	"application/gzip":             {"application/gzip", []string{".gz", ".gzip"}, "Application", "Gzip archive", true},
	"application/x-tar":            {"application/x-tar", []string{".tar"}, "Application", "Tar archive", true},
	"application/x-rar-compressed": {"application/x-rar-compressed", []string{".rar"}, "Application", "RAR archive", true},
	"application/x-7z-compressed":  {"application/x-7z-compressed", []string{".7z"}, "Application", "7-Zip archive", true},
	"application/octet-stream":     {"application/octet-stream", []string{".bin"}, "Application", "Binary data", true},
	"application/x-executable":     {"application/x-executable", []string{".exe"}, "Application", "Executable", true},

	// Microsoft Office
	"application/msword": {"application/msword", []string{".doc"}, "Application", "MS Word", true},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", []string{".docx"}, "Application", "MS Word (OOXML)", true},
	"application/vnd.ms-excel": {"application/vnd.ms-excel", []string{".xls"}, "Application", "MS Excel", true},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", []string{".xlsx"}, "Application", "MS Excel (OOXML)", true},
	"application/vnd.ms-powerpoint":                                             {"application/vnd.ms-powerpoint", []string{".ppt"}, "Application", "MS PowerPoint", true},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", []string{".pptx"}, "Application", "MS PowerPoint (OOXML)", true},

	// Images
	"image/jpeg":    {"image/jpeg", []string{".jpg", ".jpeg"}, "Image", "JPEG image", true},
	"image/png":     {"image/png", []string{".png"}, "Image", "PNG image", true},
	"image/gif":     {"image/gif", []string{".gif"}, "Image", "GIF image", true},
	"image/webp":    {"image/webp", []string{".webp"}, "Image", "WebP image", true},
	"image/svg+xml": {"image/svg+xml", []string{".svg"}, "Image", "SVG image", false},
	"image/bmp":     {"image/bmp", []string{".bmp"}, "Image", "Bitmap image", true},
	"image/x-icon":  {"image/x-icon", []string{".ico"}, "Image", "Icon", true},
	"image/tiff":    {"image/tiff", []string{".tiff", ".tif"}, "Image", "TIFF image", true},
	"image/avif":    {"image/avif", []string{".avif"}, "Image", "AVIF image", true},

	// Audio
	"audio/mpeg": {"audio/mpeg", []string{".mp3"}, "Audio", "MP3 audio", true},
	"audio/ogg":  {"audio/ogg", []string{".ogg"}, "Audio", "Ogg audio", true},
	"audio/wav":  {"audio/wav", []string{".wav"}, "Audio", "WAV audio", true},
	"audio/webm": {"audio/webm", []string{".weba"}, "Audio", "WebM audio", true},
	"audio/flac": {"audio/flac", []string{".flac"}, "Audio", "FLAC audio", true},
	"audio/aac":  {"audio/aac", []string{".aac"}, "Audio", "AAC audio", true},

	// Video
	"video/mp4":        {"video/mp4", []string{".mp4"}, "Video", "MP4 video", true},
	"video/webm":       {"video/webm", []string{".webm"}, "Video", "WebM video", true},
	"video/ogg":        {"video/ogg", []string{".ogv"}, "Video", "Ogg video", true},
	"video/x-matroska": {"video/x-matroska", []string{".mkv"}, "Video", "Matroska video", true},
	"video/quicktime":  {"video/quicktime", []string{".mov"}, "Video", "QuickTime video", true},
	"video/x-msvideo":  {"video/x-msvideo", []string{".avi"}, "Video", "AVI video", true},

	// Fonts
	"font/woff":  {"font/woff", []string{".woff"}, "Font", "WOFF font", true},
	"font/woff2": {"font/woff2", []string{".woff2"}, "Font", "WOFF2 font", true},
	"font/ttf":   {"font/ttf", []string{".ttf"}, "Font", "TrueType font", true},
	"font/otf":   {"font/otf", []string{".otf"}, "Font", "OpenType font", true},
}

// Extension to MIME lookup
var extToMIME = map[string]string{}

func init() {
	for mime, info := range mimeDB {
		for _, ext := range info.Extensions {
			extToMIME[ext] = mime
		}
	}
}

func (h *MIMEHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("MIME type or extension required")
	}

	// Lookup by extension
	if strings.HasPrefix(term, ".") {
		if mime, ok := extToMIME[term]; ok {
			term = mime
		}
	} else if !strings.Contains(term, "/") {
		// Try with dot
		if mime, ok := extToMIME["."+term]; ok {
			term = mime
		}
	}

	info, ok := mimeDB[term]
	if !ok {
		return &Answer{
			Type:        AnswerTypeMIME,
			Term:        term,
			Title:       fmt.Sprintf("MIME: %s", term),
			Description: i18n.T(lang, "direct.unknown_mime_type"),
			Content:     fmt.Sprintf("<p>Unknown MIME type: <code>%s</code></p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	data := map[string]interface{}{
		"type":        info.Type,
		"extensions":  info.Extensions,
		"category":    info.Category,
		"description": info.Description,
		"binary":      info.Binary,
	}

	return &Answer{
		Type:        AnswerTypeMIME,
		Term:        term,
		Title:       info.Type,
		Description: info.Description,
		Content:     formatMIMEContent(info.Type, info.Extensions, info.Category, info.Description, info.Binary),
		Source:      "MIME Database",
		SourceURL:   "https://www.iana.org/assignments/media-types/",
		Data:        data,
	}, nil
}

func formatMIMEContent(mimeType string, extensions []string, category, description string, binary bool) string {
	var html strings.Builder
	html.WriteString("<div class=\"mime-content\">")
	fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(mimeType))

	html.WriteString("<table class=\"mime-table\">")
	html.WriteString("<tbody>")
	fmt.Fprintf(&html, "<tr><td>MIME Type</td><td><code>%s</code></td></tr>", escapeHTML(mimeType))
	fmt.Fprintf(&html, "<tr><td>Category</td><td>%s</td></tr>", escapeHTML(category))
	fmt.Fprintf(&html, "<tr><td>Description</td><td>%s</td></tr>", escapeHTML(description))
	fmt.Fprintf(&html, "<tr><td>Extensions</td><td><code>%s</code></td></tr>", escapeHTML(strings.Join(extensions, ", ")))

	binaryStr := "No"
	if binary {
		binaryStr = "Yes"
	}
	fmt.Fprintf(&html, "<tr><td>Binary</td><td>%s</td></tr>", binaryStr)

	html.WriteString("</tbody></table>")
	html.WriteString("</div>")
	return html.String()
}

// LicenseHandler handles license:{name} queries
type LicenseHandler struct {
	client *http.Client
}

// NewLicenseHandler creates a new license handler
func NewLicenseHandler() *LicenseHandler {
	return &LicenseHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *LicenseHandler) Type() AnswerType {
	return AnswerTypeLicense
}

// License database
var licenseDB = map[string]struct {
	SPDX        string
	Name        string
	Permissions []string
	Conditions  []string
	Limitations []string
	OSI         bool
	Copyleft    bool
}{
	"mit": {"MIT", "MIT License",
		[]string{"Commercial use", "Distribution", "Modification", "Private use"},
		[]string{"License and copyright notice"},
		[]string{"Liability", "Warranty"},
		true, false},
	"apache-2.0": {"Apache-2.0", "Apache License 2.0",
		[]string{"Commercial use", "Distribution", "Modification", "Patent use", "Private use"},
		[]string{"License and copyright notice", "State changes"},
		[]string{"Liability", "Trademark use", "Warranty"},
		true, false},
	"gpl-3.0": {"GPL-3.0", "GNU General Public License v3.0",
		[]string{"Commercial use", "Distribution", "Modification", "Patent use", "Private use"},
		[]string{"Disclose source", "License and copyright notice", "Same license", "State changes"},
		[]string{"Liability", "Warranty"},
		true, true},
	"gpl-2.0": {"GPL-2.0", "GNU General Public License v2.0",
		[]string{"Commercial use", "Distribution", "Modification", "Private use"},
		[]string{"Disclose source", "License and copyright notice", "Same license", "State changes"},
		[]string{"Liability", "Warranty"},
		true, true},
	"lgpl-3.0": {"LGPL-3.0", "GNU Lesser General Public License v3.0",
		[]string{"Commercial use", "Distribution", "Modification", "Patent use", "Private use"},
		[]string{"Disclose source", "License and copyright notice", "Same license (library)", "State changes"},
		[]string{"Liability", "Warranty"},
		true, true},
	"bsd-3-clause": {"BSD-3-Clause", "BSD 3-Clause License",
		[]string{"Commercial use", "Distribution", "Modification", "Private use"},
		[]string{"License and copyright notice"},
		[]string{"Liability", "Warranty"},
		true, false},
	"bsd-2-clause": {"BSD-2-Clause", "BSD 2-Clause License",
		[]string{"Commercial use", "Distribution", "Modification", "Private use"},
		[]string{"License and copyright notice"},
		[]string{"Liability", "Warranty"},
		true, false},
	"isc": {"ISC", "ISC License",
		[]string{"Commercial use", "Distribution", "Modification", "Private use"},
		[]string{"License and copyright notice"},
		[]string{"Liability", "Warranty"},
		true, false},
	"mpl-2.0": {"MPL-2.0", "Mozilla Public License 2.0",
		[]string{"Commercial use", "Distribution", "Modification", "Patent use", "Private use"},
		[]string{"Disclose source", "License and copyright notice", "Same license (file)"},
		[]string{"Liability", "Trademark use", "Warranty"},
		true, true},
	"unlicense": {"Unlicense", "The Unlicense",
		[]string{"Commercial use", "Distribution", "Modification", "Private use"},
		[]string{},
		[]string{"Liability", "Warranty"},
		true, false},
	"cc0-1.0": {"CC0-1.0", "Creative Commons Zero v1.0 Universal",
		[]string{"Commercial use", "Distribution", "Modification", "Private use"},
		[]string{},
		[]string{"Liability", "Patent use", "Trademark use", "Warranty"},
		false, false},
	"agpl-3.0": {"AGPL-3.0", "GNU Affero General Public License v3.0",
		[]string{"Commercial use", "Distribution", "Modification", "Patent use", "Private use"},
		[]string{"Disclose source", "License and copyright notice", "Network use is distribution", "Same license", "State changes"},
		[]string{"Liability", "Warranty"},
		true, true},
}

func (h *LicenseHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("license name required")
	}

	// Normalize common variations
	term = strings.ReplaceAll(term, " ", "-")
	term = strings.ReplaceAll(term, "_", "-")

	info, ok := licenseDB[term]
	if !ok {
		// Try partial match
		for key, lic := range licenseDB {
			if strings.Contains(key, term) || strings.Contains(strings.ToLower(lic.Name), term) {
				info = lic
				ok = true
				break
			}
		}
	}

	if !ok {
		return &Answer{
			Type:        AnswerTypeLicense,
			Term:        term,
			Title:       i18n.T(lang, "direct.license_lookup_title"),
			Description: i18n.T(lang, "direct.license_not_found"),
			Content:     fmt.Sprintf("<p>Unknown license: <code>%s</code></p><p>Try: MIT, Apache-2.0, GPL-3.0, BSD-3-Clause, ISC</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	data := map[string]interface{}{
		"spdx":        info.SPDX,
		"name":        info.Name,
		"permissions": info.Permissions,
		"conditions":  info.Conditions,
		"limitations": info.Limitations,
		"osi":         info.OSI,
		"copyleft":    info.Copyleft,
	}

	return &Answer{
		Type:        AnswerTypeLicense,
		Term:        term,
		Title:       info.Name,
		Description: fmt.Sprintf("SPDX: %s", info.SPDX),
		Content:     formatLicenseContent(info, fullLicenseText(info.SPDX)),
		Source:      "SPDX License List",
		SourceURL:   fmt.Sprintf("https://spdx.org/licenses/%s.html", info.SPDX),
		Data:        data,
	}, nil
}

func formatLicenseContent(info struct {
	SPDX        string
	Name        string
	Permissions []string
	Conditions  []string
	Limitations []string
	OSI         bool
	Copyleft    bool
}, licenseText string) string {
	var html strings.Builder
	html.WriteString("<div class=\"license-content\">")
	fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(info.Name))
	fmt.Fprintf(&html, "<p><code>%s</code></p>", escapeHTML(info.SPDX))

	// Badges
	if info.OSI {
		html.WriteString("<span class=\"badge osi\">OSI Approved</span> ")
	}
	if info.Copyleft {
		html.WriteString("<span class=\"badge copyleft\">Copyleft</span>")
	} else {
		html.WriteString("<span class=\"badge permissive\">Permissive</span>")
	}

	// Permissions
	if len(info.Permissions) > 0 {
		html.WriteString("<h2>Permissions</h2><ul class=\"permissions\">")
		for _, p := range info.Permissions {
			fmt.Fprintf(&html, "<li>✓ %s</li>", escapeHTML(p))
		}
		html.WriteString("</ul>")
	}

	// Conditions
	if len(info.Conditions) > 0 {
		html.WriteString("<h2>Conditions</h2><ul class=\"conditions\">")
		for _, c := range info.Conditions {
			fmt.Fprintf(&html, "<li>• %s</li>", escapeHTML(c))
		}
		html.WriteString("</ul>")
	}

	// Limitations
	if len(info.Limitations) > 0 {
		html.WriteString("<h2>Limitations</h2><ul class=\"limitations\">")
		for _, l := range info.Limitations {
			fmt.Fprintf(&html, "<li>✗ %s</li>", escapeHTML(l))
		}
		html.WriteString("</ul>")
	}

	if licenseText != "" {
		html.WriteString("<h2>Full License Text</h2>")
		fmt.Fprintf(&html, "<pre><code>%s</code></pre>", escapeHTML(licenseText))
		html.WriteString("<button type=\"button\" class=\"copy-btn\">Copy</button>")
	}

	html.WriteString("</div>")
	return html.String()
}

// CountryHandler handles country:{code} queries
type CountryHandler struct {
	client *http.Client
}

// NewCountryHandler creates a new country handler
func NewCountryHandler() *CountryHandler {
	return &CountryHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *CountryHandler) Type() AnswerType {
	return AnswerTypeCountry
}

// countryDataset is the URL for the mledoze/countries static dataset.
// restcountries.com's free v3.1 API was deprecated (returns HTTP 200 with
// a {success:false} error body instead of the documented array) and its
// v5 API now requires a paid authorization key, so it can no longer be
// used without credentials. mledoze/countries is the same community
// dataset restcountries.com itself is built from, published as a static
// JSON file with no authentication required.
const countryDataset = "https://raw.githubusercontent.com/mledoze/countries/master/dist/countries.json"

func (h *CountryHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("country code or name required")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", countryDataset, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return &Answer{
			Type:        AnswerTypeCountry,
			Term:        term,
			Title:       i18n.T(lang, "direct.country_lookup_title"),
			Description: i18n.T(lang, "direct.country_data_source_unavailable"),
			Content:     "<p>Country data source is temporarily unavailable.</p>",
			Error:       "upstream_error",
		}, nil
	}

	var countries []struct {
		Name struct {
			Common   string `json:"common"`
			Official string `json:"official"`
		} `json:"name"`
		CCA2       string   `json:"cca2"`
		CCA3       string   `json:"cca3"`
		CCN3       string   `json:"ccn3"`
		Capital    []string `json:"capital"`
		Region     string   `json:"region"`
		Subregion  string   `json:"subregion"`
		Area       float64  `json:"area"`
		Population int      `json:"population"`
		Timezones  []string `json:"timezones"`
		Car        struct {
			Side string `json:"side"`
		} `json:"car"`
		Flags struct {
			SVG string `json:"svg"`
		} `json:"flags"`
		Maps struct {
			OpenStreetMaps string `json:"openStreetMaps"`
		} `json:"maps"`
		Currencies map[string]struct {
			Name   string `json:"name"`
			Symbol string `json:"symbol"`
		} `json:"currencies"`
		Languages map[string]string `json:"languages"`
		Flag      string            `json:"flag"`
		TLD       []string          `json:"tld"`
		IDD       struct {
			Root     string   `json:"root"`
			Suffixes []string `json:"suffixes"`
		} `json:"idd"`
	}

	if err := decodeBoundedJSON(resp.Body, 2<<20, &countries); err != nil {
		return nil, err
	}

	if len(countries) == 0 {
		return &Answer{
			Type:        AnswerTypeCountry,
			Term:        term,
			Title:       i18n.T(lang, "direct.country_lookup_title"),
			Description: i18n.T(lang, "direct.country_not_found"),
			Content:     fmt.Sprintf("<p>Country not found: <code>%s</code></p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	upperTerm := strings.ToUpper(term)
	lowerTerm := strings.ToLower(term)
	matchIndex := -1
	for i, c := range countries {
		switch {
		case len(term) == 2 && c.CCA2 == upperTerm:
			matchIndex = i
		case len(term) == 3 && c.CCA3 == upperTerm:
			matchIndex = i
		case len(term) == 3 && c.CCN3 == term:
			matchIndex = i
		case strings.ToLower(c.Name.Common) == lowerTerm:
			matchIndex = i
		case strings.ToLower(c.Name.Official) == lowerTerm:
			matchIndex = i
		}
		if matchIndex != -1 {
			break
		}
	}
	if matchIndex == -1 {
		for i, c := range countries {
			if strings.Contains(strings.ToLower(c.Name.Common), lowerTerm) {
				matchIndex = i
				break
			}
		}
	}
	if matchIndex == -1 {
		return &Answer{
			Type:        AnswerTypeCountry,
			Term:        term,
			Title:       i18n.T(lang, "direct.country_lookup_title"),
			Description: i18n.T(lang, "direct.country_not_found"),
			Content:     fmt.Sprintf("<p>Country not found: <code>%s</code></p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	country := countries[matchIndex]

	// Get currency info
	var currencies []string
	for code, cur := range country.Currencies {
		currencies = append(currencies, fmt.Sprintf("%s (%s, %s)", cur.Name, code, cur.Symbol))
	}

	// Get languages
	var languages []string
	for _, lang := range country.Languages {
		languages = append(languages, lang)
	}

	// Get calling code
	callingCode := ""
	if country.IDD.Root != "" && len(country.IDD.Suffixes) > 0 {
		callingCode = country.IDD.Root + country.IDD.Suffixes[0]
	}

	data := map[string]interface{}{
		"name":        country.Name.Common,
		"official":    country.Name.Official,
		"cca2":        country.CCA2,
		"cca3":        country.CCA3,
		"ccn3":        country.CCN3,
		"capital":     country.Capital,
		"population":  country.Population,
		"timezones":   country.Timezones,
		"drivingSide": country.Car.Side,
		"flagSVG":     country.Flags.SVG,
		"mapURL":      country.Maps.OpenStreetMaps,
		"region":      country.Region,
		"subregion":   country.Subregion,
		"area":        country.Area,
		"currencies":  currencies,
		"languages":   languages,
		"tld":         country.TLD,
		"callingCode": callingCode,
		"flag":        country.Flag,
	}

	return &Answer{
		Type:        AnswerTypeCountry,
		Term:        term,
		Title:       fmt.Sprintf("%s %s", country.Flag, country.Name.Common),
		Description: country.Name.Official,
		Content:     formatCountryContent(country.Flag, country.Flags.SVG, country.Name.Common, country.Name.Official, data, UnitsFromContext(ctx)),
		Source:      "mledoze/countries",
		SourceURL:   "https://github.com/mledoze/countries",
		Data:        data,
	}, nil
}

func formatCountryContent(flagEmoji, flagURL, name, official string, data map[string]interface{}, units string) string {
	var html strings.Builder
	html.WriteString("<div class=\"country-content\">")

	// Flag and name
	html.WriteString("<div class=\"country-header\">")
	fmt.Fprintf(&html, "<span class=\"flag-emoji\">%s</span>", escapeHTML(flagEmoji))
	if flagURL != "" {
		fmt.Fprintf(&html, "<img class=\"country-flag\" src=\"%s\" alt=\"Flag of %s\" loading=\"lazy\">", escapeHTML(flagURL), escapeHTML(name))
	}
	fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(name))
	html.WriteString("</div>")
	fmt.Fprintf(&html, "<p class=\"official\">%s</p>", escapeHTML(official))

	html.WriteString("<table class=\"country-table\">")
	html.WriteString("<tbody>")

	if cca2, ok := data["cca2"].(string); ok {
		fmt.Fprintf(&html, "<tr><td>ISO Alpha-2</td><td><code>%s</code></td></tr>", escapeHTML(cca2))
	}
	if cca3, ok := data["cca3"].(string); ok {
		fmt.Fprintf(&html, "<tr><td>ISO Alpha-3</td><td><code>%s</code></td></tr>", escapeHTML(cca3))
	}
	if ccn3, ok := data["ccn3"].(string); ok {
		fmt.Fprintf(&html, "<tr><td>ISO Numeric</td><td><code>%s</code></td></tr>", escapeHTML(ccn3))
	}
	if capital, ok := data["capital"].([]string); ok && len(capital) > 0 {
		fmt.Fprintf(&html, "<tr><td>Capital</td><td>%s</td></tr>", escapeHTML(strings.Join(capital, ", ")))
	}
	if region, ok := data["region"].(string); ok {
		fmt.Fprintf(&html, "<tr><td>Region</td><td>%s</td></tr>", escapeHTML(region))
	}
	if subregion, ok := data["subregion"].(string); ok && subregion != "" {
		fmt.Fprintf(&html, "<tr><td>Subregion</td><td>%s</td></tr>", escapeHTML(subregion))
	}
	if population, ok := data["population"].(int); ok {
		fmt.Fprintf(&html, "<tr><td>Population</td><td>%s</td></tr>", formatNumber(population))
	}
	if area, ok := data["area"].(float64); ok {
		if units == "imperial" {
			areaSqMi := area * 0.386102
			fmt.Fprintf(&html, "<tr><td>Area</td><td>%s sq mi</td></tr>", formatNumber(int(areaSqMi)))
		} else {
			fmt.Fprintf(&html, "<tr><td>Area</td><td>%s km²</td></tr>", formatNumber(int(area)))
		}
	}
	if currencies, ok := data["currencies"].([]string); ok && len(currencies) > 0 {
		fmt.Fprintf(&html, "<tr><td>Currency</td><td>%s</td></tr>", escapeHTML(strings.Join(currencies, ", ")))
	}
	if languages, ok := data["languages"].([]string); ok && len(languages) > 0 {
		fmt.Fprintf(&html, "<tr><td>Languages</td><td>%s</td></tr>", escapeHTML(strings.Join(languages, ", ")))
	}
	if callingCode, ok := data["callingCode"].(string); ok && callingCode != "" {
		fmt.Fprintf(&html, "<tr><td>Calling Code</td><td>%s</td></tr>", escapeHTML(callingCode))
	}
	if tld, ok := data["tld"].([]string); ok && len(tld) > 0 {
		fmt.Fprintf(&html, "<tr><td>TLD</td><td><code>%s</code></td></tr>", escapeHTML(strings.Join(tld, ", ")))
	}
	if drivingSide, ok := data["drivingSide"].(string); ok && drivingSide != "" {
		fmt.Fprintf(&html, "<tr><td>Driving Side</td><td>%s</td></tr>", escapeHTML(cases.Title(language.Und, cases.NoLower).String(drivingSide)))
	}
	if timezones, ok := data["timezones"].([]string); ok && len(timezones) > 0 {
		fmt.Fprintf(&html, "<tr><td>Timezones</td><td>%s</td></tr>", escapeHTML(strings.Join(timezones, ", ")))
	}
	if mapURL, ok := data["mapURL"].(string); ok && mapURL != "" {
		fmt.Fprintf(&html, "<tr><td>Map</td><td><a href=\"%s\" rel=\"noopener\">OpenStreetMap</a></td></tr>", escapeHTML(mapURL))
	}

	html.WriteString("</tbody></table>")
	html.WriteString("</div>")
	return html.String()
}

func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	// Add thousand separators
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}

// ASCIIHandler handles ascii:{text} queries (ASCII art generator)
type ASCIIHandler struct{}

// NewASCIIHandler creates a new ASCII art handler
func NewASCIIHandler() *ASCIIHandler {
	return &ASCIIHandler{}
}

func (h *ASCIIHandler) Type() AnswerType {
	return AnswerTypeASCII
}

// Simple ASCII font (5x7)
var asciiFontBasic = map[rune][]string{
	'A': {"  █  ", " █ █ ", "█   █", "█████", "█   █", "█   █", "█   █"},
	'B': {"████ ", "█   █", "█   █", "████ ", "█   █", "█   █", "████ "},
	'C': {" ███ ", "█   █", "█    ", "█    ", "█    ", "█   █", " ███ "},
	'D': {"████ ", "█   █", "█   █", "█   █", "█   █", "█   █", "████ "},
	'E': {"█████", "█    ", "█    ", "████ ", "█    ", "█    ", "█████"},
	'F': {"█████", "█    ", "█    ", "████ ", "█    ", "█    ", "█    "},
	'G': {" ███ ", "█   █", "█    ", "█ ███", "█   █", "█   █", " ███ "},
	'H': {"█   █", "█   █", "█   █", "█████", "█   █", "█   █", "█   █"},
	'I': {" ███ ", "  █  ", "  █  ", "  █  ", "  █  ", "  █  ", " ███ "},
	'J': {"  ███", "   █ ", "   █ ", "   █ ", "█  █ ", "█  █ ", " ██  "},
	'K': {"█   █", "█  █ ", "█ █  ", "██   ", "█ █  ", "█  █ ", "█   █"},
	'L': {"█    ", "█    ", "█    ", "█    ", "█    ", "█    ", "█████"},
	'M': {"█   █", "██ ██", "█ █ █", "█   █", "█   █", "█   █", "█   █"},
	'N': {"█   █", "██  █", "█ █ █", "█  ██", "█   █", "█   █", "█   █"},
	'O': {" ███ ", "█   █", "█   █", "█   █", "█   █", "█   █", " ███ "},
	'P': {"████ ", "█   █", "█   █", "████ ", "█    ", "█    ", "█    "},
	'Q': {" ███ ", "█   █", "█   █", "█   █", "█ █ █", "█  █ ", " ██ █"},
	'R': {"████ ", "█   █", "█   █", "████ ", "█ █  ", "█  █ ", "█   █"},
	'S': {" ████", "█    ", "█    ", " ███ ", "    █", "    █", "████ "},
	'T': {"█████", "  █  ", "  █  ", "  █  ", "  █  ", "  █  ", "  █  "},
	'U': {"█   █", "█   █", "█   █", "█   █", "█   █", "█   █", " ███ "},
	'V': {"█   █", "█   █", "█   █", "█   █", "█   █", " █ █ ", "  █  "},
	'W': {"█   █", "█   █", "█   █", "█   █", "█ █ █", "██ ██", "█   █"},
	'X': {"█   █", "█   █", " █ █ ", "  █  ", " █ █ ", "█   █", "█   █"},
	'Y': {"█   █", "█   █", " █ █ ", "  █  ", "  █  ", "  █  ", "  █  "},
	'Z': {"█████", "    █", "   █ ", "  █  ", " █   ", "█    ", "█████"},
	' ': {"     ", "     ", "     ", "     ", "     ", "     ", "     "},
	'!': {"  █  ", "  █  ", "  █  ", "  █  ", "  █  ", "     ", "  █  "},
	'?': {" ███ ", "█   █", "    █", "   █ ", "  █  ", "     ", "  █  "},
	'.': {"     ", "     ", "     ", "     ", "     ", "     ", "  █  "},
	',': {"     ", "     ", "     ", "     ", "     ", "  █  ", " █   "},
	'-': {"     ", "     ", "     ", "█████", "     ", "     ", "     "},
	'0': {" ███ ", "█   █", "█  ██", "█ █ █", "██  █", "█   █", " ███ "},
	'1': {"  █  ", " ██  ", "  █  ", "  █  ", "  █  ", "  █  ", " ███ "},
	'2': {" ███ ", "█   █", "    █", "   █ ", "  █  ", " █   ", "█████"},
	'3': {"█████", "   █ ", "  █  ", "   █ ", "    █", "█   █", " ███ "},
	'4': {"   █ ", "  ██ ", " █ █ ", "█  █ ", "█████", "   █ ", "   █ "},
	'5': {"█████", "█    ", "████ ", "    █", "    █", "█   █", " ███ "},
	'6': {"  ██ ", " █   ", "█    ", "████ ", "█   █", "█   █", " ███ "},
	'7': {"█████", "    █", "   █ ", "  █  ", " █   ", " █   ", " █   "},
	'8': {" ███ ", "█   █", "█   █", " ███ ", "█   █", "█   █", " ███ "},
	'9': {" ███ ", "█   █", "█   █", " ████", "    █", "   █ ", " ██  "},
}

func (h *ASCIIHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("text required")
	}

	// Limit length
	if len(term) > 20 {
		term = term[:20]
	}

	art := generateASCIIArt(strings.ToUpper(term))

	data := map[string]interface{}{
		"text": term,
		"art":  art,
	}

	return &Answer{
		Type:        AnswerTypeASCII,
		Term:        term,
		Title:       i18n.T(lang, "direct.ascii_art_title"),
		Description: fmt.Sprintf("ASCII art for: %s", term),
		Content:     formatASCIIContent(term, art),
		Source:      "ASCII Art Generator",
		Data:        data,
	}, nil
}

func generateASCIIArt(text string) string {
	lines := make([]strings.Builder, 7)

	for _, r := range text {
		glyph, ok := asciiFontBasic[r]
		if !ok {
			glyph = asciiFontBasic[' ']
		}

		for i, line := range glyph {
			lines[i].WriteString(line)
			lines[i].WriteString(" ")
		}
	}

	var result strings.Builder
	for _, line := range lines {
		result.WriteString(line.String())
		result.WriteString("\n")
	}

	return result.String()
}

func formatASCIIContent(text, art string) string {
	var html strings.Builder
	html.WriteString("<div class=\"ascii-content\">")
	html.WriteString("<h1>ASCII Art</h1>")
	fmt.Fprintf(&html, "<p>Text: <strong>%s</strong></p>", escapeHTML(text))

	html.WriteString("<pre class=\"ascii-art\">")
	html.WriteString(escapeHTML(art))
	html.WriteString("</pre>")
	html.WriteString("<button type=\"button\" class=\"copy-btn\">Copy</button>")

	html.WriteString("</div>")
	return html.String()
}

// QRHandler handles qr:{text} queries
type QRHandler struct{}

// NewQRHandler creates a new QR code handler
func NewQRHandler() *QRHandler {
	return &QRHandler{}
}

func (h *QRHandler) Type() AnswerType {
	return AnswerTypeQR
}

func (h *QRHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("text or URL required")
	}
	options := qrOptionsFromContext(ctx)
	if options.Size != 128 && options.Size != 256 && options.Size != 512 && options.Size != 1024 {
		options.Size = 256
	}
	if options.Level != "L" && options.Level != "M" && options.Level != "Q" && options.Level != "H" {
		options.Level = "M"
	}
	if !qr.ValidColor(options.Foreground) {
		options.Foreground = "#000000"
	}
	if !qr.ValidColor(options.Background) {
		options.Background = "#FFFFFF"
	}
	if len(options.Logo) > 0 {
		options.Level = "H"
	}
	imageSizes := []int{128, 256, 512, 1024}
	pngImages := make(map[int]string, len(imageSizes))
	for _, size := range imageSizes {
		png, err := qr.PNGWithLogo(term, options.Level, size, options.Foreground, options.Background, options.Logo)
		if err != nil {
			return nil, fmt.Errorf("generate %d-pixel QR code: %w", size, err)
		}
		pngImages[size] = base64.StdEncoding.EncodeToString(png)
	}
	vector, err := qr.SVGWithLogo(term, options.Level, options.Foreground, options.Background, options.Logo)
	if err != nil {
		return nil, fmt.Errorf("generate SVG QR code: %w", err)
	}
	correctionOptions := make(map[string]string, 3)
	correctionLevels := []string{"L", "Q", "H"}
	if len(options.Logo) > 0 {
		correctionLevels = []string{"H"}
	}
	for _, level := range correctionLevels {
		image, err := qr.SVGWithLogo(term, level, options.Foreground, options.Background, options.Logo)
		if err != nil {
			return nil, fmt.Errorf("generate %s-recovery QR code: %w", level, err)
		}
		correctionOptions[level] = base64.StdEncoding.EncodeToString(image)
	}
	data := map[string]interface{}{
		"text": term, "image_type": "png", "image_b64": pngImages[options.Size],
		"png_sizes": pngImages, "svg_b64": base64.StdEncoding.EncodeToString(vector),
		"error_correction_svg_b64": correctionOptions, "size": options.Size,
		"level": options.Level, "foreground": options.Foreground, "background": options.Background,
	}
	return &Answer{
		Type: AnswerTypeQR, Term: truncateString(term, 50),
		Title:       i18n.T(lang, "direct.qr_code_generator_title"),
		Description: i18n.T(lang, "direct.qr_code_description"),
		Content:     formatQRContent(lang, term, pngImages, vector, correctionOptions, options),
		Source:      "QR Code Generator", Data: data,
	}, nil
}

func isShortenableQRURL(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil
}

func formatQRContent(lang, text string, pngImages map[int]string, svg []byte, correctionOptions map[string]string, options QROptions) string {
	var html strings.Builder
	html.WriteString(`<div class="qr-content"><h1>QR Code</h1>`)
	action := options.FormAction
	fmt.Fprintf(&html, `<form method="post" action="%s" enctype="multipart/form-data" class="qr-options" aria-label="%s"><input type="hidden" name="csrf_token" value="%s"><label>%s <select name="size">`, escapeHTML(action), escapeHTML(i18n.T(lang, "direct.qr_options")), escapeHTML(options.CSRFToken), escapeHTML(i18n.T(lang, "direct.qr_size")))
	for _, option := range []struct {
		label string
		size  int
	}{{"S", 128}, {"M", 256}, {"L", 512}, {"XL", 1024}} {
		selected := ""
		if options.Size == option.size {
			selected = ` selected`
		}
		fmt.Fprintf(&html, `<option value="%s"%s>%s (%dpx)</option>`, option.label, selected, option.label, option.size)
	}
	fmt.Fprintf(&html, `</select></label><label>%s <select name="level">`, escapeHTML(i18n.T(lang, "direct.qr_error_correction")))
	for _, level := range []string{"L", "M", "Q", "H"} {
		selected := ""
		if options.Level == level {
			selected = ` selected`
		}
		fmt.Fprintf(&html, `<option value="%s"%s>%s</option>`, level, selected, level)
	}
	fmt.Fprintf(&html, `</select></label><label>%s <input type="color" name="foreground" value="%s"></label><label>%s <input type="color" name="background" value="%s"></label><label>%s <input type="file" name="logo" accept="image/png,image/jpeg"></label>`, escapeHTML(i18n.T(lang, "direct.qr_foreground")), options.Foreground, escapeHTML(i18n.T(lang, "direct.qr_background")), options.Background, escapeHTML(i18n.T(lang, "direct.qr_logo")))
	if isShortenableQRURL(text) {
		fmt.Fprintf(&html, `<label><input id="shorten_url" type="checkbox" name="shorten_url" value="1"> %s</label>`, escapeHTML(i18n.T(lang, "direct.qr_shorten_url")))
	}
	fmt.Fprintf(&html, `<button type="submit">%s</button></form>`, escapeHTML(i18n.T(lang, "direct.qr_apply")))
	if options.ShortURL != "" {
		fmt.Fprintf(&html, `<p class="qr-short-url">%s <a href="%s" rel="nofollow">%s</a></p>`, escapeHTML(i18n.T(lang, "direct.qr_short_url_label")), escapeHTML(options.ShortURL), escapeHTML(options.ShortURL))
	}
	html.WriteString(`<div class="qr-image">`)
	fmt.Fprintf(&html, `<img src="data:image/png;base64,%s" alt="QR Code" class="qr-code" width="%d" height="%d">`, pngImages[options.Size], options.Size, options.Size)
	html.WriteString(`</div><h2>Encoded Text</h2><pre><code>`)
	html.WriteString(escapeHTML(text))
	fmt.Fprintf(&html, `</code></pre><h2>%s</h2><ul>`, escapeHTML(i18n.T(lang, "direct.qr_download_png")))
	for _, size := range []int{128, 256, 512, 1024} {
		fmt.Fprintf(&html, `<li><a href="data:image/png;base64,%s" download="qr-%d.png">%s</a></li>`, pngImages[size], size, escapeHTML(i18n.T(lang, "direct.qr_size_pixels", size)))
	}
	fmt.Fprintf(&html, `</ul><h2>%s</h2><ul>`, escapeHTML(i18n.T(lang, "direct.qr_download_svg")))
	fmt.Fprintf(&html, `<li><a href="data:image/svg+xml;base64,%s" download="qr.svg">M</a></li>`, base64.StdEncoding.EncodeToString(svg))
	for _, level := range []string{"L", "Q", "H"} {
		image, ok := correctionOptions[level]
		if !ok {
			continue
		}
		label := i18n.T(lang, map[string]string{"L": "direct.qr_recovery_low", "Q": "direct.qr_recovery_quartile", "H": "direct.qr_recovery_high"}[level])
		fmt.Fprintf(&html, `<li><a href="data:image/svg+xml;base64,%s" download="qr-recovery-%s.svg">%s</a></li>`, image, strings.ToLower(level), label)
	}
	html.WriteString(`</ul></div>`)
	return html.String()
}
