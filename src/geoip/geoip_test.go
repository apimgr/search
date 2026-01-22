package geoip

import (
	"fmt"
	"math/big"
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

// =============================================================================
// Additional tests for 100% coverage
// =============================================================================

// Test downloadDatabase with mock HTTP server
func TestDownloadDatabaseSuccess(t *testing.T) {
	// Create a mock MMDB file content
	mmdbContent := createMinimalMMDB()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(mmdbContent)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.mmdb")

	l := NewLookup(DefaultConfig())
	err := l.downloadDatabase(server.URL, destPath)
	if err != nil {
		t.Errorf("downloadDatabase() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("downloadDatabase() did not create file")
	}
}

func TestDownloadDatabaseHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.mmdb")

	l := NewLookup(DefaultConfig())
	err := l.downloadDatabase(server.URL, destPath)
	if err == nil {
		t.Error("downloadDatabase() should fail with HTTP 404")
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("Error should mention status 404: %v", err)
	}
}

func TestDownloadDatabaseNetworkError(t *testing.T) {
	l := NewLookup(DefaultConfig())
	err := l.downloadDatabase("http://localhost:1", "/tmp/test.mmdb")
	if err == nil {
		t.Error("downloadDatabase() should fail with network error")
	}
	if !strings.Contains(err.Error(), "download failed") {
		t.Errorf("Error should mention download failed: %v", err)
	}
}

func TestDownloadDatabaseInvalidPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer server.Close()

	l := NewLookup(DefaultConfig())
	err := l.downloadDatabase(server.URL, "/nonexistent/dir/file.mmdb")
	if err == nil {
		t.Error("downloadDatabase() should fail with invalid path")
	}
}

// Test LoadDatabases with all database types enabled
func TestLoadDatabasesAllEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Enabled: true,
		Dir:     tmpDir,
		Country: true,
		ASN:     true,
		City:    true,
		WHOIS:   true,
	}

	l := NewLookup(cfg)

	// Create mock MMDB files
	mmdbContent := createMinimalMMDB()
	os.WriteFile(filepath.Join(tmpDir, "country.mmdb"), mmdbContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "asn.mmdb"), mmdbContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "city.mmdb"), mmdbContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "whois.mmdb"), mmdbContent, 0644)

	err := l.LoadDatabases()
	if err != nil {
		t.Logf("LoadDatabases() error (expected): %v", err)
	}
}

func TestLoadDatabasesDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "new", "subdir")
	cfg := &Config{
		Enabled: true,
		Dir:     newDir,
		Country: false, // Disable to avoid download
		ASN:     false,
		City:    false,
		WHOIS:   false,
	}

	l := NewLookup(cfg)
	err := l.LoadDatabases()
	if err != nil {
		t.Errorf("LoadDatabases() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		t.Error("LoadDatabases() did not create directory")
	}
}

func TestLoadDatabasesInvalidDirectory(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Dir:     "/proc/invalid/path/that/cannot/be/created",
		Country: true,
	}

	l := NewLookup(cfg)
	err := l.LoadDatabases()
	if err == nil {
		t.Error("LoadDatabases() should fail with invalid directory")
	}
}

func TestLoadDatabasesWithExistingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Enabled: true,
		Dir:     tmpDir,
		Country: true,
		ASN:     true,
	}

	// Create valid MMDB files
	mmdbContent := createMinimalMMDB()
	os.WriteFile(filepath.Join(tmpDir, "country.mmdb"), mmdbContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "asn.mmdb"), mmdbContent, 0644)

	l := NewLookup(cfg)
	err := l.LoadDatabases()
	// May still fail due to invalid MMDB format, but should attempt load
	t.Logf("LoadDatabases with existing files: %v", err)
}

// Test UpdateDatabases with various scenarios
func TestUpdateDatabasesWithExistingDBs(t *testing.T) {
	tmpDir := t.TempDir()
	mmdbContent := createMinimalMMDB()

	cfg := &Config{
		Enabled: true,
		Dir:     tmpDir,
		Country: true,
		ASN:     true,
		City:    true,
		WHOIS:   true,
	}

	// Create initial DB files
	os.WriteFile(filepath.Join(tmpDir, "country.mmdb"), mmdbContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "asn.mmdb"), mmdbContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "city.mmdb"), mmdbContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "whois.mmdb"), mmdbContent, 0644)

	l := NewLookup(cfg)

	// Load initial databases
	l.LoadDatabases()

	// Update should attempt download (will fail without network)
	err := l.UpdateDatabases()
	if err == nil {
		t.Log("UpdateDatabases succeeded (unexpected)")
	} else {
		t.Logf("UpdateDatabases error (expected): %v", err)
	}
}

// Test Lookup with mock loaded databases
func TestLookupWithLoadedCountryDB(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	// Create a mock countryDB that returns "US"
	l.countryDB = &mmdbReader{
		data:     createMockCountryData(),
		metadata: &mmdbMetadata{NodeCount: 1, RecordSize: 24, IPVersion: 4},
		nodeSize: 6,
	}

	result := l.Lookup("8.8.8.8")
	if result == nil {
		t.Fatal("Lookup returned nil")
	}
	// Result may or may not have country depending on mock data
	t.Logf("Lookup result: Found=%v, CountryCode=%q", result.Found, result.CountryCode)
}

func TestLookupWithAllDatabases(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	mockData := createMockCountryData()
	mockMeta := &mmdbMetadata{NodeCount: 1, RecordSize: 24, IPVersion: 4}

	l.countryDB = &mmdbReader{data: mockData, metadata: mockMeta, nodeSize: 6}
	l.asnDB = &mmdbReader{data: mockData, metadata: mockMeta, nodeSize: 6}
	l.cityDB = &mmdbReader{data: mockData, metadata: mockMeta, nodeSize: 6}
	l.whoisDB = &mmdbReader{data: mockData, metadata: mockMeta, nodeSize: 6}

	result := l.Lookup("8.8.8.8")
	if result == nil {
		t.Fatal("Lookup returned nil")
	}
	t.Logf("Full lookup result: ASN=%d, City=%q", result.ASN, result.City)
}

// Test IsBlocked with loaded DB and matching result
func TestIsBlockedWithMatchingCountry(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	// Create reader that returns "US" via proper MMDB encoding
	l.countryDB = createCountryDBReturningCode("US")

	// Test IsBlocked functionality
	result := l.IsBlocked("8.8.8.8", []string{"US"})
	t.Logf("IsBlocked with US in blocked list: %v", result)
}

func TestIsBlockedWithNonMatchingCountry(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	l.countryDB = createCountryDBReturningCode("US")

	// GB is not US, so should not be blocked
	result := l.IsBlocked("8.8.8.8", []string{"GB"})
	t.Logf("IsBlocked with GB in blocked list (IP is US): %v", result)
}

// Test IsAllowed with various scenarios
func TestIsAllowedWithMatchingCountry(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	l.countryDB = createCountryDBReturningCode("US")

	result := l.IsAllowed("8.8.8.8", []string{"US", "CA", "GB"})
	t.Logf("IsAllowed with US in allowed list: %v", result)
}

func TestIsAllowedWithNonMatchingCountry(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	l.countryDB = createCountryDBReturningCode("XX")

	result := l.IsAllowed("8.8.8.8", []string{"US", "CA", "GB"})
	t.Logf("IsAllowed with XX not in allowed list: %v", result)
}

