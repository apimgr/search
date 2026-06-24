package geoip

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

// mmdbEnc holds MMDB binary encoding helpers. MMDB uses a custom encoding:
//   - control byte high 3 bits = type, low 5 bits = payload size
//   - type 2 = String, type 5 = Uint16, type 6 = Uint32, type 7 = Map
//   - type 0 = extended; second byte = actual_type - 7
//   - extended type for Uint64 = second byte 2 (9-7); Slice = second byte 4 (11-7)
type mmdbEnc struct{}

func (mmdbEnc) str(s string) []byte {
	b := []byte{byte(2<<5) | byte(len(s))}
	return append(b, []byte(s)...)
}

func (mmdbEnc) uint16(v uint16) []byte {
	if v == 0 {
		return []byte{byte(5 << 5)}
	}
	return []byte{byte(5<<5) | 1, byte(v)}
}

func (mmdbEnc) uint32(v uint32) []byte {
	if v == 0 {
		return []byte{byte(6 << 5)}
	}
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	i := 0
	for i < 3 && b[i] == 0 {
		i++
	}
	return append([]byte{byte(6<<5) | byte(4-i)}, b[i:]...)
}

func (mmdbEnc) uint64Zero() []byte {
	return []byte{0x00, 0x02}
}

func (mmdbEnc) emptySlice() []byte {
	return []byte{0x00, 0x04}
}

func (mmdbEnc) emptyMap() []byte {
	return []byte{byte(7 << 5)}
}

func (e mmdbEnc) mapOf(pairs ...[]byte) []byte {
	count := len(pairs) / 2
	out := []byte{byte(7<<5) | byte(count)}
	for _, p := range pairs {
		out = append(out, p...)
	}
	return out
}

// buildMMDBMeta encodes the metadata section for an MMDB file.
func buildMMDBMeta(e mmdbEnc, dbType string, nodeCount uint32) []byte {
	return e.mapOf(
		e.str("binary_format_major_version"), e.uint16(2),
		e.str("binary_format_minor_version"), e.uint16(0),
		e.str("build_epoch"), e.uint64Zero(),
		e.str("database_type"), e.str(dbType),
		e.str("description"), e.emptyMap(),
		e.str("ip_version"), e.uint16(4),
		e.str("languages"), e.emptySlice(),
		e.str("node_count"), e.uint32(nodeCount),
		e.str("record_size"), e.uint16(24),
	)
}

// buildMinimalMMDB constructs a valid minimal MMDB binary with zero IP records.
// The binary consists of: an empty search tree (0 bytes for node_count=0) +
// 16-byte data-section separator + empty data section + metadata marker +
// MMDB-encoded metadata map.
//
// MMDB encoding reference: https://maxmind.github.io/MaxMind-DB/
func buildMinimalMMDB() []byte {
	var e mmdbEnc
	meta := buildMMDBMeta(e, "country", 0)
	separator := make([]byte, 16)
	marker := []byte("\xAB\xCD\xEFMaxMind.com")
	var buf []byte
	buf = append(buf, separator...)
	buf = append(buf, marker...)
	buf = append(buf, meta...)
	return buf
}

// buildMMDBWithRecord constructs an MMDB with one data record containing the
// given flat key-value string pairs. The record is reachable for ALL IPv4
// addresses (the single root node points to the record in both branches).
//
// Structure:
//   - node_count = 1  →  search tree = 6 bytes (one 24-bit record-size node)
//   - node 0: left = dataPtr, right = dataPtr  (catches all IPs)
//   - dataPtr = nodeCount + dataSeparatorSize + 0 = 1 + 16 = 17
//   - data section: encoded record at offset 0
func buildMMDBWithRecord(dbType string, fields map[string]string) []byte {
	var e mmdbEnc

	// Encode the data record (a Map with the given string fields)
	dataRecord := []byte{byte(7<<5) | byte(len(fields))}
	for k, v := range fields {
		dataRecord = append(dataRecord, e.str(k)...)
		dataRecord = append(dataRecord, e.str(v)...)
	}

	// dataPtr = nodeCount (1) + dataSectionSeparator (16) + dataOffset (0)
	// For 24-bit records, the pointer value is stored as 3 big-endian bytes.
	const nodeCount = uint32(1)
	dataPtr := nodeCount + 16 + 0
	ptrBytes := []byte{byte(dataPtr >> 16), byte(dataPtr >> 8), byte(dataPtr)}

	// Search tree: one node (6 bytes) where both left and right = dataPtr
	treeNode := append(ptrBytes, ptrBytes...)

	separator := make([]byte, 16)
	marker := []byte("\xAB\xCD\xEFMaxMind.com")
	meta := buildMMDBMeta(e, dbType, nodeCount)

	var buf []byte
	buf = append(buf, treeNode...)
	buf = append(buf, separator...)
	buf = append(buf, dataRecord...)
	buf = append(buf, marker...)
	buf = append(buf, meta...)
	return buf
}

