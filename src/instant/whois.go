package instant

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"
)

// WHOISHandler handles domain WHOIS lookups
type WHOISHandler struct {
	patterns    []*regexp.Regexp
	whoisServers map[string]string
}

// NewWHOISHandler creates a new WHOIS handler
func NewWHOISHandler() *WHOISHandler {
	return &WHOISHandler{
		patterns: []*regexp.Regexp{
			// "whois:example.com" or "whois: example.com"
			regexp.MustCompile(`(?i)^whois[:\s]+([a-zA-Z0-9][-a-zA-Z0-9]*(?:\.[a-zA-Z0-9][-a-zA-Z0-9]*)+)$`),
			// "whois lookup example.com"
			regexp.MustCompile(`(?i)^whois\s+lookup[:\s]+([a-zA-Z0-9][-a-zA-Z0-9]*(?:\.[a-zA-Z0-9][-a-zA-Z0-9]*)+)$`),
			// "domain whois example.com"
			regexp.MustCompile(`(?i)^domain\s+whois[:\s]+([a-zA-Z0-9][-a-zA-Z0-9]*(?:\.[a-zA-Z0-9][-a-zA-Z0-9]*)+)$`),
		},
		whoisServers: map[string]string{
			// Generic TLDs
			"com":  "whois.verisign-grs.com",
			"net":  "whois.verisign-grs.com",
			"org":  "whois.pir.org",
			"info": "whois.afilias.net",
			"biz":  "whois.biz",
			"name": "whois.nic.name",
			"mobi": "whois.dotmobiregistry.net",
			"pro":  "whois.registrypro.pro",
			"aero": "whois.aero",
			"asia": "whois.nic.asia",
			"coop": "whois.nic.coop",
			"edu":  "whois.educause.edu",
			"gov":  "whois.dotgov.gov",
			"int":  "whois.iana.org",
			"jobs": "whois.nic.jobs",
			"mil":  "whois.nic.mil",
			"tel":  "whois.nic.tel",
			"xxx":  "whois.nic.xxx",
			"io":   "whois.nic.io",
			"co":   "whois.nic.co",
			"me":   "whois.nic.me",
			"tv":   "whois.nic.tv",
			"cc":   "ccwhois.verisign-grs.com",
			"ws":   "whois.website.ws",
			"ly":   "whois.nic.ly",

			// Country code TLDs
			"ac":  "whois.nic.ac",
			"ad":  "whois.ripe.net",
			"ae":  "whois.aeda.net.ae",
			"af":  "whois.nic.af",
			"ag":  "whois.nic.ag",
			"ai":  "whois.nic.ai",
			"al":  "whois.ripe.net",
			"am":  "whois.amnic.net",
			"ar":  "whois.nic.ar",
			"as":  "whois.nic.as",
			"at":  "whois.nic.at",
			"au":  "whois.auda.org.au",
			"ax":  "whois.ax",
			"az":  "whois.ripe.net",
			"ba":  "whois.ripe.net",
			"be":  "whois.dns.be",
			"bg":  "whois.register.bg",
			"bi":  "whois1.nic.bi",
			"bj":  "whois.nic.bj",
			"bn":  "whois.bnnic.bn",
			"bo":  "whois.nic.bo",
			"br":  "whois.registro.br",
			"by":  "whois.cctld.by",
			"bz":  "whois.afilias-grs.info",
			"ca":  "whois.cira.ca",
			"ch":  "whois.nic.ch",
			"ci":  "whois.nic.ci",
			"cl":  "whois.nic.cl",
			"cn":  "whois.cnnic.cn",
			"cr":  "whois.nic.cr",
			"cx":  "whois.nic.cx",
			"cz":  "whois.nic.cz",
			"de":  "whois.denic.de",
			"dk":  "whois.dk-hostmaster.dk",
			"dm":  "whois.nic.dm",
			"do":  "whois.nic.do",
			"dz":  "whois.nic.dz",
			"ec":  "whois.nic.ec",
			"ee":  "whois.tld.ee",
			"es":  "whois.nic.es",
			"eu":  "whois.eu",
			"fi":  "whois.fi",
			"fm":  "whois.nic.fm",
			"fo":  "whois.nic.fo",
			"fr":  "whois.nic.fr",
			"gd":  "whois.nic.gd",
			"ge":  "whois.nic.ge",
			"gg":  "whois.gg",
			"gi":  "whois2.afilias-grs.net",
			"gl":  "whois.nic.gl",
			"gp":  "whois.nic.gp",
			"gr":  "grweb.ics.forth.gr",
			"gs":  "whois.nic.gs",
			"gy":  "whois.registry.gy",
			"hk":  "whois.hkirc.hk",
			"hn":  "whois.nic.hn",
			"hr":  "whois.dns.hr",
			"ht":  "whois.nic.ht",
			"hu":  "whois.nic.hu",
			"id":  "whois.pandi.or.id",
			"ie":  "whois.iedr.ie",
			"il":  "whois.isoc.org.il",
			"im":  "whois.nic.im",
			"in":  "whois.registry.in",
			"iq":  "whois.cmc.iq",
			"ir":  "whois.nic.ir",
			"is":  "whois.isnic.is",
			"it":  "whois.nic.it",
			"je":  "whois.je",
			"jp":  "whois.jprs.jp",
			"ke":  "whois.kenic.or.ke",
			"kg":  "whois.kg",
			"ki":  "whois.nic.ki",
			"kr":  "whois.kr",
			"kw":  "whois.nic.kw",
			"kz":  "whois.nic.kz",
			"la":  "whois.nic.la",
			"li":  "whois.nic.li",
			"lt":  "whois.domreg.lt",
			"lu":  "whois.dns.lu",
			"lv":  "whois.nic.lv",
			"ma":  "whois.registre.ma",
			"md":  "whois.nic.md",
			"mg":  "whois.nic.mg",
			"mk":  "whois.marnet.mk",
			"ml":  "whois.dot.ml",
			"mn":  "whois.nic.mn",
			"mo":  "whois.monic.mo",
			"mp":  "whois.nic.mp",
			"mq":  "whois.mediaserv.net",
			"ms":  "whois.nic.ms",
			"mt":  "whois.nic.org.mt",
			"mu":  "whois.nic.mu",
			"mw":  "whois.nic.mw",
			"mx":  "whois.mx",
			"my":  "whois.mynic.my",
			"mz":  "whois.nic.mz",
			"na":  "whois.na-nic.com.na",
			"nc":  "whois.nc",
			"nf":  "whois.nic.nf",
			"ng":  "whois.nic.net.ng",
			"nl":  "whois.domain-registry.nl",
			"no":  "whois.norid.no",
			"nu":  "whois.iis.nu",
			"nz":  "whois.srs.net.nz",
			"om":  "whois.registry.om",
			"pe":  "kero.yachay.pe",
			"pf":  "whois.registry.pf",
			"pk":  "whois.pknic.net.pk",
			"pl":  "whois.dns.pl",
			"pm":  "whois.nic.pm",
			"pr":  "whois.nic.pr",
			"ps":  "whois.pnina.ps",
			"pt":  "whois.dns.pt",
			"pw":  "whois.nic.pw",
			"qa":  "whois.registry.qa",
			"re":  "whois.nic.re",
			"ro":  "whois.rotld.ro",
			"rs":  "whois.rnids.rs",
			"ru":  "whois.tcinet.ru",
			"rw":  "whois.ricta.org.rw",
			"sa":  "whois.nic.net.sa",
			"sb":  "whois.nic.net.sb",
			"sc":  "whois.nic.sc",
			"se":  "whois.iis.se",
			"sg":  "whois.sgnic.sg",
			"sh":  "whois.nic.sh",
			"si":  "whois.register.si",
			"sk":  "whois.sk-nic.sk",
			"sl":  "whois.nic.sl",
			"sm":  "whois.nic.sm",
			"sn":  "whois.nic.sn",
			"so":  "whois.nic.so",
			"st":  "whois.nic.st",
			"su":  "whois.tcinet.ru",
			"sx":  "whois.sx",
			"sy":  "whois.tld.sy",
			"tc":  "whois.nic.tc",
			"tf":  "whois.nic.tf",
			"th":  "whois.thnic.co.th",
			"tj":  "whois.nic.tj",
			"tk":  "whois.dot.tk",
			"tl":  "whois.nic.tl",
			"tm":  "whois.nic.tm",
			"tn":  "whois.ati.tn",
			"to":  "whois.tonic.to",
			"tr":  "whois.nic.tr",
			"tw":  "whois.twnic.net.tw",
			"tz":  "whois.tznic.or.tz",
			"ua":  "whois.ua",
			"ug":  "whois.co.ug",
			"uk":  "whois.nic.uk",
			"us":  "whois.nic.us",
			"uy":  "whois.nic.org.uy",
			"uz":  "whois.cctld.uz",
			"vc":  "whois.nic.vc",
			"ve":  "whois.nic.ve",
			"vg":  "whois.nic.vg",
			"wf":  "whois.nic.wf",
			"yt":  "whois.nic.yt",
			"za":  "whois.registry.net.za",
		},
	}
}

