package instant

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// DirectAnswerManager handles "type:query" pattern instant answers
type DirectAnswerManager struct {
	handlers []Handler
}

// NewDirectAnswerManager creates a new direct answer manager with all handlers
func NewDirectAnswerManager() *DirectAnswerManager {
	m := &DirectAnswerManager{
		handlers: make([]Handler, 0),
	}

	// Register all direct answer handlers
	m.Register(NewTLDRHandler())
	m.Register(NewHTTPCodeHandler())
	m.Register(NewPortHandler())
	m.Register(NewCronHandler())
	m.Register(NewChmodHandler())
	m.Register(NewTimestampHandler())
	m.Register(NewSubnetHandler())
	m.Register(NewJWTHandler())

	return m
}

// Register adds a handler to the manager
func (m *DirectAnswerManager) Register(h Handler) {
	m.handlers = append(m.handlers, h)
}

// Process checks if query matches any direct answer and returns the result
func (m *DirectAnswerManager) Process(ctx context.Context, query string) (*Answer, error) {
	query = strings.TrimSpace(query)

	for _, handler := range m.handlers {
		if handler.CanHandle(query) {
			return handler.Handle(ctx, query)
		}
	}

	return nil, nil
}

// GetHandlers returns all registered handlers
func (m *DirectAnswerManager) GetHandlers() []Handler {
	return m.handlers
}

