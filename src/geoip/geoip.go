package geoip

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Database URLs from sapics/ip-location-db (NON-NEGOTIABLE per AI.md)
const (
	ASNURL     = "https://cdn.jsdelivr.net/npm/@ip-location-db/asn-mmdb/asn.mmdb"
	CountryURL = "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country-mmdb/geo-whois-asn-country.mmdb"
	CityURL    = "https://cdn.jsdelivr.net/npm/@ip-location-db/dbip-city-mmdb/dbip-city-ipv4.mmdb"
	// WHOIS database per AI.md PART 20
	WhoisURL = "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country-mmdb/geo-whois-asn-country.mmdb"
)

// Lookup represents a GeoIP lookup service using MMDB format
type Lookup struct {
	mu           sync.RWMutex
	countryDB    *mmdbReader
	asnDB        *mmdbReader
	cityDB       *mmdbReader
	whoisDB      *mmdbReader // WHOIS registrant database (per AI.md PART 20)
	loaded       bool
	dbDir        string
	countries    map[string]*Country
	lastUpdate   time.Time
	config       *Config
}

// Country represents country information
type Country struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Continent string `json:"continent"`
}

// Result represents a GeoIP lookup result
type Result struct {
	IP            string  `json:"ip"`
	CountryCode   string  `json:"country_code"`
	CountryName   string  `json:"country_name"`
	Continent     string  `json:"continent"`
	City          string  `json:"city,omitempty"`
	Region        string  `json:"region,omitempty"`
	PostalCode    string  `json:"postal_code,omitempty"`
	Latitude      float64 `json:"latitude,omitempty"`
	Longitude     float64 `json:"longitude,omitempty"`
	Timezone      string  `json:"timezone,omitempty"`
	ASN           uint    `json:"asn,omitempty"`
	ASNOrg        string  `json:"asn_org,omitempty"`
	// WHOIS registrant data (per AI.md PART 20)
	RegistrantOrg string `json:"registrant_org,omitempty"`
	RegistrantNet string `json:"registrant_net,omitempty"`
	Found         bool   `json:"found"`
}

// Config holds GeoIP configuration
type Config struct {
	Enabled          bool     `yaml:"enabled"`
	Dir              string   `yaml:"dir"`
	Update           string   `yaml:"update"` // never, daily, weekly, monthly
	DenyCountries    []string `yaml:"deny_countries"`
	AllowedCountries []string `yaml:"allowed_countries"`
	// Database toggles
	ASN     bool `yaml:"asn"`
	Country bool `yaml:"country"`
	City    bool `yaml:"city"`
	WHOIS   bool `yaml:"whois"` // Enable WHOIS registrant data (per AI.md PART 20)
}

// DefaultConfig returns default GeoIP configuration
// Per AI.md PART 20: GeoIP dir is {config_dir}/security/geoip
func DefaultConfig() *Config {
	return &Config{
		Enabled:          false,
		Dir:              "/config/security/geoip",
		Update:           "weekly",
		DenyCountries:    []string{},
		AllowedCountries: []string{},
		ASN:              true,
		Country:          true,
		City:             false, // Larger download, disabled by default
		WHOIS:            false, // WHOIS registrant data, disabled by default
	}
}

// NewLookup creates a new GeoIP lookup service
func NewLookup(cfg *Config) *Lookup {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	l := &Lookup{
		countries: make(map[string]*Country),
		dbDir:     cfg.Dir,
		config:    cfg,
	}

	// Initialize country names
	l.initCountries()

	return l
}