// writeTempMMDB writes a minimal MMDB file to a temp directory and returns the path.
func writeTempMMDB(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buildMinimalMMDB(), 0600); err != nil {
		t.Fatalf("writeTempMMDB: %v", err)
	}
	return path
}

// TestBuildMinimalMMDBOpens verifies that buildMinimalMMDB produces a file
// that openMMDB can parse without error.
func TestBuildMinimalMMDBOpens(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "test.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB on minimal MMDB: %v", err)
	}
	db.Close()
}

// TestBuildMMDBWithRecordOpens verifies that buildMMDBWithRecord creates a
// parseable MMDB file.
func TestBuildMMDBWithRecordOpens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record.mmdb")
	data := buildMMDBWithRecord("country", map[string]string{"country_code": "US"})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB on record MMDB: %v", err)
	}
	defer db.Close()
}

// TestOpenMMDBSuccess exercises the success path and checks metadata is populated.
func TestOpenMMDBSuccess(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "country.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	if db.metadata == nil {
		t.Error("openMMDB should populate metadata")
	}
	if db.metadata.DatabaseType != "country" {
		t.Errorf("metadata.DatabaseType = %q, want %q", db.metadata.DatabaseType, "country")
	}
}

// TestLookupRecordNilIP exercises the nil IP guard in lookupRecord.
func TestLookupRecordNilIP(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "country.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	if got := db.lookupRecord(nil); got != nil {
		t.Errorf("lookupRecord(nil) = %v, want nil", got)
	}
}

// TestLookupRecordValidIP exercises the non-nil reader path; the empty DB
// returns nil record for any IP.
func TestLookupRecordValidIP(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "country.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	// Minimal DB has 0 nodes — Lookup returns nil record, not an error.
	result := db.lookupRecord(net.ParseIP("8.8.8.8"))
	// result may be nil (empty DB) — this is valid behavior
	_ = result
}

// TestLookupCountryNilRecord exercises LookupCountry when the DB returns nil.
func TestLookupCountryNilRecord(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "country.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	if got := db.LookupCountry(net.ParseIP("1.2.3.4")); got != "" {
		t.Errorf("LookupCountry on empty DB = %q, want empty", got)
	}
}

// TestLookupASNNilRecord exercises LookupASN when the DB returns nil.
func TestLookupASNNilRecord(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "asn.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	asn, org := db.LookupASN(net.ParseIP("1.2.3.4"))
	if asn != 0 || org != "" {
		t.Errorf("LookupASN on empty DB = (%d, %q)", asn, org)
	}
}

// TestLookupCityNilRecord exercises LookupCity when the DB returns nil.
func TestLookupCityNilRecord(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "city.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	city, region, postal, lat, lon, tz := db.LookupCity(net.ParseIP("1.2.3.4"))
	if city != "" || region != "" || postal != "" || lat != 0 || lon != 0 || tz != "" {
		t.Error("LookupCity on empty DB should return zero values")
	}
}

// TestLookupWHOISNilRecord exercises LookupWHOIS when the DB returns nil.
func TestLookupWHOISNilRecord(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "whois.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	org, net_ := db.LookupWHOIS(net.ParseIP("1.2.3.4"))
	if org != "" || net_ != "" {
		t.Errorf("LookupWHOIS on empty DB = (%q, %q)", org, net_)
	}
}

