package direct

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
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

func (h *UserAgentHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" || strings.ToLower(term) == "my" {
		// Return current UA
		term = version.BrowserUserAgent
	}

	// Parse the user agent
	parsed := parseUserAgent(term)

	data := map[string]interface{}{
		"raw":     term,
		"parsed":  parsed,
	}

	return &Answer{
		Type:        AnswerTypeUserAgent,
		Term:        truncateString(term, 50),
		Title:       "User Agent Parser",
		Description: fmt.Sprintf("%s on %s", parsed["browser"], parsed["os"]),
		Content:     formatUserAgentContent(term, parsed),
		Source:      "User Agent Parser",
		Data:        data,
	}, nil
}

func parseUserAgent(ua string) map[string]string {
	result := map[string]string{
		"browser":     "Unknown",
		"browserVer":  "",
		"engine":      "Unknown",
		"os":          "Unknown",
		"osVer":       "",
		"device":      "Desktop",
		"architecture": "",
		"isBot":       "No",
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
	html.WriteString(fmt.Sprintf("<pre class=\"ua-string\"><code>%s</code></pre>", escapeHTML(ua)))

	html.WriteString("<h2>Parsed Information</h2>")
	html.WriteString("<table class=\"ua-table\">")
	html.WriteString("<tbody>")

	browserFull := parsed["browser"]
	if parsed["browserVer"] != "" {
		browserFull += " " + parsed["browserVer"]
	}
	html.WriteString(fmt.Sprintf("<tr><td>Browser</td><td>%s</td></tr>", escapeHTML(browserFull)))
	html.WriteString(fmt.Sprintf("<tr><td>Rendering Engine</td><td>%s</td></tr>", escapeHTML(parsed["engine"])))

	osFull := parsed["os"]
	if parsed["osVer"] != "" {
		osFull += " " + parsed["osVer"]
	}
	html.WriteString(fmt.Sprintf("<tr><td>Operating System</td><td>%s</td></tr>", escapeHTML(osFull)))
	html.WriteString(fmt.Sprintf("<tr><td>Device Type</td><td>%s</td></tr>", escapeHTML(parsed["device"])))

	if parsed["architecture"] != "" {
		html.WriteString(fmt.Sprintf("<tr><td>Architecture</td><td>%s</td></tr>", escapeHTML(parsed["architecture"])))
	}

	html.WriteString(fmt.Sprintf("<tr><td>Is Bot</td><td>%s</td></tr>", escapeHTML(parsed["isBot"])))

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
	"text/plain":             {"text/plain", []string{".txt"}, "Text", "Plain text", false},
	"text/html":              {"text/html", []string{".html", ".htm"}, "Text", "HTML document", false},
	"text/css":               {"text/css", []string{".css"}, "Text", "CSS stylesheet", false},
	"text/javascript":        {"text/javascript", []string{".js", ".mjs"}, "Text", "JavaScript", false},
	"text/csv":               {"text/csv", []string{".csv"}, "Text", "CSV data", false},
	"text/xml":               {"text/xml", []string{".xml"}, "Text", "XML document", false},
	"text/markdown":          {"text/markdown", []string{".md", ".markdown"}, "Text", "Markdown", false},

	// Application
	"application/json":       {"application/json", []string{".json"}, "Application", "JSON data", false},
	"application/xml":        {"application/xml", []string{".xml"}, "Application", "XML document", false},
	"application/pdf":        {"application/pdf", []string{".pdf"}, "Application", "PDF document", true},
	"application/zip":        {"application/zip", []string{".zip"}, "Application", "ZIP archive", true},
	"application/gzip":       {"application/gzip", []string{".gz", ".gzip"}, "Application", "Gzip archive", true},
	"application/x-tar":      {"application/x-tar", []string{".tar"}, "Application", "Tar archive", true},
	"application/x-rar-compressed": {"application/x-rar-compressed", []string{".rar"}, "Application", "RAR archive", true},
	"application/x-7z-compressed":  {"application/x-7z-compressed", []string{".7z"}, "Application", "7-Zip archive", true},
	"application/octet-stream":     {"application/octet-stream", []string{".bin"}, "Application", "Binary data", true},
	"application/x-executable":     {"application/x-executable", []string{".exe"}, "Application", "Executable", true},

	// Microsoft Office
	"application/msword":     {"application/msword", []string{".doc"}, "Application", "MS Word", true},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", []string{".docx"}, "Application", "MS Word (OOXML)", true},
	"application/vnd.ms-excel": {"application/vnd.ms-excel", []string{".xls"}, "Application", "MS Excel", true},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", []string{".xlsx"}, "Application", "MS Excel (OOXML)", true},
	"application/vnd.ms-powerpoint": {"application/vnd.ms-powerpoint", []string{".ppt"}, "Application", "MS PowerPoint", true},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", []string{".pptx"}, "Application", "MS PowerPoint (OOXML)", true},

	// Images
	"image/jpeg":             {"image/jpeg", []string{".jpg", ".jpeg"}, "Image", "JPEG image", true},
	"image/png":              {"image/png", []string{".png"}, "Image", "PNG image", true},
	"image/gif":              {"image/gif", []string{".gif"}, "Image", "GIF image", true},
	"image/webp":             {"image/webp", []string{".webp"}, "Image", "WebP image", true},
	"image/svg+xml":          {"image/svg+xml", []string{".svg"}, "Image", "SVG image", false},
	"image/bmp":              {"image/bmp", []string{".bmp"}, "Image", "Bitmap image", true},
	"image/x-icon":           {"image/x-icon", []string{".ico"}, "Image", "Icon", true},
	"image/tiff":             {"image/tiff", []string{".tiff", ".tif"}, "Image", "TIFF image", true},
	"image/avif":             {"image/avif", []string{".avif"}, "Image", "AVIF image", true},

	// Audio
	"audio/mpeg":             {"audio/mpeg", []string{".mp3"}, "Audio", "MP3 audio", true},
	"audio/ogg":              {"audio/ogg", []string{".ogg"}, "Audio", "Ogg audio", true},
	"audio/wav":              {"audio/wav", []string{".wav"}, "Audio", "WAV audio", true},
	"audio/webm":             {"audio/webm", []string{".weba"}, "Audio", "WebM audio", true},
	"audio/flac":             {"audio/flac", []string{".flac"}, "Audio", "FLAC audio", true},
	"audio/aac":              {"audio/aac", []string{".aac"}, "Audio", "AAC audio", true},

	// Video
	"video/mp4":              {"video/mp4", []string{".mp4"}, "Video", "MP4 video", true},
	"video/webm":             {"video/webm", []string{".webm"}, "Video", "WebM video", true},
	"video/ogg":              {"video/ogg", []string{".ogv"}, "Video", "Ogg video", true},
	"video/x-matroska":       {"video/x-matroska", []string{".mkv"}, "Video", "Matroska video", true},
	"video/quicktime":        {"video/quicktime", []string{".mov"}, "Video", "QuickTime video", true},
	"video/x-msvideo":        {"video/x-msvideo", []string{".avi"}, "Video", "AVI video", true},

	// Fonts
	"font/woff":              {"font/woff", []string{".woff"}, "Font", "WOFF font", true},
	"font/woff2":             {"font/woff2", []string{".woff2"}, "Font", "WOFF2 font", true},
	"font/ttf":               {"font/ttf", []string{".ttf"}, "Font", "TrueType font", true},
	"font/otf":               {"font/otf", []string{".otf"}, "Font", "OpenType font", true},
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

func (h *MIMEHandler) Handle(ctx context.Context, term string) (*Answer, error) {
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
			Description: "Unknown MIME type",
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
	html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(mimeType)))

	html.WriteString("<table class=\"mime-table\">")
	html.WriteString("<tbody>")
	html.WriteString(fmt.Sprintf("<tr><td>MIME Type</td><td><code>%s</code></td></tr>", escapeHTML(mimeType)))
	html.WriteString(fmt.Sprintf("<tr><td>Category</td><td>%s</td></tr>", escapeHTML(category)))
	html.WriteString(fmt.Sprintf("<tr><td>Description</td><td>%s</td></tr>", escapeHTML(description)))
	html.WriteString(fmt.Sprintf("<tr><td>Extensions</td><td><code>%s</code></td></tr>", escapeHTML(strings.Join(extensions, ", "))))

	binaryStr := "No"
	if binary {
		binaryStr = "Yes"
	}
	html.WriteString(fmt.Sprintf("<tr><td>Binary</td><td>%s</td></tr>", binaryStr))

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

func (h *LicenseHandler) Handle(ctx context.Context, term string) (*Answer, error) {
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
			Title:       "License Lookup",
			Description: "License not found",
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
		Content:     formatLicenseContent(info),
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
}) string {
	var html strings.Builder
	html.WriteString("<div class=\"license-content\">")
	html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(info.Name)))
	html.WriteString(fmt.Sprintf("<p><code>%s</code></p>", escapeHTML(info.SPDX)))

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
			html.WriteString(fmt.Sprintf("<li>✓ %s</li>", escapeHTML(p)))
		}
		html.WriteString("</ul>")
	}

	// Conditions
	if len(info.Conditions) > 0 {
		html.WriteString("<h2>Conditions</h2><ul class=\"conditions\">")
		for _, c := range info.Conditions {
			html.WriteString(fmt.Sprintf("<li>• %s</li>", escapeHTML(c)))
		}
		html.WriteString("</ul>")
	}

	// Limitations
	if len(info.Limitations) > 0 {
		html.WriteString("<h2>Limitations</h2><ul class=\"limitations\">")
		for _, l := range info.Limitations {
			html.WriteString(fmt.Sprintf("<li>✗ %s</li>", escapeHTML(l)))
		}
		html.WriteString("</ul>")
	}

	html.WriteString(fmt.Sprintf("<p><a href=\"https://choosealicense.com/licenses/%s/\" target=\"_blank\" rel=\"noopener\">View full license text</a></p>",
		strings.ToLower(info.SPDX)))

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