// TLDRHandler fetches command summaries from tldr.sh
type TLDRHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewTLDRHandler creates a new TLDR handler
func NewTLDRHandler() *TLDRHandler {
	return &TLDRHandler{
		client: &http.Client{Timeout: 10 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^tldr[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^man[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^command[:\s]+(.+)$`),
		},
	}
}

func (h *TLDRHandler) Name() string              { return "tldr" }
func (h *TLDRHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *TLDRHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *TLDRHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	command := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			command = strings.TrimSpace(strings.ToLower(matches[1]))
			break
		}
	}

	if command == "" {
		return nil, nil
	}

	// Fetch from tldr.sh GitHub raw content (common pages)
	platforms := []string{"common", "linux", "osx", "windows"}
	var content string
	var found bool

	for _, platform := range platforms {
		apiURL := fmt.Sprintf("https://raw.githubusercontent.com/tldr-pages/tldr/main/pages/%s/%s.md", platform, url.PathEscape(command))

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", version.BrowserUserAgent)

		resp, err := h.client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			body := make([]byte, 8192)
			n, _ := resp.Body.Read(body)
			content = string(body[:n])
			found = true
			resp.Body.Close()
			break
		}
		resp.Body.Close()
	}

	if !found {
		return &Answer{
			Type:    AnswerTypeTLDR,
			Query:   query,
			Title:   fmt.Sprintf("TLDR: %s", command),
			Content: fmt.Sprintf("No TLDR page found for '%s'", command),
		}, nil
	}

	// Parse markdown content
	htmlContent := parseTLDRMarkdown(content, command)

	return &Answer{
		Type:      AnswerTypeTLDR,
		Query:     query,
		Title:     fmt.Sprintf("TLDR: %s", command),
		Content:   htmlContent,
		Source:    "tldr-pages",
		SourceURL: "https://tldr.sh/",
		Data: map[string]interface{}{
			"command": command,
		},
	}, nil
}

// parseTLDRMarkdown converts TLDR markdown to HTML
func parseTLDRMarkdown(md, command string) string {
	lines := strings.Split(md, "\n")
	var result strings.Builder

	result.WriteString(fmt.Sprintf("<strong>%s</strong><br><br>", command))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			// Skip title, we already have it
			continue
		}
		if strings.HasPrefix(line, "> ") {
			// Description
			desc := strings.TrimPrefix(line, "> ")
			result.WriteString(fmt.Sprintf("<em>%s</em><br><br>", desc))
		} else if strings.HasPrefix(line, "- ") {
			// Example description
			desc := strings.TrimPrefix(line, "- ")
			result.WriteString(fmt.Sprintf("&#8226; %s<br>", desc))
		} else if strings.HasPrefix(line, "`") && strings.HasSuffix(line, "`") {
			// Code example
			code := strings.Trim(line, "`")
			result.WriteString(fmt.Sprintf("<code>%s</code><br><br>", code))
		}
	}

	return result.String()
}

// HTTPCodeHandler explains HTTP status codes
type HTTPCodeHandler struct {
	patterns []*regexp.Regexp
	codes    map[int]httpCodeInfo
}

type httpCodeInfo struct {
	Name        string
	Description string
	Category    string
}

// NewHTTPCodeHandler creates a new HTTP code handler
func NewHTTPCodeHandler() *HTTPCodeHandler {
	return &HTTPCodeHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^http[:\s]+(\d{3})$`),
			regexp.MustCompile(`(?i)^http\s+status[:\s]+(\d{3})$`),
			regexp.MustCompile(`(?i)^status\s+code[:\s]+(\d{3})$`),
			regexp.MustCompile(`(?i)^http\s+(\d{3})$`),
		},
		codes: map[int]httpCodeInfo{
			// 1xx Informational
			100: {"Continue", "The server has received the request headers and the client should proceed to send the request body.", "Informational"},
			101: {"Switching Protocols", "The requester has asked the server to switch protocols and the server has agreed to do so.", "Informational"},
			102: {"Processing", "The server has received and is processing the request, but no response is available yet.", "Informational"},
			103: {"Early Hints", "Used to return some response headers before final HTTP message.", "Informational"},

			// 2xx Success
			200: {"OK", "The request has succeeded. The meaning depends on the HTTP method used.", "Success"},
			201: {"Created", "The request has been fulfilled and a new resource has been created.", "Success"},
			202: {"Accepted", "The request has been accepted for processing, but processing has not been completed.", "Success"},
			203: {"Non-Authoritative Information", "The returned information is from a local or third-party copy.", "Success"},
			204: {"No Content", "The server successfully processed the request but is not returning any content.", "Success"},
			205: {"Reset Content", "The server successfully processed the request, asks that the requester reset its document view.", "Success"},
			206: {"Partial Content", "The server is delivering only part of the resource due to a range header sent by the client.", "Success"},
			207: {"Multi-Status", "The message body contains separate response codes for a number of requests.", "Success"},
			208: {"Already Reported", "Members of a DAV binding have already been enumerated.", "Success"},
			226: {"IM Used", "The server has fulfilled a request for the resource with instance-manipulations applied.", "Success"},

			// 3xx Redirection
			300: {"Multiple Choices", "There are multiple options for the resource that the client may follow.", "Redirection"},
			301: {"Moved Permanently", "The resource has been moved permanently to a new URL.", "Redirection"},
			302: {"Found", "The resource has been temporarily moved to a different URL.", "Redirection"},
			303: {"See Other", "The response can be found under a different URI using GET method.", "Redirection"},
			304: {"Not Modified", "The resource has not been modified since the version specified by the request headers.", "Redirection"},
			305: {"Use Proxy", "The requested resource is only available through a proxy.", "Redirection"},
			307: {"Temporary Redirect", "The request should be repeated with another URI but future requests should still use the original URI.", "Redirection"},
			308: {"Permanent Redirect", "The request should be repeated with another URI and future requests should use the new URI.", "Redirection"},

			// 4xx Client Errors
			400: {"Bad Request", "The server cannot process the request due to something that is perceived to be a client error.", "Client Error"},
			401: {"Unauthorized", "Authentication is required and has failed or has not yet been provided.", "Client Error"},
			402: {"Payment Required", "Reserved for future use. Originally intended for digital payment systems.", "Client Error"},
			403: {"Forbidden", "The server understood the request but refuses to authorize it.", "Client Error"},
			404: {"Not Found", "The requested resource could not be found on the server.", "Client Error"},
			405: {"Method Not Allowed", "The request method is not supported for the requested resource.", "Client Error"},
			406: {"Not Acceptable", "The requested resource can only generate content not acceptable according to Accept headers.", "Client Error"},
			407: {"Proxy Authentication Required", "The client must first authenticate itself with the proxy.", "Client Error"},
			408: {"Request Timeout", "The server timed out waiting for the request.", "Client Error"},
			409: {"Conflict", "The request could not be processed because of conflict in the request.", "Client Error"},
			410: {"Gone", "The resource requested is no longer available and will not be available again.", "Client Error"},
			411: {"Length Required", "The request did not specify the length of its content, which is required.", "Client Error"},
			412: {"Precondition Failed", "The server does not meet one of the preconditions specified in the request.", "Client Error"},
			413: {"Payload Too Large", "The request is larger than the server is willing or able to process.", "Client Error"},
			414: {"URI Too Long", "The URI provided was too long for the server to process.", "Client Error"},
			415: {"Unsupported Media Type", "The request entity has a media type which the server does not support.", "Client Error"},
			416: {"Range Not Satisfiable", "The client has asked for a portion of the file, but the server cannot supply that portion.", "Client Error"},
			417: {"Expectation Failed", "The server cannot meet the requirements of the Expect request-header field.", "Client Error"},
			418: {"I'm a teapot", "The server refuses to brew coffee because it is a teapot (RFC 2324).", "Client Error"},
			421: {"Misdirected Request", "The request was directed at a server that is not able to produce a response.", "Client Error"},
			422: {"Unprocessable Entity", "The request was well-formed but was unable to be followed due to semantic errors.", "Client Error"},
			423: {"Locked", "The resource that is being accessed is locked.", "Client Error"},
			424: {"Failed Dependency", "The request failed because it depended on another request that failed.", "Client Error"},
			425: {"Too Early", "The server is unwilling to risk processing a request that might be replayed.", "Client Error"},
			426: {"Upgrade Required", "The client should switch to a different protocol.", "Client Error"},
			428: {"Precondition Required", "The origin server requires the request to be conditional.", "Client Error"},
			429: {"Too Many Requests", "The user has sent too many requests in a given amount of time.", "Client Error"},
			431: {"Request Header Fields Too Large", "The server is unwilling to process the request because its header fields are too large.", "Client Error"},
			451: {"Unavailable For Legal Reasons", "The resource is unavailable due to legal reasons (censorship, etc.).", "Client Error"},

			// 5xx Server Errors
			500: {"Internal Server Error", "A generic error message when the server encounters an unexpected condition.", "Server Error"},
			501: {"Not Implemented", "The server does not support the functionality required to fulfill the request.", "Server Error"},
			502: {"Bad Gateway", "The server was acting as a gateway or proxy and received an invalid response.", "Server Error"},
			503: {"Service Unavailable", "The server is currently unavailable (overloaded or down for maintenance).", "Server Error"},
			504: {"Gateway Timeout", "The server was acting as a gateway or proxy and did not receive a timely response.", "Server Error"},
			505: {"HTTP Version Not Supported", "The server does not support the HTTP protocol version used in the request.", "Server Error"},
			506: {"Variant Also Negotiates", "The server has an internal configuration error.", "Server Error"},
			507: {"Insufficient Storage", "The server is unable to store the representation needed to complete the request.", "Server Error"},
			508: {"Loop Detected", "The server detected an infinite loop while processing the request.", "Server Error"},
			510: {"Not Extended", "Further extensions to the request are required for the server to fulfill it.", "Server Error"},
			511: {"Network Authentication Required", "The client needs to authenticate to gain network access.", "Server Error"},
		},
	}
}

func (h *HTTPCodeHandler) Name() string              { return "httpcode" }
func (h *HTTPCodeHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *HTTPCodeHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *HTTPCodeHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	var code int
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			code, _ = strconv.Atoi(matches[1])
			break
		}
	}

	if code == 0 {
		return nil, nil
	}

	info, ok := h.codes[code]
	if !ok {
		return &Answer{
			Type:    AnswerTypeHTTPCode,
			Query:   query,
			Title:   fmt.Sprintf("HTTP Status %d", code),
			Content: fmt.Sprintf("Unknown HTTP status code: %d", code),
		}, nil
	}

	categoryColor := "#666"
	switch {
	case code >= 100 && code < 200:
		categoryColor = "#17a2b8" // info blue
	case code >= 200 && code < 300:
		categoryColor = "#28a745" // success green
	case code >= 300 && code < 400:
		categoryColor = "#ffc107" // redirect yellow
	case code >= 400 && code < 500:
		categoryColor = "#fd7e14" // client error orange
	case code >= 500:
		categoryColor = "#dc3545" // server error red
	}

	content := fmt.Sprintf(`<div class="http-code-result">
<span style="color: %s; font-size: 1.5em; font-weight: bold;">%d %s</span><br><br>
<strong>Category:</strong> %s<br><br>
<strong>Description:</strong><br>%s
</div>`, categoryColor, code, info.Name, info.Category, info.Description)

	return &Answer{
		Type:      AnswerTypeHTTPCode,
		Query:     query,
		Title:     fmt.Sprintf("HTTP %d %s", code, info.Name),
		Content:   content,
		Source:    "HTTP Status Codes",
		SourceURL: "https://developer.mozilla.org/en-US/docs/Web/HTTP/Status",
		Data: map[string]interface{}{
			"code":        code,
			"name":        info.Name,
			"description": info.Description,
			"category":    info.Category,
		},
	}, nil
}

// PortHandler looks up service information for port numbers
type PortHandler struct {
	patterns []*regexp.Regexp
	ports    map[int]portInfo
}

type portInfo struct {
	Service     string
	Description string
	Protocol    string
}

// NewPortHandler creates a new port handler
func NewPortHandler() *PortHandler {
	return &PortHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^port[:\s]+(\d+)$`),
			regexp.MustCompile(`(?i)^what\s+is\s+port\s+(\d+)\??$`),
			regexp.MustCompile(`(?i)^service\s+on\s+port[:\s]+(\d+)$`),
		},
		ports: map[int]portInfo{
			// Well-known ports
			20:    {"FTP Data", "File Transfer Protocol - data transfer", "TCP"},
			21:    {"FTP Control", "File Transfer Protocol - control/command", "TCP"},
			22:    {"SSH", "Secure Shell - encrypted remote login", "TCP"},
			23:    {"Telnet", "Unencrypted text communications", "TCP"},
			25:    {"SMTP", "Simple Mail Transfer Protocol", "TCP"},
			53:    {"DNS", "Domain Name System", "TCP/UDP"},
			67:    {"DHCP Server", "Dynamic Host Configuration Protocol - server", "UDP"},
			68:    {"DHCP Client", "Dynamic Host Configuration Protocol - client", "UDP"},
			69:    {"TFTP", "Trivial File Transfer Protocol", "UDP"},
			80:    {"HTTP", "Hypertext Transfer Protocol", "TCP"},
			110:   {"POP3", "Post Office Protocol v3", "TCP"},
			119:   {"NNTP", "Network News Transfer Protocol", "TCP"},
			123:   {"NTP", "Network Time Protocol", "UDP"},
			137:   {"NetBIOS Name", "NetBIOS Name Service", "UDP"},
			138:   {"NetBIOS Datagram", "NetBIOS Datagram Service", "UDP"},
			139:   {"NetBIOS Session", "NetBIOS Session Service", "TCP"},
			143:   {"IMAP", "Internet Message Access Protocol", "TCP"},
			161:   {"SNMP", "Simple Network Management Protocol", "UDP"},
			162:   {"SNMP Trap", "SNMP Trap messages", "UDP"},
			179:   {"BGP", "Border Gateway Protocol", "TCP"},
			194:   {"IRC", "Internet Relay Chat", "TCP"},
			389:   {"LDAP", "Lightweight Directory Access Protocol", "TCP"},
			443:   {"HTTPS", "HTTP over TLS/SSL", "TCP"},
			445:   {"SMB", "Server Message Block / Microsoft-DS", "TCP"},
			465:   {"SMTPS", "SMTP over SSL (deprecated)", "TCP"},
			514:   {"Syslog", "System Logging Protocol", "UDP"},
			515:   {"LPD", "Line Printer Daemon", "TCP"},
			587:   {"SMTP Submission", "Mail submission (with STARTTLS)", "TCP"},
			636:   {"LDAPS", "LDAP over SSL", "TCP"},
			993:   {"IMAPS", "IMAP over SSL", "TCP"},
			995:   {"POP3S", "POP3 over SSL", "TCP"},
			1080:  {"SOCKS", "SOCKS proxy protocol", "TCP"},
			1433:  {"MSSQL", "Microsoft SQL Server", "TCP"},
			1434:  {"MSSQL Browser", "Microsoft SQL Server Browser", "UDP"},
			1521:  {"Oracle", "Oracle Database", "TCP"},
			1723:  {"PPTP", "Point-to-Point Tunneling Protocol", "TCP"},
			2049:  {"NFS", "Network File System", "TCP/UDP"},
			2082:  {"cPanel", "cPanel default port", "TCP"},
			2083:  {"cPanel SSL", "cPanel SSL", "TCP"},
			2222:  {"SSH Alt", "Alternative SSH port", "TCP"},
			3306:  {"MySQL", "MySQL Database", "TCP"},
			3389:  {"RDP", "Remote Desktop Protocol", "TCP"},
			3690:  {"SVN", "Subversion", "TCP"},
			4443:  {"HTTPS Alt", "Alternative HTTPS", "TCP"},
			5000:  {"UPnP", "Universal Plug and Play / Flask default", "TCP"},
			5432:  {"PostgreSQL", "PostgreSQL Database", "TCP"},
			5672:  {"AMQP", "Advanced Message Queuing Protocol", "TCP"},
			5900:  {"VNC", "Virtual Network Computing", "TCP"},
			5984:  {"CouchDB", "Apache CouchDB", "TCP"},
			6379:  {"Redis", "Redis key-value store", "TCP"},
			6443:  {"Kubernetes API", "Kubernetes API Server", "TCP"},
			6667:  {"IRC", "Internet Relay Chat (common)", "TCP"},
			8000:  {"HTTP Alt", "Alternative HTTP / Django default", "TCP"},
			8080:  {"HTTP Proxy", "HTTP proxy / alternative HTTP", "TCP"},
			8443:  {"HTTPS Alt", "Alternative HTTPS", "TCP"},
			8888:  {"HTTP Alt", "Alternative HTTP / Jupyter default", "TCP"},
			9000:  {"PHP-FPM", "PHP FastCGI Process Manager", "TCP"},
			9090:  {"Prometheus", "Prometheus metrics", "TCP"},
			9200:  {"Elasticsearch", "Elasticsearch HTTP", "TCP"},
			9300:  {"Elasticsearch", "Elasticsearch transport", "TCP"},
			9418:  {"Git", "Git protocol", "TCP"},
			11211: {"Memcached", "Memcached caching system", "TCP"},
			15672: {"RabbitMQ", "RabbitMQ management", "TCP"},
			27017: {"MongoDB", "MongoDB Database", "TCP"},
			28017: {"MongoDB Web", "MongoDB Web Status", "TCP"},
		},
	}
}