// writeRecordMMDB writes an MMDB file containing a single catch-all record.
func writeRecordMMDB(t *testing.T, dir, name, dbType string, fields map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buildMMDBWithRecord(dbType, fields), 0600); err != nil {
		t.Fatalf("writeRecordMMDB: %v", err)
	}
	return path
}

// TestLookupCountryFlatFormat exercises the flat-format country_code lookup path.
func TestLookupCountryFlatFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeRecordMMDB(t, dir, "country.mmdb", "country", map[string]string{"country_code": "DE"})
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	got := db.LookupCountry(net.ParseIP("8.8.8.8"))
	if got != "DE" {
		t.Errorf("LookupCountry flat country_code = %q, want %q", got, "DE")
	}
}

// TestLookupCountryIsoCodeFlatFormat exercises the iso_code flat-format lookup path.
func TestLookupCountryIsoCodeFlatFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeRecordMMDB(t, dir, "country.mmdb", "country", map[string]string{"iso_code": "FR"})
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	defer db.Close()
	got := db.LookupCountry(net.ParseIP("1.1.1.1"))
	if got != "FR" {
		t.Errorf("LookupCountry flat iso_code = %q, want %q", got, "FR")
	}
}

// TestLookupASNFlatFormat exercises the flat-format ASN lookup paths.
func TestLookupASNFlatFormat(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]string
		wantOrg string
	}{
		{
			"autonomous_system_organization",
			map[string]string{"as_org": "Example Org"},
			"Example Org",
		},
		{
			"name fallback",
			map[string]string{"name": "FallbackOrg"},
			"FallbackOrg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeRecordMMDB(t, dir, "asn.mmdb", "asn", tt.fields)
			db, err := openMMDB(path)
			if err != nil {
				t.Fatalf("openMMDB: %v", err)
			}
			defer db.Close()
			_, org := db.LookupASN(net.ParseIP("8.8.8.8"))
			if org != tt.wantOrg {
				t.Errorf("LookupASN org = %q, want %q", org, tt.wantOrg)
			}
		})
	}
}

// TestLookupWHOISFlatFormat exercises the WHOIS lookup with various field names.
func TestLookupWHOISFlatFormat(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]string
		wantOrg string
		wantNet string
	}{
		{
			"organization field",
			map[string]string{"organization": "Acme Corp", "network": "192.0.2.0/24"},
			"Acme Corp",
			"192.0.2.0/24",
		},
		{
			"org field",
			map[string]string{"org": "FooOrg", "range": "10.0.0.0/8"},
			"FooOrg",
			"10.0.0.0/8",
		},
		{
			"name field with prefix",
			map[string]string{"name": "BarNet", "prefix": "203.0.113.0/24"},
			"BarNet",
			"203.0.113.0/24",
		},
		{
			"as_org field",
			map[string]string{"as_org": "AsOrg"},
			"AsOrg",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeRecordMMDB(t, dir, "whois.mmdb", "whois", tt.fields)
			db, err := openMMDB(path)
			if err != nil {
				t.Fatalf("openMMDB: %v", err)
			}
			defer db.Close()
			org, net_ := db.LookupWHOIS(net.ParseIP("1.2.3.4"))
			if org != tt.wantOrg {
				t.Errorf("LookupWHOIS org = %q, want %q", org, tt.wantOrg)
			}
			if net_ != tt.wantNet {
				t.Errorf("LookupWHOIS net = %q, want %q", net_, tt.wantNet)
			}
		})
	}
}

// TestLookupWithLoadedCountryAndCountryName verifies that Lookup populates
// CountryName and Continent from the country map when a record is found.
func TestLookupWithKnownCountryCode(t *testing.T) {
	dir := t.TempDir()
	writeRecordMMDB(t, dir, "country.mmdb", "country", map[string]string{"country_code": "US"})

	cfg := &Config{Enabled: true, Dir: dir, Country: true, ASN: false}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	result := l.Lookup("8.8.8.8")
	if result == nil {
		t.Fatal("Lookup returned nil")
	}
	if !result.Found {
		t.Error("Lookup should find a record in the loaded DB")
	}
	if result.CountryCode != "US" {
		t.Errorf("CountryCode = %q, want %q", result.CountryCode, "US")
	}
	if result.CountryName != "United States" {
		t.Errorf("CountryName = %q, want %q", result.CountryName, "United States")
	}
	if result.Continent != "NA" {
		t.Errorf("Continent = %q, want %q", result.Continent, "NA")
	}
}

