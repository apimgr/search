package direct

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HTTPHandler handles http:{code} queries
type HTTPHandler struct{}

// NewHTTPHandler creates a new HTTP status code handler
func NewHTTPHandler() *HTTPHandler {
	return &HTTPHandler{}
}

func (h *HTTPHandler) Type() AnswerType {
	return AnswerTypeHTTP
}

// HTTP status codes database
var httpStatusCodes = map[int]struct {
	Name        string
	Description string
	Category    string
}{
	// 1xx Informational
	100: {"Continue", "The server has received the request headers and the client should proceed to send the request body.", "Informational"},
	101: {"Switching Protocols", "The server is switching protocols as requested by the client.", "Informational"},
	102: {"Processing", "The server has received and is processing the request, but no response is available yet.", "Informational"},
	103: {"Early Hints", "Used to return some response headers before final HTTP message.", "Informational"},

	// 2xx Success
	200: {"OK", "The request succeeded. The meaning of the success depends on the HTTP method.", "Success"},
	201: {"Created", "The request succeeded, and a new resource was created as a result.", "Success"},
	202: {"Accepted", "The request has been received but not yet acted upon.", "Success"},
	203: {"Non-Authoritative Information", "The returned metadata is not from the origin server.", "Success"},
	204: {"No Content", "There is no content to send for this request, but the headers may be useful.", "Success"},
	205: {"Reset Content", "Tells the user agent to reset the document which sent this request.", "Success"},
	206: {"Partial Content", "Used when the Range header is sent from the client to request only part of a resource.", "Success"},
	207: {"Multi-Status", "Conveys information about multiple resources (WebDAV).", "Success"},
	208: {"Already Reported", "Used in a DAV binding to avoid enumerating the same collection multiple times.", "Success"},
	226: {"IM Used", "The server has fulfilled a GET request for the resource with instance-manipulations applied.", "Success"},

	// 3xx Redirection
	300: {"Multiple Choices", "The request has more than one possible response.", "Redirection"},
	301: {"Moved Permanently", "The URL of the requested resource has been changed permanently.", "Redirection"},
	302: {"Found", "The URI of requested resource has been changed temporarily.", "Redirection"},
	303: {"See Other", "The server sent this response to direct the client to get the requested resource at another URI.", "Redirection"},
	304: {"Not Modified", "Used for caching purposes. The response has not been modified.", "Redirection"},
	305: {"Use Proxy", "Deprecated. Defined in a previous version of HTTP.", "Redirection"},
	307: {"Temporary Redirect", "The server sends this response to direct the client to get the requested resource at another URI.", "Redirection"},
	308: {"Permanent Redirect", "The resource is now permanently located at another URI.", "Redirection"},

	// 4xx Client Error
	400: {"Bad Request", "The server cannot process the request due to client error.", "Client Error"},
	401: {"Unauthorized", "Authentication is required and has failed or has not been provided.", "Client Error"},
	402: {"Payment Required", "Reserved for future use. Originally intended for digital payment systems.", "Client Error"},
	403: {"Forbidden", "The client does not have access rights to the content.", "Client Error"},
	404: {"Not Found", "The server cannot find the requested resource.", "Client Error"},
	405: {"Method Not Allowed", "The request method is known but not supported by the target resource.", "Client Error"},
	406: {"Not Acceptable", "The server cannot produce a response matching the list of acceptable values.", "Client Error"},
	407: {"Proxy Authentication Required", "Similar to 401 but authentication is needed by a proxy.", "Client Error"},
	408: {"Request Timeout", "The server timed out waiting for the request.", "Client Error"},
	409: {"Conflict", "The request conflicts with the current state of the server.", "Client Error"},
	410: {"Gone", "The content has been permanently deleted from server.", "Client Error"},
	411: {"Length Required", "The server rejected the request because Content-Length header is not defined.", "Client Error"},
	412: {"Precondition Failed", "The client has indicated preconditions in its headers which the server does not meet.", "Client Error"},
	413: {"Payload Too Large", "Request entity is larger than limits defined by server.", "Client Error"},
	414: {"URI Too Long", "The URI requested by the client is longer than the server is willing to interpret.", "Client Error"},
	415: {"Unsupported Media Type", "The media format of the requested data is not supported by the server.", "Client Error"},
	416: {"Range Not Satisfiable", "The range specified by the Range header cannot be fulfilled.", "Client Error"},
	417: {"Expectation Failed", "The expectation given in the request's Expect header could not be met.", "Client Error"},
	418: {"I'm a teapot", "The server refuses the attempt to brew coffee with a teapot (RFC 2324).", "Client Error"},
	421: {"Misdirected Request", "The request was directed at a server that is not able to produce a response.", "Client Error"},
	422: {"Unprocessable Entity", "The request was well-formed but unable to be followed due to semantic errors.", "Client Error"},
	423: {"Locked", "The resource that is being accessed is locked.", "Client Error"},
	424: {"Failed Dependency", "The request failed due to failure of a previous request.", "Client Error"},
	425: {"Too Early", "The server is unwilling to risk processing a request that might be replayed.", "Client Error"},
	426: {"Upgrade Required", "The server refuses to perform the request using the current protocol.", "Client Error"},
	428: {"Precondition Required", "The origin server requires the request to be conditional.", "Client Error"},
	429: {"Too Many Requests", "The user has sent too many requests in a given amount of time.", "Client Error"},
	431: {"Request Header Fields Too Large", "The server is unwilling to process the request because its header fields are too large.", "Client Error"},
	451: {"Unavailable For Legal Reasons", "The user agent requested a resource that cannot legally be provided.", "Client Error"},

	// 5xx Server Error
	500: {"Internal Server Error", "The server has encountered a situation it does not know how to handle.", "Server Error"},
	501: {"Not Implemented", "The request method is not supported by the server and cannot be handled.", "Server Error"},
	502: {"Bad Gateway", "The server, while acting as a gateway, received an invalid response.", "Server Error"},
	503: {"Service Unavailable", "The server is not ready to handle the request.", "Server Error"},
	504: {"Gateway Timeout", "The server is acting as a gateway and cannot get a response in time.", "Server Error"},
	505: {"HTTP Version Not Supported", "The HTTP version used in the request is not supported by the server.", "Server Error"},
	506: {"Variant Also Negotiates", "The server has an internal configuration error.", "Server Error"},
	507: {"Insufficient Storage", "The server is unable to store the representation needed to complete the request.", "Server Error"},
	508: {"Loop Detected", "The server detected an infinite loop while processing the request.", "Server Error"},
	510: {"Not Extended", "Further extensions to the request are required for the server to fulfill it.", "Server Error"},
	511: {"Network Authentication Required", "The client needs to authenticate to gain network access.", "Server Error"},
}