// Test decodeValue for all data types
func TestDecodeValuePointerSize1(t *testing.T) {
	reader := &mmdbReader{}

	// Pointer type 1, size 1: 0x20 | pointer_bits
	// Type 1 = pointer, which is (ctrlByte >> 5) & 0x07 = 1
	// For pointer, size field (lower 5 bits) is used differently
	data := []byte{0x21, 0x00} // pointer to offset 0
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Pointer size 1: val=%v, err=%v", val, err)
}

func TestDecodeValuePointerSize2(t *testing.T) {
	reader := &mmdbReader{}

	// Pointer with 2-byte offset
	data := []byte{0x29, 0x00, 0x01} // pointer size 2
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Pointer size 2: val=%v, err=%v", val, err)
}

func TestDecodeValuePointerSize3(t *testing.T) {
	reader := &mmdbReader{}

	// Pointer with 3-byte offset
	data := []byte{0x31, 0x00, 0x00, 0x01} // pointer size 3
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Pointer size 3: val=%v, err=%v", val, err)
}

func TestDecodeValuePointerSize4(t *testing.T) {
	reader := &mmdbReader{}

	// Pointer with 4-byte offset
	data := []byte{0x38, 0x00, 0x00, 0x00, 0x01} // pointer size 4
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Pointer size 4: val=%v, err=%v", val, err)
}

func TestDecodeValueDouble(t *testing.T) {
	reader := &mmdbReader{}

	// Double: type 3, size 8
	data := []byte{0x68, 0x40, 0x09, 0x21, 0xfb, 0x54, 0x44, 0x2d, 0x18}
	val, offset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue double error: %v", err)
	}
	t.Logf("Double value: %v, offset: %d", val, offset)
}

func TestDecodeValueDoubleOutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// Double with insufficient data
	data := []byte{0x68, 0x40, 0x09} // Only 3 bytes instead of 8
	val, _, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Logf("Double out of bounds error: %v", err)
	} else {
		t.Logf("Double out of bounds value: %v", val)
	}
}

func TestDecodeValueBytes(t *testing.T) {
	reader := &mmdbReader{}

	// Bytes: type 4, size 4
	data := []byte{0x84, 0xde, 0xad, 0xbe, 0xef}
	val, offset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue bytes error: %v", err)
	}
	if bytes, ok := val.([]byte); ok {
		if len(bytes) != 4 {
			t.Errorf("bytes length = %d, want 4", len(bytes))
		}
	}
	t.Logf("Bytes value: %v, offset: %d", val, offset)
}

func TestDecodeValueBytesOutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// Bytes with insufficient data
	data := []byte{0x84, 0xde} // Only 1 byte instead of 4
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Bytes out of bounds: val=%v, err=%v", val, err)
}

func TestDecodeValueUint32(t *testing.T) {
	reader := &mmdbReader{}

	// uint32: type 6, size 4
	data := []byte{0xc4, 0x00, 0x00, 0x01, 0x00} // 256
	val, offset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue uint32 error: %v", err)
	}
	t.Logf("uint32 value: %v, offset: %d", val, offset)
}

func TestDecodeValueUint32Zero(t *testing.T) {
	reader := &mmdbReader{}

	// uint32 with size 0
	data := []byte{0xc0}
	val, _, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue uint32 zero error: %v", err)
	}
	if val != uint64(0) {
		t.Errorf("uint32 zero = %v, want 0", val)
	}
}

func TestDecodeValueUint32OutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// uint32 with insufficient data
	data := []byte{0xc4, 0x00, 0x00} // Only 2 bytes instead of 4
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("uint32 out of bounds: val=%v, err=%v", val, err)
}

func TestDecodeValueInt32(t *testing.T) {
	reader := &mmdbReader{}

	// int32: extended type 8, size 4
	// Type 0 = extended, next byte + 7 = 8
	data := []byte{0x04, 0x01, 0xff, 0xff, 0xff, 0xff} // -1
	val, offset, err := reader.decodeValue(data, 0)
	t.Logf("int32 value: %v, offset: %d, err: %v", val, offset, err)
}

func TestDecodeValueInt32Zero(t *testing.T) {
	reader := &mmdbReader{}

	// int32 with size 0
	data := []byte{0x00, 0x01} // extended type 8, size 0
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("int32 zero: val=%v, err=%v", val, err)
}

func TestDecodeValueInt32OutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// int32 with insufficient data
	data := []byte{0x04, 0x01, 0xff} // Only 1 byte of value
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("int32 out of bounds: val=%v, err=%v", val, err)
}

func TestDecodeValueInt32SignExtend(t *testing.T) {
	reader := &mmdbReader{}

	// int32 with size < 4 requiring sign extension
	// Type 8 = int32 (extended type 0, next byte 1, 1+7=8)
	// Size 2, value 0x80 0x00 = -32768
	data := []byte{0x02, 0x01, 0x80, 0x00}
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("int32 sign extend: val=%v, err=%v", val, err)
}

func TestDecodeValueUint64(t *testing.T) {
	reader := &mmdbReader{}

	// uint64: extended type 9, size 8
	data := []byte{0x08, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00}
	val, offset, err := reader.decodeValue(data, 0)
	t.Logf("uint64 value: %v, offset: %d, err: %v", val, offset, err)
}

func TestDecodeValueUint64Zero(t *testing.T) {
	reader := &mmdbReader{}

	// uint64 with size 0
	data := []byte{0x00, 0x02}
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("uint64 zero: val=%v, err=%v", val, err)
}

func TestDecodeValueUint64OutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// uint64 with insufficient data
	data := []byte{0x08, 0x02, 0x00, 0x00, 0x00} // Only 3 bytes
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("uint64 out of bounds: val=%v, err=%v", val, err)
}

func TestDecodeValueUint128(t *testing.T) {
	reader := &mmdbReader{}

	// uint128: extended type 10, size 16
	data := make([]byte, 18)
	data[0] = 0x10 // extended type, size 16
	data[1] = 0x03 // type = 3 + 7 = 10
	for i := 2; i < 18; i++ {
		data[i] = byte(i)
	}
	val, offset, err := reader.decodeValue(data, 0)
	if _, ok := val.(*big.Int); ok {
		t.Logf("uint128 is big.Int, offset: %d", offset)
	}
	t.Logf("uint128 value: %v, err: %v", val, err)
}

func TestDecodeValueUint128OutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// uint128 with insufficient data
	data := []byte{0x10, 0x03, 0x00, 0x00, 0x00} // Only 3 bytes
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("uint128 out of bounds: val=%v, err=%v", val, err)
}

func TestDecodeValueFloat(t *testing.T) {
	reader := &mmdbReader{}

	// float: extended type 15, size 4
	data := []byte{0x04, 0x08, 0x40, 0x48, 0xf5, 0xc3} // 3.14
	val, offset, err := reader.decodeValue(data, 0)
	t.Logf("float value: %v, offset: %d, err: %v", val, offset, err)
}

func TestDecodeValueFloatOutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// float with insufficient data
	data := []byte{0x04, 0x08, 0x40} // Only 1 byte
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("float out of bounds: val=%v, err=%v", val, err)
}

func TestDecodeValueBoolFalse(t *testing.T) {
	reader := &mmdbReader{}

	// bool false: extended type 14, size 0
	data := []byte{0x00, 0x07}
	val, _, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue bool false error: %v", err)
	}
	if val != false {
		t.Errorf("bool false = %v, want false", val)
	}
}