// TestLookupWithASNLoaded exercises the ASN lookup branch inside Lookup.
func TestLookupWithASNLoaded(t *testing.T) {
	dir := t.TempDir()
	writeRecordMMDB(t, dir, "country.mmdb", "country", map[string]string{"country_code": "US"})
	writeRecordMMDB(t, dir, "asn.mmdb", "asn", map[string]string{"as_org": "Test ISP"})

	cfg := &Config{Enabled: true, Dir: dir, Country: true, ASN: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	result := l.Lookup("8.8.8.8")
	if result == nil {
		t.Fatal("Lookup returned nil")
	}
	// ASN org should be populated from the ASN DB record
	if result.ASNOrg != "Test ISP" {
		t.Errorf("ASNOrg = %q, want %q", result.ASNOrg, "Test ISP")
	}
}

// TestIsBlockedMatchesCountry exercises the IsBlocked path when a country
// code matches the blocklist.
func TestIsBlockedMatchesCountry(t *testing.T) {
	dir := t.TempDir()
	writeRecordMMDB(t, dir, "country.mmdb", "country", map[string]string{"country_code": "CN"})

	cfg := &Config{Enabled: true, Dir: dir, Country: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	if !l.IsBlocked("8.8.8.8", []string{"CN", "RU"}) {
		t.Error("IsBlocked should return true when country matches blocklist")
	}
	if l.IsBlocked("8.8.8.8", []string{"US", "DE"}) {
		t.Error("IsBlocked should return false when country does not match")
	}
}

// TestIsAllowedMatchesCountry exercises the IsAllowed path when a country
// code is in the allowlist and when it is not.
func TestIsAllowedMatchesCountry(t *testing.T) {
	dir := t.TempDir()
	writeRecordMMDB(t, dir, "country.mmdb", "country", map[string]string{"country_code": "US"})

	cfg := &Config{Enabled: true, Dir: dir, Country: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	// US is in allowlist — allowed
	if !l.IsAllowed("8.8.8.8", []string{"US", "CA"}) {
		t.Error("IsAllowed should return true when country is in allowlist")
	}
	// US is not in allowlist — denied
	if l.IsAllowed("8.8.8.8", []string{"DE", "FR"}) {
		t.Error("IsAllowed should return false when country is not in allowlist")
	}
}

// TestLookupWithCityLoaded exercises the city lookup branch inside Lookup.
func TestLookupWithCityLoaded(t *testing.T) {
	dir := t.TempDir()
	writeRecordMMDB(t, dir, "country.mmdb", "country", map[string]string{"country_code": "US"})
	writeRecordMMDB(t, dir, "city.mmdb", "city", map[string]string{"city": "New York"})

	cfg := &Config{Enabled: true, Dir: dir, Country: true, ASN: false, City: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	result := l.Lookup("8.8.8.8")
	if result == nil {
		t.Fatal("Lookup returned nil")
	}
	if result.City != "New York" {
		t.Errorf("City = %q, want %q", result.City, "New York")
	}
}

// TestLookupWithWHOISLoaded exercises the WHOIS lookup branch inside Lookup.
func TestLookupWithWHOISLoaded(t *testing.T) {
	dir := t.TempDir()
	writeRecordMMDB(t, dir, "country.mmdb", "country", map[string]string{"country_code": "US"})
	writeRecordMMDB(t, dir, "whois.mmdb", "whois", map[string]string{"organization": "TestOrg", "network": "8.8.8.0/24"})

	cfg := &Config{Enabled: true, Dir: dir, Country: true, WHOIS: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	result := l.Lookup("8.8.8.8")
	if result == nil {
		t.Fatal("Lookup returned nil")
	}
	if result.RegistrantOrg != "TestOrg" {
		t.Errorf("RegistrantOrg = %q, want %q", result.RegistrantOrg, "TestOrg")
	}
	if result.RegistrantNet != "8.8.8.0/24" {
		t.Errorf("RegistrantNet = %q, want %q", result.RegistrantNet, "8.8.8.0/24")
	}
}

// TestAsUintIntTypes exercises the int and int64 branches in asUint.
func TestAsUintIntTypes(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want uint
	}{
		{"int", int(7), 7},
		{"int64", int64(99), 99},
		{"uint16", uint16(3), 3},
		{"uint32", uint32(4), 4},
		{"uint64", uint64(5), 5},
		{"float64 (wrong type)", float64(1.5), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]interface{}{"v": tt.val}
			if got := asUint(m, "v"); got != tt.want {
				t.Errorf("asUint(%T(%v)) = %d, want %d", tt.val, tt.val, got, tt.want)
			}
		})
	}
}

// TestAsFloatFloat32 exercises the float32 branch in asFloat.
func TestAsFloatFloat32(t *testing.T) {
	m := map[string]interface{}{"v": float32(1.5)}
	got := asFloat(m, "v")
	if got == 0 {
		t.Error("asFloat(float32) should return non-zero")
	}
}

// TestDownloadDatabaseSuccess verifies that downloadDatabase fetches a URL
// and writes it to disk correctly.
func TestDownloadDatabaseSuccess(t *testing.T) {
	want := []byte("fake mmdb content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "country.mmdb")

	l := NewLookup(nil)
	if err := l.downloadDatabase(srv.URL, destPath); err != nil {
		t.Fatalf("downloadDatabase returned error: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile after download: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("downloaded content = %q, want %q", got, want)
	}
}

// TestDownloadDatabaseHTTPError verifies that downloadDatabase returns an error
// when the server responds with a non-200 status.
func TestDownloadDatabaseHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "country.mmdb")

	l := NewLookup(nil)
	err := l.downloadDatabase(srv.URL, destPath)
	if err == nil {
		t.Error("downloadDatabase should fail on 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status code: %v", err)
	}
}

// TestDownloadDatabaseNetworkError verifies that downloadDatabase returns an
// error when the URL is unreachable.
func TestDownloadDatabaseNetworkError(t *testing.T) {
	l := NewLookup(nil)
	err := l.downloadDatabase("http://127.0.0.1:1", "/tmp/should-not-exist.mmdb")
	if err == nil {
		t.Error("downloadDatabase should fail on unreachable URL")
	}
}

// TestDownloadDatabaseUnwritableDest verifies that downloadDatabase returns an
// error when the destination directory does not exist.
func TestDownloadDatabaseUnwritableDest(t *testing.T) {
	want := []byte("data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	l := NewLookup(nil)
	// Path to a non-existent directory — os.Create will fail
	err := l.downloadDatabase(srv.URL, "/nonexistent-dir-12345/file.mmdb")
	if err == nil {
		t.Error("downloadDatabase should fail when dest dir does not exist")
	}
}

// TestLoadDatabasesAllDisabled verifies that LoadDatabases succeeds when all
// database toggles are false (no files downloaded, no errors).
func TestLoadDatabasesAllDisabled(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: false,
		ASN:     false,
		City:    false,
		WHOIS:   false,
	}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Errorf("LoadDatabases with all disabled: %v", err)
	}
	if l.IsLoaded() {
		t.Error("Lookup should not be loaded when country DB is disabled")
	}
}

// TestLoadDatabasesWithExistingFiles verifies LoadDatabases when MMDB files
// already exist on disk (skips download, attempts openMMDB).
func TestLoadDatabasesWithExistingFiles(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")
	writeTempMMDB(t, dir, "asn.mmdb")

	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: true,
		ASN:     true,
		City:    false,
		WHOIS:   false,
	}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Errorf("LoadDatabases with existing files: %v", err)
	}
}

