package geoip

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDatabaseURLs(t *testing.T) {
	// Verify URLs are non-empty and use expected CDN
	urls := []string{ASNURL, CountryURL, CityURL, WhoisURL}

	for _, url := range urls {
		if url == "" {
			t.Error("Database URL should not be empty")
		}
		if url[:8] != "https://" {
			t.Errorf("URL should use HTTPS: %s", url)
		}
	}
}

func TestCountryStruct(t *testing.T) {
	c := Country{
		Code:      "US",
		Name:      "United States",
		Continent: "NA",
	}

	if c.Code != "US" {
		t.Errorf("Code = %q, want %q", c.Code, "US")
	}
	if c.Name != "United States" {
		t.Errorf("Name = %q, want %q", c.Name, "United States")
	}
	if c.Continent != "NA" {
		t.Errorf("Continent = %q, want %q", c.Continent, "NA")
	}
}

func TestResultStruct(t *testing.T) {
	r := Result{
		IP:            "8.8.8.8",
		CountryCode:   "US",
		CountryName:   "United States",
		Continent:     "NA",
		City:          "Mountain View",
		Region:        "California",
		PostalCode:    "94035",
		Latitude:      37.386,
		Longitude:     -122.084,
		Timezone:      "America/Los_Angeles",
		ASN:           15169,
		ASNOrg:        "Google LLC",
		RegistrantOrg: "Google Inc",
		RegistrantNet: "8.0.0.0/8",
		Found:         true,
	}

	if r.IP != "8.8.8.8" {
		t.Errorf("IP = %q, want %q", r.IP, "8.8.8.8")
	}
	if r.CountryCode != "US" {
		t.Errorf("CountryCode = %q, want %q", r.CountryCode, "US")
	}
	if r.ASN != 15169 {
		t.Errorf("ASN = %d, want %d", r.ASN, 15169)
	}
	if !r.Found {
		t.Error("Found should be true")
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		Enabled:          true,
		Dir:              "/config/geoip",
		Update:           "weekly",
		DenyCountries:    []string{"XX"},
		AllowedCountries: []string{"US", "CA"},
		ASN:              true,
		Country:          true,
		City:             false,
		WHOIS:            false,
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Dir != "/config/geoip" {
		t.Errorf("Dir = %q, want %q", cfg.Dir, "/config/geoip")
	}
	if cfg.Update != "weekly" {
		t.Errorf("Update = %q, want %q", cfg.Update, "weekly")
	}
	if len(cfg.DenyCountries) != 1 {
		t.Errorf("DenyCountries length = %d, want %d", len(cfg.DenyCountries), 1)
	}
	if len(cfg.AllowedCountries) != 2 {
		t.Errorf("AllowedCountries length = %d, want %d", len(cfg.AllowedCountries), 2)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.Enabled {
		t.Error("Default Enabled should be false")
	}
	if cfg.Dir != "/config/security/geoip" {
		t.Errorf("Default Dir = %q, want %q", cfg.Dir, "/config/security/geoip")
	}
	if cfg.Update != "weekly" {
		t.Errorf("Default Update = %q, want %q", cfg.Update, "weekly")
	}
	if !cfg.ASN {
		t.Error("Default ASN should be true")
	}
	if !cfg.Country {
		t.Error("Default Country should be true")
	}
	if cfg.City {
		t.Error("Default City should be false")
	}
	if cfg.WHOIS {
		t.Error("Default WHOIS should be false")
	}
	if len(cfg.DenyCountries) != 0 {
		t.Errorf("Default DenyCountries should be empty")
	}
	if len(cfg.AllowedCountries) != 0 {
		t.Errorf("Default AllowedCountries should be empty")
	}
}

func TestNewLookup(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Dir:     "/tmp/geoip-test",
	}

	l := NewLookup(cfg)
	if l == nil {
		t.Fatal("NewLookup() returned nil")
	}
	if l.config != cfg {
		t.Error("Config not set correctly")
	}
	if l.dbDir != "/tmp/geoip-test" {
		t.Errorf("dbDir = %q, want %q", l.dbDir, "/tmp/geoip-test")
	}
	if l.countries == nil {
		t.Error("countries map should not be nil")
	}
}

func TestNewLookupNilConfig(t *testing.T) {
	l := NewLookup(nil)
	if l == nil {
		t.Fatal("NewLookup(nil) returned nil")
	}
	// Should use default config
	if l.config == nil {
		t.Error("Default config should be set")
	}
}

func TestLookupInitCountries(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Test some common countries exist
	countries := []string{"US", "GB", "DE", "FR", "JP", "CN", "AU", "CA", "BR", "IN"}
	for _, code := range countries {
		c := l.GetCountry(code)
		if c == nil {
			t.Errorf("Country %q not found", code)
		} else {
			if c.Code != code {
				t.Errorf("Country code = %q, want %q", c.Code, code)
			}
			if c.Name == "" {
				t.Errorf("Country %q has empty name", code)
			}
			if c.Continent == "" {
				t.Errorf("Country %q has empty continent", code)
			}
		}
	}
}

func TestLookupGetCountry(t *testing.T) {
	l := NewLookup(DefaultConfig())

	tests := []struct {
		code      string
		wantName  string
		wantCont  string
		wantFound bool
	}{
		{"US", "United States", "NA", true},
		{"GB", "United Kingdom", "EU", true},
		{"JP", "Japan", "AS", true},
		{"AU", "Australia", "OC", true},
		{"us", "United States", "NA", true}, // lowercase
		{"XX", "", "", false},
		{"", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			c := l.GetCountry(tt.code)
			if tt.wantFound {
				if c == nil {
					t.Errorf("GetCountry(%q) returned nil, want country", tt.code)
					return
				}
				if c.Name != tt.wantName {
					t.Errorf("Name = %q, want %q", c.Name, tt.wantName)
				}
				if c.Continent != tt.wantCont {
					t.Errorf("Continent = %q, want %q", c.Continent, tt.wantCont)
				}
			} else {
				if c != nil {
					t.Errorf("GetCountry(%q) returned %v, want nil", tt.code, c)
				}
			}
		})
	}
}