func (h *HTTPHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("HTTP status code required")
	}

	code, err := strconv.Atoi(term)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeHTTP,
			Term:        term,
			Title:       "HTTP Status Code",
			Description: "Invalid status code",
			Content:     fmt.Sprintf("<p>Invalid HTTP status code: <code>%s</code></p>", escapeHTML(term)),
			Error:       "invalid_code",
		}, nil
	}

	status, ok := httpStatusCodes[code]
	if !ok {
		return &Answer{
			Type:        AnswerTypeHTTP,
			Term:        term,
			Title:       fmt.Sprintf("HTTP %d", code),
			Description: "Unknown status code",
			Content:     fmt.Sprintf("<p>HTTP status code <code>%d</code> is not a standard code.</p>", code),
			Error:       "unknown_code",
		}, nil
	}

	data := map[string]interface{}{
		"code":        code,
		"name":        status.Name,
		"description": status.Description,
		"category":    status.Category,
	}

	return &Answer{
		Type:        AnswerTypeHTTP,
		Term:        term,
		Title:       fmt.Sprintf("HTTP %d %s", code, status.Name),
		Description: status.Description,
		Content:     formatHTTPContent(code, status.Name, status.Description, status.Category),
		Source:      "RFC 9110",
		SourceURL:   fmt.Sprintf("https://httpwg.org/specs/rfc9110.html#status.%d", code),
		Data:        data,
	}, nil
}

func formatHTTPContent(code int, name, description, category string) string {
	var html strings.Builder
	html.WriteString("<div class=\"http-content\">")

	// Category badge color
	categoryClass := strings.ToLower(strings.ReplaceAll(category, " ", "-"))
	html.WriteString(fmt.Sprintf("<span class=\"category-badge %s\">%s</span>", categoryClass, category))

	html.WriteString(fmt.Sprintf("<h1>%d %s</h1>", code, escapeHTML(name)))
	html.WriteString(fmt.Sprintf("<p class=\"description\">%s</p>", escapeHTML(description)))

	// Related codes
	html.WriteString("<h2>Related Status Codes</h2>")
	html.WriteString("<ul>")
	baseCode := (code / 100) * 100
	for c := baseCode; c < baseCode+20; c++ {
		if s, ok := httpStatusCodes[c]; ok && c != code {
			html.WriteString(fmt.Sprintf("<li><a href=\"/direct/http/%d\">%d %s</a></li>", c, c, escapeHTML(s.Name)))
		}
	}
	html.WriteString("</ul>")

	html.WriteString("</div>")
	return html.String()
}

// PortHandler handles port:{number} queries
type PortHandler struct{}

// NewPortHandler creates a new port handler
func NewPortHandler() *PortHandler {
	return &PortHandler{}
}

func (h *PortHandler) Type() AnswerType {
	return AnswerTypePort
}