func (h *WHOISHandler) Name() string {
	return "whois"
}

func (h *WHOISHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *WHOISHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *WHOISHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract domain from query
	var domain string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			domain = strings.ToLower(matches[1])
			break
		}
	}

	if domain == "" {
		return nil, nil
	}

	// Get the TLD
	tld := h.getTLD(domain)
	if tld == "" {
		return &Answer{
			Type:    AnswerTypeWHOIS,
			Query:   query,
			Title:   fmt.Sprintf("WHOIS: %s", domain),
			Content: "Unable to determine TLD for domain",
		}, nil
	}

	// Get WHOIS server for TLD
	whoisServer, ok := h.whoisServers[tld]
	if !ok {
		// Try IANA as fallback
		whoisServer = "whois.iana.org"
	}

	// Query WHOIS server
	rawData, err := h.queryWHOIS(ctx, whoisServer, domain)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeWHOIS,
			Query:   query,
			Title:   fmt.Sprintf("WHOIS: %s", domain),
			Content: fmt.Sprintf("Error querying WHOIS server: %v", err),
		}, nil
	}

	// Parse WHOIS response
	parsed := h.parseWHOISResponse(rawData)

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<div class=\"whois-result\">"))
	content.WriteString(fmt.Sprintf("<strong>Domain:</strong> %s<br><br>", domain))

	if parsed.registrar != "" {
		content.WriteString(fmt.Sprintf("<strong>Registrar:</strong> %s<br>", parsed.registrar))
	}

	if parsed.registrantOrg != "" {
		content.WriteString(fmt.Sprintf("<strong>Registrant Organization:</strong> %s<br>", parsed.registrantOrg))
	}

	if parsed.creationDate != "" {
		content.WriteString(fmt.Sprintf("<strong>Creation Date:</strong> %s<br>", parsed.creationDate))
	}

	if parsed.expirationDate != "" {
		content.WriteString(fmt.Sprintf("<strong>Expiration Date:</strong> %s<br>", parsed.expirationDate))
	}

	if parsed.updatedDate != "" {
		content.WriteString(fmt.Sprintf("<strong>Updated Date:</strong> %s<br>", parsed.updatedDate))
	}

	if len(parsed.nameServers) > 0 {
		content.WriteString("<br><strong>Name Servers:</strong><br>")
		for _, ns := range parsed.nameServers {
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;%s<br>", ns))
		}
	}

	if len(parsed.status) > 0 {
		content.WriteString("<br><strong>Status:</strong><br>")
		for _, s := range parsed.status {
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;%s<br>", s))
		}
	}

	if parsed.dnssec != "" {
		content.WriteString(fmt.Sprintf("<br><strong>DNSSEC:</strong> %s<br>", parsed.dnssec))
	}

	content.WriteString("</div>")

	// Build data map
	dataMap := map[string]interface{}{
		"domain":       domain,
		"whois_server": whoisServer,
	}

	if parsed.registrar != "" {
		dataMap["registrar"] = parsed.registrar
	}
	if parsed.registrantOrg != "" {
		dataMap["registrant_organization"] = parsed.registrantOrg
	}
	if parsed.creationDate != "" {
		dataMap["creation_date"] = parsed.creationDate
	}
	if parsed.expirationDate != "" {
		dataMap["expiration_date"] = parsed.expirationDate
	}
	if parsed.updatedDate != "" {
		dataMap["updated_date"] = parsed.updatedDate
	}
	if len(parsed.nameServers) > 0 {
		dataMap["name_servers"] = parsed.nameServers
	}
	if len(parsed.status) > 0 {
		dataMap["status"] = parsed.status
	}

	return &Answer{
		Type:      AnswerTypeWHOIS,
		Query:     query,
		Title:     fmt.Sprintf("WHOIS: %s", domain),
		Content:   content.String(),
		Source:    "WHOIS",
		SourceURL: fmt.Sprintf("https://who.is/whois/%s", domain),
		Data:      dataMap,
	}, nil
}