// LoadDatabases loads all configured MMDB databases
func (l *Lookup) LoadDatabases() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.config == nil {
		return fmt.Errorf("config not set")
	}

	// Ensure directory exists
	if err := os.MkdirAll(l.dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create geoip directory: %w", err)
	}

	var loadErrors []string

	// Load country database
	if l.config.Country {
		countryPath := filepath.Join(l.dbDir, "country.mmdb")
		if _, err := os.Stat(countryPath); os.IsNotExist(err) {
			if err := l.downloadDatabase(CountryURL, countryPath); err != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("country: %v", err))
			}
		}
		if db, err := openMMDB(countryPath); err == nil {
			l.countryDB = db
		} else {
			loadErrors = append(loadErrors, fmt.Sprintf("country load: %v", err))
		}
	}

	// Load ASN database
	if l.config.ASN {
		asnPath := filepath.Join(l.dbDir, "asn.mmdb")
		if _, err := os.Stat(asnPath); os.IsNotExist(err) {
			if err := l.downloadDatabase(ASNURL, asnPath); err != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("asn: %v", err))
			}
		}
		if db, err := openMMDB(asnPath); err == nil {
			l.asnDB = db
		} else {
			loadErrors = append(loadErrors, fmt.Sprintf("asn load: %v", err))
		}
	}

	// Load city database
	if l.config.City {
		cityPath := filepath.Join(l.dbDir, "city.mmdb")
		if _, err := os.Stat(cityPath); os.IsNotExist(err) {
			if err := l.downloadDatabase(CityURL, cityPath); err != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("city: %v", err))
			}
		}
		if db, err := openMMDB(cityPath); err == nil {
			l.cityDB = db
		} else {
			loadErrors = append(loadErrors, fmt.Sprintf("city load: %v", err))
		}
	}

	// Load WHOIS database (per AI.md PART 20)
	if l.config.WHOIS {
		whoisPath := filepath.Join(l.dbDir, "whois.mmdb")
		if _, err := os.Stat(whoisPath); os.IsNotExist(err) {
			if err := l.downloadDatabase(WhoisURL, whoisPath); err != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("whois: %v", err))
			}
		}
		if db, err := openMMDB(whoisPath); err == nil {
			l.whoisDB = db
		} else {
			loadErrors = append(loadErrors, fmt.Sprintf("whois load: %v", err))
		}
	}

	// Mark as loaded if at least country database is available
	l.loaded = l.countryDB != nil
	l.lastUpdate = time.Now()

	if len(loadErrors) > 0 && !l.loaded {
		return fmt.Errorf("failed to load databases: %s", strings.Join(loadErrors, "; "))
	}

	return nil
}

// downloadDatabase downloads an MMDB database from URL
func (l *Lookup) downloadDatabase(url, destPath string) error {
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	// Create temp file first
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Rename to final path
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// UpdateDatabases updates all databases
func (l *Lookup) UpdateDatabases() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.config == nil {
		return fmt.Errorf("config not set")
	}

	var updateErrors []string

	if l.config.Country {
		countryPath := filepath.Join(l.dbDir, "country.mmdb")
		if err := l.downloadDatabase(CountryURL, countryPath); err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("country: %v", err))
		} else if db, err := openMMDB(countryPath); err == nil {
			if l.countryDB != nil {
				l.countryDB.Close()
			}
			l.countryDB = db
		}
	}

	if l.config.ASN {
		asnPath := filepath.Join(l.dbDir, "asn.mmdb")
		if err := l.downloadDatabase(ASNURL, asnPath); err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("asn: %v", err))
		} else if db, err := openMMDB(asnPath); err == nil {
			if l.asnDB != nil {
				l.asnDB.Close()
			}
			l.asnDB = db
		}
	}

	if l.config.City {
		cityPath := filepath.Join(l.dbDir, "city.mmdb")
		if err := l.downloadDatabase(CityURL, cityPath); err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("city: %v", err))
		} else if db, err := openMMDB(cityPath); err == nil {
			if l.cityDB != nil {
				l.cityDB.Close()
			}
			l.cityDB = db
		}
	}

	// Update WHOIS database (per AI.md PART 20)
	if l.config.WHOIS {
		whoisPath := filepath.Join(l.dbDir, "whois.mmdb")
		if err := l.downloadDatabase(WhoisURL, whoisPath); err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("whois: %v", err))
		} else if db, err := openMMDB(whoisPath); err == nil {
			if l.whoisDB != nil {
				l.whoisDB.Close()
			}
			l.whoisDB = db
		}
	}

	l.lastUpdate = time.Now()

	if len(updateErrors) > 0 {
		return fmt.Errorf("update errors: %s", strings.Join(updateErrors, "; "))
	}

	return nil
}