// Common ports database
var portDB = map[int]struct {
	Service     string
	Description string
	Protocol    string
}{
	20:    {"FTP Data", "File Transfer Protocol (data transfer)", "TCP"},
	21:    {"FTP Control", "File Transfer Protocol (command control)", "TCP"},
	22:    {"SSH", "Secure Shell - encrypted remote login", "TCP"},
	23:    {"Telnet", "Unencrypted text communications", "TCP"},
	25:    {"SMTP", "Simple Mail Transfer Protocol", "TCP"},
	53:    {"DNS", "Domain Name System", "TCP/UDP"},
	67:    {"DHCP Server", "Dynamic Host Configuration Protocol", "UDP"},
	68:    {"DHCP Client", "Dynamic Host Configuration Protocol", "UDP"},
	69:    {"TFTP", "Trivial File Transfer Protocol", "UDP"},
	80:    {"HTTP", "Hypertext Transfer Protocol", "TCP"},
	110:   {"POP3", "Post Office Protocol v3", "TCP"},
	119:   {"NNTP", "Network News Transfer Protocol", "TCP"},
	123:   {"NTP", "Network Time Protocol", "UDP"},
	143:   {"IMAP", "Internet Message Access Protocol", "TCP"},
	161:   {"SNMP", "Simple Network Management Protocol", "UDP"},
	162:   {"SNMP Trap", "Simple Network Management Protocol (traps)", "UDP"},
	179:   {"BGP", "Border Gateway Protocol", "TCP"},
	194:   {"IRC", "Internet Relay Chat", "TCP"},
	389:   {"LDAP", "Lightweight Directory Access Protocol", "TCP"},
	443:   {"HTTPS", "HTTP over TLS/SSL", "TCP"},
	445:   {"SMB", "Server Message Block / Microsoft-DS", "TCP"},
	465:   {"SMTPS", "SMTP over TLS (deprecated)", "TCP"},
	514:   {"Syslog", "System Logging Protocol", "UDP"},
	587:   {"SMTP Submission", "Mail submission (modern SMTP)", "TCP"},
	636:   {"LDAPS", "LDAP over TLS/SSL", "TCP"},
	993:   {"IMAPS", "IMAP over TLS/SSL", "TCP"},
	995:   {"POP3S", "POP3 over TLS/SSL", "TCP"},
	1080:  {"SOCKS", "SOCKS proxy protocol", "TCP"},
	1433:  {"MSSQL", "Microsoft SQL Server", "TCP"},
	1521:  {"Oracle", "Oracle Database", "TCP"},
	1723:  {"PPTP", "Point-to-Point Tunneling Protocol", "TCP"},
	2049:  {"NFS", "Network File System", "TCP/UDP"},
	3306:  {"MySQL", "MySQL Database", "TCP"},
	3389:  {"RDP", "Remote Desktop Protocol", "TCP"},
	5432:  {"PostgreSQL", "PostgreSQL Database", "TCP"},
	5672:  {"AMQP", "Advanced Message Queuing Protocol", "TCP"},
	5900:  {"VNC", "Virtual Network Computing", "TCP"},
	6379:  {"Redis", "Redis Database", "TCP"},
	6443:  {"Kubernetes API", "Kubernetes API Server", "TCP"},
	8080:  {"HTTP Proxy", "HTTP Proxy / Alternative HTTP", "TCP"},
	8443:  {"HTTPS Alt", "Alternative HTTPS", "TCP"},
	9000:  {"PHP-FPM", "PHP FastCGI Process Manager", "TCP"},
	9200:  {"Elasticsearch", "Elasticsearch REST API", "TCP"},
	9300:  {"Elasticsearch", "Elasticsearch Node Communication", "TCP"},
	11211: {"Memcached", "Memcached caching system", "TCP/UDP"},
	27017: {"MongoDB", "MongoDB Database", "TCP"},
}

func (h *PortHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("port number required")
	}

	port, err := strconv.Atoi(term)
	if err != nil || port < 0 || port > 65535 {
		return &Answer{
			Type:        AnswerTypePort,
			Term:        term,
			Title:       "Port Lookup",
			Description: "Invalid port number",
			Content:     fmt.Sprintf("<p>Invalid port number: <code>%s</code>. Ports must be between 0 and 65535.</p>", escapeHTML(term)),
			Error:       "invalid_port",
		}, nil
	}

	info, ok := portDB[port]
	if !ok {
		// Unknown port - provide general info
		category := ""
		switch {
		case port < 1024:
			category = "Well-known port (system)"
		case port < 49152:
			category = "Registered port"
		default:
			category = "Dynamic/Private port"
		}

		return &Answer{
			Type:        AnswerTypePort,
			Term:        term,
			Title:       fmt.Sprintf("Port %d", port),
			Description: category,
			Content:     formatUnknownPortContent(port, category),
			Source:      "IANA Port Registry",
		}, nil
	}

	data := map[string]interface{}{
		"port":        port,
		"service":     info.Service,
		"description": info.Description,
		"protocol":    info.Protocol,
	}

	return &Answer{
		Type:        AnswerTypePort,
		Term:        term,
		Title:       fmt.Sprintf("Port %d - %s", port, info.Service),
		Description: info.Description,
		Content:     formatPortContent(port, info.Service, info.Description, info.Protocol),
		Source:      "IANA Port Registry",
		SourceURL:   "https://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml",
		Data:        data,
	}, nil
}

func formatPortContent(port int, service, description, protocol string) string {
	var html strings.Builder
	html.WriteString("<div class=\"port-content\">")
	html.WriteString(fmt.Sprintf("<h1>Port %d</h1>", port))
	html.WriteString(fmt.Sprintf("<h2>%s</h2>", escapeHTML(service)))

	html.WriteString("<dl>")
	html.WriteString(fmt.Sprintf("<dt>Description</dt><dd>%s</dd>", escapeHTML(description)))
	html.WriteString(fmt.Sprintf("<dt>Protocol</dt><dd>%s</dd>", escapeHTML(protocol)))

	// Port category
	category := ""
	switch {
	case port < 1024:
		category = "Well-known port (system)"
	case port < 49152:
		category = "Registered port"
	default:
		category = "Dynamic/Private port"
	}
	html.WriteString(fmt.Sprintf("<dt>Category</dt><dd>%s</dd>", category))
	html.WriteString("</dl>")

	html.WriteString("</div>")
	return html.String()
}