func (h *PortHandler) Name() string              { return "port" }
func (h *PortHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *PortHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *PortHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	var port int
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			port, _ = strconv.Atoi(matches[1])
			break
		}
	}

	if port == 0 {
		return nil, nil
	}

	info, ok := h.ports[port]
	if !ok {
		// Provide generic info for unknown ports
		category := "Dynamic/Private"
		if port < 1024 {
			category = "Well-Known"
		} else if port < 49152 {
			category = "Registered"
		}

		return &Answer{
			Type:  AnswerTypePort,
			Query: query,
			Title: fmt.Sprintf("Port %d", port),
			Content: fmt.Sprintf(`<div class="port-result">
<strong>Port:</strong> %d<br>
<strong>Category:</strong> %s<br>
<strong>Service:</strong> Unknown/Custom<br><br>
<em>This port is not associated with a well-known service. It may be used by custom applications.</em>
</div>`, port, category),
		}, nil
	}

	content := fmt.Sprintf(`<div class="port-result">
<strong>Port:</strong> %d<br>
<strong>Service:</strong> %s<br>
<strong>Protocol:</strong> %s<br>
<strong>Description:</strong> %s
</div>`, port, info.Service, info.Protocol, info.Description)

	return &Answer{
		Type:      AnswerTypePort,
		Query:     query,
		Title:     fmt.Sprintf("Port %d - %s", port, info.Service),
		Content:   content,
		Source:    "IANA Port Assignments",
		SourceURL: "https://www.iana.org/assignments/service-names-port-numbers/",
		Data: map[string]interface{}{
			"port":        port,
			"service":     info.Service,
			"protocol":    info.Protocol,
			"description": info.Description,
		},
	}, nil
}