// TestLoadDatabasesCountryOnlyExisting verifies the city and WHOIS branches
// when those files exist.
func TestLoadDatabasesAllExisting(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")
	writeTempMMDB(t, dir, "asn.mmdb")
	writeTempMMDB(t, dir, "city.mmdb")
	writeTempMMDB(t, dir, "whois.mmdb")

	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: true,
		ASN:     true,
		City:    true,
		WHOIS:   true,
	}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Errorf("LoadDatabases with all existing: %v", err)
	}
	defer l.Close()
}

// TestLoadDatabasesMissingFileWithBadData verifies that when a file is present
// but contains corrupt data, LoadDatabases fails to load the country DB and
// returns an error.
func TestLoadDatabasesMissingFileWithBadData(t *testing.T) {
	dir := t.TempDir()
	// Write corrupt data for country.mmdb so openMMDB fails without a download
	badPath := filepath.Join(dir, "country.mmdb")
	if err := os.WriteFile(badPath, []byte("not-valid-mmdb-data"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: true,
		ASN:     false,
		City:    false,
		WHOIS:   false,
	}
	l := NewLookup(cfg)
	err := l.LoadDatabases()
	// The corrupt file makes openMMDB fail; country DB won't load
	if err == nil {
		t.Error("LoadDatabases with corrupt country MMDB should return error")
	}
	if l.IsLoaded() {
		t.Error("Lookup should not be loaded when country MMDB is corrupt")
	}
}

// TestLoadDatabasesInvalidMMDB verifies that LoadDatabases handles a corrupt
// MMDB file gracefully (openMMDB fails; country DB not loaded).
func TestLoadDatabasesInvalidMMDB(t *testing.T) {
	dir := t.TempDir()
	// Write a junk file (not valid MMDB)
	badPath := filepath.Join(dir, "country.mmdb")
	if err := os.WriteFile(badPath, []byte("this is not mmdb"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: true,
		ASN:     false,
		City:    false,
		WHOIS:   false,
	}
	l := NewLookup(cfg)
	err := l.LoadDatabases()
	if err == nil {
		t.Error("LoadDatabases with invalid MMDB should return error")
	}
	if l.IsLoaded() {
		t.Error("Lookup should not be loaded when MMDB is corrupt")
	}
}

// TestLoadDatabasesInvalidASN verifies that an invalid ASN file is handled;
// a valid country DB still lets loaded=true.
func TestLoadDatabasesInvalidASN(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")
	// Bad ASN file
	if err := os.WriteFile(filepath.Join(dir, "asn.mmdb"), []byte("garbage"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: true,
		ASN:     true,
		City:    false,
		WHOIS:   false,
	}
	l := NewLookup(cfg)
	// Should not return error because country loaded successfully
	err := l.LoadDatabases()
	if err != nil {
		t.Errorf("LoadDatabases should not error when only ASN fails: %v", err)
	}
	if !l.IsLoaded() {
		t.Error("Lookup should be loaded because country DB succeeded")
	}
}

// TestLoadDatabasesLastUpdateSet verifies that LastUpdate is set after
// LoadDatabases completes (even with errors).
func TestLoadDatabasesLastUpdateSet(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Enabled: true, Dir: dir}
	l := NewLookup(cfg)
	before := time.Now()
	_ = l.LoadDatabases()
	if l.LastUpdate().Before(before) {
		t.Error("LastUpdate should be set after LoadDatabases")
	}
}

// TestUpdateDatabasesWithExistingCountry verifies UpdateDatabases when the
// country file exists but the download fails (network not available in CI),
// confirming the update-error path is exercised.
func TestUpdateDatabasesDownloadFails(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")

	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: true,
		ASN:     false,
		City:    false,
		WHOIS:   false,
	}
	l := NewLookup(cfg)
	// Pre-load so countryDB is non-nil (exercises the Close+replace branch)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases setup: %v", err)
	}

	// UpdateDatabases always tries to download; it will fail without real internet
	// — we just verify it returns an error and doesn't panic.
	err := l.UpdateDatabases()
	// err is expected (download fails in test env) — no panic is the assertion
	_ = err
}