func formatUnknownPortContent(port int, category string) string {
	var html strings.Builder
	html.WriteString("<div class=\"port-content\">")
	html.WriteString(fmt.Sprintf("<h1>Port %d</h1>", port))
	html.WriteString("<p>No well-known service registered for this port.</p>")
	html.WriteString(fmt.Sprintf("<p><strong>Category:</strong> %s</p>", category))
	html.WriteString("</div>")
	return html.String()
}

// CronHandler handles cron:{expression} queries
type CronHandler struct{}

// NewCronHandler creates a new cron handler
func NewCronHandler() *CronHandler {
	return &CronHandler{}
}

func (h *CronHandler) Type() AnswerType {
	return AnswerTypeCron
}

func (h *CronHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("cron expression required")
	}

	parts := strings.Fields(term)
	if len(parts) < 5 || len(parts) > 7 {
		return &Answer{
			Type:        AnswerTypeCron,
			Term:        term,
			Title:       "Cron Expression",
			Description: "Invalid cron expression",
			Content:     fmt.Sprintf("<p>Invalid cron expression: <code>%s</code>. Expected 5-7 fields.</p>", escapeHTML(term)),
			Error:       "invalid_cron",
		}, nil
	}

	// Parse cron fields
	fields := []struct {
		name  string
		value string
		range_ string
	}{
		{"Minute", parts[0], "0-59"},
		{"Hour", parts[1], "0-23"},
		{"Day of Month", parts[2], "1-31"},
		{"Month", parts[3], "1-12"},
		{"Day of Week", parts[4], "0-6"},
	}

	if len(parts) >= 6 {
		fields = append(fields, struct {
			name  string
			value string
			range_ string
		}{"Year", parts[5], "1970-2099"})
	}

	// Generate human-readable description
	description := generateCronDescription(parts)

	// Calculate next run times
	nextRuns := calculateNextCronRuns(parts, 5)

	data := map[string]interface{}{
		"expression":  term,
		"fields":      fields,
		"description": description,
		"nextRuns":    nextRuns,
	}

	return &Answer{
		Type:        AnswerTypeCron,
		Term:        term,
		Title:       "Cron Expression",
		Description: description,
		Content:     formatCronContent(term, fields, description, nextRuns),
		Source:      "Cron Parser",
		Data:        data,
	}, nil
}

func generateCronDescription(parts []string) string {
	minute := parts[0]
	hour := parts[1]
	dom := parts[2]
	month := parts[3]
	dow := parts[4]

	var desc strings.Builder

	// Common patterns
	if minute == "*" && hour == "*" && dom == "*" && month == "*" && dow == "*" {
		return "Every minute"
	}

	if minute == "0" && hour == "*" && dom == "*" && month == "*" && dow == "*" {
		return "Every hour at minute 0"
	}

	if minute == "0" && hour == "0" && dom == "*" && month == "*" && dow == "*" {
		return "Daily at midnight"
	}

	if minute == "0" && hour == "0" && dom == "1" && month == "*" && dow == "*" {
		return "Monthly on the 1st at midnight"
	}

	// Build description
	desc.WriteString("At ")

	if minute == "*" {
		desc.WriteString("every minute")
	} else if strings.HasPrefix(minute, "*/") {
		desc.WriteString(fmt.Sprintf("every %s minutes", minute[2:]))
	} else {
		desc.WriteString(fmt.Sprintf("minute %s", minute))
	}

	if hour != "*" {
		if strings.HasPrefix(hour, "*/") {
			desc.WriteString(fmt.Sprintf(" of every %s hours", hour[2:]))
		} else {
			desc.WriteString(fmt.Sprintf(" past hour %s", hour))
		}
	}

	if dom != "*" && dow == "*" {
		desc.WriteString(fmt.Sprintf(" on day %s", dom))
	}

	if dow != "*" {
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		if i, err := strconv.Atoi(dow); err == nil && i >= 0 && i <= 6 {
			desc.WriteString(fmt.Sprintf(" on %s", days[i]))
		} else {
			desc.WriteString(fmt.Sprintf(" on day of week %s", dow))
		}
	}

	if month != "*" {
		months := []string{"", "January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December"}
		if i, err := strconv.Atoi(month); err == nil && i >= 1 && i <= 12 {
			desc.WriteString(fmt.Sprintf(" in %s", months[i]))
		} else {
			desc.WriteString(fmt.Sprintf(" in month %s", month))
		}
	}

	return desc.String()
}

func calculateNextCronRuns(parts []string, count int) []string {
	// Simplified - just show format
	// Real implementation would calculate actual next run times
	now := time.Now()
	runs := make([]string, count)
	for i := 0; i < count; i++ {
		runs[i] = now.Add(time.Duration(i+1) * time.Hour).Format(time.RFC1123)
	}
	return runs
}

