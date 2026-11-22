package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/health"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/ssl"
)

var healthKeySeparators = regexp.MustCompile(`[^a-z0-9]+`)

func newHealthRegistry(server *Server) *health.Registry {
	checks := health.NewRegistry()
	if server == nil {
		return checks
	}
	registerCheck := func(key string, critical bool, fn health.CheckFunc) {
		if err := checks.Register(key, critical, fn); err != nil {
			slog.Error("health check registration failed", "key", key, "err", err)
		}
	}
	registerStat := func(key string, fn health.StatFunc) {
		if err := checks.RegisterStat(key, fn); err != nil {
			slog.Error("health stat registration failed", "key", key, "err", err)
		}
	}
	registry := server.registry
	var engines []search.Engine
	if registry != nil {
		engines = registry.GetAll()
		sort.Slice(engines, func(i, j int) bool { return engines[i].Name() < engines[j].Name() })
		for _, configured := range engines {
			current := configured
			key := "engine_" + healthKeySeparators.ReplaceAllString(strings.ToLower(current.Name()), "_")
			key = strings.Trim(key, "_")
			registerCheck(key, false, func() string {
				if !current.IsEnabled() {
					return "disabled"
				}
				return engineHealthStatus(current)
			})
		}
		if len(engines) > 0 {
			registerCheck("engines", true, func() string {
				okCount := 0
				failed := false
				enabledCount := 0
				for _, configured := range engines {
					if !configured.IsEnabled() {
						continue
					}
					enabledCount++
					switch engineHealthStatus(configured) {
					case "ok":
						okCount++
					case "degraded":
						okCount++
						failed = true
					default:
						failed = true
					}
				}
				if enabledCount == 0 || okCount == 0 {
					return "error"
				}
				if failed {
					return "degraded"
				}
				return "ok"
			})
		}
		registerStat("engines_total", func() int64 {
			var count int64
			for _, configured := range engines {
				if configured.IsEnabled() {
					count++
				}
			}
			return count
		})
		registerStat("engines_healthy", func() int64 {
			var count int64
			for _, configured := range engines {
				if configured.IsEnabled() && engineHealthStatus(configured) == "ok" {
					count++
				}
			}
			return count
		})
	}
	for _, database := range []string{"asn", "city_ipv4", "city_ipv6", "country"} {
		name := database
		registerCheck("geoip_"+name, false, func() string {
			if server.config == nil || !server.config.Server.GeoIP.Enabled || !geoIPDatabaseEnabled(server.config, name) {
				return "disabled"
			}
			if server.geoipLookup == nil {
				return "error"
			}
			return server.geoipLookup.DatabaseHealth()[name]
		})
	}
	if server.blocklistManager != nil {
		names := server.blocklistManager.HealthSourceNames()
		sort.Strings(names)
		for _, sourceName := range names {
			name := sourceName
			key := "blocklist_" + strings.Trim(healthKeySeparators.ReplaceAllString(strings.ToLower(name), "_"), "_")
			registerCheck(key, false, func() string {
				status, ok := server.blocklistManager.HealthStates()[name]
				if !ok {
					return "disabled"
				}
				return status
			})
		}
	}
	if server.cveManager != nil {
		registerCheck("data_nvd", false, func() string {
			if server.cveManager == nil {
				return "error"
			}
			return nvdHealthStatus(server.cveManager.Count(), server.cveManager.LastSync(), time.Now())
		})
	}
	registerCheck("data_trivy", false, func() string {
		if server.config == nil || !server.config.Data.Trivy.Enabled {
			return "disabled"
		}
		if server.cveManager == nil {
			return "error"
		}
		return server.cveManager.TrivyHealthStatus()
	})
	registerCheck("data_ssl_dns_providers", false, func() string {
		if server.config == nil || !server.config.Server.SSL.Enabled || !server.config.Server.SSL.LetsEncrypt.Enabled {
			return "disabled"
		}
		return ssl.DNSProviderCatalogHealth()
	})
	if server.i18nManager != nil {
		for _, language := range server.i18nManager.SupportedLanguageCodes() {
			code := language
			registerCheck("data_locale_"+code, false, func() string {
				if _, ok := i18n.GetLanguage(code); !ok {
					return "error"
				}
				data, err := fs.ReadFile(i18n.LocalesFS(), "locales/"+code+".json")
				if err != nil {
					return "error"
				}
				var translations map[string]any
				if err := json.Unmarshal(data, &translations); err != nil || len(translations) == 0 {
					return "error"
				}
				return "ok"
			})
		}
	}
	registerCheck("dependency_tls_certificate", false, func() string {
		if server.config == nil || !server.config.Server.SSL.Enabled {
			return "disabled"
		}
		if server.tlsManager == nil {
			return "error"
		}
		return server.tlsManager.HealthStatus()
	})
	return checks
}

func nvdHealthStatus(count int, lastSync, now time.Time) string {
	if count <= 0 {
		return "error"
	}
	if lastSync.IsZero() || now.Sub(lastSync) > 48*time.Hour {
		return "degraded"
	}
	return "ok"
}

func geoIPDatabaseEnabled(cfg *config.Config, name string) bool {
	switch name {
	case "asn":
		return cfg.Server.GeoIP.Databases.ASN
	case "country":
		return cfg.Server.GeoIP.Databases.Country
	case "city_ipv4", "city_ipv6":
		return cfg.Server.GeoIP.Databases.City
	default:
		return false
	}
}

func engineHealthStatus(configured search.Engine) string {
	provider, ok := configured.(interface{ GetHealth() search.EngineHealth })
	if !ok {
		return "error"
	}
	switch provider.GetHealth().Status {
	case "healthy", "unknown":
		return "ok"
	case "degraded":
		return "degraded"
	default:
		return "error"
	}
}