func (h *CountryHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("country code or name required")
	}

	// Use REST Countries API
	var apiURL string
	if len(term) == 2 {
		apiURL = fmt.Sprintf("https://restcountries.com/v3.1/alpha/%s", url.PathEscape(strings.ToUpper(term)))
	} else if len(term) == 3 {
		apiURL = fmt.Sprintf("https://restcountries.com/v3.1/alpha/%s", url.PathEscape(strings.ToUpper(term)))
	} else {
		apiURL = fmt.Sprintf("https://restcountries.com/v3.1/name/%s", url.PathEscape(term))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypeCountry,
			Term:        term,
			Title:       "Country Lookup",
			Description: "Country not found",
			Content:     fmt.Sprintf("<p>Country not found: <code>%s</code></p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	var countries []struct {
		Name struct {
			Common   string `json:"common"`
			Official string `json:"official"`
		} `json:"name"`
		CCA2       string            `json:"cca2"`
		CCA3       string            `json:"cca3"`
		CCN3       string            `json:"ccn3"`
		Capital    []string          `json:"capital"`
		Region     string            `json:"region"`
		Subregion  string            `json:"subregion"`
		Population int               `json:"population"`
		Area       float64           `json:"area"`
		Currencies map[string]struct {
			Name   string `json:"name"`
			Symbol string `json:"symbol"`
		} `json:"currencies"`
		Languages   map[string]string `json:"languages"`
		Timezones   []string          `json:"timezones"`
		Flag        string            `json:"flag"`
		TLD         []string          `json:"tld"`
		CallingCode []string          `json:"callingCodes"`
		IDD         struct {
			Root     string   `json:"root"`
			Suffixes []string `json:"suffixes"`
		} `json:"idd"`
		Car struct {
			Side string `json:"side"`
		} `json:"car"`
		Flags struct {
			PNG string `json:"png"`
			SVG string `json:"svg"`
		} `json:"flags"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&countries); err != nil {
		return nil, err
	}

	if len(countries) == 0 {
		return &Answer{
			Type:        AnswerTypeCountry,
			Term:        term,
			Title:       "Country Lookup",
			Description: "Country not found",
			Content:     fmt.Sprintf("<p>Country not found: <code>%s</code></p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	country := countries[0]

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
		"name":       country.Name.Common,
		"official":   country.Name.Official,
		"cca2":       country.CCA2,
		"cca3":       country.CCA3,
		"capital":    country.Capital,
		"region":     country.Region,
		"subregion":  country.Subregion,
		"population": country.Population,
		"area":       country.Area,
		"currencies": currencies,
		"languages":  languages,
		"timezones":  country.Timezones,
		"tld":        country.TLD,
		"callingCode": callingCode,
		"drivingSide": country.Car.Side,
		"flag":       country.Flag,
	}

	return &Answer{
		Type:        AnswerTypeCountry,
		Term:        term,
		Title:       fmt.Sprintf("%s %s", country.Flag, country.Name.Common),
		Description: country.Name.Official,
		Content:     formatCountryContent(country.Flag, country.Flags.PNG, country.Name.Common, country.Name.Official, data),
		Source:      "REST Countries",
		SourceURL:   "https://restcountries.com/",
		Data:        data,
	}, nil
}

func formatCountryContent(flagEmoji, flagURL, name, official string, data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"country-content\">")

	// Flag and name
	html.WriteString("<div class=\"country-header\">")
	html.WriteString(fmt.Sprintf("<span class=\"flag-emoji\">%s</span>", flagEmoji))
	html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(name)))
	html.WriteString("</div>")
	html.WriteString(fmt.Sprintf("<p class=\"official\">%s</p>", escapeHTML(official)))

	html.WriteString("<table class=\"country-table\">")
	html.WriteString("<tbody>")

	if cca2, ok := data["cca2"].(string); ok {
		html.WriteString(fmt.Sprintf("<tr><td>ISO Alpha-2</td><td><code>%s</code></td></tr>", escapeHTML(cca2)))
	}
	if cca3, ok := data["cca3"].(string); ok {
		html.WriteString(fmt.Sprintf("<tr><td>ISO Alpha-3</td><td><code>%s</code></td></tr>", escapeHTML(cca3)))
	}
	if capital, ok := data["capital"].([]string); ok && len(capital) > 0 {
		html.WriteString(fmt.Sprintf("<tr><td>Capital</td><td>%s</td></tr>", escapeHTML(strings.Join(capital, ", "))))
	}
	if region, ok := data["region"].(string); ok {
		html.WriteString(fmt.Sprintf("<tr><td>Region</td><td>%s</td></tr>", escapeHTML(region)))
	}
	if subregion, ok := data["subregion"].(string); ok && subregion != "" {
		html.WriteString(fmt.Sprintf("<tr><td>Subregion</td><td>%s</td></tr>", escapeHTML(subregion)))
	}
	if population, ok := data["population"].(int); ok {
		html.WriteString(fmt.Sprintf("<tr><td>Population</td><td>%s</td></tr>", formatNumber(population)))
	}
	if area, ok := data["area"].(float64); ok {
		html.WriteString(fmt.Sprintf("<tr><td>Area</td><td>%s km²</td></tr>", formatNumber(int(area))))
	}
	if currencies, ok := data["currencies"].([]string); ok && len(currencies) > 0 {
		html.WriteString(fmt.Sprintf("<tr><td>Currency</td><td>%s</td></tr>", escapeHTML(strings.Join(currencies, ", "))))
	}
	if languages, ok := data["languages"].([]string); ok && len(languages) > 0 {
		html.WriteString(fmt.Sprintf("<tr><td>Languages</td><td>%s</td></tr>", escapeHTML(strings.Join(languages, ", "))))
	}
	if callingCode, ok := data["callingCode"].(string); ok && callingCode != "" {
		html.WriteString(fmt.Sprintf("<tr><td>Calling Code</td><td>%s</td></tr>", escapeHTML(callingCode)))
	}
	if tld, ok := data["tld"].([]string); ok && len(tld) > 0 {
		html.WriteString(fmt.Sprintf("<tr><td>TLD</td><td><code>%s</code></td></tr>", escapeHTML(strings.Join(tld, ", "))))
	}
	if drivingSide, ok := data["drivingSide"].(string); ok && drivingSide != "" {
		html.WriteString(fmt.Sprintf("<tr><td>Driving Side</td><td>%s</td></tr>", escapeHTML(strings.Title(drivingSide))))
	}
	if timezones, ok := data["timezones"].([]string); ok && len(timezones) > 0 {
		html.WriteString(fmt.Sprintf("<tr><td>Timezones</td><td>%s</td></tr>", escapeHTML(strings.Join(timezones, ", "))))
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

func (h *ASCIIHandler) Handle(ctx context.Context, term string) (*Answer, error) {
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
		Title:       "ASCII Art",
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
	html.WriteString(fmt.Sprintf("<p>Text: <strong>%s</strong></p>", escapeHTML(text)))

	html.WriteString("<pre class=\"ascii-art\">")
	html.WriteString(escapeHTML(art))
	html.WriteString("</pre>")
	html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")

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

func (h *QRHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("text or URL required")
	}

	// Generate QR code URL using Google Charts API (simple, no dependency)
	qrURL := fmt.Sprintf("https://chart.googleapis.com/chart?cht=qr&chs=300x300&chl=%s&choe=UTF-8", url.QueryEscape(term))

	data := map[string]interface{}{
		"text":  term,
		"qrURL": qrURL,
	}

	return &Answer{
		Type:        AnswerTypeQR,
		Term:        truncateString(term, 50),
		Title:       "QR Code Generator",
		Description: "QR code for your text/URL",
		Content:     formatQRContent(term, qrURL),
		Source:      "QR Code Generator",
		Data:        data,
	}, nil
}

func formatQRContent(text, qrURL string) string {
	var html strings.Builder
	html.WriteString("<div class=\"qr-content\">")
	html.WriteString("<h1>QR Code</h1>")

	html.WriteString("<div class=\"qr-image\">")
	html.WriteString(fmt.Sprintf("<img src=\"%s\" alt=\"QR Code\" class=\"qr-code\">", escapeHTML(qrURL)))
	html.WriteString("</div>")

	html.WriteString("<h2>Encoded Text</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(text)))

	html.WriteString("<h2>Download</h2>")
	html.WriteString("<p>Right-click the QR code and select \"Save image as...\" to download.</p>")

	// Size options
	html.WriteString("<h2>Other Sizes</h2>")
	html.WriteString("<ul>")
	for _, size := range []int{150, 200, 300, 400, 500} {
		sizeURL := fmt.Sprintf("https://chart.googleapis.com/chart?cht=qr&chs=%dx%d&chl=%s&choe=UTF-8", size, size, url.QueryEscape(text))
		html.WriteString(fmt.Sprintf("<li><a href=\"%s\" target=\"_blank\">%dx%d</a></li>", sizeURL, size, size))
	}
	html.WriteString("</ul>")

	html.WriteString("</div>")
	return html.String()
}