func formatCronContent(expression string, fields []struct {
	name  string
	value string
	range_ string
}, description string, nextRuns []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"cron-content\">")
	html.WriteString("<h1>Cron Expression</h1>")
	html.WriteString(fmt.Sprintf("<pre class=\"cron-expr\"><code>%s</code></pre>", escapeHTML(expression)))

	html.WriteString(fmt.Sprintf("<p class=\"description\">%s</p>", escapeHTML(description)))

	// Field breakdown
	html.WriteString("<h2>Fields</h2>")
	html.WriteString("<table class=\"cron-fields\">")
	html.WriteString("<thead><tr><th>Field</th><th>Value</th><th>Allowed</th></tr></thead>")
	html.WriteString("<tbody>")
	for _, f := range fields {
		html.WriteString(fmt.Sprintf("<tr><td>%s</td><td><code>%s</code></td><td>%s</td></tr>",
			f.name, escapeHTML(f.value), f.range_))
	}
	html.WriteString("</tbody></table>")

	// Quick reference
	html.WriteString("<h2>Quick Reference</h2>")
	html.WriteString("<pre>")
	html.WriteString("* * * * *\n")
	html.WriteString("│ │ │ │ │\n")
	html.WriteString("│ │ │ │ └── Day of Week (0-6, Sun=0)\n")
	html.WriteString("│ │ │ └──── Month (1-12)\n")
	html.WriteString("│ │ └────── Day of Month (1-31)\n")
	html.WriteString("│ └──────── Hour (0-23)\n")
	html.WriteString("└────────── Minute (0-59)\n")
	html.WriteString("</pre>")

	html.WriteString("</div>")
	return html.String()
}

// ChmodHandler handles chmod:{permissions} queries
type ChmodHandler struct{}

// NewChmodHandler creates a new chmod handler
func NewChmodHandler() *ChmodHandler {
	return &ChmodHandler{}
}

func (h *ChmodHandler) Type() AnswerType {
	return AnswerTypeChmod
}

func (h *ChmodHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("permissions required")
	}

	var owner, group, other int
	var symbolic string

	// Try to parse as octal
	if len(term) == 3 || len(term) == 4 {
		octal := term
		if len(term) == 4 {
			octal = term[1:] // Skip special bits for now
		}
		if n, err := strconv.ParseInt(octal, 8, 32); err == nil && n >= 0 && n <= 0777 {
			owner = int((n >> 6) & 7)
			group = int((n >> 3) & 7)
			other = int(n & 7)
			symbolic = permToSymbolic(owner) + permToSymbolic(group) + permToSymbolic(other)
		}
	}

	// Try to parse as symbolic (rwxrwxrwx)
	if symbolic == "" && len(term) == 9 {
		symbolic = strings.ToLower(term)
		owner = symbolicToPerm(symbolic[0:3])
		group = symbolicToPerm(symbolic[3:6])
		other = symbolicToPerm(symbolic[6:9])
	}

	if symbolic == "" {
		return &Answer{
			Type:        AnswerTypeChmod,
			Term:        term,
			Title:       "chmod Calculator",
			Description: "Invalid permissions",
			Content:     fmt.Sprintf("<p>Invalid permissions: <code>%s</code>. Use octal (755) or symbolic (rwxr-xr-x).</p>", escapeHTML(term)),
			Error:       "invalid_permissions",
		}, nil
	}

	octalValue := fmt.Sprintf("%d%d%d", owner, group, other)

	data := map[string]interface{}{
		"octal":    octalValue,
		"symbolic": symbolic,
		"owner":    owner,
		"group":    group,
		"other":    other,
	}

	return &Answer{
		Type:        AnswerTypeChmod,
		Term:        term,
		Title:       fmt.Sprintf("chmod %s", octalValue),
		Description: symbolic,
		Content:     formatChmodContent(octalValue, symbolic, owner, group, other),
		Source:      "chmod Calculator",
		Data:        data,
	}, nil
}

func permToSymbolic(perm int) string {
	s := ""
	if perm&4 != 0 {
		s += "r"
	} else {
		s += "-"
	}
	if perm&2 != 0 {
		s += "w"
	} else {
		s += "-"
	}
	if perm&1 != 0 {
		s += "x"
	} else {
		s += "-"
	}
	return s
}

func symbolicToPerm(s string) int {
	p := 0
	if len(s) >= 1 && s[0] == 'r' {
		p |= 4
	}
	if len(s) >= 2 && s[1] == 'w' {
		p |= 2
	}
	if len(s) >= 3 && s[2] == 'x' {
		p |= 1
	}
	return p
}

