package direct

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/version"
)

const (
	maxASNNeighbours        = 50
	maxRIPEStatCacheEntries = 256
	maxRIPEStatCacheBytes   = 16 << 20
	ripeStatCacheTTL        = 30 * time.Minute
)

type ripeStatCacheEntry struct {
	data     []byte
	storedAt time.Time
}

var ripeStatCache = struct {
	sync.Mutex
	entries map[string]ripeStatCacheEntry
	bytes   int
}{entries: make(map[string]ripeStatCacheEntry)}

// ASNHandler serves live ASN and routing data from RIPEstat's documented Data API.
type ASNHandler struct {
	client *http.Client
}

// NewASNHandler creates an ASN lookup client with a bounded request timeout.
func NewASNHandler() *ASNHandler {
	return &ASNHandler{client: &http.Client{Timeout: 10 * time.Second}}
}

func (h *ASNHandler) Type() AnswerType { return AnswerTypeASN }

func (h *ASNHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToUpper(term))
	term = strings.TrimPrefix(term, "AS")
	asn, err := strconv.ParseUint(term, 10, 32)
	if err != nil || asn == 0 {
		return nil, fmt.Errorf("invalid ASN")
	}
	resource := "AS" + strconv.FormatUint(asn, 10)

	var detail struct {
		Holder string `json:"holder"`
		Block  struct {
			Description string `json:"desc"`
		} `json:"block"`
	}
	if err := h.fetchRIPEStat(ctx, "as-overview", resource, &detail); err != nil {
		return nil, err
	}
	if detail.Holder == "" {
		return &Answer{Type: AnswerTypeASN, Term: resource, Title: "ASN: " + resource,
			Description: i18n.T(LangFromContext(ctx), "direct.asn_not_found"),
			Content:     fmt.Sprintf("<p>No information found for ASN <code>%s</code>.</p>", escapeHTML(resource)), Error: "not_found"}, nil
	}

	var ipv4Prefixes, ipv6Prefixes []string
	var upstreams, downstreams []asnNeighbour
	var history []asnHistory
	var registration asnRegistration
	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); ipv4Prefixes, ipv6Prefixes = h.fetchASNPrefixes(ctx, resource) }()
	go func() { defer wg.Done(); upstreams, downstreams = h.fetchASNNeighbours(ctx, resource) }()
	go func() { defer wg.Done(); history = h.fetchASNHistory(ctx, resource) }()
	go func() { defer wg.Done(); registration = h.fetchASNRegistration(ctx, resource) }()
	wg.Wait()

	data := map[string]interface{}{
		"asn": asn, "name": detail.Holder, "description": detail.Block.Description,
		"country": registration.Country, "rir": registration.Authority, "dateAllocated": registration.Created,
		"ipv4Prefixes": ipv4Prefixes, "ipv6Prefixes": ipv6Prefixes,
		"ipv4PrefixCount": len(ipv4Prefixes), "ipv6PrefixCount": len(ipv6Prefixes),
		"ipv4AddressCount": countAnnouncedAddresses(ipv4Prefixes).String(),
		"ipv6AddressCount": countAnnouncedAddresses(ipv6Prefixes).String(),
		"upstreams":        upstreams, "downstreams": downstreams, "history": history,
		"upstreamSource": "RIPE RIS observed AS paths; this does not establish commercial relationships",
		"lookingGlass": []string{
			"https://stat.ripe.net/" + resource,
			"https://bgp.tools/as" + strconv.FormatUint(asn, 10),
		},
	}
	return &Answer{
		Type: AnswerTypeASN, Term: resource, Title: fmt.Sprintf("%s - %s", resource, detail.Holder),
		Description: detail.Block.Description, Content: formatASNData(data),
		Source: "RIPEstat Data API", SourceURL: "https://stat.ripe.net/" + resource, Data: data,
	}, nil
}

type ripeStatEnvelope struct {
	DataCallStatus string          `json:"data_call_status"`
	Data           json.RawMessage `json:"data"`
	Messages       []string        `json:"messages"`
}

func (h *ASNHandler) fetchRIPEStat(ctx context.Context, endpoint, resource string, destination interface{}) error {
	data, err := h.fetchRIPEStatData(ctx, endpoint, resource)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode RIPEstat %s data: %w", endpoint, err)
	}
	return nil
}