// Lookup looks up an IP address
func (l *Lookup) Lookup(ipStr string) *Result {
	result := &Result{
		IP:    ipStr,
		Found: false,
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return result
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.loaded {
		return result
	}

	// Country lookup
	if l.countryDB != nil {
		if countryCode := l.countryDB.LookupCountry(ip); countryCode != "" {
			result.CountryCode = countryCode
			result.Found = true
			if country, ok := l.countries[countryCode]; ok {
				result.CountryName = country.Name
				result.Continent = country.Continent
			}
		}
	}

	// ASN lookup
	if l.asnDB != nil {
		asn, org := l.asnDB.LookupASN(ip)
		result.ASN = asn
		result.ASNOrg = org
	}

	// City lookup
	if l.cityDB != nil {
		city, region, postal, lat, lon, tz := l.cityDB.LookupCity(ip)
		result.City = city
		result.Region = region
		result.PostalCode = postal
		result.Latitude = lat
		result.Longitude = lon
		result.Timezone = tz
	}

	// WHOIS registrant lookup (per AI.md PART 20)
	if l.whoisDB != nil {
		registrantOrg, registrantNet := l.whoisDB.LookupWHOIS(ip)
		result.RegistrantOrg = registrantOrg
		result.RegistrantNet = registrantNet
	}

	return result
}

// IsLoaded returns true if database is loaded
func (l *Lookup) IsLoaded() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.loaded
}

// LastUpdate returns the last update time
func (l *Lookup) LastUpdate() time.Time {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.lastUpdate
}

// GetCountry returns country info by code
func (l *Lookup) GetCountry(code string) *Country {
	if c, ok := l.countries[strings.ToUpper(code)]; ok {
		return c
	}
	return nil
}

// IsBlocked checks if an IP is from a blocked country
func (l *Lookup) IsBlocked(ipStr string, blockedCountries []string) bool {
	if len(blockedCountries) == 0 {
		return false
	}

	result := l.Lookup(ipStr)
	if !result.Found {
		return false
	}

	for _, blocked := range blockedCountries {
		if strings.EqualFold(result.CountryCode, blocked) {
			return true
		}
	}
	return false
}

// IsAllowed checks if an IP is from an allowed country
func (l *Lookup) IsAllowed(ipStr string, allowedCountries []string) bool {
	// No restrictions when allowedCountries is empty
	if len(allowedCountries) == 0 {
		return true
	}

	result := l.Lookup(ipStr)
	// Allow if country unknown
	if !result.Found {
		return true
	}

	for _, allowed := range allowedCountries {
		if strings.EqualFold(result.CountryCode, allowed) {
			return true
		}
	}
	return false
}

// Close closes all database connections
func (l *Lookup) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.countryDB != nil {
		l.countryDB.Close()
		l.countryDB = nil
	}
	if l.asnDB != nil {
		l.asnDB.Close()
		l.asnDB = nil
	}
	if l.cityDB != nil {
		l.cityDB.Close()
		l.cityDB = nil
	}
	// Close WHOIS database (per AI.md PART 20)
	if l.whoisDB != nil {
		l.whoisDB.Close()
		l.whoisDB = nil
	}
	l.loaded = false
}