func (h *WHOISHandler) getTLD(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

func (h *WHOISHandler) queryWHOIS(ctx context.Context, server, domain string) (string, error) {
	// Create a dialer with context
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", server+":43")
	if err != nil {
		return "", fmt.Errorf("failed to connect to WHOIS server: %v", err)
	}
	defer conn.Close()

	// Set deadline for read/write operations
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	// Send query
	query := domain + "\r\n"
	// Special handling for some servers
	if server == "whois.verisign-grs.com" {
		query = "=" + domain + "\r\n"
	} else if server == "whois.denic.de" {
		query = "-T dn,ace " + domain + "\r\n"
	}

	_, err = conn.Write([]byte(query))
	if err != nil {
		return "", fmt.Errorf("failed to send query: %v", err)
	}

	// Read response
	var result strings.Builder
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			// Ignore read errors after we have some data
			if result.Len() > 0 {
				break
			}
			return "", fmt.Errorf("failed to read response: %v", err)
		}
		result.WriteString(line)
	}

	return result.String(), nil
}

type whoisParsed struct {
	registrar      string
	registrantOrg  string
	creationDate   string
	expirationDate string
	updatedDate    string
	nameServers    []string
	status         []string
	dnssec         string
}

func (h *WHOISHandler) parseWHOISResponse(raw string) *whoisParsed {
	result := &whoisParsed{
		nameServers: make([]string, 0),
		status:      make([]string, 0),
	}

	// Common field patterns
	patterns := map[string]*regexp.Regexp{
		"registrar":       regexp.MustCompile(`(?i)(?:Registrar|Sponsoring Registrar)[:\s]+(.+)`),
		"registrant_org":  regexp.MustCompile(`(?i)(?:Registrant Organization|Registrant|Organisation)[:\s]+(.+)`),
		"creation_date":   regexp.MustCompile(`(?i)(?:Creation Date|Created|Created On|Registration Date|Domain Registration Date)[:\s]+(.+)`),
		"expiration_date": regexp.MustCompile(`(?i)(?:Registry Expiry Date|Expiration Date|Expiry Date|Expires On|Expires|Valid Until)[:\s]+(.+)`),
		"updated_date":    regexp.MustCompile(`(?i)(?:Updated Date|Last Updated|Modified)[:\s]+(.+)`),
		"name_server":     regexp.MustCompile(`(?i)(?:Name Server|Nameserver|nserver)[:\s]+(.+)`),
		"status":          regexp.MustCompile(`(?i)(?:Domain Status|Status)[:\s]+(.+)`),
		"dnssec":          regexp.MustCompile(`(?i)DNSSEC[:\s]+(.+)`),
	}

	lines := strings.Split(raw, "\n")
	seenNameServers := make(map[string]bool)
	seenStatus := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}

		// Match each pattern
		if result.registrar == "" {
			if matches := patterns["registrar"].FindStringSubmatch(line); len(matches) > 1 {
				result.registrar = strings.TrimSpace(matches[1])
			}
		}

		if result.registrantOrg == "" {
			if matches := patterns["registrant_org"].FindStringSubmatch(line); len(matches) > 1 {
				result.registrantOrg = strings.TrimSpace(matches[1])
			}
		}

		if result.creationDate == "" {
			if matches := patterns["creation_date"].FindStringSubmatch(line); len(matches) > 1 {
				result.creationDate = strings.TrimSpace(matches[1])
			}
		}

		if result.expirationDate == "" {
			if matches := patterns["expiration_date"].FindStringSubmatch(line); len(matches) > 1 {
				result.expirationDate = strings.TrimSpace(matches[1])
			}
		}

		if result.updatedDate == "" {
			if matches := patterns["updated_date"].FindStringSubmatch(line); len(matches) > 1 {
				result.updatedDate = strings.TrimSpace(matches[1])
			}
		}

		if matches := patterns["name_server"].FindStringSubmatch(line); len(matches) > 1 {
			ns := strings.ToLower(strings.TrimSpace(matches[1]))
			// Remove any IP address suffix
			if idx := strings.Index(ns, " "); idx > 0 {
				ns = ns[:idx]
			}
			if !seenNameServers[ns] && ns != "" {
				seenNameServers[ns] = true
				result.nameServers = append(result.nameServers, ns)
			}
		}

		if matches := patterns["status"].FindStringSubmatch(line); len(matches) > 1 {
			status := strings.TrimSpace(matches[1])
			// Clean up status - remove URLs
			if idx := strings.Index(status, "http"); idx > 0 {
				status = strings.TrimSpace(status[:idx])
			}
			if !seenStatus[status] && status != "" {
				seenStatus[status] = true
				result.status = append(result.status, status)
			}
		}

		if result.dnssec == "" {
			if matches := patterns["dnssec"].FindStringSubmatch(line); len(matches) > 1 {
				result.dnssec = strings.TrimSpace(matches[1])
			}
		}
	}

	return result
}