func (h *ASNHandler) fetchRIPEStatData(ctx context.Context, endpoint, resource string) ([]byte, error) {
	cacheKey := endpoint + ":" + resource
	now := time.Now()
	if cached, ok := getRIPEStatCache(cacheKey, now); ok {
		return cached, nil
	}
	query := url.Values{"resource": {resource}}
	requestURL := "https://stat.ripe.net/data/" + endpoint + "/data.json?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "search/"+version.Version+" (ASN lookup; RIPEstat Data API)")
	response, err := h.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("RIPEstat %s request failed: %w", endpoint, err)
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("RIPEstat %s returned HTTP %d", endpoint, response.StatusCode)
	}
	var envelope ripeStatEnvelope
	if err := decodeBoundedJSON(response.Body, 4<<20, &envelope); err != nil {
		return nil, fmt.Errorf("decode RIPEstat %s response: %w", endpoint, err)
	}
	if envelope.DataCallStatus != "ok" || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil, fmt.Errorf("RIPEstat %s returned no usable data", endpoint)
	}
	storeRIPEStatCache(cacheKey, envelope.Data, time.Now())
	return envelope.Data, nil
}

func getRIPEStatCache(key string, now time.Time) ([]byte, bool) {
	ripeStatCache.Lock()
	defer ripeStatCache.Unlock()
	entry, ok := ripeStatCache.entries[key]
	if !ok {
		return nil, false
	}
	if now.Sub(entry.storedAt) >= ripeStatCacheTTL {
		delete(ripeStatCache.entries, key)
		ripeStatCache.bytes -= len(entry.data)
		return nil, false
	}
	return entry.data, true
}

func storeRIPEStatCache(key string, data []byte, now time.Time) {
	if len(data) == 0 || len(data) > maxRIPEStatCacheBytes {
		return
	}
	ripeStatCache.Lock()
	defer ripeStatCache.Unlock()
	for cacheKey, entry := range ripeStatCache.entries {
		if now.Sub(entry.storedAt) >= ripeStatCacheTTL {
			delete(ripeStatCache.entries, cacheKey)
			ripeStatCache.bytes -= len(entry.data)
		}
	}
	if previous, ok := ripeStatCache.entries[key]; ok {
		ripeStatCache.bytes -= len(previous.data)
		delete(ripeStatCache.entries, key)
	}
	for len(ripeStatCache.entries) >= maxRIPEStatCacheEntries || ripeStatCache.bytes+len(data) > maxRIPEStatCacheBytes {
		oldestKey := ""
		var oldest time.Time
		for cacheKey, entry := range ripeStatCache.entries {
			if oldestKey == "" || entry.storedAt.Before(oldest) {
				oldestKey, oldest = cacheKey, entry.storedAt
			}
		}
		if oldestKey == "" {
			return
		}
		entry := ripeStatCache.entries[oldestKey]
		delete(ripeStatCache.entries, oldestKey)
		ripeStatCache.bytes -= len(entry.data)
	}
	copyData := append([]byte(nil), data...)
	ripeStatCache.entries[key] = ripeStatCacheEntry{data: copyData, storedAt: now}
	ripeStatCache.bytes += len(copyData)
}

func (h *ASNHandler) fetchASNPrefixes(ctx context.Context, resource string) ([]string, []string) {
	var response struct {
		Prefixes []struct {
			Prefix string `json:"prefix"`
		} `json:"prefixes"`
	}
	if h.fetchRIPEStat(ctx, "announced-prefixes", resource, &response) != nil {
		return nil, nil
	}
	ipv4 := make([]string, 0, len(response.Prefixes))
	ipv6 := make([]string, 0, len(response.Prefixes))
	for _, entry := range response.Prefixes {
		prefix := strings.TrimSpace(entry.Prefix)
		if prefix == "" {
			continue
		}
		if strings.Contains(prefix, ":") {
			ipv6 = append(ipv6, prefix)
		} else {
			ipv4 = append(ipv4, prefix)
		}
	}
	return ipv4, ipv6
}

type asnNeighbour struct {
	ASN  int    `json:"asn"`
	Name string `json:"name,omitempty"`
	Type string `json:"observed_position"`
}