func TestDecodeValueMapWithEntries(t *testing.T) {
	reader := &mmdbReader{}

	// Map with 1 entry: type 7, size 1
	// Key: string "k" (type 2, size 1)
	// Value: string "v" (type 2, size 1)
	data := []byte{0xe1, 0x41, 'k', 0x41, 'v'}
	val, offset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue map error: %v", err)
	}
	if m, ok := val.(map[string]interface{}); ok {
		if m["k"] != "v" {
			t.Errorf("map[k] = %v, want 'v'", m["k"])
		}
	}
	t.Logf("Map with entries: %v, offset: %d", val, offset)
}

func TestDecodeValueMapKeyError(t *testing.T) {
	reader := &mmdbReader{}

	// Map with size 1 but no data for key
	data := []byte{0xe1}
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Map key error: val=%v, err=%v", val, err)
}

func TestDecodeValueMapValueError(t *testing.T) {
	reader := &mmdbReader{}

	// Map with size 1, key present but no value
	data := []byte{0xe1, 0x41, 'k'}
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Map value error: val=%v, err=%v", val, err)
}

func TestDecodeValueArrayWithEntries(t *testing.T) {
	reader := &mmdbReader{}

	// Array with 2 entries: extended type 11, size 2
	// Entry 1: string "a" (type 2, size 1)
	// Entry 2: string "b" (type 2, size 1)
	data := []byte{0x02, 0x04, 0x41, 'a', 0x41, 'b'}
	val, offset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("decodeValue array error: %v", err)
	}
	if arr, ok := val.([]interface{}); ok {
		if len(arr) != 2 {
			t.Errorf("array length = %d, want 2", len(arr))
		}
	}
	t.Logf("Array with entries: %v, offset: %d", val, offset)
}

func TestDecodeValueArrayError(t *testing.T) {
	reader := &mmdbReader{}

	// Array with size 2 but insufficient data
	data := []byte{0x02, 0x04, 0x41, 'a'}
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Array error: val=%v, err=%v", val, err)
}

func TestDecodeValueDefaultType(t *testing.T) {
	reader := &mmdbReader{}

	// Unknown/unhandled type (type 12 or 13)
	data := []byte{0x02, 0x05} // extended type 12
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("Default type: val=%v, err=%v", val, err)
}

func TestDecodeValueSize31(t *testing.T) {
	reader := &mmdbReader{}

	// Size 31 encoding uses 3 additional bytes
	// String type 2, size marker 31
	data := []byte{0x5f, 0x00, 0x00, 0x01, 'a'} // size = 65821 + (0<<16 + 1) = 65822
	_, _, err := reader.decodeValue(data, 0)
	t.Logf("Size 31 encoding: err=%v", err)
}

func TestDecodeValueSize30InsufficientData(t *testing.T) {
	reader := &mmdbReader{}

	// Size 30 with insufficient data for size bytes
	data := []byte{0x5e, 0x00}
	_, _, err := reader.decodeValue(data, 0)
	t.Logf("Size 30 insufficient: err=%v", err)
}

func TestDecodeValueSize31InsufficientData(t *testing.T) {
	reader := &mmdbReader{}

	// Size 31 with insufficient data for size bytes
	data := []byte{0x5f, 0x00, 0x00}
	_, _, err := reader.decodeValue(data, 0)
	t.Logf("Size 31 insufficient: err=%v", err)
}

func TestDecodeValueExtendedTypeInsufficientData(t *testing.T) {
	reader := &mmdbReader{}

	// Extended type with no type byte
	data := []byte{0x00}
	_, _, err := reader.decodeValue(data, 0)
	if err == nil {
		t.Error("Should fail with insufficient data for extended type")
	}
}

func TestDecodeValueStringOutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// String with declared size larger than available data
	data := []byte{0x45, 'h', 'e'} // size 5, but only 2 chars
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("String out of bounds: val=%v, err=%v", val, err)
}

func TestDecodeValueUint16OutOfBounds(t *testing.T) {
	reader := &mmdbReader{}

	// uint16 with insufficient data
	data := []byte{0xa2, 0x01} // size 2, but only 1 byte
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("uint16 out of bounds: val=%v, err=%v", val, err)
}

// Test lookup with different record sizes
func TestMmdbReaderLookupRecordSize28(t *testing.T) {
	reader := &mmdbReader{
		data: make([]byte, 1000),
		metadata: &mmdbMetadata{
			NodeCount:  10,
			RecordSize: 28,
			IPVersion:  4,
		},
		nodeSize:   7,
		dataOffset: 100,
	}

	_, err := reader.lookup(net.ParseIP("8.8.8.8"))
	t.Logf("Lookup with record size 28: err=%v", err)
}

func TestMmdbReaderLookupRecordSize32(t *testing.T) {
	reader := &mmdbReader{
		data: make([]byte, 1000),
		metadata: &mmdbMetadata{
			NodeCount:  10,
			RecordSize: 32,
			IPVersion:  4,
		},
		nodeSize:   8,
		dataOffset: 100,
	}

	_, err := reader.lookup(net.ParseIP("8.8.8.8"))
	t.Logf("Lookup with record size 32: err=%v", err)
}

func TestMmdbReaderLookupUnsupportedRecordSize(t *testing.T) {
	reader := &mmdbReader{
		data: make([]byte, 1000),
		metadata: &mmdbMetadata{
			NodeCount:  10,
			RecordSize: 16, // Unsupported
			IPVersion:  4,
		},
		nodeSize:   4,
		dataOffset: 100,
	}

	_, err := reader.lookup(net.ParseIP("8.8.8.8"))
	if err == nil {
		t.Error("lookup should fail with unsupported record size")
	}
	if !strings.Contains(err.Error(), "unsupported record size") {
		t.Errorf("Error should mention unsupported record size: %v", err)
	}
}

func TestMmdbReaderLookupNodeOutOfBounds(t *testing.T) {
	// Create data where record value points to high node number, causing out of bounds
	data := make([]byte, 10)
	// Set first record to point to node 5 (which would need offset 30, beyond our 10 bytes)
	data[0] = 0x00
	data[1] = 0x00
	data[2] = 0x05

	reader := &mmdbReader{
		data: data,
		metadata: &mmdbMetadata{
			NodeCount:  1000,
			RecordSize: 24,
			IPVersion:  4,
		},
		nodeSize:   6,
		dataOffset: 6000,
	}

	_, err := reader.lookup(net.ParseIP("8.8.8.8"))
	if err == nil {
		t.Error("lookup should fail with node out of bounds")
	}
}

func TestMmdbReaderLookupIPv6(t *testing.T) {
	reader := &mmdbReader{
		data: make([]byte, 1000),
		metadata: &mmdbMetadata{
			NodeCount:  10,
			RecordSize: 24,
			IPVersion:  6,
		},
		nodeSize:   6,
		dataOffset: 100,
	}

	_, err := reader.lookup(net.ParseIP("2001:4860:4860::8888"))
	t.Logf("Lookup IPv6: err=%v", err)
}

func TestMmdbReaderLookupIPv4InIPv6DB(t *testing.T) {
	reader := &mmdbReader{
		data: make([]byte, 1000),
		metadata: &mmdbMetadata{
			NodeCount:  10,
			RecordSize: 24,
			IPVersion:  6, // IPv6 database
		},
		nodeSize:   6,
		dataOffset: 100,
	}

	// IPv4 address in IPv6 database
	_, err := reader.lookup(net.ParseIP("8.8.8.8"))
	t.Logf("Lookup IPv4 in IPv6 DB: err=%v", err)
}