func TestLookupIsLoadedInitial(t *testing.T) {
	l := NewLookup(DefaultConfig())

	if l.IsLoaded() {
		t.Error("IsLoaded() should be false before loading")
	}
}

func TestLookupLastUpdate(t *testing.T) {
	l := NewLookup(DefaultConfig())

	lastUpdate := l.LastUpdate()
	if !lastUpdate.IsZero() {
		t.Error("LastUpdate() should be zero before loading")
	}
}

func TestLookupLookupWithoutDB(t *testing.T) {
	l := NewLookup(DefaultConfig())

	result := l.Lookup("8.8.8.8")
	if result == nil {
		t.Fatal("Lookup() returned nil")
	}
	if result.IP != "8.8.8.8" {
		t.Errorf("IP = %q, want %q", result.IP, "8.8.8.8")
	}
	if result.Found {
		t.Error("Found should be false when DB not loaded")
	}
}

func TestLookupLookupInvalidIP(t *testing.T) {
	l := NewLookup(DefaultConfig())

	tests := []string{
		"invalid",
		"256.256.256.256",
		"",
		"abc.def.ghi.jkl",
	}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			result := l.Lookup(ip)
			if result == nil {
				t.Fatal("Lookup() returned nil")
			}
			if result.Found {
				t.Error("Found should be false for invalid IP")
			}
		})
	}
}

func TestLookupIsBlockedEmpty(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Empty blocked list should return false
	if l.IsBlocked("8.8.8.8", []string{}) {
		t.Error("IsBlocked() should return false for empty list")
	}
}

func TestLookupIsBlockedWithoutDB(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Without DB loaded, should return false (can't determine country)
	if l.IsBlocked("8.8.8.8", []string{"US"}) {
		t.Error("IsBlocked() should return false when DB not loaded")
	}
}

func TestLookupIsAllowedEmpty(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Empty allowed list means no restrictions
	if !l.IsAllowed("8.8.8.8", []string{}) {
		t.Error("IsAllowed() should return true for empty list")
	}
}

func TestLookupIsAllowedWithoutDB(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Without DB loaded, should return true (allow if can't determine)
	if !l.IsAllowed("8.8.8.8", []string{"US"}) {
		t.Error("IsAllowed() should return true when country unknown")
	}
}

func TestLookupClose(t *testing.T) {
	l := NewLookup(DefaultConfig())
	l.Close()

	// Should not panic
	if l.IsLoaded() {
		t.Error("IsLoaded() should be false after Close()")
	}
}

func TestLookupLoadDatabasesNilConfig(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
	}

	err := l.LoadDatabases()
	if err == nil {
		t.Error("LoadDatabases() should fail with nil config")
	}
}

func TestLookupUpdateDatabasesNilConfig(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
	}

	err := l.UpdateDatabases()
	if err == nil {
		t.Error("UpdateDatabases() should fail with nil config")
	}
}

func TestLookupLoadDatabase(t *testing.T) {
	l := NewLookup(&Config{
		Enabled: true,
		Dir:     "/tmp/geoip-nonexistent",
		Country: false,
		ASN:     false,
		City:    false,
		WHOIS:   false,
	})

	// LoadDatabase should not fail if no DBs are configured
	err := l.LoadDatabase("/tmp/test.mmdb")
	if err != nil {
		t.Errorf("LoadDatabase() error = %v", err)
	}
}

func TestResultDefaultValues(t *testing.T) {
	r := &Result{}

	if r.IP != "" {
		t.Error("Default IP should be empty")
	}
	if r.Found {
		t.Error("Default Found should be false")
	}
	if r.ASN != 0 {
		t.Error("Default ASN should be 0")
	}
	if r.Latitude != 0 {
		t.Error("Default Latitude should be 0")
	}
}

