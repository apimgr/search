package instant

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// TechHandler detects technology stack of a website
type TechHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// TechInfo represents a detected technology
type TechInfo struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Version  string `json:"version,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

// NewTechHandler creates a new technology detection handler
func NewTechHandler() *TechHandler {
	return &TechHandler{
		client: &http.Client{Timeout: 15 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^tech[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^technology[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^stack[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^builtwith[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^what\s+is\s+(.+)\s+built\s+with\??$`),
			regexp.MustCompile(`(?i)^detect\s+tech[:\s]+(.+)$`),
		},
	}
}

func (h *TechHandler) Name() string {
	return "tech"
}

func (h *TechHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *TechHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *TechHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract domain from query
	domain := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			domain = strings.TrimSpace(matches[1])
			break
		}
	}

	if domain == "" {
		return nil, nil
	}

	// Normalize domain to URL
	baseURL := normalizeDomainToURL(domain)

	// Detect technologies
	techs, err := h.detectTechnologies(ctx, baseURL)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeTech,
			Query:   query,
			Title:   fmt.Sprintf("Technology Stack: %s", domain),
			Content: fmt.Sprintf("Error detecting technologies: %v", err),
		}, nil
	}

	if len(techs) == 0 {
		return &Answer{
			Type:    AnswerTypeTech,
			Query:   query,
			Title:   fmt.Sprintf("Technology Stack: %s", domain),
			Content: "No technologies detected.",
		}, nil
	}

	// Group by category
	byCategory := make(map[string][]TechInfo)
	for _, tech := range techs {
		byCategory[tech.Category] = append(byCategory[tech.Category], tech)
	}

	// Sort categories
	categories := make([]string, 0, len(byCategory))
	for cat := range byCategory {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>Technologies detected on %s:</strong><br><br>", domain))

	for _, category := range categories {
		content.WriteString(fmt.Sprintf("<strong>%s:</strong><br>", category))
		for _, tech := range byCategory[category] {
			if tech.Version != "" {
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;- %s (%s)<br>", tech.Name, tech.Version))
			} else {
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;- %s<br>", tech.Name))
			}
		}
		content.WriteString("<br>")
	}

	return &Answer{
		Type:    AnswerTypeTech,
		Query:   query,
		Title:   fmt.Sprintf("Technology Stack: %s", domain),
		Content: content.String(),
		Data: map[string]interface{}{
			"domain":       domain,
			"technologies": techs,
			"count":        len(techs),
		},
	}, nil
}

// detectTechnologies detects technologies used by a website
func (h *TechHandler) detectTechnologies(ctx context.Context, baseURL string) ([]TechInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read body (limited)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}

	bodyStr := string(body)
	lowerBody := strings.ToLower(bodyStr)

	var techs []TechInfo
	seen := make(map[string]bool)

	addTech := func(name, category, ver, evidence string) {
		key := name + category
		if !seen[key] {
			seen[key] = true
			techs = append(techs, TechInfo{
				Name:     name,
				Category: category,
				Version:  ver,
				Evidence: evidence,
			})
		}
	}

	// Detect from HTTP headers
	h.detectFromHeaders(resp.Header, addTech)

	// Detect from HTML content
	h.detectFromHTML(lowerBody, bodyStr, addTech)

	return techs, nil
}

