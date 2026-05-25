package geoip

import (
	"net"
	"strings"
	"sync"
	"testing"
)

// Tests cover the public Lookup/Config/Result/Country API. Internal MMDB
// decoding is delegated to github.com/oschwald/maxminddb-golang (PART 19)
// so we do not duplicate its parser tests here.

func TestDatabaseURLs(t *testing.T) {
	urls := []string{ASNURL, CountryURL, CityURL, WhoisURL}
	for _, u := range urls {
		if u == "" {
			t.Error("database URL is empty")
		}
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("database URL %q is not https", u)
		}
	}
}

func TestDatabaseURLsJsdelivrCDN(t *testing.T) {
	for _, u := range []string{ASNURL, CountryURL, CityURL, WhoisURL} {
		if !strings.Contains(u, "cdn.jsdelivr.net") {
			t.Errorf("URL %q is not on jsdelivr CDN", u)
		}
	}
}

func TestCountryStruct(t *testing.T) {
	c := Country{Code: "US", Name: "United States", Continent: "NA"}
	if c.Code != "US" || c.Name != "United States" || c.Continent != "NA" {
		t.Errorf("Country struct field mismatch: %+v", c)
	}
}

func TestResultStruct(t *testing.T) {
	r := Result{IP: "8.8.8.8", CountryCode: "US", Found: true}
	if r.IP != "8.8.8.8" || r.CountryCode != "US" || !r.Found {
		t.Errorf("Result struct field mismatch: %+v", r)
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := &Config{Enabled: true, Dir: "/tmp/geoip", Country: true}
	if !cfg.Enabled || cfg.Dir != "/tmp/geoip" || !cfg.Country {
		t.Errorf("Config struct field mismatch: %+v", cfg)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if cfg.Enabled {
		t.Error("default config should be disabled")
	}
	if !cfg.Country {
		t.Error("default config should enable country DB")
	}
	if !cfg.ASN {
		t.Error("default config should enable ASN DB")
	}
}

func TestNewLookup(t *testing.T) {
	cfg := DefaultConfig()
	l := NewLookup(cfg)
	if l == nil {
		t.Fatal("NewLookup returned nil")
	}
	if l.config != cfg {
		t.Error("NewLookup did not store config")
	}
	if len(l.countries) == 0 {
		t.Error("NewLookup did not initialize country map")
	}
}

func TestNewLookupNilConfig(t *testing.T) {
	l := NewLookup(nil)
	if l == nil {
		t.Fatal("NewLookup(nil) returned nil")
	}
	if l.config == nil {
		t.Error("NewLookup(nil) should apply default config")
	}
}

func TestLookupGetCountry(t *testing.T) {
	l := NewLookup(nil)
	if got := l.GetCountry("US"); got == nil || got.Name != "United States" {
		t.Errorf("GetCountry(US) = %+v", got)
	}
	if got := l.GetCountry("us"); got == nil || got.Code != "US" {
		t.Error("GetCountry should be case-insensitive")
	}
	if got := l.GetCountry("ZZ"); got != nil {
		t.Errorf("GetCountry(ZZ) should be nil, got %+v", got)
	}
}

func TestLookupIsLoadedInitial(t *testing.T) {
	l := NewLookup(nil)
	if l.IsLoaded() {
		t.Error("new Lookup should not be loaded")
	}
}

func TestLookupLookupInvalidIP(t *testing.T) {
	l := NewLookup(nil)
	r := l.Lookup("not-an-ip")
	if r == nil {
		t.Fatal("Lookup returned nil result")
	}
	if r.Found {
		t.Error("invalid IP should not be found")
	}
}

func TestLookupLookupWithoutDB(t *testing.T) {
	l := NewLookup(nil)
	r := l.Lookup("8.8.8.8")
	if r == nil || r.Found {
		t.Error("Lookup without loaded DB should return Found=false")
	}
}

func TestLookupIsBlockedEmpty(t *testing.T) {
	l := NewLookup(nil)
	if l.IsBlocked("8.8.8.8", nil) {
		t.Error("empty blocklist should allow all IPs")
	}
}

func TestLookupIsAllowedEmpty(t *testing.T) {
	l := NewLookup(nil)
	if !l.IsAllowed("8.8.8.8", nil) {
		t.Error("empty allowlist should allow all IPs")
	}
}

func TestLookupClose(t *testing.T) {
	l := NewLookup(nil)
	l.Close()
	l.Close()
	if l.IsLoaded() {
		t.Error("Lookup should not be loaded after Close")
	}
}

func TestLookupLoadDatabasesNilConfig(t *testing.T) {
	l := &Lookup{}
	if err := l.LoadDatabases(); err == nil {
		t.Error("LoadDatabases with nil config should error")
	}
}

func TestLookupUpdateDatabasesNilConfig(t *testing.T) {
	l := &Lookup{}
	if err := l.UpdateDatabases(); err == nil {
		t.Error("UpdateDatabases with nil config should error")
	}
}

func TestOpenMMDBInvalidPath(t *testing.T) {
	if _, err := openMMDB("/no/such/file.mmdb"); err == nil {
		t.Error("openMMDB on missing path should error")
	}
}

func TestOpenMMDBEmptyPath(t *testing.T) {
	if _, err := openMMDB(""); err == nil {
		t.Error("openMMDB with empty path should error")
	}
}

func TestMmdbReaderClosedLookups(t *testing.T) {
	r := &mmdbReader{}
	if r.LookupCountry(net.ParseIP("8.8.8.8")) != "" {
		t.Error("LookupCountry on closed reader should return empty")
	}
	asn, org := r.LookupASN(net.ParseIP("8.8.8.8"))
	if asn != 0 || org != "" {
		t.Errorf("LookupASN on closed reader = (%d, %q)", asn, org)
	}
	city, region, postal, lat, lon, tz := r.LookupCity(net.ParseIP("8.8.8.8"))
	if city != "" || region != "" || postal != "" || lat != 0 || lon != 0 || tz != "" {
		t.Errorf("LookupCity on closed reader returned non-zero values")
	}
	o, n := r.LookupWHOIS(net.ParseIP("8.8.8.8"))
	if o != "" || n != "" {
		t.Errorf("LookupWHOIS on closed reader = (%q, %q)", o, n)
	}
}

func TestMmdbReaderCloseIdempotent(t *testing.T) {
	r := &mmdbReader{}
	r.Close()
	r.Close()
}

func TestLookupConcurrency(t *testing.T) {
	l := NewLookup(nil)
	defer l.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.Lookup("8.8.8.8")
			_ = l.IsBlocked("1.1.1.1", []string{"CN"})
			_ = l.IsAllowed("1.1.1.1", []string{"US"})
		}()
	}
	wg.Wait()
}