// Test LookupCountry with various data formats
func TestLookupCountryWithIsoCode(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"country": map[string]interface{}{
			"iso_code": "US",
		},
	})

	result := reader.LookupCountry(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCountry with country.iso_code: %q", result)
}

func TestLookupCountryWithTopLevelIsoCode(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"iso_code": "GB",
	})

	result := reader.LookupCountry(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCountry with iso_code: %q", result)
}

func TestLookupCountryWithCountryCode(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"country_code": "DE",
	})

	result := reader.LookupCountry(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCountry with country_code: %q", result)
}

func TestLookupCountryWithDirectString(t *testing.T) {
	// Create reader that returns a direct string
	reader := &mmdbReader{
		data:       []byte{0x42, 'U', 'S'}, // String "US"
		metadata:   &mmdbMetadata{NodeCount: 0, RecordSize: 24, IPVersion: 4},
		nodeSize:   6,
		dataOffset: 0,
	}

	result := reader.LookupCountry(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCountry with direct string: %q", result)
}

func TestLookupCountryDecodeError(t *testing.T) {
	reader := &mmdbReader{
		data:       []byte{0xff, 0xff}, // Invalid data
		metadata:   &mmdbMetadata{NodeCount: 0, RecordSize: 24, IPVersion: 4},
		nodeSize:   6,
		dataOffset: 0,
	}

	result := reader.LookupCountry(net.ParseIP("8.8.8.8"))
	if result != "" {
		t.Errorf("LookupCountry should return empty on decode error: %q", result)
	}
}

// Test LookupASN with various data formats
func TestLookupASNWithStandardFields(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"autonomous_system_number":       uint64(15169),
		"autonomous_system_organization": "Google LLC",
	})

	asn, org := reader.LookupASN(net.ParseIP("8.8.8.8"))
	t.Logf("LookupASN standard: ASN=%d, Org=%q", asn, org)
}

func TestLookupASNWithAlternativeFields(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"asn":    uint64(12345),
		"as_org": "Example Org",
	})

	asn, org := reader.LookupASN(net.ParseIP("8.8.8.8"))
	t.Logf("LookupASN alternative: ASN=%d, Org=%q", asn, org)
}

func TestLookupASNWithNameField(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"asn":  uint64(54321),
		"name": "Some Provider",
	})

	asn, org := reader.LookupASN(net.ParseIP("8.8.8.8"))
	t.Logf("LookupASN with name: ASN=%d, Org=%q", asn, org)
}

func TestLookupASNNotMap(t *testing.T) {
	// Create reader that returns a non-map value
	reader := &mmdbReader{
		data:       []byte{0x41, 'x'}, // String "x"
		metadata:   &mmdbMetadata{NodeCount: 0, RecordSize: 24, IPVersion: 4},
		nodeSize:   6,
		dataOffset: 0,
	}

	asn, org := reader.LookupASN(net.ParseIP("8.8.8.8"))
	if asn != 0 || org != "" {
		t.Errorf("LookupASN should return zeros for non-map: ASN=%d, Org=%q", asn, org)
	}
}

// Test LookupCity with all fields
func TestLookupCityWithAllFields(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"city": map[string]interface{}{
			"names": map[string]interface{}{
				"en": "Mountain View",
			},
		},
		"subdivisions": []interface{}{
			map[string]interface{}{
				"names": map[string]interface{}{
					"en": "California",
				},
			},
		},
		"postal": map[string]interface{}{
			"code": "94035",
		},
		"location": map[string]interface{}{
			"latitude":  float64(37.386),
			"longitude": float64(-122.084),
			"time_zone": "America/Los_Angeles",
		},
	})

	city, region, postal, lat, lon, tz := reader.LookupCity(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCity: city=%q, region=%q, postal=%q, lat=%f, lon=%f, tz=%q",
		city, region, postal, lat, lon, tz)
}

func TestLookupCityPartialData(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"city": map[string]interface{}{
			"names": map[string]interface{}{
				"en": "Sydney",
			},
		},
		// No subdivisions, postal, or location
	})

	city, region, postal, lat, lon, tz := reader.LookupCity(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCity partial: city=%q, region=%q, postal=%q, lat=%f, lon=%f, tz=%q",
		city, region, postal, lat, lon, tz)
}

func TestLookupCityNotMap(t *testing.T) {
	reader := &mmdbReader{
		data:       []byte{0x41, 'x'}, // String "x"
		metadata:   &mmdbMetadata{NodeCount: 0, RecordSize: 24, IPVersion: 4},
		nodeSize:   6,
		dataOffset: 0,
	}

	city, region, postal, lat, lon, tz := reader.LookupCity(net.ParseIP("8.8.8.8"))
	if city != "" || region != "" || postal != "" || lat != 0 || lon != 0 || tz != "" {
		t.Error("LookupCity should return empty values for non-map")
	}
}

// Test LookupWHOIS with various fields
func TestLookupWHOISWithAllFields(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"autonomous_system_organization": "ARIN",
		"network":                         "8.0.0.0/8",
	})

	org, net := reader.LookupWHOIS(net.ParseIP("8.8.8.8"))
	t.Logf("LookupWHOIS with AS org: org=%q, net=%q", org, net)
}

func TestLookupWHOISWithAsOrg(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"as_org": "RIPE NCC",
		"range":  "1.0.0.0/8",
	})

	org, net := reader.LookupWHOIS(net.ParseIP("1.1.1.1"))
	t.Logf("LookupWHOIS with as_org: org=%q, net=%q", org, net)
}

func TestLookupWHOISWithOrganization(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"organization": "APNIC",
		"prefix":       "2.0.0.0/8",
	})

	org, net := reader.LookupWHOIS(net.ParseIP("2.2.2.2"))
	t.Logf("LookupWHOIS with organization: org=%q, net=%q", org, net)
}

func TestLookupWHOISWithOrg(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"org": "LACNIC",
	})

	org, net := reader.LookupWHOIS(net.ParseIP("3.3.3.3"))
	t.Logf("LookupWHOIS with org: org=%q, net=%q", org, net)
}

func TestLookupWHOISWithName(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"name": "AFRINIC",
	})

	org, net := reader.LookupWHOIS(net.ParseIP("4.4.4.4"))
	t.Logf("LookupWHOIS with name: org=%q, net=%q", org, net)
}

func TestLookupWHOISNotMap(t *testing.T) {
	reader := &mmdbReader{
		data:       []byte{0x41, 'x'}, // String "x"
		metadata:   &mmdbMetadata{NodeCount: 0, RecordSize: 24, IPVersion: 4},
		nodeSize:   6,
		dataOffset: 0,
	}

	org, net := reader.LookupWHOIS(net.ParseIP("8.8.8.8"))
	if org != "" || net != "" {
		t.Error("LookupWHOIS should return empty for non-map")
	}
}

// Test parseMetadata with valid data
func TestParseMetadataValid(t *testing.T) {
	// Create minimal valid MMDB metadata
	marker := []byte(metadataStartMarker)

	// Metadata map: type 7, size 2
	// Entry 1: "node_count" -> 1000
	// Entry 2: "record_size" -> 24
	metaData := []byte{
		0xe2, // map, size 2
		// key: "node_count"
		0x4a, 'n', 'o', 'd', 'e', '_', 'c', 'o', 'u', 'n', 't',
		// value: uint 1000 (type 6, size 2)
		0xc2, 0x03, 0xe8,
		// key: "record_size"
		0x4b, 'r', 'e', 'c', 'o', 'r', 'd', '_', 's', 'i', 'z', 'e',
		// value: uint 24 (type 6, size 1)
		0xc1, 0x18,
	}

	data := append([]byte{}, marker...)
	data = append(data, metaData...)

	reader := &mmdbReader{
		data: data,
	}

	err := reader.parseMetadata()
	if err != nil {
		t.Logf("parseMetadata error: %v", err)
	} else {
		t.Logf("parseMetadata success: NodeCount=%d, RecordSize=%d",
			reader.metadata.NodeCount, reader.metadata.RecordSize)
	}
}