// CronHandler explains cron expressions
type CronHandler struct {
	patterns []*regexp.Regexp
}

// NewCronHandler creates a new cron handler
func NewCronHandler() *CronHandler {
	return &CronHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^cron[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^crontab[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^explain\s+cron[:\s]+(.+)$`),
		},
	}
}

func (h *CronHandler) Name() string              { return "cron" }
func (h *CronHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *CronHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *CronHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	expr := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			expr = strings.TrimSpace(matches[1])
			break
		}
	}

	if expr == "" {
		return nil, nil
	}

	// Parse cron expression
	explanation, err := explainCron(expr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeCron,
			Query:   query,
			Title:   "Cron Expression",
			Content: fmt.Sprintf("Invalid cron expression: %v", err),
		}, nil
	}

	return &Answer{
		Type:      AnswerTypeCron,
		Query:     query,
		Title:     "Cron Expression",
		Content:   explanation,
		Source:    "Cron Expression Explainer",
		SourceURL: "https://crontab.guru/",
		Data: map[string]interface{}{
			"expression": expr,
		},
	}, nil
}

// explainCron parses and explains a cron expression
func explainCron(expr string) (string, error) {
	// Handle special strings
	specialCrons := map[string]string{
		"@yearly":   "0 0 1 1 *",
		"@annually": "0 0 1 1 *",
		"@monthly":  "0 0 1 * *",
		"@weekly":   "0 0 * * 0",
		"@daily":    "0 0 * * *",
		"@midnight": "0 0 * * *",
		"@hourly":   "0 * * * *",
		"@reboot":   "@reboot",
	}

	if replacement, ok := specialCrons[strings.ToLower(expr)]; ok {
		if expr == "@reboot" {
			return `<div class="cron-result">
<strong>Expression:</strong> <code>@reboot</code><br><br>
<strong>Meaning:</strong> Run once at startup
</div>`, nil
		}
		expr = replacement
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 && len(parts) != 6 {
		return "", fmt.Errorf("cron expression must have 5 or 6 fields (got %d)", len(parts))
	}

	// For 6 fields, first is seconds (we'll skip it for basic explanation)
	if len(parts) == 6 {
		parts = parts[1:]
	}

	fieldNames := []string{"Minute", "Hour", "Day of Month", "Month", "Day of Week"}
	fieldRanges := []string{"0-59", "0-23", "1-31", "1-12", "0-6 (Sun-Sat)"}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("<strong>Expression:</strong> <code>%s</code><br><br>", expr))
	result.WriteString("<strong>Field Breakdown:</strong><br>")

	for i, part := range parts {
		explanation := explainCronField(part, i)
		result.WriteString(fmt.Sprintf("&#8226; <strong>%s</strong> (%s): %s = %s<br>",
			fieldNames[i], fieldRanges[i], part, explanation))
	}

	result.WriteString("<br><strong>Human Readable:</strong><br>")
	result.WriteString(humanReadableCron(parts))

	return result.String(), nil
}

// explainCronField explains a single cron field
func explainCronField(field string, fieldIndex int) string {
	if field == "*" {
		return "every value"
	}

	if strings.HasPrefix(field, "*/") {
		step := strings.TrimPrefix(field, "*/")
		return fmt.Sprintf("every %s", step)
	}

	if strings.Contains(field, ",") {
		return "specific values: " + field
	}

	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) == 2 {
			return fmt.Sprintf("from %s to %s", parts[0], parts[1])
		}
	}

	// Month names
	if fieldIndex == 3 {
		months := map[string]string{
			"1": "January", "2": "February", "3": "March", "4": "April",
			"5": "May", "6": "June", "7": "July", "8": "August",
			"9": "September", "10": "October", "11": "November", "12": "December",
		}
		if name, ok := months[field]; ok {
			return name
		}
	}

	// Day of week names
	if fieldIndex == 4 {
		days := map[string]string{
			"0": "Sunday", "1": "Monday", "2": "Tuesday", "3": "Wednesday",
			"4": "Thursday", "5": "Friday", "6": "Saturday", "7": "Sunday",
		}
		if name, ok := days[field]; ok {
			return name
		}
	}

	return fmt.Sprintf("at %s", field)
}

// humanReadableCron generates a human-readable description
func humanReadableCron(parts []string) string {
	minute, hour, dayOfMonth, month, dayOfWeek := parts[0], parts[1], parts[2], parts[3], parts[4]

	var desc strings.Builder

	// Time
	if minute == "*" && hour == "*" {
		desc.WriteString("Every minute")
	} else if minute == "0" && hour == "*" {
		desc.WriteString("Every hour")
	} else if minute == "0" && hour == "0" {
		desc.WriteString("At midnight")
	} else if strings.HasPrefix(minute, "*/") {
		step := strings.TrimPrefix(minute, "*/")
		desc.WriteString(fmt.Sprintf("Every %s minutes", step))
	} else if hour != "*" && minute != "*" {
		desc.WriteString(fmt.Sprintf("At %s:%s", hour, fmt.Sprintf("%02s", minute)))
	} else {
		desc.WriteString(fmt.Sprintf("At minute %s", minute))
		if hour != "*" {
			desc.WriteString(fmt.Sprintf(" past hour %s", hour))
		}
	}

	// Day of month
	if dayOfMonth != "*" {
		desc.WriteString(fmt.Sprintf(" on day %s of the month", dayOfMonth))
	}

	// Month
	if month != "*" {
		months := []string{"", "January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December"}
		if m, err := strconv.Atoi(month); err == nil && m >= 1 && m <= 12 {
			desc.WriteString(fmt.Sprintf(" in %s", months[m]))
		} else {
			desc.WriteString(fmt.Sprintf(" in month %s", month))
		}
	}

	// Day of week
	if dayOfWeek != "*" {
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		if d, err := strconv.Atoi(dayOfWeek); err == nil && d >= 0 && d <= 6 {
			desc.WriteString(fmt.Sprintf(" on %s", days[d]))
		} else {
			desc.WriteString(fmt.Sprintf(" on day %s of the week", dayOfWeek))
		}
	}

	return desc.String()
}

// ChmodHandler calculates Unix file permissions
type ChmodHandler struct {
	patterns []*regexp.Regexp
}

// NewChmodHandler creates a new chmod handler
func NewChmodHandler() *ChmodHandler {
	return &ChmodHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^chmod[:\s]+(\d{3,4})$`),
			regexp.MustCompile(`(?i)^permissions[:\s]+(\d{3,4})$`),
			regexp.MustCompile(`(?i)^chmod[:\s]+([rwx-]{9,10})$`),
		},
	}
}