// initCountries initializes the country name map
func (l *Lookup) initCountries() {
	l.countries = map[string]*Country{
		"AD": {Code: "AD", Name: "Andorra", Continent: "EU"},
		"AE": {Code: "AE", Name: "United Arab Emirates", Continent: "AS"},
		"AF": {Code: "AF", Name: "Afghanistan", Continent: "AS"},
		"AG": {Code: "AG", Name: "Antigua and Barbuda", Continent: "NA"},
		"AI": {Code: "AI", Name: "Anguilla", Continent: "NA"},
		"AL": {Code: "AL", Name: "Albania", Continent: "EU"},
		"AM": {Code: "AM", Name: "Armenia", Continent: "AS"},
		"AO": {Code: "AO", Name: "Angola", Continent: "AF"},
		"AQ": {Code: "AQ", Name: "Antarctica", Continent: "AN"},
		"AR": {Code: "AR", Name: "Argentina", Continent: "SA"},
		"AS": {Code: "AS", Name: "American Samoa", Continent: "OC"},
		"AT": {Code: "AT", Name: "Austria", Continent: "EU"},
		"AU": {Code: "AU", Name: "Australia", Continent: "OC"},
		"AW": {Code: "AW", Name: "Aruba", Continent: "NA"},
		"AZ": {Code: "AZ", Name: "Azerbaijan", Continent: "AS"},
		"BA": {Code: "BA", Name: "Bosnia and Herzegovina", Continent: "EU"},
		"BB": {Code: "BB", Name: "Barbados", Continent: "NA"},
		"BD": {Code: "BD", Name: "Bangladesh", Continent: "AS"},
		"BE": {Code: "BE", Name: "Belgium", Continent: "EU"},
		"BF": {Code: "BF", Name: "Burkina Faso", Continent: "AF"},
		"BG": {Code: "BG", Name: "Bulgaria", Continent: "EU"},
		"BH": {Code: "BH", Name: "Bahrain", Continent: "AS"},
		"BI": {Code: "BI", Name: "Burundi", Continent: "AF"},
		"BJ": {Code: "BJ", Name: "Benin", Continent: "AF"},
		"BM": {Code: "BM", Name: "Bermuda", Continent: "NA"},
		"BN": {Code: "BN", Name: "Brunei", Continent: "AS"},
		"BO": {Code: "BO", Name: "Bolivia", Continent: "SA"},
		"BR": {Code: "BR", Name: "Brazil", Continent: "SA"},
		"BS": {Code: "BS", Name: "Bahamas", Continent: "NA"},
		"BT": {Code: "BT", Name: "Bhutan", Continent: "AS"},
		"BW": {Code: "BW", Name: "Botswana", Continent: "AF"},
		"BY": {Code: "BY", Name: "Belarus", Continent: "EU"},
		"BZ": {Code: "BZ", Name: "Belize", Continent: "NA"},
		"CA": {Code: "CA", Name: "Canada", Continent: "NA"},
		"CD": {Code: "CD", Name: "DR Congo", Continent: "AF"},
		"CF": {Code: "CF", Name: "Central African Republic", Continent: "AF"},
		"CG": {Code: "CG", Name: "Congo", Continent: "AF"},
		"CH": {Code: "CH", Name: "Switzerland", Continent: "EU"},
		"CI": {Code: "CI", Name: "Ivory Coast", Continent: "AF"},
		"CL": {Code: "CL", Name: "Chile", Continent: "SA"},
		"CM": {Code: "CM", Name: "Cameroon", Continent: "AF"},
		"CN": {Code: "CN", Name: "China", Continent: "AS"},
		"CO": {Code: "CO", Name: "Colombia", Continent: "SA"},
		"CR": {Code: "CR", Name: "Costa Rica", Continent: "NA"},
		"CU": {Code: "CU", Name: "Cuba", Continent: "NA"},
		"CV": {Code: "CV", Name: "Cape Verde", Continent: "AF"},
		"CY": {Code: "CY", Name: "Cyprus", Continent: "EU"},
		"CZ": {Code: "CZ", Name: "Czechia", Continent: "EU"},
		"DE": {Code: "DE", Name: "Germany", Continent: "EU"},
		"DJ": {Code: "DJ", Name: "Djibouti", Continent: "AF"},
		"DK": {Code: "DK", Name: "Denmark", Continent: "EU"},
		"DM": {Code: "DM", Name: "Dominica", Continent: "NA"},
		"DO": {Code: "DO", Name: "Dominican Republic", Continent: "NA"},
		"DZ": {Code: "DZ", Name: "Algeria", Continent: "AF"},
		"EC": {Code: "EC", Name: "Ecuador", Continent: "SA"},
		"EE": {Code: "EE", Name: "Estonia", Continent: "EU"},
		"EG": {Code: "EG", Name: "Egypt", Continent: "AF"},
		"ER": {Code: "ER", Name: "Eritrea", Continent: "AF"},
		"ES": {Code: "ES", Name: "Spain", Continent: "EU"},
		"ET": {Code: "ET", Name: "Ethiopia", Continent: "AF"},
		"FI": {Code: "FI", Name: "Finland", Continent: "EU"},
		"FJ": {Code: "FJ", Name: "Fiji", Continent: "OC"},
		"FM": {Code: "FM", Name: "Micronesia", Continent: "OC"},
		"FR": {Code: "FR", Name: "France", Continent: "EU"},
		"GA": {Code: "GA", Name: "Gabon", Continent: "AF"},
		"GB": {Code: "GB", Name: "United Kingdom", Continent: "EU"},
		"GD": {Code: "GD", Name: "Grenada", Continent: "NA"},
		"GE": {Code: "GE", Name: "Georgia", Continent: "AS"},
		"GH": {Code: "GH", Name: "Ghana", Continent: "AF"},
		"GM": {Code: "GM", Name: "Gambia", Continent: "AF"},
		"GN": {Code: "GN", Name: "Guinea", Continent: "AF"},
		"GQ": {Code: "GQ", Name: "Equatorial Guinea", Continent: "AF"},
		"GR": {Code: "GR", Name: "Greece", Continent: "EU"},
		"GT": {Code: "GT", Name: "Guatemala", Continent: "NA"},
		"GW": {Code: "GW", Name: "Guinea-Bissau", Continent: "AF"},
		"GY": {Code: "GY", Name: "Guyana", Continent: "SA"},
		"HK": {Code: "HK", Name: "Hong Kong", Continent: "AS"},
		"HN": {Code: "HN", Name: "Honduras", Continent: "NA"},
		"HR": {Code: "HR", Name: "Croatia", Continent: "EU"},
		"HT": {Code: "HT", Name: "Haiti", Continent: "NA"},
		"HU": {Code: "HU", Name: "Hungary", Continent: "EU"},
		"ID": {Code: "ID", Name: "Indonesia", Continent: "AS"},
		"IE": {Code: "IE", Name: "Ireland", Continent: "EU"},
		"IL": {Code: "IL", Name: "Israel", Continent: "AS"},
		"IN": {Code: "IN", Name: "India", Continent: "AS"},
		"IQ": {Code: "IQ", Name: "Iraq", Continent: "AS"},
		"IR": {Code: "IR", Name: "Iran", Continent: "AS"},
		"IS": {Code: "IS", Name: "Iceland", Continent: "EU"},
		"IT": {Code: "IT", Name: "Italy", Continent: "EU"},
		"JM": {Code: "JM", Name: "Jamaica", Continent: "NA"},
		"JO": {Code: "JO", Name: "Jordan", Continent: "AS"},
		"JP": {Code: "JP", Name: "Japan", Continent: "AS"},
		"KE": {Code: "KE", Name: "Kenya", Continent: "AF"},
		"KG": {Code: "KG", Name: "Kyrgyzstan", Continent: "AS"},
		"KH": {Code: "KH", Name: "Cambodia", Continent: "AS"},
		"KI": {Code: "KI", Name: "Kiribati", Continent: "OC"},
		"KM": {Code: "KM", Name: "Comoros", Continent: "AF"},
		"KN": {Code: "KN", Name: "Saint Kitts and Nevis", Continent: "NA"},
		"KP": {Code: "KP", Name: "North Korea", Continent: "AS"},
		"KR": {Code: "KR", Name: "South Korea", Continent: "AS"},
		"KW": {Code: "KW", Name: "Kuwait", Continent: "AS"},
		"KZ": {Code: "KZ", Name: "Kazakhstan", Continent: "AS"},
		"LA": {Code: "LA", Name: "Laos", Continent: "AS"},
		"LB": {Code: "LB", Name: "Lebanon", Continent: "AS"},
		"LC": {Code: "LC", Name: "Saint Lucia", Continent: "NA"},
		"LI": {Code: "LI", Name: "Liechtenstein", Continent: "EU"},
		"LK": {Code: "LK", Name: "Sri Lanka", Continent: "AS"},
		"LR": {Code: "LR", Name: "Liberia", Continent: "AF"},
		"LS": {Code: "LS", Name: "Lesotho", Continent: "AF"},
		"LT": {Code: "LT", Name: "Lithuania", Continent: "EU"},
		"LU": {Code: "LU", Name: "Luxembourg", Continent: "EU"},
		"LV": {Code: "LV", Name: "Latvia", Continent: "EU"},
		"LY": {Code: "LY", Name: "Libya", Continent: "AF"},
		"MA": {Code: "MA", Name: "Morocco", Continent: "AF"},
		"MC": {Code: "MC", Name: "Monaco", Continent: "EU"},
		"MD": {Code: "MD", Name: "Moldova", Continent: "EU"},
		"ME": {Code: "ME", Name: "Montenegro", Continent: "EU"},
		"MG": {Code: "MG", Name: "Madagascar", Continent: "AF"},
		"MH": {Code: "MH", Name: "Marshall Islands", Continent: "OC"},
		"MK": {Code: "MK", Name: "North Macedonia", Continent: "EU"},
		"ML": {Code: "ML", Name: "Mali", Continent: "AF"},
		"MM": {Code: "MM", Name: "Myanmar", Continent: "AS"},
		"MN": {Code: "MN", Name: "Mongolia", Continent: "AS"},
		"MO": {Code: "MO", Name: "Macau", Continent: "AS"},
		"MR": {Code: "MR", Name: "Mauritania", Continent: "AF"},
		"MT": {Code: "MT", Name: "Malta", Continent: "EU"},
		"MU": {Code: "MU", Name: "Mauritius", Continent: "AF"},
		"MV": {Code: "MV", Name: "Maldives", Continent: "AS"},
		"MW": {Code: "MW", Name: "Malawi", Continent: "AF"},
		"MX": {Code: "MX", Name: "Mexico", Continent: "NA"},
		"MY": {Code: "MY", Name: "Malaysia", Continent: "AS"},
		"MZ": {Code: "MZ", Name: "Mozambique", Continent: "AF"},
		"NA": {Code: "NA", Name: "Namibia", Continent: "AF"},
		"NE": {Code: "NE", Name: "Niger", Continent: "AF"},
		"NG": {Code: "NG", Name: "Nigeria", Continent: "AF"},
		"NI": {Code: "NI", Name: "Nicaragua", Continent: "NA"},
		"NL": {Code: "NL", Name: "Netherlands", Continent: "EU"},
		"NO": {Code: "NO", Name: "Norway", Continent: "EU"},
		"NP": {Code: "NP", Name: "Nepal", Continent: "AS"},
		"NR": {Code: "NR", Name: "Nauru", Continent: "OC"},
		"NZ": {Code: "NZ", Name: "New Zealand", Continent: "OC"},
		"OM": {Code: "OM", Name: "Oman", Continent: "AS"},
		"PA": {Code: "PA", Name: "Panama", Continent: "NA"},
		"PE": {Code: "PE", Name: "Peru", Continent: "SA"},
		"PG": {Code: "PG", Name: "Papua New Guinea", Continent: "OC"},
		"PH": {Code: "PH", Name: "Philippines", Continent: "AS"},
		"PK": {Code: "PK", Name: "Pakistan", Continent: "AS"},
		"PL": {Code: "PL", Name: "Poland", Continent: "EU"},
		"PT": {Code: "PT", Name: "Portugal", Continent: "EU"},
		"PW": {Code: "PW", Name: "Palau", Continent: "OC"},
		"PY": {Code: "PY", Name: "Paraguay", Continent: "SA"},
		"QA": {Code: "QA", Name: "Qatar", Continent: "AS"},
		"RO": {Code: "RO", Name: "Romania", Continent: "EU"},
		"RS": {Code: "RS", Name: "Serbia", Continent: "EU"},
		"RU": {Code: "RU", Name: "Russia", Continent: "EU"},
		"RW": {Code: "RW", Name: "Rwanda", Continent: "AF"},
		"SA": {Code: "SA", Name: "Saudi Arabia", Continent: "AS"},
		"SB": {Code: "SB", Name: "Solomon Islands", Continent: "OC"},
		"SC": {Code: "SC", Name: "Seychelles", Continent: "AF"},
		"SD": {Code: "SD", Name: "Sudan", Continent: "AF"},
		"SE": {Code: "SE", Name: "Sweden", Continent: "EU"},
		"SG": {Code: "SG", Name: "Singapore", Continent: "AS"},
		"SI": {Code: "SI", Name: "Slovenia", Continent: "EU"},
		"SK": {Code: "SK", Name: "Slovakia", Continent: "EU"},
		"SL": {Code: "SL", Name: "Sierra Leone", Continent: "AF"},
		"SM": {Code: "SM", Name: "San Marino", Continent: "EU"},
		"SN": {Code: "SN", Name: "Senegal", Continent: "AF"},
		"SO": {Code: "SO", Name: "Somalia", Continent: "AF"},
		"SR": {Code: "SR", Name: "Suriname", Continent: "SA"},
		"SS": {Code: "SS", Name: "South Sudan", Continent: "AF"},
		"ST": {Code: "ST", Name: "Sao Tome and Principe", Continent: "AF"},
		"SV": {Code: "SV", Name: "El Salvador", Continent: "NA"},
		"SY": {Code: "SY", Name: "Syria", Continent: "AS"},
		"SZ": {Code: "SZ", Name: "Eswatini", Continent: "AF"},
		"TD": {Code: "TD", Name: "Chad", Continent: "AF"},
		"TG": {Code: "TG", Name: "Togo", Continent: "AF"},
		"TH": {Code: "TH", Name: "Thailand", Continent: "AS"},
		"TJ": {Code: "TJ", Name: "Tajikistan", Continent: "AS"},
		"TL": {Code: "TL", Name: "Timor-Leste", Continent: "AS"},
		"TM": {Code: "TM", Name: "Turkmenistan", Continent: "AS"},
		"TN": {Code: "TN", Name: "Tunisia", Continent: "AF"},
		"TO": {Code: "TO", Name: "Tonga", Continent: "OC"},
		"TR": {Code: "TR", Name: "Turkey", Continent: "AS"},
		"TT": {Code: "TT", Name: "Trinidad and Tobago", Continent: "NA"},
		"TV": {Code: "TV", Name: "Tuvalu", Continent: "OC"},
		"TW": {Code: "TW", Name: "Taiwan", Continent: "AS"},
		"TZ": {Code: "TZ", Name: "Tanzania", Continent: "AF"},
		"UA": {Code: "UA", Name: "Ukraine", Continent: "EU"},
		"UG": {Code: "UG", Name: "Uganda", Continent: "AF"},
		"US": {Code: "US", Name: "United States", Continent: "NA"},
		"UY": {Code: "UY", Name: "Uruguay", Continent: "SA"},
		"UZ": {Code: "UZ", Name: "Uzbekistan", Continent: "AS"},
		"VA": {Code: "VA", Name: "Vatican City", Continent: "EU"},
		"VC": {Code: "VC", Name: "Saint Vincent and the Grenadines", Continent: "NA"},
		"VE": {Code: "VE", Name: "Venezuela", Continent: "SA"},
		"VN": {Code: "VN", Name: "Vietnam", Continent: "AS"},
		"VU": {Code: "VU", Name: "Vanuatu", Continent: "OC"},
		"WS": {Code: "WS", Name: "Samoa", Continent: "OC"},
		"YE": {Code: "YE", Name: "Yemen", Continent: "AS"},
		"ZA": {Code: "ZA", Name: "South Africa", Continent: "AF"},
		"ZM": {Code: "ZM", Name: "Zambia", Continent: "AF"},
		"ZW": {Code: "ZW", Name: "Zimbabwe", Continent: "AF"},
	}
}

// LoadDatabase loads a database from path (for backwards compatibility)
func (l *Lookup) LoadDatabase(path string) error {
	return l.LoadDatabases()
}
