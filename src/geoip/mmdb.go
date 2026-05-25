// Package geoip MMDB reader implementation.
//
// Per AI.md PART 19: use github.com/oschwald/maxminddb-golang to read
// ip-location-db MMDB files. ip-location-db embeds custom database_type
// strings (e.g. "asn ipv4", "city ipv6") that geoip2-golang rejects, so we
// use the lower-level maxminddb reader and decode into generic maps.
package geoip

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// mmdbMetadata mirrors the subset of MMDB metadata callers care about.
type mmdbMetadata struct {
	NodeCount                uint32
	RecordSize               uint16
	IPVersion                uint16
	DatabaseType             string
	Languages                []string
	BinaryFormatMajorVersion uint16
	BinaryFormatMinorVersion uint16
	BuildEpoch               uint64
	Description              map[string]string
}

// mmdbReader wraps a maxminddb.Reader and decodes records as generic maps.
// The wrapper keeps the same public method set the rest of the package depends
// on so callers (geoip.go) do not need to change.
type mmdbReader struct {
	mu       sync.RWMutex
	reader   *maxminddb.Reader
	metadata *mmdbMetadata
}

// openMMDB opens an MMDB database file from disk.
func openMMDB(path string) (*mmdbReader, error) {
	if path == "" {
		return nil, errors.New("mmdb path is empty")
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open mmdb: %w", err)
	}
	meta := &mmdbMetadata{
		NodeCount:                uint32(r.Metadata.NodeCount),
		RecordSize:               uint16(r.Metadata.RecordSize),
		IPVersion:                uint16(r.Metadata.IPVersion),
		DatabaseType:             r.Metadata.DatabaseType,
		Languages:                r.Metadata.Languages,
		BinaryFormatMajorVersion: uint16(r.Metadata.BinaryFormatMajorVersion),
		BinaryFormatMinorVersion: uint16(r.Metadata.BinaryFormatMinorVersion),
		BuildEpoch:               uint64(r.Metadata.BuildEpoch),
		Description:              r.Metadata.Description,
	}
	return &mmdbReader{reader: r, metadata: meta}, nil
}

// lookupRecord decodes the record for ip into a generic map. Returns nil when
// the IP has no record or the reader is closed.
func (r *mmdbReader) lookupRecord(ip net.IP) map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.reader == nil || ip == nil {
		return nil
	}
	var record interface{}
	if err := r.reader.Lookup(ip, &record); err != nil {
		return nil
	}
	if record == nil {
		return nil
	}
	if m, ok := record.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// asString returns the string at key from a generic decoded map.
func asString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// asUint returns the uint at key from a generic decoded map.
func asUint(m map[string]interface{}, key string) uint {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case uint64:
			return uint(n)
		case uint32:
			return uint(n)
		case uint16:
			return uint(n)
		case int:
			return uint(n)
		case int64:
			return uint(n)
		}
	}
	return 0
}

// asFloat returns the float at key from a generic decoded map.
func asFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		}
	}
	return 0
}

// asMap returns the nested map at key from a generic decoded map.
func asMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if inner, ok := v.(map[string]interface{}); ok {
			return inner
		}
	}
	return nil
}

// LookupCountry returns the ISO 3166-1 alpha-2 country code for an IP.
// Supports both MaxMind-style (country.iso_code) and ip-location-db
// flat-format (country_code or iso_code at the top level).
func (r *mmdbReader) LookupCountry(ip net.IP) string {
	m := r.lookupRecord(ip)
	if m == nil {
		return ""
	}
	// MaxMind-style nested country.iso_code
	if country := asMap(m, "country"); country != nil {
		if code := asString(country, "iso_code"); code != "" {
			return code
		}
	}
	// ip-location-db flat format
	if code := asString(m, "country_code"); code != "" {
		return code
	}
	if code := asString(m, "iso_code"); code != "" {
		return code
	}
	return ""
}

// LookupASN returns the autonomous system number and organization for an IP.
func (r *mmdbReader) LookupASN(ip net.IP) (uint, string) {
	m := r.lookupRecord(ip)
	if m == nil {
		return 0, ""
	}
	asn := asUint(m, "autonomous_system_number")
	if asn == 0 {
		asn = asUint(m, "asn")
	}
	org := asString(m, "autonomous_system_organization")
	if org == "" {
		org = asString(m, "as_org")
	}
	if org == "" {
		org = asString(m, "name")
	}
	return asn, org
}

// LookupCity returns the city, region, postal, lat/lon and timezone for an IP.
func (r *mmdbReader) LookupCity(ip net.IP) (city, region, postal string, lat, lon float64, tz string) {
	m := r.lookupRecord(ip)
	if m == nil {
		return
	}
	// MaxMind-style: city.names.en
	if cityMap := asMap(m, "city"); cityMap != nil {
		if names := asMap(cityMap, "names"); names != nil {
			city = asString(names, "en")
		}
	}
	// Flat format used by ip-location-db
	if city == "" {
		city = asString(m, "city")
	}
	// Region / subdivision (MaxMind-style first, then flat)
	if subs, ok := m["subdivisions"].([]interface{}); ok && len(subs) > 0 {
		if sub, ok := subs[0].(map[string]interface{}); ok {
			if names := asMap(sub, "names"); names != nil {
				region = asString(names, "en")
			}
		}
	}
	if region == "" {
		region = asString(m, "state1")
	}
	// Postal code
	if postalMap := asMap(m, "postal"); postalMap != nil {
		postal = asString(postalMap, "code")
	}
	if postal == "" {
		postal = asString(m, "postcode")
	}
	// Location (MaxMind-style location.{latitude,longitude,time_zone})
	if loc := asMap(m, "location"); loc != nil {
		lat = asFloat(loc, "latitude")
		lon = asFloat(loc, "longitude")
		tz = asString(loc, "time_zone")
	}
	// Flat format fallback
	if lat == 0 {
		lat = asFloat(m, "latitude")
	}
	if lon == 0 {
		lon = asFloat(m, "longitude")
	}
	if tz == "" {
		tz = asString(m, "timezone")
	}
	return
}

// LookupWHOIS returns registrant org and network range for an IP.
// Per AI.md PART 19: WHOIS database support via ip-location-db
// geo-whois-asn-country dataset (ASN org often represents the registrant).
func (r *mmdbReader) LookupWHOIS(ip net.IP) (registrantOrg, registrantNet string) {
	m := r.lookupRecord(ip)
	if m == nil {
		return
	}
	for _, k := range []string{
		"autonomous_system_organization",
		"as_org",
		"organization",
		"org",
		"name",
	} {
		if v := asString(m, k); v != "" {
			registrantOrg = v
			break
		}
	}
	for _, k := range []string{"network", "range", "prefix"} {
		if v := asString(m, k); v != "" {
			registrantNet = v
			break
		}
	}
	return
}

// Close releases the underlying reader.
func (r *mmdbReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reader != nil {
		_ = r.reader.Close()
		r.reader = nil
	}
	r.metadata = nil
}