func (h *ChmodHandler) Name() string              { return "chmod" }
func (h *ChmodHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *ChmodHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *ChmodHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	perm := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			perm = strings.TrimSpace(matches[1])
			break
		}
	}

	if perm == "" {
		return nil, nil
	}

	var octal string
	var symbolic string

	// Check if it's numeric or symbolic
	if matched, _ := regexp.MatchString(`^\d{3,4}$`, perm); matched {
		octal = perm
		symbolic = octalToSymbolic(perm)
	} else {
		symbolic = perm
		octal = symbolicToOctal(perm)
	}

	// Parse octal for detailed breakdown
	explanation := explainChmod(octal)

	content := fmt.Sprintf(`<div class="chmod-result">
<strong>Octal:</strong> <code>%s</code><br>
<strong>Symbolic:</strong> <code>%s</code><br><br>
%s
</div>`, octal, symbolic, explanation)

	return &Answer{
		Type:    AnswerTypeChmod,
		Query:   query,
		Title:   fmt.Sprintf("chmod %s", perm),
		Content: content,
		Data: map[string]interface{}{
			"octal":    octal,
			"symbolic": symbolic,
		},
	}, nil
}

// octalToSymbolic converts octal permission to symbolic
func octalToSymbolic(octal string) string {
	// Pad to 4 digits
	for len(octal) < 4 {
		octal = "0" + octal
	}

	permChars := func(digit byte) string {
		n := int(digit - '0')
		r := "-"
		w := "-"
		x := "-"
		if n&4 != 0 {
			r = "r"
		}
		if n&2 != 0 {
			w = "w"
		}
		if n&1 != 0 {
			x = "x"
		}
		return r + w + x
	}

	special := int(octal[0] - '0')
	result := permChars(octal[1]) + permChars(octal[2]) + permChars(octal[3])

	// Handle special bits
	runes := []rune(result)
	if special&4 != 0 { // setuid
		if runes[2] == 'x' {
			runes[2] = 's'
		} else {
			runes[2] = 'S'
		}
	}
	if special&2 != 0 { // setgid
		if runes[5] == 'x' {
			runes[5] = 's'
		} else {
			runes[5] = 'S'
		}
	}
	if special&1 != 0 { // sticky
		if runes[8] == 'x' {
			runes[8] = 't'
		} else {
			runes[8] = 'T'
		}
	}

	return string(runes)
}