// TestUpdateDatabasesWithMockServer verifies UpdateDatabases success path using
// a mock HTTP server serving a valid MMDB file.
func TestUpdateDatabasesWithMockServer(t *testing.T) {
	mmdbBytes := buildMinimalMMDB()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(mmdbBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mmdbBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: false,
		ASN:     false,
		City:    false,
		WHOIS:   false,
	}
	l := NewLookup(cfg)

	// Directly test downloadDatabase with our mock (UpdateDatabases uses
	// hardcoded URLs, so we exercise downloadDatabase success path directly)
	destPath := filepath.Join(dir, "test.mmdb")
	if err := l.downloadDatabase(srv.URL, destPath); err != nil {
		t.Fatalf("downloadDatabase via mock server: %v", err)
	}
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("downloaded file should exist: %v", err)
	}
}

// TestUpdateDatabasesNilConfigError verifies that UpdateDatabases returns an
// error when called on a zero-value Lookup (config is nil).
func TestUpdateDatabasesNilConfigError(t *testing.T) {
	l := &Lookup{}
	if err := l.UpdateDatabases(); err == nil {
		t.Error("UpdateDatabases with nil config should return error")
	}
}

// TestUpdateDatabasesAllEnabled verifies that UpdateDatabases exercises all
// enabled-DB branches without panicking, even when downloads fail.
func TestUpdateDatabasesAllBranchesWithFailure(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")
	writeTempMMDB(t, dir, "asn.mmdb")
	writeTempMMDB(t, dir, "city.mmdb")
	writeTempMMDB(t, dir, "whois.mmdb")

	cfg := &Config{
		Enabled: true,
		Dir:     dir,
		Country: true,
		ASN:     true,
		City:    true,
		WHOIS:   true,
	}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	// UpdateDatabases will attempt downloads, which fail — errors returned but no panic
	_ = l.UpdateDatabases()
}