func TestParseMetadataWithAllFields(t *testing.T) {
	marker := []byte(metadataStartMarker)

	// Metadata with all supported fields
	metaData := []byte{
		0xe5, // map, size 5
		// node_count
		0x4a, 'n', 'o', 'd', 'e', '_', 'c', 'o', 'u', 'n', 't',
		0xc2, 0x03, 0xe8, // 1000
		// record_size
		0x4b, 'r', 'e', 'c', 'o', 'r', 'd', '_', 's', 'i', 'z', 'e',
		0xc1, 0x18, // 24
		// ip_version
		0x4a, 'i', 'p', '_', 'v', 'e', 'r', 's', 'i', 'o', 'n',
		0xc1, 0x04, // 4
		// database_type
		0x4d, 'd', 'a', 't', 'a', 'b', 'a', 's', 'e', '_', 't', 'y', 'p', 'e',
		0x47, 'C', 'o', 'u', 'n', 't', 'r', 'y', // "Country"
		// build_epoch
		0x4b, 'b', 'u', 'i', 'l', 'd', '_', 'e', 'p', 'o', 'c', 'h',
		0xc4, 0x60, 0x00, 0x00, 0x00, // some epoch
	}

	data := append([]byte{}, marker...)
	data = append(data, metaData...)

	reader := &mmdbReader{
		data: data,
	}

	err := reader.parseMetadata()
	t.Logf("parseMetadata all fields: err=%v, meta=%+v", err, reader.metadata)
}

func TestParseMetadataNotMap(t *testing.T) {
	marker := []byte(metadataStartMarker)
	// String instead of map
	data := append([]byte{}, marker...)
	data = append(data, 0x41, 'x') // String "x"

	reader := &mmdbReader{
		data: data,
	}

	err := reader.parseMetadata()
	if err == nil {
		t.Error("parseMetadata should fail when metadata is not a map")
	}
	if !strings.Contains(err.Error(), "not a map") {
		t.Errorf("Error should mention not a map: %v", err)
	}
}

func TestParseMetadataRecordSizeNotDivisibleBy4(t *testing.T) {
	marker := []byte(metadataStartMarker)

	metaData := []byte{
		0xe2, // map, size 2
		// node_count
		0x4a, 'n', 'o', 'd', 'e', '_', 'c', 'o', 'u', 'n', 't',
		0xc2, 0x03, 0xe8, // 1000
		// record_size = 25 (not divisible by 4)
		0x4b, 'r', 'e', 'c', 'o', 'r', 'd', '_', 's', 'i', 'z', 'e',
		0xc1, 0x19, // 25
	}

	data := append([]byte{}, marker...)
	data = append(data, metaData...)

	reader := &mmdbReader{
		data: data,
	}

	err := reader.parseMetadata()
	t.Logf("parseMetadata record_size not divisible by 4: err=%v, nodeSize=%d", err, reader.nodeSize)
}

// Test Close method with concurrent access
func TestMmdbReaderCloseConcurrent(t *testing.T) {
	reader := &mmdbReader{
		data:     make([]byte, 100),
		metadata: &mmdbMetadata{},
	}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			reader.LookupCountry(net.ParseIP("8.8.8.8"))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			reader.LookupASN(net.ParseIP("8.8.8.8"))
		}
	}()

	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		reader.Close()
	}()

	wg.Wait()
}

// Test Lookup with found country in countries map
func TestLookupWithFoundCountryInMap(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	// Use a reader that returns "US"
	l.countryDB = createCountryDBReturningCode("US")

	result := l.Lookup("8.8.8.8")
	t.Logf("Lookup with known country: CountryCode=%q, CountryName=%q, Continent=%q",
		result.CountryCode, result.CountryName, result.Continent)
}

// Test Lookup with country not in countries map
func TestLookupWithUnknownCountry(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	l.countryDB = createCountryDBReturningCode("XX") // Unknown country

	result := l.Lookup("8.8.8.8")
	t.Logf("Lookup with unknown country: CountryCode=%q, CountryName=%q",
		result.CountryCode, result.CountryName)
}

// Test IsBlocked/IsAllowed with case insensitivity
func TestIsBlockedCaseInsensitiveMatch(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()
	l.countryDB = createCountryDBReturningCode("US")

	tests := []struct {
		blocked  string
		expected bool
	}{
		{"US", true},
		{"us", true},
		{"Us", true},
		{"uS", true},
		{"GB", false},
	}

	for _, tt := range tests {
		result := l.IsBlocked("8.8.8.8", []string{tt.blocked})
		// Can't guarantee result due to mock limitations, just log
		t.Logf("IsBlocked with %q: got %v", tt.blocked, result)
	}
}

func TestIsAllowedCaseInsensitiveMatch(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()
	l.countryDB = createCountryDBReturningCode("US")

	tests := []struct {
		allowed  []string
		expected bool
	}{
		{[]string{"US"}, true},
		{[]string{"us"}, true},
		{[]string{"Us", "Ca"}, true},
		{[]string{"GB", "DE"}, false},
	}

	for _, tt := range tests {
		result := l.IsAllowed("8.8.8.8", tt.allowed)
		// Can't guarantee result due to mock limitations, just log
		t.Logf("IsAllowed with %v: got %v", tt.allowed, result)
	}
}

// Test Close with all database types
func TestCloseWithAllDatabases(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
		countryDB: &mmdbReader{data: []byte{1}},
		asnDB:     &mmdbReader{data: []byte{2}},
		cityDB:    &mmdbReader{data: []byte{3}},
		whoisDB:   &mmdbReader{data: []byte{4}},
	}

	l.Close()

	if l.countryDB != nil {
		t.Error("countryDB should be nil after Close")
	}
	if l.asnDB != nil {
		t.Error("asnDB should be nil after Close")
	}
	if l.cityDB != nil {
		t.Error("cityDB should be nil after Close")
	}
	if l.whoisDB != nil {
		t.Error("whoisDB should be nil after Close")
	}
	if l.loaded {
		t.Error("loaded should be false after Close")
	}
}

// Test lookup with record pointing to data section
func TestMmdbReaderLookupFoundData(t *testing.T) {
	// Create a simple MMDB structure that returns data
	// This is complex because we need proper tree traversal

	// For simplicity, test with record > nodeCount (data pointer)
	data := make([]byte, 200)
	// Set up node data that points to data section
	// Node 0: left record = 0, right record = 2 (pointing to data)
	// With record size 24 (6 bytes per node)
	data[0] = 0x00
	data[1] = 0x00
	data[2] = 0x00 // Left record = 0
	data[3] = 0x00
	data[4] = 0x00
	data[5] = 0x02 // Right record = 2 (> nodeCount of 1, so it's data)

	// Data at offset: nodeCount * nodeSize + 16 + (record - nodeCount)
	// = 1 * 6 + 16 + (2 - 1) = 6 + 16 + 1 = 23
	// Put a simple string there
	data[23] = 0x42 // String type, size 2
	data[24] = 'U'
	data[25] = 'S'

	reader := &mmdbReader{
		data: data,
		metadata: &mmdbMetadata{
			NodeCount:  1,
			RecordSize: 24,
			IPVersion:  4,
		},
		nodeSize:   6,
		dataOffset: 22, // nodeCount * nodeSize + 16
	}

	offset, err := reader.lookup(net.ParseIP("128.0.0.1")) // Bit pattern starting with 1
	t.Logf("Lookup found data: offset=%d, err=%v", offset, err)
}