// symbolicToOctal converts symbolic permission to octal
func symbolicToOctal(symbolic string) string {
	// Remove leading type character if present
	if len(symbolic) == 10 {
		symbolic = symbolic[1:]
	}

	if len(symbolic) != 9 {
		return "000"
	}

	digitValue := func(r, w, x rune) int {
		val := 0
		if r == 'r' {
			val += 4
		}
		if w == 'w' {
			val += 2
		}
		if x == 'x' || x == 's' || x == 't' {
			val += 1
		}
		return val
	}

	runes := []rune(symbolic)
	owner := digitValue(runes[0], runes[1], runes[2])
	group := digitValue(runes[3], runes[4], runes[5])
	other := digitValue(runes[6], runes[7], runes[8])

	special := 0
	if runes[2] == 's' || runes[2] == 'S' {
		special += 4
	}
	if runes[5] == 's' || runes[5] == 'S' {
		special += 2
	}
	if runes[8] == 't' || runes[8] == 'T' {
		special += 1
	}

	if special > 0 {
		return fmt.Sprintf("%d%d%d%d", special, owner, group, other)
	}
	return fmt.Sprintf("%d%d%d", owner, group, other)
}

// explainChmod provides a detailed breakdown of permissions
func explainChmod(octal string) string {
	// Pad to 4 digits
	for len(octal) < 4 {
		octal = "0" + octal
	}

	var result strings.Builder

	permDesc := func(digit byte, entity string) string {
		n := int(digit - '0')
		perms := []string{}
		if n&4 != 0 {
			perms = append(perms, "read")
		}
		if n&2 != 0 {
			perms = append(perms, "write")
		}
		if n&1 != 0 {
			perms = append(perms, "execute")
		}
		if len(perms) == 0 {
			return fmt.Sprintf("<strong>%s:</strong> no permissions", entity)
		}
		return fmt.Sprintf("<strong>%s:</strong> %s", entity, strings.Join(perms, ", "))
	}

	result.WriteString("<strong>Permissions Breakdown:</strong><br>")
	result.WriteString("&#8226; " + permDesc(octal[1], "Owner") + "<br>")
	result.WriteString("&#8226; " + permDesc(octal[2], "Group") + "<br>")
	result.WriteString("&#8226; " + permDesc(octal[3], "Others") + "<br>")

	special := int(octal[0] - '0')
	if special > 0 {
		result.WriteString("<br><strong>Special Bits:</strong><br>")
		if special&4 != 0 {
			result.WriteString("&#8226; setuid - run as owner<br>")
		}
		if special&2 != 0 {
			result.WriteString("&#8226; setgid - run as group<br>")
		}
		if special&1 != 0 {
			result.WriteString("&#8226; sticky - only owner can delete<br>")
		}
	}

	return result.String()
}