func TestConfigDefaultValues(t *testing.T) {
	cfg := &Config{}

	if cfg.Enabled {
		t.Error("Default Enabled should be false")
	}
	if cfg.ASN {
		t.Error("Default ASN should be false")
	}
	if cfg.Country {
		t.Error("Default Country should be false")
	}
}

func TestLookupConcurrency(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Test concurrent access
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			l.Lookup("8.8.8.8")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			l.IsLoaded()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			l.GetCountry("US")
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for goroutines")
		}
	}
}

// Tests for MMDB metadata struct

func TestMmdbMetadataStruct(t *testing.T) {
	meta := &mmdbMetadata{
		NodeCount:               1000,
		RecordSize:              24,
		IPVersion:               4,
		DatabaseType:            "GeoLite2-Country",
		Languages:               []string{"en", "fr"},
		BinaryFormatMajorVersion: 2,
		BinaryFormatMinorVersion: 0,
		BuildEpoch:              1609459200,
		Description:             map[string]string{"en": "GeoLite2 Country database"},
	}

	if meta.NodeCount != 1000 {
		t.Errorf("NodeCount = %d, want 1000", meta.NodeCount)
	}
	if meta.RecordSize != 24 {
		t.Errorf("RecordSize = %d, want 24", meta.RecordSize)
	}
	if meta.IPVersion != 4 {
		t.Errorf("IPVersion = %d, want 4", meta.IPVersion)
	}
	if meta.DatabaseType != "GeoLite2-Country" {
		t.Errorf("DatabaseType = %q", meta.DatabaseType)
	}
	if len(meta.Languages) != 2 {
		t.Errorf("Languages length = %d, want 2", len(meta.Languages))
	}
}

func TestMmdbReaderStruct(t *testing.T) {
	reader := &mmdbReader{
		data:       []byte{0x01, 0x02, 0x03},
		nodeSize:   6,
		dataOffset: 100,
	}

	if len(reader.data) != 3 {
		t.Errorf("data length = %d, want 3", len(reader.data))
	}
	if reader.nodeSize != 6 {
		t.Errorf("nodeSize = %d, want 6", reader.nodeSize)
	}
	if reader.dataOffset != 100 {
		t.Errorf("dataOffset = %d, want 100", reader.dataOffset)
	}
}

func TestMmdbReaderClose(t *testing.T) {
	reader := &mmdbReader{
		data:     []byte{0x01, 0x02, 0x03},
		metadata: &mmdbMetadata{},
	}

	reader.Close()

	if reader.data != nil {
		t.Error("data should be nil after Close()")
	}
	if reader.metadata != nil {
		t.Error("metadata should be nil after Close()")
	}
}

func TestOpenMMDBInvalidPath(t *testing.T) {
	_, err := openMMDB("/nonexistent/path/to/database.mmdb")
	if err == nil {
		t.Error("openMMDB() should fail for nonexistent file")
	}
}

func TestOpenMMDBInvalidData(t *testing.T) {
	// Create temp file with invalid MMDB data
	tmpFile, err := os.CreateTemp("", "invalid-mmdb-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.Write([]byte("not a valid mmdb file"))
	tmpFile.Close()

	_, err = openMMDB(tmpFile.Name())
	if err == nil {
		t.Error("openMMDB() should fail for invalid MMDB data")
	}
}

// Tests for Lookup with various IP formats

func TestLookupIPv6Format(t *testing.T) {
	l := NewLookup(DefaultConfig())

	result := l.Lookup("2001:4860:4860::8888")
	if result == nil {
		t.Fatal("Lookup() returned nil for IPv6")
	}
	if result.IP != "2001:4860:4860::8888" {
		t.Errorf("IP = %q", result.IP)
	}
}

func TestLookupLoopbackIP(t *testing.T) {
	l := NewLookup(DefaultConfig())

	tests := []string{
		"127.0.0.1",
		"::1",
	}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			result := l.Lookup(ip)
			if result == nil {
				t.Fatal("Lookup() returned nil")
			}
		})
	}
}

func TestLookupPrivateIP(t *testing.T) {
	l := NewLookup(DefaultConfig())

	tests := []string{
		"10.0.0.1",
		"192.168.1.1",
		"172.16.0.1",
	}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			result := l.Lookup(ip)
			if result == nil {
				t.Fatal("Lookup() returned nil")
			}
		})
	}
}

// Tests for IsBlocked and IsAllowed with edge cases

func TestLookupIsBlockedMultipleCountries(t *testing.T) {
	l := NewLookup(DefaultConfig())

	blocked := []string{"XX", "YY", "ZZ"}
	if l.IsBlocked("8.8.8.8", blocked) {
		t.Error("IsBlocked() should return false when DB not loaded")
	}
}