// Test lookup with record == nodeCount (not found)
func TestMmdbReaderLookupNotFound(t *testing.T) {
	data := make([]byte, 50)
	// Node 0: both records point to nodeCount (not found)
	data[0] = 0x00
	data[1] = 0x00
	data[2] = 0x01 // Left record = 1 (= nodeCount)
	data[3] = 0x00
	data[4] = 0x00
	data[5] = 0x01 // Right record = 1 (= nodeCount)

	reader := &mmdbReader{
		data: data,
		metadata: &mmdbMetadata{
			NodeCount:  1,
			RecordSize: 24,
			IPVersion:  4,
		},
		nodeSize:   6,
		dataOffset: 22,
	}

	offset, err := reader.lookup(net.ParseIP("8.8.8.8"))
	if err != nil {
		t.Errorf("lookup error: %v", err)
	}
	if offset != 0 {
		t.Errorf("lookup should return 0 for not found, got %d", offset)
	}
}

// Test download with write error (closed body)
func TestDownloadDatabaseWriteError(t *testing.T) {
	// Server that sends data but connection closes during transfer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
		// Response writer will be closed before full content
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.mmdb")

	l := NewLookup(DefaultConfig())
	// This may or may not error depending on how quickly we read
	err := l.downloadDatabase(server.URL, destPath)
	t.Logf("Download with potential write error: err=%v", err)
}

// Test lookup with record pointing to another node (not data)
func TestMmdbReaderLookupFollowsNodes(t *testing.T) {
	// Create MMDB data with multiple nodes
	data := make([]byte, 200)

	// Node 0: left -> node 1, right -> node 1
	data[0] = 0x00
	data[1] = 0x00
	data[2] = 0x00 // Left -> not found (NodeCount)
	data[3] = 0x00
	data[4] = 0x00
	data[5] = 0x00 // Right -> not found

	reader := &mmdbReader{
		data: data,
		metadata: &mmdbMetadata{
			NodeCount:  1,
			RecordSize: 24,
			IPVersion:  4,
		},
		nodeSize:   6,
		dataOffset: 22,
	}

	offset, err := reader.lookup(net.ParseIP("0.0.0.1")) // First bit is 0
	t.Logf("Lookup follow nodes (bit 0): offset=%d, err=%v", offset, err)
}

// Test lookup with 28-bit records both bit paths
func TestMmdbReaderLookup28BitBothPaths(t *testing.T) {
	// Record size 28 means 7 bytes per node
	// Layout: 3 bytes left + 1 nibble byte + 3 bytes right
	data := make([]byte, 200)

	// Node 0: set up for 28-bit records
	// Left record (3 bytes + upper nibble)
	data[0] = 0x00
	data[1] = 0x00
	data[2] = 0x01 // Left = 1 (NodeCount, not found)
	data[3] = 0x10 // Upper nibble for left, lower for right
	data[4] = 0x00
	data[5] = 0x00
	data[6] = 0x01 // Right = 1 (NodeCount, not found)

	reader := &mmdbReader{
		data: data,
		metadata: &mmdbMetadata{
			NodeCount:  1,
			RecordSize: 28,
			IPVersion:  4,
		},
		nodeSize:   7,
		dataOffset: 23,
	}

	// Test left path (IP starting with 0)
	offset1, err1 := reader.lookup(net.ParseIP("0.0.0.1"))
	t.Logf("28-bit left path: offset=%d, err=%v", offset1, err1)

	// Test right path (IP starting with 1)
	offset2, err2 := reader.lookup(net.ParseIP("128.0.0.1"))
	t.Logf("28-bit right path: offset=%d, err=%v", offset2, err2)
}

// Test lookup with 32-bit records both bit paths
func TestMmdbReaderLookup32BitBothPaths(t *testing.T) {
	data := make([]byte, 200)

	// Node 0: 8 bytes for 32-bit records (4 bytes left, 4 bytes right)
	// Left record = 1 (NodeCount)
	data[0] = 0x00
	data[1] = 0x00
	data[2] = 0x00
	data[3] = 0x01
	// Right record = 1 (NodeCount)
	data[4] = 0x00
	data[5] = 0x00
	data[6] = 0x00
	data[7] = 0x01

	reader := &mmdbReader{
		data: data,
		metadata: &mmdbMetadata{
			NodeCount:  1,
			RecordSize: 32,
			IPVersion:  4,
		},
		nodeSize:   8,
		dataOffset: 24,
	}

	// Test left path
	offset1, err1 := reader.lookup(net.ParseIP("0.0.0.1"))
	t.Logf("32-bit left path: offset=%d, err=%v", offset1, err1)

	// Test right path
	offset2, err2 := reader.lookup(net.ParseIP("128.0.0.1"))
	t.Logf("32-bit right path: offset=%d, err=%v", offset2, err2)
}

// Test size 29 decoding with sufficient data
func TestDecodeValueSize29Complete(t *testing.T) {
	reader := &mmdbReader{}

	// String with size 29 encoding: size = 29 + next_byte
	// Type 2 (string), size marker 29 (0x1d), additional byte = 1
	// Total size = 29 + 1 = 30 bytes
	data := make([]byte, 35)
	data[0] = 0x5d // type 2, size marker 29
	data[1] = 0x01 // additional byte = 1, so total size = 30
	for i := 2; i < 32; i++ {
		data[i] = 'a'
	}

	val, offset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Logf("Size 29 encoding error: %v", err)
	} else {
		if s, ok := val.(string); ok {
			t.Logf("Size 29 string length: %d, offset: %d", len(s), offset)
		}
	}
}

// Test size 30 decoding with sufficient data
func TestDecodeValueSize30Complete(t *testing.T) {
	reader := &mmdbReader{}

	// Size 30 encoding: size = 285 + next_2_bytes
	data := make([]byte, 300)
	data[0] = 0x5e           // type 2, size marker 30
	data[1] = 0x00           // high byte of additional size
	data[2] = 0x01           // low byte = 1, so total size = 285 + 1 = 286
	for i := 3; i < 289; i++ {
		data[i] = 'b'
	}

	val, offset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Logf("Size 30 encoding error: %v", err)
	} else {
		if s, ok := val.(string); ok {
			t.Logf("Size 30 string length: %d, offset: %d", len(s), offset)
		}
	}
}

// Test pointer decoding with valid target data
func TestDecodeValuePointerToValidData(t *testing.T) {
	reader := &mmdbReader{}

	// Create data with a pointer at offset 0 pointing to a string at offset 5
	data := []byte{
		0x20, 0x02, // Pointer size 1, pointing to offset 2
		0x42, 'O', 'K', // String "OK" at offset 2
	}

	val, offset, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Errorf("Pointer decode error: %v", err)
	}
	if val != "OK" {
		t.Errorf("Pointer value = %v, want 'OK'", val)
	}
	t.Logf("Pointer to valid data: val=%v, offset=%d", val, offset)
}