// TimestampHandler converts Unix timestamps
type TimestampHandler struct {
	patterns []*regexp.Regexp
}

// NewTimestampHandler creates a new timestamp handler
func NewTimestampHandler() *TimestampHandler {
	return &TimestampHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^timestamp[:\s]+(\d+)$`),
			regexp.MustCompile(`(?i)^unix[:\s]+(\d+)$`),
			regexp.MustCompile(`(?i)^epoch[:\s]+(\d+)$`),
			regexp.MustCompile(`(?i)^time[:\s]+(\d{10,13})$`),
		},
	}
}

func (h *TimestampHandler) Name() string              { return "timestamp" }
func (h *TimestampHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *TimestampHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *TimestampHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	var timestamp int64
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			timestamp, _ = strconv.ParseInt(matches[1], 10, 64)
			break
		}
	}

	if timestamp == 0 {
		return nil, nil
	}

	// Handle milliseconds
	var t time.Time
	isMillis := timestamp > 9999999999
	if isMillis {
		t = time.UnixMilli(timestamp)
	} else {
		t = time.Unix(timestamp, 0)
	}

	now := time.Now()
	diff := now.Sub(t)
	relativeTime := formatRelativeTime(diff)

	utc := t.UTC()
	local := t.Local()

	content := fmt.Sprintf(`<div class="timestamp-result">
<strong>Unix Timestamp:</strong> %d%s<br><br>
<strong>UTC:</strong> %s<br>
<strong>Local:</strong> %s<br>
<strong>ISO 8601:</strong> %s<br>
<strong>RFC 2822:</strong> %s<br><br>
<strong>Relative:</strong> %s
</div>`,
		timestamp,
		func() string {
			if isMillis {
				return " (milliseconds)"
			}
			return " (seconds)"
		}(),
		utc.Format("Monday, January 2, 2006 15:04:05 UTC"),
		local.Format("Monday, January 2, 2006 15:04:05 MST"),
		t.Format(time.RFC3339),
		t.Format(time.RFC1123Z),
		relativeTime,
	)

	return &Answer{
		Type:    AnswerTypeTimestamp,
		Query:   query,
		Title:   "Unix Timestamp Converter",
		Content: content,
		Data: map[string]interface{}{
			"timestamp": timestamp,
			"utc":       utc.Format(time.RFC3339),
			"local":     local.Format(time.RFC3339),
		},
	}, nil
}

// formatRelativeTime formats a duration as relative time
func formatRelativeTime(d time.Duration) string {
	abs := d
	if abs < 0 {
		abs = -abs
	}

	var result string
	switch {
	case abs < time.Minute:
		result = fmt.Sprintf("%.0f seconds", abs.Seconds())
	case abs < time.Hour:
		result = fmt.Sprintf("%.0f minutes", abs.Minutes())
	case abs < 24*time.Hour:
		result = fmt.Sprintf("%.1f hours", abs.Hours())
	case abs < 30*24*time.Hour:
		result = fmt.Sprintf("%.1f days", abs.Hours()/24)
	case abs < 365*24*time.Hour:
		result = fmt.Sprintf("%.1f months", abs.Hours()/(24*30))
	default:
		result = fmt.Sprintf("%.1f years", abs.Hours()/(24*365))
	}

	if d < 0 {
		return result + " from now"
	}
	return result + " ago"
}

// SubnetHandler calculates CIDR subnet information
type SubnetHandler struct {
	patterns []*regexp.Regexp
}

// NewSubnetHandler creates a new subnet handler
func NewSubnetHandler() *SubnetHandler {
	return &SubnetHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^subnet[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^cidr[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^ip\s+range[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^netmask[:\s]+(.+)$`),
		},
	}
}

func (h *SubnetHandler) Name() string              { return "subnet" }
func (h *SubnetHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *SubnetHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *SubnetHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	cidr := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			cidr = strings.TrimSpace(matches[1])
			break
		}
	}

	if cidr == "" {
		return nil, nil
	}

	// Parse CIDR
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		// Try adding /32 for single IP
		_, ipnet, err = net.ParseCIDR(cidr + "/32")
		if err != nil {
			return &Answer{
				Type:    AnswerTypeSubnet,
				Query:   query,
				Title:   "Subnet Calculator",
				Content: fmt.Sprintf("Invalid CIDR notation: %s", cidr),
			}, nil
		}
	}

	// Calculate subnet information
	ones, bits := ipnet.Mask.Size()
	networkAddr := ipnet.IP
	broadcastAddr := calculateBroadcast(ipnet)
	firstHost := incrementIP(networkAddr)
	lastHost := decrementIP(broadcastAddr)
	numHosts := int64(math.Pow(2, float64(bits-ones))) - 2
	if numHosts < 0 {
		numHosts = 0
	}

	mask := net.IP(ipnet.Mask).String()
	wildcard := calculateWildcard(ipnet.Mask)

	content := fmt.Sprintf(`<div class="subnet-result">
<strong>CIDR:</strong> %s<br>
<strong>Netmask:</strong> %s<br>
<strong>Wildcard:</strong> %s<br><br>
<strong>Network Address:</strong> %s<br>
<strong>Broadcast Address:</strong> %s<br>
<strong>First Host:</strong> %s<br>
<strong>Last Host:</strong> %s<br><br>
<strong>Total Hosts:</strong> %d<br>
<strong>Usable Hosts:</strong> %d<br>
<strong>Prefix Length:</strong> /%d
</div>`,
		ipnet.String(),
		mask,
		wildcard,
		networkAddr.String(),
		broadcastAddr.String(),
		firstHost.String(),
		lastHost.String(),
		int64(math.Pow(2, float64(bits-ones))),
		numHosts,
		ones,
	)

	return &Answer{
		Type:    AnswerTypeSubnet,
		Query:   query,
		Title:   "Subnet Calculator",
		Content: content,
		Data: map[string]interface{}{
			"cidr":         ipnet.String(),
			"netmask":      mask,
			"network":      networkAddr.String(),
			"broadcast":    broadcastAddr.String(),
			"usable_hosts": numHosts,
		},
	}, nil
}