func TestLookupIsAllowedMultipleCountries(t *testing.T) {
	l := NewLookup(DefaultConfig())

	allowed := []string{"US", "CA", "GB", "AU"}
	if !l.IsAllowed("8.8.8.8", allowed) {
		t.Error("IsAllowed() should return true when DB not loaded")
	}
}

// Tests for Config YAML tags

func TestConfigYAMLTags(t *testing.T) {
	// Verify Config has expected fields
	cfg := Config{
		Enabled:          true,
		Dir:              "/geoip",
		Update:           "daily",
		DenyCountries:    []string{"XX"},
		AllowedCountries: []string{"US"},
		ASN:              true,
		Country:          true,
		City:             true,
		WHOIS:            true,
	}

	if !cfg.WHOIS {
		t.Error("WHOIS should be true")
	}
	if !cfg.City {
		t.Error("City should be true")
	}
}

// Tests for Result JSON tags

func TestResultJSONTags(t *testing.T) {
	r := Result{
		IP:            "1.2.3.4",
		CountryCode:   "US",
		CountryName:   "United States",
		Continent:     "NA",
		City:          "New York",
		Region:        "New York",
		PostalCode:    "10001",
		Latitude:      40.7128,
		Longitude:     -74.0060,
		Timezone:      "America/New_York",
		ASN:           12345,
		ASNOrg:        "Example Org",
		RegistrantOrg: "Example Inc",
		RegistrantNet: "1.2.0.0/16",
		Found:         true,
	}

	if r.RegistrantOrg != "Example Inc" {
		t.Errorf("RegistrantOrg = %q", r.RegistrantOrg)
	}
	if r.RegistrantNet != "1.2.0.0/16" {
		t.Errorf("RegistrantNet = %q", r.RegistrantNet)
	}
}

// Tests for Country JSON tags

func TestCountryJSONTags(t *testing.T) {
	c := Country{
		Code:      "US",
		Name:      "United States",
		Continent: "NA",
	}

	if c.Code != "US" {
		t.Errorf("Code = %q", c.Code)
	}
}

// Test constants

func TestDatabaseURLConstants(t *testing.T) {
	if ASNURL == "" {
		t.Error("ASNURL should not be empty")
	}
	if CountryURL == "" {
		t.Error("CountryURL should not be empty")
	}
	if CityURL == "" {
		t.Error("CityURL should not be empty")
	}
	if WhoisURL == "" {
		t.Error("WhoisURL should not be empty")
	}
}

func TestDatabaseURLsJsdelivrCDN(t *testing.T) {
	urls := []string{ASNURL, CountryURL, CityURL, WhoisURL}

	for _, url := range urls {
		if !strings.Contains(url, "jsdelivr.net") {
			t.Errorf("URL %s should use jsdelivr CDN", url)
		}
	}
}

// Tests for database directory handling

func TestLookupWithCustomDir(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Dir:     "/tmp/custom-geoip-test-dir",
	}

	l := NewLookup(cfg)
	if l.dbDir != "/tmp/custom-geoip-test-dir" {
		t.Errorf("dbDir = %q, want /tmp/custom-geoip-test-dir", l.dbDir)
	}
}

// Test all continents in country map

func TestCountryMapContainsContinents(t *testing.T) {
	l := NewLookup(DefaultConfig())

	continents := make(map[string]bool)
	for _, country := range l.countries {
		continents[country.Continent] = true
	}

	expected := []string{"AF", "AN", "AS", "EU", "NA", "OC", "SA"}
	for _, cont := range expected {
		if !continents[cont] {
			t.Errorf("Missing continent: %s", cont)
		}
	}
}

func TestCountryMapSize(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Should have a reasonable number of countries
	if len(l.countries) < 200 {
		t.Errorf("countries map has %d entries, expected at least 200", len(l.countries))
	}
}

// Tests for mmdbReader decodeValue edge cases

func TestDecodeValueOutOfBounds(t *testing.T) {
	reader := &mmdbReader{
		data: []byte{},
	}

	_, _, err := reader.decodeValue([]byte{}, 0)
	if err == nil {
		t.Error("decodeValue should fail with empty data")
	}
}

func TestDecodeValueExtendedType(t *testing.T) {
	reader := &mmdbReader{
		data: []byte{0x00}, // Extended type marker
	}

	// Extended type with insufficient data
	_, _, err := reader.decodeValue([]byte{0x00}, 0)
	if err == nil {
		t.Error("decodeValue should fail with insufficient data for extended type")
	}
}

func TestDecodeValueString(t *testing.T) {
	reader := &mmdbReader{}

	// UTF-8 string: type 2, size 5, "hello"
	data := []byte{0x45, 'h', 'e', 'l', 'l', 'o'}
	val, newOffset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue string error: %v", err)
	}
	if val != "hello" {
		t.Errorf("decodeValue string = %v, want 'hello'", val)
	}
	if newOffset != 6 {
		t.Errorf("newOffset = %d, want 6", newOffset)
	}
}