// detectFromHeaders detects technologies from HTTP headers
func (h *TechHandler) detectFromHeaders(headers http.Header, addTech func(name, category, ver, evidence string)) {
	// Server header
	if server := headers.Get("Server"); server != "" {
		serverLower := strings.ToLower(server)
		if strings.Contains(serverLower, "nginx") {
			ver := extractVersion(server, `nginx/([0-9.]+)`)
			addTech("Nginx", "Web Server", ver, "Server header")
		}
		if strings.Contains(serverLower, "apache") {
			ver := extractVersion(server, `Apache/([0-9.]+)`)
			addTech("Apache", "Web Server", ver, "Server header")
		}
		if strings.Contains(serverLower, "cloudflare") {
			addTech("Cloudflare", "CDN", "", "Server header")
		}
		if strings.Contains(serverLower, "gws") {
			addTech("Google Web Server", "Web Server", "", "Server header")
		}
		if strings.Contains(serverLower, "iis") {
			ver := extractVersion(server, `IIS/([0-9.]+)`)
			addTech("Microsoft IIS", "Web Server", ver, "Server header")
		}
		if strings.Contains(serverLower, "litespeed") {
			addTech("LiteSpeed", "Web Server", "", "Server header")
		}
		if strings.Contains(serverLower, "openresty") {
			addTech("OpenResty", "Web Server", "", "Server header")
		}
	}

	// X-Powered-By header
	if poweredBy := headers.Get("X-Powered-By"); poweredBy != "" {
		poweredByLower := strings.ToLower(poweredBy)
		if strings.Contains(poweredByLower, "php") {
			ver := extractVersion(poweredBy, `PHP/([0-9.]+)`)
			addTech("PHP", "Programming Language", ver, "X-Powered-By header")
		}
		if strings.Contains(poweredByLower, "asp.net") {
			addTech("ASP.NET", "Framework", "", "X-Powered-By header")
		}
		if strings.Contains(poweredByLower, "express") {
			addTech("Express.js", "Framework", "", "X-Powered-By header")
		}
		if strings.Contains(poweredByLower, "next.js") {
			addTech("Next.js", "Framework", "", "X-Powered-By header")
		}
	}

	// X-Generator header
	if generator := headers.Get("X-Generator"); generator != "" {
		generatorLower := strings.ToLower(generator)
		if strings.Contains(generatorLower, "drupal") {
			addTech("Drupal", "CMS", "", "X-Generator header")
		}
		if strings.Contains(generatorLower, "wordpress") {
			addTech("WordPress", "CMS", "", "X-Generator header")
		}
	}

	// CDN detection from headers
	if headers.Get("CF-Ray") != "" || headers.Get("CF-Cache-Status") != "" {
		addTech("Cloudflare", "CDN", "", "CF headers")
	}
	if headers.Get("X-Amz-Cf-Id") != "" || headers.Get("X-Amz-Cf-Pop") != "" {
		addTech("Amazon CloudFront", "CDN", "", "AWS headers")
	}
	if headers.Get("X-Fastly-Request-ID") != "" {
		addTech("Fastly", "CDN", "", "Fastly headers")
	}
	if headers.Get("X-Akamai-Transformed") != "" {
		addTech("Akamai", "CDN", "", "Akamai headers")
	}
	if headers.Get("X-Vercel-Id") != "" {
		addTech("Vercel", "Hosting", "", "Vercel headers")
	}
	if headers.Get("X-Netlify-Request-ID") != "" {
		addTech("Netlify", "Hosting", "", "Netlify headers")
	}

	// Security headers
	if headers.Get("Strict-Transport-Security") != "" {
		addTech("HSTS", "Security", "", "HSTS header")
	}
	if headers.Get("Content-Security-Policy") != "" {
		addTech("CSP", "Security", "", "CSP header")
	}
}