// calculateBroadcast calculates the broadcast address for a network
func calculateBroadcast(ipnet *net.IPNet) net.IP {
	ip := ipnet.IP.To4()
	if ip == nil {
		ip = ipnet.IP.To16()
	}
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ip[i] | ^ipnet.Mask[i]
	}
	return broadcast
}

// calculateWildcard calculates the wildcard mask
func calculateWildcard(mask net.IPMask) string {
	wildcard := make(net.IP, len(mask))
	for i := range mask {
		wildcard[i] = ^mask[i]
	}
	return wildcard.String()
}

// incrementIP increments an IP address by 1
func incrementIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)
	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			break
		}
	}
	return result
}

// decrementIP decrements an IP address by 1
func decrementIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)
	for i := len(result) - 1; i >= 0; i-- {
		result[i]--
		if result[i] != 255 {
			break
		}
	}
	return result
}

// JWTHandler decodes JWT tokens
type JWTHandler struct {
	patterns []*regexp.Regexp
}

// NewJWTHandler creates a new JWT handler
func NewJWTHandler() *JWTHandler {
	return &JWTHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^jwt[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^decode\s+jwt[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^token[:\s]+(eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)$`),
		},
	}
}

func (h *JWTHandler) Name() string              { return "jwt" }
func (h *JWTHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *JWTHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *JWTHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	token := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			token = strings.TrimSpace(matches[1])
			break
		}
	}

	if token == "" {
		return nil, nil
	}

	// Parse JWT
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return &Answer{
			Type:    AnswerTypeJWT,
			Query:   query,
			Title:   "JWT Decoder",
			Content: "Invalid JWT format. A JWT should have three parts separated by dots.",
		}, nil
	}

	// Decode header
	header, err := decodeJWTSegment(parts[0])
	if err != nil {
		return &Answer{
			Type:    AnswerTypeJWT,
			Query:   query,
			Title:   "JWT Decoder",
			Content: fmt.Sprintf("Error decoding header: %v", err),
		}, nil
	}

	// Decode payload
	payload, err := decodeJWTSegment(parts[1])
	if err != nil {
		return &Answer{
			Type:    AnswerTypeJWT,
			Query:   query,
			Title:   "JWT Decoder",
			Content: fmt.Sprintf("Error decoding payload: %v", err),
		}, nil
	}

	// Format header and payload as JSON
	headerJSON, _ := json.MarshalIndent(header, "", "  ")
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")

	// Check for common claims
	var claimsInfo strings.Builder
	if exp, ok := payload["exp"].(float64); ok {
		expTime := time.Unix(int64(exp), 0)
		if time.Now().After(expTime) {
			claimsInfo.WriteString("<span style=\"color: red;\">&#9888; Token is EXPIRED</span><br>")
		}
		claimsInfo.WriteString(fmt.Sprintf("<strong>Expires:</strong> %s<br>", expTime.Format(time.RFC3339)))
	}
	if iat, ok := payload["iat"].(float64); ok {
		iatTime := time.Unix(int64(iat), 0)
		claimsInfo.WriteString(fmt.Sprintf("<strong>Issued At:</strong> %s<br>", iatTime.Format(time.RFC3339)))
	}
	if sub, ok := payload["sub"].(string); ok {
		claimsInfo.WriteString(fmt.Sprintf("<strong>Subject:</strong> %s<br>", sub))
	}
	if iss, ok := payload["iss"].(string); ok {
		claimsInfo.WriteString(fmt.Sprintf("<strong>Issuer:</strong> %s<br>", iss))
	}

	content := fmt.Sprintf(`<div class="jwt-result">
<strong>Header:</strong><br>
<pre><code>%s</code></pre><br>
<strong>Payload:</strong><br>
<pre><code>%s</code></pre><br>
%s
<em>Note: Signature is not verified (would require the secret key)</em>
</div>`, string(headerJSON), string(payloadJSON), claimsInfo.String())

	return &Answer{
		Type:    AnswerTypeJWT,
		Query:   query,
		Title:   "JWT Decoder",
		Content: content,
		Data: map[string]interface{}{
			"header":  header,
			"payload": payload,
		},
	}, nil
}

// decodeJWTSegment decodes a base64url-encoded JWT segment
func decodeJWTSegment(segment string) (map[string]interface{}, error) {
	// Add padding if necessary
	switch len(segment) % 4 {
	case 2:
		segment += "=="
	case 3:
		segment += "="
	}

	// Replace URL-safe characters
	segment = strings.ReplaceAll(segment, "-", "+")
	segment = strings.ReplaceAll(segment, "_", "/")

	decoded, err := base64.StdEncoding.DecodeString(segment)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(decoded, &result); err != nil {
		return nil, err
	}

	return result, nil
}