func TestCountryMapSize(t *testing.T) {
	l := NewLookup(nil)
	// Sanity: must have a reasonable number of countries.
	if len(l.countries) < 150 {
		t.Errorf("country map has %d entries, expected >= 150", len(l.countries))
	}
}

func TestLastUpdateZeroInitially(t *testing.T) {
	l := NewLookup(nil)
	if !l.LastUpdate().IsZero() {
		t.Error("LastUpdate should be zero before any load")
	}
}

func TestHelperConverters(t *testing.T) {
	m := map[string]interface{}{
		"s":    "hello",
		"u":    uint64(42),
		"f":    3.14,
		"nest": map[string]interface{}{"k": "v"},
	}
	if asString(m, "s") != "hello" {
		t.Error("asString failed")
	}
	if asUint(m, "u") != 42 {
		t.Error("asUint failed")
	}
	if asFloat(m, "f") != 3.14 {
		t.Error("asFloat failed")
	}
	if asMap(m, "nest") == nil || asString(asMap(m, "nest"), "k") != "v" {
		t.Error("asMap failed")
	}
	if asString(m, "missing") != "" {
		t.Error("asString missing should be empty")
	}
	if asUint(m, "missing") != 0 {
		t.Error("asUint missing should be 0")
	}
	if asFloat(m, "missing") != 0 {
		t.Error("asFloat missing should be 0")
	}
	if asMap(m, "missing") != nil {
		t.Error("asMap missing should be nil")
	}
}