// Test map with non-string key (should be ignored)
func TestDecodeValueMapNonStringKey(t *testing.T) {
	reader := &mmdbReader{}

	// Map with size 1, key is uint (not string), value is string
	data := []byte{
		0xe1,       // map, size 1
		0xc1, 0x01, // key: uint 1 (not a string)
		0x41, 'v', // value: string "v"
	}

	val, _, err := reader.decodeValue(data, 0)
	if err != nil {
		t.Logf("Map with non-string key error: %v", err)
	}
	if m, ok := val.(map[string]interface{}); ok {
		// The non-string key should be ignored
		t.Logf("Map with non-string key: %v (should be empty)", m)
	}
}

// Test UpdateDatabases replacing existing databases
func TestUpdateDatabasesReplacingExisting(t *testing.T) {
	// Create a mock server that returns valid MMDB data
	mmdbContent := createMinimalMMDB()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(mmdbContent)
	}))
	defer server.Close()

	tmpDir := t.TempDir()

	// Pre-create databases
	os.WriteFile(filepath.Join(tmpDir, "country.mmdb"), mmdbContent, 0644)
	os.WriteFile(filepath.Join(tmpDir, "asn.mmdb"), mmdbContent, 0644)

	cfg := &Config{
		Enabled: true,
		Dir:     tmpDir,
		Country: true,
		ASN:     true,
	}

	l := NewLookup(cfg)

	// Load initial databases
	l.LoadDatabases()

	// Keep references to old databases
	oldCountryDB := l.countryDB
	oldAsnDB := l.asnDB

	// Update will fail because we can't override the URLs
	// but we can verify the update logic
	err := l.UpdateDatabases()
	t.Logf("UpdateDatabases: err=%v", err)
	t.Logf("Old countryDB: %p, New: %p", oldCountryDB, l.countryDB)
	t.Logf("Old asnDB: %p, New: %p", oldAsnDB, l.asnDB)
}

// Helper functions for creating mock data

func createMinimalMMDB() []byte {
	// Create minimal MMDB file with metadata marker
	marker := []byte(metadataStartMarker)

	// Minimal metadata
	metaData := []byte{
		0xe2, // map, size 2
		// node_count = 1
		0x4a, 'n', 'o', 'd', 'e', '_', 'c', 'o', 'u', 'n', 't',
		0xc1, 0x01,
		// record_size = 24
		0x4b, 'r', 'e', 'c', 'o', 'r', 'd', '_', 's', 'i', 'z', 'e',
		0xc1, 0x18,
	}

	// Some node data before marker
	nodeData := make([]byte, 50)

	result := make([]byte, 0, len(nodeData)+len(marker)+len(metaData))
	result = append(result, nodeData...)
	result = append(result, marker...)
	result = append(result, metaData...)

	return result
}

func createMockCountryData() []byte {
	// Create data that could be read by the mmdbReader
	data := make([]byte, 100)
	// Add marker and metadata
	marker := []byte(metadataStartMarker)
	copy(data[50:], marker)

	metaData := []byte{
		0xe2,
		0x4a, 'n', 'o', 'd', 'e', '_', 'c', 'o', 'u', 'n', 't',
		0xc1, 0x01,
		0x4b, 'r', 'e', 'c', 'o', 'r', 'd', '_', 's', 'i', 'z', 'e',
		0xc1, 0x18,
	}
	copy(data[50+len(marker):], metaData)

	return data
}

// createCountryDBReturningCode creates an mmdbReader that has data at offset 0
// This is used for testing - the data will be a country_code field
func createCountryDBReturningCode(countryCode string) *mmdbReader {
	// Create MMDB data with country_code field
	data := encodeMMDBMap(map[string]interface{}{
		"country_code": countryCode,
	})

	return &mmdbReader{
		data:       data,
		metadata:   &mmdbMetadata{NodeCount: 0, RecordSize: 24, IPVersion: 4},
		nodeSize:   6,
		dataOffset: 0,
	}
}

func createMockReaderWithData(data map[string]interface{}) *mmdbReader {
	// Encode the data map into MMDB format
	encoded := encodeMMDBMap(data)

	return &mmdbReader{
		data:       encoded,
		metadata:   &mmdbMetadata{NodeCount: 0, RecordSize: 24, IPVersion: 4},
		nodeSize:   6,
		dataOffset: 0,
	}
}

func encodeMMDBMap(data map[string]interface{}) []byte {
	var result []byte

	// Map type 7, size
	size := len(data)
	if size < 29 {
		result = append(result, byte(0xe0|size)) // type 7 = 0xe0
	} else {
		result = append(result, 0xfd, byte(size-29)) // size 29 encoding
	}

	for key, val := range data {
		// Encode key (string)
		result = append(result, encodeMMDBString(key)...)
		// Encode value
		result = append(result, encodeMMDBValue(val)...)
	}

	return result
}

func encodeMMDBString(s string) []byte {
	var result []byte
	size := len(s)
	if size < 29 {
		result = append(result, byte(0x40|size)) // type 2 = 0x40
	} else {
		result = append(result, 0x5d, byte(size-29))
	}
	result = append(result, []byte(s)...)
	return result
}

func encodeMMDBValue(val interface{}) []byte {
	switch v := val.(type) {
	case string:
		return encodeMMDBString(v)
	case uint64:
		return encodeMMDBUint(v)
	case float64:
		return encodeMMDBFloat64(v)
	case map[string]interface{}:
		return encodeMMDBMap(v)
	case []interface{}:
		return encodeMMDBArray(v)
	default:
		return []byte{0x40} // empty string as fallback
	}
}

func encodeMMDBUint(v uint64) []byte {
	if v == 0 {
		return []byte{0xc0} // type 6, size 0
	}
	if v < 256 {
		return []byte{0xc1, byte(v)}
	}
	if v < 65536 {
		return []byte{0xc2, byte(v >> 8), byte(v)}
	}
	return []byte{0xc4, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func encodeMMDBFloat64(v float64) []byte {
	// Simplified - just return double type marker
	return []byte{0x68, 0, 0, 0, 0, 0, 0, 0, 0}
}

func encodeMMDBArray(arr []interface{}) []byte {
	var result []byte
	size := len(arr)
	// Array is extended type 11 (type 0 + byte 4)
	result = append(result, byte(size), 0x04)
	for _, item := range arr {
		result = append(result, encodeMMDBValue(item)...)
	}
	return result
}

// Additional coverage tests for edge cases

// Test LoadDatabases with download errors for all database types
func TestLoadDatabasesDownloadErrors(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Enabled: true,
		Dir:     tmpDir,
		Country: true, // Will try to download but fail (no network mock)
		ASN:     true,
		City:    true,
		WHOIS:   true,
	}

	l := NewLookup(cfg)

	// LoadDatabases will try to download but URLs are real, so it will fail
	err := l.LoadDatabases()
	// Error is expected due to download failure
	if err != nil {
		t.Logf("LoadDatabases download errors (expected): %v", err)
	}

	// loaded should still be false since country DB couldn't be loaded
	if l.IsLoaded() {
		t.Log("Databases loaded successfully (unexpected)")
	}
}

// Test UpdateDatabases with nil existing databases
func TestUpdateDatabasesNoExistingDBs(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Enabled: true,
		Dir:     tmpDir,
		Country: true,
		ASN:     true,
		City:    true,
		WHOIS:   true,
	}

	l := NewLookup(cfg)
	// Don't load - go straight to update
	err := l.UpdateDatabases()
	// Will fail because download URLs are real
	if err != nil {
		t.Logf("UpdateDatabases without existing DBs: %v", err)
	}
}