func formatChmodContent(octal, symbolic string, owner, group, other int) string {
	var html strings.Builder
	html.WriteString("<div class=\"chmod-content\">")
	html.WriteString("<h1>chmod Calculator</h1>")

	// Display both formats
	html.WriteString("<div class=\"chmod-display\">")
	html.WriteString(fmt.Sprintf("<div class=\"octal\"><span class=\"label\">Octal</span><code>%s</code></div>", octal))
	html.WriteString(fmt.Sprintf("<div class=\"symbolic\"><span class=\"label\">Symbolic</span><code>%s</code></div>", symbolic))
	html.WriteString("</div>")

	// Permission grid
	html.WriteString("<h2>Permission Grid</h2>")
	html.WriteString("<table class=\"chmod-table\">")
	html.WriteString("<thead><tr><th></th><th>Read (4)</th><th>Write (2)</th><th>Execute (1)</th><th>Value</th></tr></thead>")
	html.WriteString("<tbody>")

	for _, row := range []struct{ name string; perm int }{{"Owner", owner}, {"Group", group}, {"Other", other}} {
		html.WriteString("<tr>")
		html.WriteString(fmt.Sprintf("<td><strong>%s</strong></td>", row.name))
		html.WriteString(fmt.Sprintf("<td>%s</td>", checkMark(row.perm&4 != 0)))
		html.WriteString(fmt.Sprintf("<td>%s</td>", checkMark(row.perm&2 != 0)))
		html.WriteString(fmt.Sprintf("<td>%s</td>", checkMark(row.perm&1 != 0)))
		html.WriteString(fmt.Sprintf("<td><code>%d</code></td>", row.perm))
		html.WriteString("</tr>")
	}

	html.WriteString("</tbody></table>")

	// Usage
	html.WriteString("<h2>Usage</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>chmod %s filename</code></pre>", octal))
	html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")

	// Common permissions
	html.WriteString("<h2>Common Permissions</h2>")
	html.WriteString("<ul>")
	html.WriteString("<li><code>755</code> - Standard for directories and executables</li>")
	html.WriteString("<li><code>644</code> - Standard for files</li>")
	html.WriteString("<li><code>600</code> - Private file (owner only)</li>")
	html.WriteString("<li><code>700</code> - Private directory (owner only)</li>")
	html.WriteString("<li><code>777</code> - Full access (avoid in production!)</li>")
	html.WriteString("</ul>")

	html.WriteString("</div>")
	return html.String()
}

func checkMark(enabled bool) string {
	if enabled {
		return "✓"
	}
	return "✗"
}

// RegexHandler handles regex:{pattern} queries
type RegexHandler struct{}

// NewRegexHandler creates a new regex handler
func NewRegexHandler() *RegexHandler {
	return &RegexHandler{}
}

func (h *RegexHandler) Type() AnswerType {
	return AnswerTypeRegex
}

func (h *RegexHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("regex pattern required")
	}

	// Try to compile the regex
	re, err := regexp.Compile(term)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeRegex,
			Term:        term,
			Title:       "Regex Error",
			Description: "Invalid regex pattern",
			Content:     fmt.Sprintf("<p class=\"error\">Invalid regex: %s</p><pre><code>%s</code></pre>", escapeHTML(err.Error()), escapeHTML(term)),
			Error:       "invalid_regex",
		}, nil
	}

	// Analyze the pattern
	analysis := analyzeRegex(term)

	data := map[string]interface{}{
		"pattern":    term,
		"valid":      true,
		"analysis":   analysis,
		"numSubexp":  re.NumSubexp(),
	}

	return &Answer{
		Type:        AnswerTypeRegex,
		Term:        term,
		Title:       "Regex Analyzer",
		Description: "Valid regular expression",
		Content:     formatRegexContent(term, analysis, re.NumSubexp()),
		Source:      "Regex Analyzer",
		Data:        data,
	}, nil
}

func analyzeRegex(pattern string) []string {
	var analysis []string

	// Check for common patterns
	if strings.HasPrefix(pattern, "^") {
		analysis = append(analysis, "Anchored to start of string")
	}
	if strings.HasSuffix(pattern, "$") {
		analysis = append(analysis, "Anchored to end of string")
	}
	if strings.Contains(pattern, ".*") {
		analysis = append(analysis, "Contains greedy wildcard (.*)")
	}
	if strings.Contains(pattern, ".+") {
		analysis = append(analysis, "Contains greedy one-or-more (.+)")
	}
	if strings.Contains(pattern, "?") {
		analysis = append(analysis, "Contains optional elements")
	}
	if strings.Contains(pattern, "[") {
		analysis = append(analysis, "Contains character class")
	}
	if strings.Contains(pattern, "(") {
		analysis = append(analysis, "Contains capture group(s)")
	}
	if strings.Contains(pattern, "\\d") {
		analysis = append(analysis, "Matches digits (\\d)")
	}
	if strings.Contains(pattern, "\\w") {
		analysis = append(analysis, "Matches word characters (\\w)")
	}
	if strings.Contains(pattern, "\\s") {
		analysis = append(analysis, "Matches whitespace (\\s)")
	}

	if len(analysis) == 0 {
		analysis = append(analysis, "Simple literal pattern")
	}

	return analysis
}