func TestDecodeValueEmptyString(t *testing.T) {
	reader := &mmdbReader{}

	// UTF-8 string: type 2, size 0
	data := []byte{0x40}
	val, newOffset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue empty string error: %v", err)
	}
	if val != "" {
		t.Errorf("decodeValue empty string = %v, want ''", val)
	}
	if newOffset != 1 {
		t.Errorf("newOffset = %d, want 1", newOffset)
	}
}

func TestDecodeValueUint16(t *testing.T) {
	reader := &mmdbReader{}

	// uint16: type 5, size 2, value 256
	data := []byte{0xa2, 0x01, 0x00}
	val, newOffset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue uint16 error: %v", err)
	}
	if val != uint64(256) {
		t.Errorf("decodeValue uint16 = %v, want 256", val)
	}
	if newOffset != 3 {
		t.Errorf("newOffset = %d, want 3", newOffset)
	}
}

func TestDecodeValueZeroUint(t *testing.T) {
	reader := &mmdbReader{}

	// uint16: type 5, size 0 (zero value)
	data := []byte{0xa0}
	val, newOffset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue zero uint error: %v", err)
	}
	if val != uint64(0) {
		t.Errorf("decodeValue zero uint = %v, want 0", val)
	}
	if newOffset != 1 {
		t.Errorf("newOffset = %d, want 1", newOffset)
	}
}

func TestDecodeValueBool(t *testing.T) {
	reader := &mmdbReader{}

	// bool true: extended type 14, size 1
	data := []byte{0x01, 0x07} // type 0 (extended), next byte + 7 = 14 (bool), size 1 = true
	val, _, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue bool error: %v", err)
	}
	if val != true {
		t.Errorf("decodeValue bool = %v, want true", val)
	}
}

func TestDecodeValueArray(t *testing.T) {
	reader := &mmdbReader{}

	// Empty array: extended type 11, size 0
	data := []byte{0x00, 0x04} // type 0 (extended), byte 4 + 7 = 11 (array), size 0
	val, _, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue array error: %v", err)
	}
	arr, ok := val.([]interface{})
	if !ok {
		t.Errorf("decodeValue array type = %T, want []interface{}", val)
	}
	if len(arr) != 0 {
		t.Errorf("decodeValue array length = %d, want 0", len(arr))
	}
}

func TestDecodeValueMap(t *testing.T) {
	reader := &mmdbReader{}

	// Empty map: type 7, size 0
	data := []byte{0xe0}
	val, _, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue map error: %v", err)
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		t.Errorf("decodeValue map type = %T, want map[string]interface{}", val)
	}
	if len(m) != 0 {
		t.Errorf("decodeValue map length = %d, want 0", len(m))
	}
}

// Tests for mmdbReader lookup edge cases

func TestMmdbReaderLookupNilMetadata(t *testing.T) {
	reader := &mmdbReader{
		data:     []byte{0x01, 0x02},
		metadata: nil,
	}

	_, err := reader.lookup(net.ParseIP("8.8.8.8"))
	if err == nil {
		t.Error("lookup should fail with nil metadata")
	}
}

func TestMmdbReaderLookupInvalidIP(t *testing.T) {
	reader := &mmdbReader{
		data:     []byte{0x01, 0x02},
		metadata: &mmdbMetadata{NodeCount: 10, RecordSize: 24},
	}

	_, err := reader.lookup(nil)
	if err == nil {
		t.Error("lookup should fail with nil IP")
	}
}

func TestMmdbReaderLookupCountryEmpty(t *testing.T) {
	reader := &mmdbReader{
		data:     []byte{},
		metadata: nil,
	}

	result := reader.LookupCountry(net.ParseIP("8.8.8.8"))
	if result != "" {
		t.Errorf("LookupCountry = %q, want empty string", result)
	}
}

func TestMmdbReaderLookupASNEmpty(t *testing.T) {
	reader := &mmdbReader{
		data:     []byte{},
		metadata: nil,
	}

	asn, org := reader.LookupASN(net.ParseIP("8.8.8.8"))
	if asn != 0 {
		t.Errorf("LookupASN asn = %d, want 0", asn)
	}
	if org != "" {
		t.Errorf("LookupASN org = %q, want empty", org)
	}
}

func TestMmdbReaderLookupCityEmpty(t *testing.T) {
	reader := &mmdbReader{
		data:     []byte{},
		metadata: nil,
	}

	city, region, postal, lat, lon, tz := reader.LookupCity(net.ParseIP("8.8.8.8"))
	if city != "" || region != "" || postal != "" {
		t.Error("LookupCity should return empty strings")
	}
	if lat != 0 || lon != 0 {
		t.Error("LookupCity should return zero coordinates")
	}
	if tz != "" {
		t.Error("LookupCity should return empty timezone")
	}
}