// detectFromHTML detects technologies from HTML content
func (h *TechHandler) detectFromHTML(lowerBody, bodyStr string, addTech func(name, category, ver, evidence string)) {
	// CMS detection
	if strings.Contains(lowerBody, "wp-content") || strings.Contains(lowerBody, "wp-includes") {
		ver := ""
		if matches := regexp.MustCompile(`content="WordPress ([0-9.]+)"`).FindStringSubmatch(bodyStr); len(matches) > 1 {
			ver = matches[1]
		}
		addTech("WordPress", "CMS", ver, "HTML content")
	}
	if strings.Contains(lowerBody, "joomla") {
		addTech("Joomla", "CMS", "", "HTML content")
	}
	if strings.Contains(lowerBody, "drupal") || strings.Contains(lowerBody, "/sites/default/files") {
		addTech("Drupal", "CMS", "", "HTML content")
	}
	if strings.Contains(lowerBody, "shopify") || strings.Contains(lowerBody, "cdn.shopify.com") {
		addTech("Shopify", "E-commerce", "", "HTML content")
	}
	if strings.Contains(lowerBody, "wix.com") || strings.Contains(lowerBody, "wixstatic.com") {
		addTech("Wix", "Website Builder", "", "HTML content")
	}
	if strings.Contains(lowerBody, "squarespace") {
		addTech("Squarespace", "Website Builder", "", "HTML content")
	}
	if strings.Contains(lowerBody, "ghost") && strings.Contains(lowerBody, "ghost-") {
		addTech("Ghost", "CMS", "", "HTML content")
	}
	if strings.Contains(lowerBody, "webflow") {
		addTech("Webflow", "Website Builder", "", "HTML content")
	}

	// JavaScript frameworks
	if strings.Contains(lowerBody, "react") || strings.Contains(lowerBody, "_react") || strings.Contains(lowerBody, "__next") {
		addTech("React", "JavaScript Framework", "", "HTML content")
	}
	if strings.Contains(lowerBody, "vue") || strings.Contains(lowerBody, "__vue__") {
		addTech("Vue.js", "JavaScript Framework", "", "HTML content")
	}
	if strings.Contains(lowerBody, "ng-version") || strings.Contains(lowerBody, "ng-app") {
		ver := ""
		if matches := regexp.MustCompile(`ng-version="([0-9.]+)"`).FindStringSubmatch(bodyStr); len(matches) > 1 {
			ver = matches[1]
		}
		addTech("Angular", "JavaScript Framework", ver, "HTML content")
	}
	if strings.Contains(lowerBody, "jquery") {
		ver := ""
		if matches := regexp.MustCompile(`jquery[.-]([0-9.]+)(?:\.min)?\.js`).FindStringSubmatch(lowerBody); len(matches) > 1 {
			ver = matches[1]
		}
		addTech("jQuery", "JavaScript Library", ver, "HTML content")
	}
	if strings.Contains(lowerBody, "bootstrap") {
		ver := ""
		if matches := regexp.MustCompile(`bootstrap[.-]([0-9.]+)(?:\.min)?\.(?:js|css)`).FindStringSubmatch(lowerBody); len(matches) > 1 {
			ver = matches[1]
		}
		addTech("Bootstrap", "CSS Framework", ver, "HTML content")
	}
	if strings.Contains(lowerBody, "tailwindcss") || strings.Contains(lowerBody, "tailwind") {
		addTech("Tailwind CSS", "CSS Framework", "", "HTML content")
	}
	if strings.Contains(lowerBody, "svelte") {
		addTech("Svelte", "JavaScript Framework", "", "HTML content")
	}

	// Analytics and tracking
	if strings.Contains(lowerBody, "google-analytics") || strings.Contains(lowerBody, "ga.js") || strings.Contains(lowerBody, "gtag") {
		addTech("Google Analytics", "Analytics", "", "HTML content")
	}
	if strings.Contains(lowerBody, "googletagmanager") {
		addTech("Google Tag Manager", "Tag Manager", "", "HTML content")
	}
	if strings.Contains(lowerBody, "facebook.net/en_us/fbevents.js") || strings.Contains(lowerBody, "fbq(") {
		addTech("Facebook Pixel", "Analytics", "", "HTML content")
	}
	if strings.Contains(lowerBody, "hotjar") {
		addTech("Hotjar", "Analytics", "", "HTML content")
	}
	if strings.Contains(lowerBody, "segment.com") || strings.Contains(lowerBody, "segment.io") {
		addTech("Segment", "Analytics", "", "HTML content")
	}
	if strings.Contains(lowerBody, "mixpanel") {
		addTech("Mixpanel", "Analytics", "", "HTML content")
	}

	// Fonts
	if strings.Contains(lowerBody, "fonts.googleapis.com") || strings.Contains(lowerBody, "fonts.gstatic.com") {
		addTech("Google Fonts", "Fonts", "", "HTML content")
	}
	if strings.Contains(lowerBody, "use.typekit.net") {
		addTech("Adobe Fonts", "Fonts", "", "HTML content")
	}
	if strings.Contains(lowerBody, "fontawesome") {
		addTech("Font Awesome", "Icons", "", "HTML content")
	}

	// CDN detection from content
	if strings.Contains(lowerBody, "cdnjs.cloudflare.com") {
		addTech("cdnjs", "CDN", "", "HTML content")
	}
	if strings.Contains(lowerBody, "unpkg.com") {
		addTech("unpkg", "CDN", "", "HTML content")
	}
	if strings.Contains(lowerBody, "jsdelivr.net") {
		addTech("jsDelivr", "CDN", "", "HTML content")
	}

	// Other services
	if strings.Contains(lowerBody, "recaptcha") || strings.Contains(lowerBody, "grecaptcha") {
		addTech("reCAPTCHA", "Security", "", "HTML content")
	}
	if strings.Contains(lowerBody, "hcaptcha") {
		addTech("hCaptcha", "Security", "", "HTML content")
	}
	if strings.Contains(lowerBody, "stripe.com") || strings.Contains(lowerBody, "stripe.js") {
		addTech("Stripe", "Payment", "", "HTML content")
	}
	if strings.Contains(lowerBody, "paypal") {
		addTech("PayPal", "Payment", "", "HTML content")
	}
	if strings.Contains(lowerBody, "intercom") {
		addTech("Intercom", "Chat", "", "HTML content")
	}
	if strings.Contains(lowerBody, "crisp.chat") {
		addTech("Crisp", "Chat", "", "HTML content")
	}
	if strings.Contains(lowerBody, "zendesk") {
		addTech("Zendesk", "Support", "", "HTML content")
	}
	if strings.Contains(lowerBody, "tawk.to") {
		addTech("Tawk.to", "Chat", "", "HTML content")
	}

	// Meta generator
	if matches := regexp.MustCompile(`<meta[^>]+name=["']generator["'][^>]+content=["']([^"']+)["']`).FindStringSubmatch(lowerBody); len(matches) > 1 {
		generator := matches[1]
		generatorLower := strings.ToLower(generator)
		if strings.Contains(generatorLower, "hugo") {
			addTech("Hugo", "Static Site Generator", "", "Meta generator")
		}
		if strings.Contains(generatorLower, "jekyll") {
			addTech("Jekyll", "Static Site Generator", "", "Meta generator")
		}
		if strings.Contains(generatorLower, "gatsby") {
			addTech("Gatsby", "Static Site Generator", "", "Meta generator")
		}
		if strings.Contains(generatorLower, "eleventy") {
			addTech("Eleventy", "Static Site Generator", "", "Meta generator")
		}
	}
}

// extractVersion extracts version from a string using regex
func extractVersion(s string, pattern string) string {
	re := regexp.MustCompile(pattern)
	if matches := re.FindStringSubmatch(s); len(matches) > 1 {
		return matches[1]
	}
	return ""
}