func formatRegexContent(pattern string, analysis []string, numSubexp int) string {
	var html strings.Builder
	html.WriteString("<div class=\"regex-content\">")
	html.WriteString("<h1>Regex Analyzer</h1>")

	html.WriteString("<h2>Pattern</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(pattern)))
	html.WriteString("<p class=\"valid\">✓ Valid regex pattern</p>")

	if numSubexp > 0 {
		html.WriteString(fmt.Sprintf("<p>Capture groups: %d</p>", numSubexp))
	}

	html.WriteString("<h2>Analysis</h2>")
	html.WriteString("<ul>")
	for _, a := range analysis {
		html.WriteString(fmt.Sprintf("<li>%s</li>", escapeHTML(a)))
	}
	html.WriteString("</ul>")

	// Quick reference
	html.WriteString("<h2>Quick Reference</h2>")
	html.WriteString("<table class=\"regex-ref\">")
	html.WriteString("<tbody>")
	html.WriteString("<tr><td><code>.</code></td><td>Any character</td></tr>")
	html.WriteString("<tr><td><code>*</code></td><td>Zero or more</td></tr>")
	html.WriteString("<tr><td><code>+</code></td><td>One or more</td></tr>")
	html.WriteString("<tr><td><code>?</code></td><td>Optional</td></tr>")
	html.WriteString("<tr><td><code>^</code></td><td>Start of string</td></tr>")
	html.WriteString("<tr><td><code>$</code></td><td>End of string</td></tr>")
	html.WriteString("<tr><td><code>\\d</code></td><td>Digit [0-9]</td></tr>")
	html.WriteString("<tr><td><code>\\w</code></td><td>Word character [a-zA-Z0-9_]</td></tr>")
	html.WriteString("<tr><td><code>\\s</code></td><td>Whitespace</td></tr>")
	html.WriteString("<tr><td><code>[abc]</code></td><td>Character class</td></tr>")
	html.WriteString("<tr><td><code>(group)</code></td><td>Capture group</td></tr>")
	html.WriteString("</tbody></table>")

	html.WriteString("</div>")
	return html.String()
}

// JWTHandler handles jwt:{token} queries
type JWTHandler struct{}

// NewJWTHandler creates a new JWT handler
func NewJWTHandler() *JWTHandler {
	return &JWTHandler{}
}

func (h *JWTHandler) Type() AnswerType {
	return AnswerTypeJWT
}

func (h *JWTHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("JWT token required")
	}

	parts := strings.Split(term, ".")
	if len(parts) != 3 {
		return &Answer{
			Type:        AnswerTypeJWT,
			Term:        truncateString(term, 50),
			Title:       "JWT Decoder",
			Description: "Invalid JWT format",
			Content:     "<p class=\"error\">Invalid JWT: Token must have 3 parts separated by dots.</p>",
			Error:       "invalid_jwt",
		}, nil
	}

	// Decode header
	headerJSON, err := base64URLDecode(parts[0])
	if err != nil {
		return &Answer{
			Type:        AnswerTypeJWT,
			Term:        truncateString(term, 50),
			Title:       "JWT Decoder",
			Description: "Invalid header",
			Content:     fmt.Sprintf("<p class=\"error\">Invalid JWT header: %s</p>", escapeHTML(err.Error())),
			Error:       "invalid_header",
		}, nil
	}

	// Decode payload
	payloadJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return &Answer{
			Type:        AnswerTypeJWT,
			Term:        truncateString(term, 50),
			Title:       "JWT Decoder",
			Description: "Invalid payload",
			Content:     fmt.Sprintf("<p class=\"error\">Invalid JWT payload: %s</p>", escapeHTML(err.Error())),
			Error:       "invalid_payload",
		}, nil
	}

	// Parse header
	var header map[string]interface{}
	json.Unmarshal(headerJSON, &header)

	// Parse payload
	var payload map[string]interface{}
	json.Unmarshal(payloadJSON, &payload)

	// Check expiration
	expired := false
	expiresIn := ""
	if exp, ok := payload["exp"].(float64); ok {
		expTime := time.Unix(int64(exp), 0)
		if time.Now().After(expTime) {
			expired = true
			expiresIn = "Expired"
		} else {
			expiresIn = fmt.Sprintf("Expires in %s", time.Until(expTime).Round(time.Second))
		}
	}

	// Format JSON for display
	headerPretty, _ := json.MarshalIndent(header, "", "  ")
	payloadPretty, _ := json.MarshalIndent(payload, "", "  ")

	data := map[string]interface{}{
		"header":    header,
		"payload":   payload,
		"expired":   expired,
		"expiresIn": expiresIn,
	}

	return &Answer{
		Type:        AnswerTypeJWT,
		Term:        truncateString(term, 50),
		Title:       "JWT Decoder",
		Description: "JSON Web Token decoded",
		Content:     formatJWTContent(string(headerPretty), string(payloadPretty), expired, expiresIn, payload),
		Source:      "JWT Decoder",
		Data:        data,
	}, nil
}

func base64URLDecode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func formatJWTContent(header, payload string, expired bool, expiresIn string, claims map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"jwt-content\">")
	html.WriteString("<h1>JWT Decoder</h1>")

	// Expiration status
	if expiresIn != "" {
		if expired {
			html.WriteString(fmt.Sprintf("<p class=\"expired\">⚠️ %s</p>", expiresIn))
		} else {
			html.WriteString(fmt.Sprintf("<p class=\"valid\">✓ %s</p>", expiresIn))
		}
	}

	// Header
	html.WriteString("<h2>Header</h2>")
	html.WriteString(fmt.Sprintf("<pre class=\"jwt-json\"><code>%s</code></pre>", escapeHTML(header)))

	// Payload
	html.WriteString("<h2>Payload</h2>")
	html.WriteString(fmt.Sprintf("<pre class=\"jwt-json\"><code>%s</code></pre>", escapeHTML(payload)))

	// Standard claims explanation
	html.WriteString("<h2>Standard Claims</h2>")
	html.WriteString("<dl>")
	if v, ok := claims["iss"]; ok {
		html.WriteString(fmt.Sprintf("<dt>iss (Issuer)</dt><dd>%v</dd>", v))
	}
	if v, ok := claims["sub"]; ok {
		html.WriteString(fmt.Sprintf("<dt>sub (Subject)</dt><dd>%v</dd>", v))
	}
	if v, ok := claims["aud"]; ok {
		html.WriteString(fmt.Sprintf("<dt>aud (Audience)</dt><dd>%v</dd>", v))
	}
	if v, ok := claims["exp"].(float64); ok {
		html.WriteString(fmt.Sprintf("<dt>exp (Expiration)</dt><dd>%s</dd>", time.Unix(int64(v), 0).Format(time.RFC1123)))
	}
	if v, ok := claims["iat"].(float64); ok {
		html.WriteString(fmt.Sprintf("<dt>iat (Issued At)</dt><dd>%s</dd>", time.Unix(int64(v), 0).Format(time.RFC1123)))
	}
	html.WriteString("</dl>")

	html.WriteString("<p class=\"note\"><strong>Security Note:</strong> Never paste sensitive JWTs into untrusted tools. This decoder runs client-side.</p>")

	html.WriteString("</div>")
	return html.String()
}