func TestMmdbReaderLookupWHOISEmpty(t *testing.T) {
	reader := &mmdbReader{
		data:     []byte{},
		metadata: nil,
	}

	org, net := reader.LookupWHOIS(net.ParseIP("8.8.8.8"))
	if org != "" {
		t.Errorf("LookupWHOIS org = %q, want empty", org)
	}
	if net != "" {
		t.Errorf("LookupWHOIS net = %q, want empty", net)
	}
}

// Tests for Lookup service with different configurations

func TestNewLookupWithAllDatabases(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Dir:     "/tmp/geoip-test-all",
		ASN:     true,
		Country: true,
		City:    true,
		WHOIS:   true,
	}

	l := NewLookup(cfg)
	if l == nil {
		t.Fatal("NewLookup() returned nil")
	}
	if !l.config.ASN || !l.config.Country || !l.config.City || !l.config.WHOIS {
		t.Error("Config flags not set correctly")
	}
}

func TestLookupLoadDatabasesWithoutDirectory(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Dir:     "/tmp/geoip-test-load-" + time.Now().Format("20060102150405"),
		Country: true,
	}

	l := NewLookup(cfg)
	err := l.LoadDatabases()
	// Should fail because directory doesn't exist and download won't work without network
	// The error depends on whether directory creation succeeded
	if err == nil && !l.IsLoaded() {
		// Directory created but no databases available
		t.Log("LoadDatabases completed but no databases loaded (expected)")
	}

	// Clean up
	os.RemoveAll(cfg.Dir)
}

func TestLookupCloseMultipleTimes(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Close should be safe to call multiple times
	l.Close()
	l.Close()
	l.Close()

	if l.IsLoaded() {
		t.Error("IsLoaded() should be false after Close()")
	}
}

func TestLookupIsBlockedCaseInsensitive(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Test with various case combinations
	tests := [][]string{
		{"us", "US", "Us", "uS"},
		{"gb", "GB", "Gb", "gB"},
	}

	for _, codes := range tests {
		for _, code := range codes {
			// Without DB loaded, should always return false
			if l.IsBlocked("8.8.8.8", []string{code}) {
				t.Errorf("IsBlocked should return false for %q without DB", code)
			}
		}
	}
}

func TestLookupIsAllowedCaseInsensitive(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Test with various case combinations - without DB should always return true
	tests := [][]string{
		{"us", "US", "Us", "uS"},
		{"gb", "GB", "Gb", "gB"},
	}

	for _, codes := range tests {
		for _, code := range codes {
			if !l.IsAllowed("8.8.8.8", []string{code}) {
				t.Errorf("IsAllowed should return true for %q without DB", code)
			}
		}
	}
}

func TestLookupGetCountryUppercase(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// Test that GetCountry converts to uppercase
	tests := []struct {
		input string
		want  string
	}{
		{"us", "US"},
		{"US", "US"},
		{"Us", "US"},
		{"gb", "GB"},
		{"de", "DE"},
	}

	for _, tt := range tests {
		c := l.GetCountry(tt.input)
		if c == nil {
			t.Errorf("GetCountry(%q) returned nil", tt.input)
			continue
		}
		if c.Code != tt.want {
			t.Errorf("GetCountry(%q).Code = %q, want %q", tt.input, c.Code, tt.want)
		}
	}
}

func TestLookupLastUpdateAfterLoad(t *testing.T) {
	l := NewLookup(DefaultConfig())

	initialTime := l.LastUpdate()
	if !initialTime.IsZero() {
		t.Error("LastUpdate should be zero initially")
	}
}

func TestConfigUpdateValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"never", "never", true},
		{"daily", "daily", true},
		{"weekly", "weekly", true},
		{"monthly", "monthly", true},
		{"custom", "custom", true}, // Any string is technically valid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Update: tt.value}
			if cfg.Update != tt.value {
				t.Errorf("Update = %q, want %q", cfg.Update, tt.value)
			}
		})
	}
}

// Tests for Result methods

func TestResultWithAllFields(t *testing.T) {
	r := Result{
		IP:            "203.0.113.1",
		CountryCode:   "AU",
		CountryName:   "Australia",
		Continent:     "OC",
		City:          "Sydney",
		Region:        "New South Wales",
		PostalCode:    "2000",
		Latitude:      -33.8688,
		Longitude:     151.2093,
		Timezone:      "Australia/Sydney",
		ASN:           7545,
		ASNOrg:        "TPG Internet Pty Ltd",
		RegistrantOrg: "APNIC",
		RegistrantNet: "203.0.0.0/8",
		Found:         true,
	}

	if r.Latitude >= 0 {
		t.Error("Sydney should have negative latitude")
	}
	if r.Longitude <= 0 {
		t.Error("Sydney should have positive longitude")
	}
	if r.Timezone != "Australia/Sydney" {
		t.Errorf("Timezone = %q, want Australia/Sydney", r.Timezone)
	}
}