func (h *ASNHandler) fetchASNNeighbours(ctx context.Context, resource string) ([]asnNeighbour, []asnNeighbour) {
	var response struct {
		Neighbours []struct {
			ASN  int    `json:"asn"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"neighbours"`
	}
	if h.fetchRIPEStat(ctx, "asn-neighbours", resource, &response) != nil {
		return nil, nil
	}
	upstreams := make([]asnNeighbour, 0)
	downstreams := make([]asnNeighbour, 0)
	for _, entry := range response.Neighbours {
		neighbour := asnNeighbour{ASN: entry.ASN, Name: entry.Name, Type: entry.Type}
		switch entry.Type {
		case "left":
			upstreams = append(upstreams, neighbour)
		case "right":
			downstreams = append(downstreams, neighbour)
		}
	}
	return capASNNeighbours(upstreams), capASNNeighbours(downstreams)
}

type ipRange struct {
	start *big.Int
	end   *big.Int
}

func countAnnouncedAddresses(prefixes []string) *big.Int {
	ranges := make([]ipRange, 0, len(prefixes))
	for _, value := range prefixes {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			continue
		}
		prefix = prefix.Masked()
		address := prefix.Addr()
		start := new(big.Int).SetBytes(address.AsSlice())
		size := new(big.Int).Lsh(big.NewInt(1), uint(address.BitLen()-prefix.Bits()))
		end := new(big.Int).Sub(new(big.Int).Add(new(big.Int).Set(start), size), big.NewInt(1))
		ranges = append(ranges, ipRange{start: start, end: end})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start.Cmp(ranges[j].start) < 0 })
	total := new(big.Int)
	if len(ranges) == 0 {
		return total
	}
	currentStart := new(big.Int).Set(ranges[0].start)
	currentEnd := new(big.Int).Set(ranges[0].end)
	for _, candidate := range ranges[1:] {
		nextAfterEnd := new(big.Int).Add(new(big.Int).Set(currentEnd), big.NewInt(1))
		if candidate.start.Cmp(nextAfterEnd) <= 0 {
			if candidate.end.Cmp(currentEnd) > 0 {
				currentEnd.Set(candidate.end)
			}
			continue
		}
		addRangeSize(total, currentStart, currentEnd)
		currentStart.Set(candidate.start)
		currentEnd.Set(candidate.end)
	}
	addRangeSize(total, currentStart, currentEnd)
	return total
}

func addRangeSize(total, start, end *big.Int) {
	rangeSize := new(big.Int).Sub(new(big.Int).Set(end), start)
	rangeSize.Add(rangeSize, big.NewInt(1))
	total.Add(total, rangeSize)
}

func capASNNeighbours(neighbours []asnNeighbour) []asnNeighbour {
	if len(neighbours) > maxASNNeighbours {
		return neighbours[:maxASNNeighbours]
	}
	return neighbours
}

type asnHistory struct {
	ASN       int      `json:"asn"`
	FirstSeen string   `json:"first_seen,omitempty"`
	LastSeen  string   `json:"last_seen,omitempty"`
	Changes   []string `json:"changes,omitempty"`
}

func (h *ASNHandler) fetchASNHistory(ctx context.Context, resource string) []asnHistory {
	var response struct {
		Neighbours []struct {
			Neighbour int `json:"neighbour"`
			Timelines []struct {
				Start string `json:"starttime"`
				End   string `json:"endtime"`
			} `json:"timelines"`
		} `json:"neighbours"`
	}
	if h.fetchRIPEStat(ctx, "asn-neighbours-history", resource, &response) != nil {
		return nil
	}
	result := make([]asnHistory, 0, len(response.Neighbours))
	for _, entry := range response.Neighbours {
		history := asnHistory{ASN: entry.Neighbour}
		for _, timeline := range entry.Timelines {
			if history.FirstSeen == "" || timeline.Start < history.FirstSeen {
				history.FirstSeen = timeline.Start
			}
			if timeline.End > history.LastSeen {
				history.LastSeen = timeline.End
			}
			history.Changes = append(history.Changes, timeline.Start)
		}
		result = append(result, history)
	}
	if len(result) > maxASNNeighbours {
		result = result[:maxASNNeighbours]
	}
	return result
}