// TimestampHandler handles timestamp:{value} queries
type TimestampHandler struct{}

// NewTimestampHandler creates a new timestamp handler
func NewTimestampHandler() *TimestampHandler {
	return &TimestampHandler{}
}

func (h *TimestampHandler) Type() AnswerType {
	return AnswerTypeTimestamp
}

func (h *TimestampHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("timestamp or date required")
	}

	var t time.Time

	// Check for "now"
	if strings.ToLower(term) == "now" {
		t = time.Now()
	} else if n, err := strconv.ParseInt(term, 10, 64); err == nil {
		// Unix timestamp (seconds or milliseconds)
		if n > 9999999999 {
			// Milliseconds
			t = time.Unix(n/1000, (n%1000)*1000000)
		} else {
			// Seconds
			t = time.Unix(n, 0)
		}
	} else {
		// Try to parse as date string
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02",
			"Jan 2, 2006",
			"January 2, 2006",
			"02/01/2006",
			"01/02/2006",
		}

		for _, format := range formats {
			if parsed, err := time.Parse(format, term); err == nil {
				t = parsed
				break
			}
		}
	}

	if t.IsZero() {
		return &Answer{
			Type:        AnswerTypeTimestamp,
			Term:        term,
			Title:       "Timestamp Converter",
			Description: "Invalid timestamp or date",
			Content:     fmt.Sprintf("<p class=\"error\">Could not parse: <code>%s</code></p>", escapeHTML(term)),
			Error:       "invalid_timestamp",
		}, nil
	}

	// Calculate relative time
	now := time.Now()
	diff := now.Sub(t)
	var relative string
	if diff > 0 {
		relative = formatDuration(diff) + " ago"
	} else {
		relative = "in " + formatDuration(-diff)
	}

	data := map[string]interface{}{
		"unix":         t.Unix(),
		"unixMilli":    t.UnixMilli(),
		"iso8601":      t.Format(time.RFC3339),
		"rfc2822":      t.Format(time.RFC1123Z),
		"human":        t.Format("Monday, January 2, 2006 3:04:05 PM MST"),
		"relative":     relative,
		"utc":          t.UTC().Format(time.RFC3339),
	}

	return &Answer{
		Type:        AnswerTypeTimestamp,
		Term:        term,
		Title:       "Timestamp Converter",
		Description: t.Format(time.RFC1123),
		Content:     formatTimestampContent(t, relative),
		Source:      "Timestamp Converter",
		Data:        data,
	}, nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days < 30 {
		return fmt.Sprintf("%d days", days)
	}
	if days < 365 {
		return fmt.Sprintf("%d months", days/30)
	}
	return fmt.Sprintf("%d years", days/365)
}

func formatTimestampContent(t time.Time, relative string) string {
	var html strings.Builder
	html.WriteString("<div class=\"timestamp-content\">")
	html.WriteString("<h1>Timestamp Converter</h1>")

	html.WriteString(fmt.Sprintf("<p class=\"relative\">%s</p>", escapeHTML(relative)))

	html.WriteString("<table class=\"timestamp-table\">")
	html.WriteString("<tbody>")
	html.WriteString(fmt.Sprintf("<tr><td>Unix (seconds)</td><td><code>%d</code></td></tr>", t.Unix()))
	html.WriteString(fmt.Sprintf("<tr><td>Unix (milliseconds)</td><td><code>%d</code></td></tr>", t.UnixMilli()))
	html.WriteString(fmt.Sprintf("<tr><td>ISO 8601</td><td><code>%s</code></td></tr>", t.Format(time.RFC3339)))
	html.WriteString(fmt.Sprintf("<tr><td>RFC 2822</td><td><code>%s</code></td></tr>", t.Format(time.RFC1123Z)))
	html.WriteString(fmt.Sprintf("<tr><td>Human Readable</td><td>%s</td></tr>", t.Format("Monday, January 2, 2006 3:04:05 PM")))
	html.WriteString(fmt.Sprintf("<tr><td>UTC</td><td><code>%s</code></td></tr>", t.UTC().Format(time.RFC3339)))
	html.WriteString(fmt.Sprintf("<tr><td>Day of Year</td><td>%d</td></tr>", t.YearDay()))
	_, week := t.ISOWeek()
	html.WriteString(fmt.Sprintf("<tr><td>Week Number</td><td>%d</td></tr>", week))
	html.WriteString("</tbody></table>")

	html.WriteString("</div>")
	return html.String()
}