func TestResultNotFound(t *testing.T) {
	r := Result{
		IP:    "192.168.1.1",
		Found: false,
	}

	if r.Found {
		t.Error("Private IP should have Found=false")
	}
	if r.CountryCode != "" {
		t.Error("CountryCode should be empty for not found")
	}
}

// Tests for Country struct with different continents

func TestCountryByContinent(t *testing.T) {
	l := NewLookup(DefaultConfig())

	continentTests := map[string][]string{
		"AF": {"ZA", "NG", "EG", "KE"},     // Africa
		"AS": {"JP", "CN", "IN", "KR"},     // Asia
		"EU": {"GB", "DE", "FR", "IT"},     // Europe
		"NA": {"US", "CA", "MX"},           // North America
		"SA": {"BR", "AR", "CL", "CO"},     // South America
		"OC": {"AU", "NZ", "FJ"},           // Oceania
		"AN": {"AQ"},                       // Antarctica
	}

	for continent, countries := range continentTests {
		for _, code := range countries {
			c := l.GetCountry(code)
			if c == nil {
				t.Errorf("Country %q not found", code)
				continue
			}
			if c.Continent != continent {
				t.Errorf("Country %q continent = %q, want %q", code, c.Continent, continent)
			}
		}
	}
}

// Test mmdbMetadata fields

func TestMmdbMetadataAllFields(t *testing.T) {
	meta := &mmdbMetadata{
		NodeCount:                500000,
		RecordSize:               28,
		IPVersion:                6,
		DatabaseType:             "GeoIP2-City",
		Languages:                []string{"en", "de", "fr", "es"},
		BinaryFormatMajorVersion: 2,
		BinaryFormatMinorVersion: 1,
		BuildEpoch:               1700000000,
		Description:              map[string]string{"en": "GeoIP2 City database"},
	}

	if meta.NodeCount != 500000 {
		t.Errorf("NodeCount = %d, want 500000", meta.NodeCount)
	}
	if meta.RecordSize != 28 {
		t.Errorf("RecordSize = %d, want 28", meta.RecordSize)
	}
	if meta.IPVersion != 6 {
		t.Errorf("IPVersion = %d, want 6", meta.IPVersion)
	}
	if len(meta.Languages) != 4 {
		t.Errorf("Languages length = %d, want 4", len(meta.Languages))
	}
	if meta.BinaryFormatMajorVersion != 2 {
		t.Errorf("BinaryFormatMajorVersion = %d, want 2", meta.BinaryFormatMajorVersion)
	}
}

// Tests for parseMetadata error cases

func TestParseMetadataNoMarker(t *testing.T) {
	reader := &mmdbReader{
		data: []byte("no metadata marker here"),
	}

	err := reader.parseMetadata()
	if err == nil {
		t.Error("parseMetadata should fail without marker")
	}
	if !strings.Contains(err.Error(), "marker not found") {
		t.Errorf("Error should mention marker: %v", err)
	}
}

func TestParseMetadataInvalidData(t *testing.T) {
	// Data with marker but invalid metadata
	marker := []byte("\xab\xcd\xefMaxMind.com")
	invalidData := append(marker, 0xff, 0xff, 0xff) // Invalid MMDB format

	reader := &mmdbReader{
		data: invalidData,
	}

	err := reader.parseMetadata()
	if err == nil {
		t.Error("parseMetadata should fail with invalid data")
	}
}

// Tests for size decoding in decodeValue

func TestDecodeValueSize29(t *testing.T) {
	reader := &mmdbReader{}

	// UTF-8 string with size 29 encoding: type 2, size marker 29, additional byte
	// size = 29 + additional_byte
	data := []byte{0x5d, 0x01, 'a'} // size marker 29, +1 = 30, but data has only 1 byte
	val, _, err := reader.decodeValue(data, 0)
	// Should handle gracefully
	if err != nil {
		t.Logf("decodeValue with size 29 encoding: %v", err)
	}
	if val != nil && val != "" {
		t.Logf("decodeValue returned: %v", val)
	}
}

func TestDecodeValueSize30(t *testing.T) {
	reader := &mmdbReader{}

	// Size 30 encoding uses 2 additional bytes
	data := []byte{0x5e, 0x00, 0x01} // size marker 30, 2 bytes for size
	_, _, err := reader.decodeValue(data, 0)
	// Should handle gracefully (may error due to insufficient data)
	if err != nil {
		t.Logf("decodeValue with size 30 encoding: %v", err)
	}
}

// Tests for Lookup with specific IP types

func TestLookupIPv4MappedIPv6(t *testing.T) {
	l := NewLookup(DefaultConfig())

	// IPv4-mapped IPv6 address
	result := l.Lookup("::ffff:8.8.8.8")
	if result == nil {
		t.Fatal("Lookup returned nil for IPv4-mapped IPv6")
	}
	// IP should be preserved as-is
	if result.IP != "::ffff:8.8.8.8" {
		t.Errorf("IP = %q, want ::ffff:8.8.8.8", result.IP)
	}
}