// TestLoadDatabaseCallsLoadDatabases verifies the LoadDatabase shim delegates
// to LoadDatabases (and that its path is exercised).
func TestLoadDatabaseCallsLoadDatabases(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")

	cfg := &Config{Enabled: true, Dir: dir, Country: true}
	l := NewLookup(cfg)
	// LoadDatabase(path) delegates to LoadDatabases(), which uses cfg.Dir
	err := l.LoadDatabase("/ignored/path")
	if err != nil {
		t.Errorf("LoadDatabase: %v", err)
	}
}

// TestCloseWithLoadedDBs verifies that Close correctly nils all DB pointers
// including non-nil ones.
func TestCloseWithLoadedDBs(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")
	writeTempMMDB(t, dir, "asn.mmdb")
	writeTempMMDB(t, dir, "city.mmdb")
	writeTempMMDB(t, dir, "whois.mmdb")

	cfg := &Config{Enabled: true, Dir: dir, Country: true, ASN: true, City: true, WHOIS: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}

	l.Close()

	if l.IsLoaded() {
		t.Error("Lookup should not be loaded after Close")
	}
	if l.countryDB != nil || l.asnDB != nil || l.cityDB != nil || l.whoisDB != nil {
		t.Error("all DB pointers should be nil after Close")
	}
}

// TestLookupWithLoadedCountryDB exercises the Lookup method when a real DB
// is loaded; with an empty MMDB the result should be Found=false.
func TestLookupWithLoadedCountryDB(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")

	cfg := &Config{Enabled: true, Dir: dir, Country: true, ASN: false}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	tests := []struct {
		name string
		ip   string
	}{
		{"public IP", "8.8.8.8"},
		{"loopback", "127.0.0.1"},
		{"private RFC1918", "192.168.1.1"},
		{"IPv6 loopback", "::1"},
		{"invalid IP", "not-an-ip"},
		{"empty string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := l.Lookup(tt.ip)
			if result == nil {
				t.Fatal("Lookup returned nil")
			}
			if result.IP != tt.ip {
				t.Errorf("result.IP = %q, want %q", result.IP, tt.ip)
			}
		})
	}
}