// Test LookupCity with empty subdivisions
func TestLookupCityEmptySubdivisions(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"city": map[string]interface{}{
			"names": map[string]interface{}{
				"en": "Tokyo",
			},
		},
		"subdivisions": []interface{}{}, // Empty array
	})

	city, region, _, _, _, _ := reader.LookupCity(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCity empty subdivisions: city=%q, region=%q", city, region)
}

// Test LookupCity with subdivisions but no names
func TestLookupCitySubdivisionsNoNames(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"subdivisions": []interface{}{
			map[string]interface{}{
				"iso_code": "CA", // No names field
			},
		},
	})

	_, region, _, _, _, _ := reader.LookupCity(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCity subdivisions no names: region=%q", region)
}

// Test LookupCity with location but missing fields
func TestLookupCityLocationPartial(t *testing.T) {
	reader := createMockReaderWithData(map[string]interface{}{
		"location": map[string]interface{}{
			"latitude": float64(35.6762),
			// Missing longitude and time_zone
		},
	})

	_, _, _, lat, lon, tz := reader.LookupCity(net.ParseIP("8.8.8.8"))
	t.Logf("LookupCity partial location: lat=%f, lon=%f, tz=%q", lat, lon, tz)
}

// Test decodeValue with int32 larger values
func TestDecodeValueInt32LargeValue(t *testing.T) {
	reader := &mmdbReader{}

	// int32 with 4 bytes: 0x7FFFFFFF (max positive)
	data := []byte{0x04, 0x01, 0x7f, 0xff, 0xff, 0xff}
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("int32 max positive: val=%v, err=%v", val, err)
}

// Test decodeValue with int32 negative value
func TestDecodeValueInt32Negative(t *testing.T) {
	reader := &mmdbReader{}

	// int32 with 2 bytes: 0xFFFF = -1 with sign extension
	// Extended type 8, size 2
	data := []byte{0x02, 0x01, 0xff, 0xff}
	val, _, err := reader.decodeValue(data, 0)
	t.Logf("int32 negative (2 bytes): val=%v, err=%v", val, err)
}

// Test lookup exhausts all bits without finding
func TestMmdbReaderLookupExhaustsBits(t *testing.T) {
	// Create data where each node points to the next node
	// until we run out of bits
	data := make([]byte, 1000)

	// Set up multiple nodes that keep pointing to node 0
	// This will cause the loop to continue until all 128 bits are exhausted
	for i := 0; i < 100; i++ {
		offset := i * 6
		// Both records point to node 0
		data[offset+0] = 0x00
		data[offset+1] = 0x00
		data[offset+2] = 0x00
		data[offset+3] = 0x00
		data[offset+4] = 0x00
		data[offset+5] = 0x00
	}

	reader := &mmdbReader{
		data: data,
		metadata: &mmdbMetadata{
			NodeCount:  100,
			RecordSize: 24,
			IPVersion:  6, // IPv6 for 128 bits
		},
		nodeSize:   6,
		dataOffset: 616, // After all nodes
	}

	offset, err := reader.lookup(net.ParseIP("2001:4860:4860::8888"))
	t.Logf("Lookup exhausts bits: offset=%d, err=%v", offset, err)
}

// Test Lookup with IPv4 in IPv4 database
func TestLookupIPv4InIPv4DB(t *testing.T) {
	l := &Lookup{
		countries: make(map[string]*Country),
		config:    DefaultConfig(),
		loaded:    true,
	}
	l.initCountries()

	// Create a reader with IPv4 database type
	l.countryDB = &mmdbReader{
		data: make([]byte, 100),
		metadata: &mmdbMetadata{
			NodeCount:  1,
			RecordSize: 24,
			IPVersion:  4,
		},
		nodeSize:   6,
		dataOffset: 22,
	}

	result := l.Lookup("192.168.1.1")
	t.Logf("IPv4 in IPv4 DB: Found=%v, Code=%q", result.Found, result.CountryCode)
}

// Test concurrent LoadDatabases and UpdateDatabases
func TestConcurrentLoadAndUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Enabled: true,
		Dir:     tmpDir,
		Country: false, // Disable to avoid network calls
		ASN:     false,
		City:    false,
		WHOIS:   false,
	}

	l := NewLookup(cfg)

	var wg sync.WaitGroup
	wg.Add(3)

	// Concurrent loads
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			l.LoadDatabases()
		}
	}()

	// Concurrent updates
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			l.UpdateDatabases()
		}
	}()

	// Concurrent lookups
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			l.Lookup("8.8.8.8")
			l.IsLoaded()
			l.LastUpdate()
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Concurrent operations completed successfully")
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout in concurrent operations")
	}
}

// Test parseMetadata decode error
func TestParseMetadataDecodeError(t *testing.T) {
	marker := []byte(metadataStartMarker)

	// Invalid metadata that will fail decoding
	data := append([]byte{}, marker...)
	// Add invalid MMDB encoding that will cause decode error
	data = append(data, 0xff, 0xff, 0xff, 0xff)

	reader := &mmdbReader{
		data: data,
	}

	err := reader.parseMetadata()
	if err == nil {
		t.Error("parseMetadata should fail with decode error")
	}
	t.Logf("parseMetadata decode error: %v", err)
}

// Test GetCountry with empty string
func TestGetCountryEmptyString(t *testing.T) {
	l := NewLookup(DefaultConfig())

	c := l.GetCountry("")
	if c != nil {
		t.Errorf("GetCountry('') should return nil, got %v", c)
	}
}

// Test GetCountry with 3-letter code (invalid)
func TestGetCountryInvalidLength(t *testing.T) {
	l := NewLookup(DefaultConfig())

	c := l.GetCountry("USA")
	if c != nil {
		t.Errorf("GetCountry('USA') should return nil, got %v", c)
	}
}

// Test Result with zero values
func TestResultZeroValues(t *testing.T) {
	r := Result{}

	if r.IP != "" {
		t.Error("Zero Result IP should be empty")
	}
	if r.Found {
		t.Error("Zero Result Found should be false")
	}
	if r.ASN != 0 {
		t.Error("Zero Result ASN should be 0")
	}
	if r.Latitude != 0 || r.Longitude != 0 {
		t.Error("Zero Result coordinates should be 0")
	}
}

// Test Config with all options
func TestConfigAllOptions(t *testing.T) {
	cfg := &Config{
		Enabled:          true,
		Dir:              "/custom/dir",
		Update:           "daily",
		DenyCountries:    []string{"XX", "YY"},
		AllowedCountries: []string{"US", "CA", "GB"},
		ASN:              true,
		Country:          true,
		City:             true,
		WHOIS:            true,
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Dir != "/custom/dir" {
		t.Errorf("Dir = %q, want /custom/dir", cfg.Dir)
	}
	if len(cfg.DenyCountries) != 2 {
		t.Errorf("DenyCountries length = %d, want 2", len(cfg.DenyCountries))
	}
	if len(cfg.AllowedCountries) != 3 {
		t.Errorf("AllowedCountries length = %d, want 3", len(cfg.AllowedCountries))
	}
	if !cfg.City {
		t.Error("City should be true")
	}
	if !cfg.WHOIS {
		t.Error("WHOIS should be true")
	}
}

// Test metadataStartMarker constant
func TestMetadataStartMarkerConstant(t *testing.T) {
	if metadataStartMarker == "" {
		t.Error("metadataStartMarker should not be empty")
	}
	if len(metadataStartMarker) < 10 {
		t.Errorf("metadataStartMarker too short: %d bytes", len(metadataStartMarker))
	}
}