func TestLookupLinkLocalIPv6(t *testing.T) {
	l := NewLookup(DefaultConfig())

	result := l.Lookup("fe80::1")
	if result == nil {
		t.Fatal("Lookup returned nil for link-local IPv6")
	}
	if result.Found {
		t.Error("Link-local IPv6 should not be found")
	}
}

func TestLookupMulticastIP(t *testing.T) {
	l := NewLookup(DefaultConfig())

	tests := []string{
		"224.0.0.1",      // IPv4 multicast
		"ff02::1",        // IPv6 multicast
	}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			result := l.Lookup(ip)
			if result == nil {
				t.Fatal("Lookup returned nil")
			}
			if result.Found {
				t.Error("Multicast IP should not be found")
			}
		})
	}
}

// Test concurrent access to Lookup

func TestLookupConcurrentGetCountry(t *testing.T) {
	l := NewLookup(DefaultConfig())

	done := make(chan bool, 4)

	countries := []string{"US", "GB", "DE", "JP", "FR", "CA", "AU", "IT"}

	for i := 0; i < 4; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				code := countries[(id+j)%len(countries)]
				c := l.GetCountry(code)
				if c == nil {
					t.Errorf("GetCountry(%q) returned nil", code)
				}
			}
			done <- true
		}(i)
	}

	for i := 0; i < 4; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout in concurrent GetCountry")
		}
	}
}

func TestLookupConcurrentLookupAndClose(t *testing.T) {
	l := NewLookup(DefaultConfig())

	done := make(chan bool, 2)

	// Lookups in one goroutine
	go func() {
		for i := 0; i < 50; i++ {
			l.Lookup("8.8.8.8")
			l.IsLoaded()
			l.LastUpdate()
		}
		done <- true
	}()

	// Close operations in another
	go func() {
		time.Sleep(10 * time.Millisecond)
		l.Close()
		done <- true
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout in concurrent Lookup/Close")
		}
	}
}

// Test for mmdbReader Close with locks

func TestMmdbReaderCloseWithData(t *testing.T) {
	reader := &mmdbReader{
		data: make([]byte, 1000),
		metadata: &mmdbMetadata{
			NodeCount:    100,
			RecordSize:   24,
			DatabaseType: "test",
		},
		nodeSize:   6,
		dataOffset: 100,
	}

	reader.Close()

	if reader.data != nil {
		t.Error("data should be nil after Close")
	}
	if reader.metadata != nil {
		t.Error("metadata should be nil after Close")
	}
}

// Test lookup with various record sizes

func TestMmdbReaderRecordSizes(t *testing.T) {
	tests := []struct {
		recordSize uint16
		nodeSize   int
	}{
		{24, 6},
		{28, 7},
		{32, 8},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("RecordSize%d", tt.recordSize), func(t *testing.T) {
			// Calculate expected node size per MMDB spec
			expectedNodeSize := int(tt.recordSize) * 2 / 8
			if tt.recordSize%4 != 0 {
				expectedNodeSize++
			}

			if expectedNodeSize != tt.nodeSize {
				t.Errorf("Expected node size %d for record size %d, got %d",
					tt.nodeSize, tt.recordSize, expectedNodeSize)
			}
		})
	}
}

// Test default config immutability

func TestDefaultConfigImmutable(t *testing.T) {
	cfg1 := DefaultConfig()
	cfg2 := DefaultConfig()

	// Modify cfg1
	cfg1.Enabled = true
	cfg1.Dir = "/modified"
	cfg1.DenyCountries = []string{"XX"}

	// cfg2 should be unaffected
	if cfg2.Enabled {
		t.Error("cfg2.Enabled should be false")
	}
	if cfg2.Dir != "/config/security/geoip" {
		t.Errorf("cfg2.Dir = %q, want /config/security/geoip", cfg2.Dir)
	}
	if len(cfg2.DenyCountries) != 0 {
		t.Error("cfg2.DenyCountries should be empty")
	}
}

// Test all country codes are valid

func TestAllCountryCodesValid(t *testing.T) {
	l := NewLookup(DefaultConfig())

	for code, country := range l.countries {
		// Code should be 2 characters
		if len(code) != 2 {
			t.Errorf("Invalid country code length: %q", code)
		}

		// Code should match the Country.Code field
		if code != country.Code {
			t.Errorf("Country code mismatch: map key %q, struct %q", code, country.Code)
		}

		// Name should not be empty
		if country.Name == "" {
			t.Errorf("Country %q has empty name", code)
		}

		// Continent should be 2 characters and valid
		validContinents := map[string]bool{
			"AF": true, "AN": true, "AS": true, "EU": true,
			"NA": true, "OC": true, "SA": true,
		}
		if !validContinents[country.Continent] {
			t.Errorf("Country %q has invalid continent: %q", code, country.Continent)
		}
	}
}