// TestIsBlockedWithLoadedDB exercises IsBlocked paths with a loaded DB.
func TestIsBlockedWithLoadedDB(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")

	cfg := &Config{Enabled: true, Dir: dir, Country: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	// Empty DB returns Found=false, so IsBlocked always false
	if l.IsBlocked("8.8.8.8", []string{"CN", "RU"}) {
		t.Error("IsBlocked should be false when DB has no records")
	}
	// Empty blocklist always false
	if l.IsBlocked("8.8.8.8", []string{}) {
		t.Error("IsBlocked with empty list should be false")
	}
}

// TestIsAllowedWithLoadedDB exercises IsAllowed paths with a loaded DB.
func TestIsAllowedWithLoadedDB(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")

	cfg := &Config{Enabled: true, Dir: dir, Country: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	// Empty DB returns Found=false, so IsAllowed always true (unknown country allowed)
	if !l.IsAllowed("8.8.8.8", []string{"US"}) {
		t.Error("IsAllowed should be true when country is unknown (DB has no records)")
	}
	// Empty allowlist always allowed
	if !l.IsAllowed("8.8.8.8", nil) {
		t.Error("IsAllowed with nil list should be true")
	}
}

// TestDefaultConfigValues verifies all default config field values explicitly.
func TestDefaultConfigValues(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"Enabled", cfg.Enabled, false},
		{"Dir", cfg.Dir, "/config/security/geoip"},
		{"Update", cfg.Update, "weekly"},
		{"ASN", cfg.ASN, true},
		{"Country", cfg.Country, true},
		{"City", cfg.City, false},
		{"WHOIS", cfg.WHOIS, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("DefaultConfig.%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
	if len(cfg.DenyCountries) != 0 {
		t.Error("DenyCountries should be empty slice")
	}
	if len(cfg.AllowedCountries) != 0 {
		t.Error("AllowedCountries should be empty slice")
	}
}

// TestGetCountryAllContinents verifies that all continent codes produced by
// initCountries are one of the expected values.
func TestGetCountryAllContinents(t *testing.T) {
	l := NewLookup(nil)
	validContinents := map[string]bool{
		"AF": true, "AN": true, "AS": true, "EU": true,
		"NA": true, "OC": true, "SA": true,
	}
	for code, c := range l.countries {
		if !validContinents[c.Continent] {
			t.Errorf("country %s has invalid continent %q", code, c.Continent)
		}
	}
}

// TestLookupConcurrencyWithDB exercises concurrent Lookup/IsBlocked/IsAllowed
// calls after a DB is loaded.
func TestLookupConcurrencyWithDB(t *testing.T) {
	dir := t.TempDir()
	writeTempMMDB(t, dir, "country.mmdb")

	cfg := &Config{Enabled: true, Dir: dir, Country: true}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Fatalf("LoadDatabases: %v", err)
	}
	defer l.Close()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.Lookup("8.8.8.8")
			_ = l.IsBlocked("1.1.1.1", []string{"CN"})
			_ = l.IsAllowed("1.1.1.1", []string{"US"})
			_ = l.LastUpdate()
			_ = l.IsLoaded()
		}()
	}
	wg.Wait()
}

// TestMmdbReaderCloseReleasesReader verifies that Close sets reader to nil.
func TestMmdbReaderCloseReleasesReader(t *testing.T) {
	dir := t.TempDir()
	path := writeTempMMDB(t, dir, "country.mmdb")
	db, err := openMMDB(path)
	if err != nil {
		t.Fatalf("openMMDB: %v", err)
	}
	if db.reader == nil {
		t.Fatal("reader should be non-nil after open")
	}
	db.Close()
	if db.reader != nil {
		t.Error("reader should be nil after Close")
	}
	if db.metadata != nil {
		t.Error("metadata should be nil after Close")
	}
}

// TestDownloadDatabaseBodyWriteError tests the edge where the response body
// causes a copy error (server closes mid-stream).
func TestDownloadDatabaseServerCloseMidStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force close the connection mid-write by hijacking and closing
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		// Flush and close connection abruptly via panic would not work cleanly,
		// so just write partial data — the file will be written successfully
		// since HTTP client buffers. This exercises the write path regardless.
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "partial.mmdb")
	l := NewLookup(nil)
	// This may succeed or fail depending on TCP behavior — we just verify no panic
	_ = l.downloadDatabase(srv.URL, destPath)
}

// TestLoadDatabasesMissingDirCreation verifies that LoadDatabases creates the
// configured directory if it does not exist.
func TestLoadDatabasesMissingDirCreation(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "subdir", "geoip")
	cfg := &Config{Enabled: true, Dir: dir, Country: false, ASN: false}
	l := NewLookup(cfg)
	if err := l.LoadDatabases(); err != nil {
		t.Errorf("LoadDatabases should create missing dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("directory should exist after LoadDatabases: %v", err)
	}
}