type asnRegistration struct {
	Country   string
	Authority string
	Created   string
}

func (h *ASNHandler) fetchASNRegistration(ctx context.Context, resource string) asnRegistration {
	var response struct {
		Authorities []string `json:"authorities"`
		Records     [][]struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"records"`
	}
	if h.fetchRIPEStat(ctx, "whois", resource, &response) != nil {
		return asnRegistration{}
	}
	var result asnRegistration
	if len(response.Authorities) > 0 {
		result.Authority = response.Authorities[0]
	}
	for _, record := range response.Records {
		for _, entry := range record {
			switch strings.ToLower(entry.Key) {
			case "country":
				if result.Country == "" && len(entry.Value) == 2 {
					result.Country = strings.ToUpper(entry.Value)
				}
			case "created":
				if result.Created == "" {
					result.Created = entry.Value
				}
			}
		}
	}
	return result
}

func formatASNData(data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"asn-data\">")
	fmt.Fprintf(&html, "<h2>AS%s</h2>", escapeHTML(fmt.Sprint(data["asn"])))
	fmt.Fprintf(&html, "<p class=\"asn-name\">%s</p>", escapeHTML(fmt.Sprint(data["name"])))
	html.WriteString("<dl class=\"asn-details\">")
	for _, field := range []struct{ key, label string }{
		{"description", "Description"}, {"country", "Registration country"}, {"rir", "Registry"}, {"dateAllocated", "Registration created"},
	} {
		if value := strings.TrimSpace(fmt.Sprint(data[field.key])); value != "" && value != "<nil>" {
			fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", field.label, escapeHTML(value))
		}
	}
	html.WriteString("</dl>")
	for _, family := range []struct{ key, title string }{{"ipv4Prefixes", "Announced IPv4 prefixes"}, {"ipv6Prefixes", "Announced IPv6 prefixes"}} {
		if prefixes, ok := data[family.key].([]string); ok && len(prefixes) > 0 {
			fmt.Fprintf(&html, "<h3>%s (%d)</h3><ul>", family.title, len(prefixes))
			for _, prefix := range prefixes {
				fmt.Fprintf(&html, "<li><code>%s</code></li>", escapeHTML(prefix))
			}
			html.WriteString("</ul>")
		}
	}
	for _, count := range []struct{ key, label string }{{"ipv4AddressCount", "Unique IPv4 addresses announced"}, {"ipv6AddressCount", "Unique IPv6 addresses announced"}} {
		if value, ok := data[count.key].(string); ok {
			fmt.Fprintf(&html, "<p>%s: %s</p>", count.label, escapeHTML(value))
		}
	}
	for _, relation := range []struct{ key, title string }{{"upstreams", "Observed upstream ASNs"}, {"downstreams", "Observed downstream ASNs"}} {
		if neighbours, ok := data[relation.key].([]asnNeighbour); ok && len(neighbours) > 0 {
			html.WriteString("<h3>" + relation.title + "</h3><ul>")
			for _, neighbour := range neighbours {
				fmt.Fprintf(&html, "<li>AS%d %s</li>", neighbour.ASN, escapeHTML(neighbour.Name))
			}
			html.WriteString("</ul>")
		}
	}
	if history, ok := data["history"].([]asnHistory); ok && len(history) > 0 {
		html.WriteString("<h3>Observed neighbour history</h3><ul>")
		for _, entry := range history {
			fmt.Fprintf(&html, "<li>AS%d: %s – %s (%d observed periods)</li>", entry.ASN, escapeHTML(entry.FirstSeen), escapeHTML(entry.LastSeen), len(entry.Changes))
		}
		html.WriteString("</ul>")
	}
	html.WriteString("<h3>Looking glasses</h3><ul>")
	if links, ok := data["lookingGlass"].([]string); ok {
		for _, link := range links {
			fmt.Fprintf(&html, "<li><a href=\"%s\" rel=\"nofollow noopener\">%s</a></li>", escapeHTML(link), escapeHTML(link))
		}
	}
	html.WriteString("</ul><p class=\"asn-data-note\">Upstream and downstream positions are inferred from observed RIS AS paths, not commercial relationships.</p></div>")
	return html.String()
}
