package server

import (
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/version"
)

// OpenSearchDescription represents the OpenSearch XML format
type OpenSearchDescription struct {
	XMLName        xml.Name         `xml:"OpenSearchDescription"`
	XMLNS          string           `xml:"xmlns,attr"`
	ShortName      string           `xml:"ShortName"`
	Description    string           `xml:"Description"`
	Tags           string           `xml:"Tags,omitempty"`
	Contact        string           `xml:"Contact,omitempty"`
	LongName       string           `xml:"LongName,omitempty"`
	Image          *OpenSearchImage `xml:"Image,omitempty"`
	URLs           []OpenSearchURL  `xml:"Url"`
	InputEncoding  string           `xml:"InputEncoding"`
	OutputEncoding string           `xml:"OutputEncoding"`
}

// OpenSearchImage represents the search engine icon
type OpenSearchImage struct {
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
	Type   string `xml:"type,attr"`
	URL    string `xml:",chardata"`
}

// OpenSearchURL represents a search URL template
type OpenSearchURL struct {
	Type     string `xml:"type,attr"`
	Method   string `xml:"method,attr,omitempty"`
	Template string `xml:"template,attr"`
	Rel      string `xml:"rel,attr,omitempty"`
}

// handleOpenSearch generates the OpenSearch XML description
func (s *Server) handleOpenSearch(w http.ResponseWriter, r *http.Request) {
	// Get base URL
	baseURL := s.getBaseURL(r)

	// Get custom name from query parameter if provided
	customName := r.URL.Query().Get("name")

	// Determine values
	shortName := s.config.Search.OpenSearch.ShortName
	if shortName == "" {
		shortName = s.config.Server.Title
	}
	if customName != "" {
		shortName = customName
	}

	description := s.config.Search.OpenSearch.Description
	if description == "" {
		description = s.config.Server.Description
	}

	longName := s.config.Search.OpenSearch.LongName
	if longName == "" {
		longName = shortName
	}

	contact := s.config.Search.OpenSearch.Contact
	if contact == "" && s.config.Server.Contact.Email != "" {
		contact = s.config.Server.Contact.Email
	}

	// Build OpenSearch description
	osd := &OpenSearchDescription{
		XMLNS:          "http://a9.com/-/spec/opensearch/1.1/",
		ShortName:      shortName,
		Description:    description,
		Tags:           s.config.Search.OpenSearch.Tags,
		Contact:        contact,
		LongName:       longName,
		InputEncoding:  "UTF-8",
		OutputEncoding: "UTF-8",
		URLs: []OpenSearchURL{
			{
				Type:     "text/html",
				Method:   "get",
				Template: baseURL + "/search?q={searchTerms}",
			},
			{
				Type:     "application/x-suggestions+json",
				Template: baseURL + api.APIPrefix + "/autocomplete?q={searchTerms}",
				Rel:      "suggestions",
			},
		},
	}

	// Add image if configured
	if s.config.Search.OpenSearch.Image != "" {
		imageURL := s.config.Search.OpenSearch.Image
		if !strings.HasPrefix(imageURL, "http") {
			imageURL = baseURL + imageURL
		}
		osd.Image = &OpenSearchImage{
			Width:  64,
			Height: 64,
			Type:   "image/png",
			URL:    imageURL,
		}
	}

	// Render XML
	w.Header().Set("Content-Type", "application/opensearchdescription+xml; charset=utf-8")

	// Write XML header
	w.Write([]byte(xml.Header))

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(osd); err != nil {
		localizedHTTPError(w, r, http.StatusInternalServerError, "errors.server_error")
		return
	}
}

// handleBangProxy handles proxied bang requests for privacy
func (s *Server) handleBangProxy(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		s.handleError(w, r, http.StatusBadRequest, "Bad Request", "Missing URL parameter")
		return
	}

	// Validate URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Bad Request", "Invalid URL")
		return
	}

	// Only allow HTTP/HTTPS
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		s.handleError(w, r, http.StatusBadRequest, "Bad Request", "Invalid URL scheme")
		return
	}

	// SSRF prevention: reject localhost and private/internal IPs
	if err := validateNotPrivateProxy(parsedURL.Hostname()); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Bad Request", "URL not allowed")
		return
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Note: Tor SOCKS proxy support can be added here if needed
	// For now, the proxy makes direct requests

	// Create request
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Error", "Failed to create request")
		return
	}

	// Set headers to appear as a normal browser
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		// If proxy fails, fall back to redirect
		http.Redirect(w, r, targetURL, http.StatusFound)
		return
	}
	defer resp.Body.Close()

	// Copy response headers (selective)
	for _, header := range []string{"Content-Type", "Content-Disposition", "Cache-Control"} {
		if val := resp.Header.Get(header); val != "" {
			w.Header().Set(header, val)
		}
	}

	// Remove tracking headers
	w.Header().Del("Set-Cookie")

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Copy body
	io.Copy(w, resp.Body)
}

// handlePreferences handles the user preferences page
func (s *Server) handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handlePreferencesSave(w, r)
		return
	}

	data := s.newPageData(w, r, "Preferences", "preferences")
	data.CSRFToken = s.getCSRFToken(r)

	// Get all available bangs for display
	data.Data = map[string]interface{}{
		"bangs":      s.bangManager.GetAll(),
		"categories": s.bangManager.GetCategories(),
		"builtins":   s.bangManager.GetBuiltins(),
	}

	if err := s.renderer.Render(w, "preferences", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handlePreferencesSave handles saving user preferences
func (s *Server) handlePreferencesSave(w http.ResponseWriter, r *http.Request) {
	// Preferences are saved client-side in localStorage
	// This endpoint just acknowledges the save
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"data": map[string]string{},
	})
}

// getBaseURL returns the base URL for the server.
// Detects HTTPS from direct TLS, configured BaseURL, and all common
// reverse proxy headers (nginx, Caddy, Traefik, Apache, HAProxy, etc.).
func (s *Server) getBaseURL(r *http.Request) string {
	// Use configured base URL if available
	if s.config.Server.BaseURL != "" {
		return strings.TrimRight(s.config.Server.BaseURL, "/")
	}

	// Detect HTTPS — check all common reverse proxy headers
	scheme := "http"
	switch {
	case r.TLS != nil:
		scheme = "https"
	case r.Header.Get("X-Forwarded-Proto") == "https":
		scheme = "https"
	case r.Header.Get("X-Forwarded-Protocol") == "https":
		scheme = "https"
	case r.Header.Get("X-Forwarded-Ssl") == "on":
		scheme = "https"
	case r.Header.Get("X-Scheme") == "https":
		scheme = "https"
	case strings.Contains(r.Header.Get("Forwarded"), "proto=https"):
		// RFC 7239: Forwarded: for=...; proto=https
		scheme = "https"
	}

	host := r.Host
	// Prefer X-Forwarded-Host for reverse proxy setups; take first value if comma-separated
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = strings.TrimSpace(strings.SplitN(fwdHost, ",", 2)[0])
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

// validateNotPrivateProxy rejects hostnames that resolve to private, loopback,
// or link-local addresses, preventing SSRF attacks via the bang proxy endpoint.
func validateNotPrivateProxy(hostname string) error {
	lower := strings.ToLower(hostname)
	if lower == "localhost" || lower == "127.0.0.1" || lower == "::1" {
		return fmt.Errorf("localhost not allowed")
	}
	if strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("internal hostname not allowed")
	}
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %w", err)
	}
	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("private/local IP not allowed")
		}
	}
	return nil
}
