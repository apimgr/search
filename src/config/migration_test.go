package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apimgr/search/src/database"
	"gopkg.in/yaml.v3"
)

func TestMigrateConfigAreasMovesLegacyAndSecrets(t *testing.T) {
	dir := t.TempDir()
	dropInDir := filepath.Join(dir, "yaml.d")
	if err := os.MkdirAll(dropInDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverPath := filepath.Join(dir, "server.yml")
	contents := "server:\n  port: 8080\n  token: operator-token\n  title: Legacy title\n  web:\n    csp:\n      frame_src_extra: https://frame.example\n  metrics:\n    endpoint: /old-metrics\nsearch:\n  safe_search: true\n"
	if err := os.WriteFile(serverPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dropInDir, "branding.yml"), []byte("server:\n  branding:\n    title: Existing title\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := migrateConfigAreas(serverPath)
	if err != nil || !changed {
		t.Fatalf("migrateConfigAreas() = %v, %v", changed, err)
	}
	server, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(server), "# "+serverConfigHeader) {
		t.Fatalf("server header missing: %s", server)
	}
	if strings.Contains(string(server), "token:") || strings.Contains(string(server), "title:") || strings.Contains(string(server), "metrics:") {
		t.Fatalf("moved keys remain in server.yml: %s", server)
	}
	backup, err := os.Stat(serverPath + ".bak")
	if err != nil || backup.Mode().Perm() != 0o600 {
		t.Fatalf("migration backup mode = %v, err = %v", backup, err)
	}
	branding, err := os.ReadFile(filepath.Join(dropInDir, "branding.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var brandingConfig struct {
		Server struct {
			Branding struct {
				Title string `yaml:"title"`
			} `yaml:"branding"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(branding, &brandingConfig); err != nil {
		t.Fatal(err)
	}
	if brandingConfig.Server.Branding.Title != "Existing title" {
		t.Fatalf("existing area value lost: %s", branding)
	}
	secrets, err := os.ReadFile(filepath.Join(dir, "secrets.yml"))
	if err != nil || !strings.Contains(string(secrets), "operator-token") {
		t.Fatalf("operator token not moved to secrets.yml: %s, err %v", secrets, err)
	}
	web, err := os.ReadFile(filepath.Join(dropInDir, "web.yml"))
	if err != nil || !strings.Contains(string(web), "frame_src_extra") {
		t.Fatalf("legacy server.web not moved to web.yml: %s, err %v", web, err)
	}
}

func TestMigrateRemovedGeoIPSettingsAndPreserveComplianceUnknownKeyInServerYML(t *testing.T) {
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "server.yml")
	contents := `server:
  port: 64580
  geoip:
    enabled: true
    dir: /old/geoip
    databases:
      country: true
    deny_countries: [CN]
  compliance:
    enabled: true
    gdpr: false
`
	if err := os.WriteFile(serverPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := migrateConfigAreas(serverPath)
	if err != nil || !changed {
		t.Fatalf("migrateConfigAreas() = %v, %v", changed, err)
	}

	security, err := os.ReadFile(filepath.Join(dir, "yaml.d", "security.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(security), "enabled: true") || !strings.Contains(string(security), "deny_countries") {
		t.Fatalf("supported GeoIP settings were not preserved: %s", security)
	}
	if strings.Contains(string(security), "dir:") || strings.Contains(string(security), "databases:") {
		t.Fatalf("removed GeoIP settings remained after migration: %s", security)
	}
	privacy, err := os.ReadFile(filepath.Join(dir, "yaml.d", "privacy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(privacy), "enabled: true") || !strings.Contains(string(privacy), "gdpr: false") {
		t.Fatalf("unmapped compliance key or standard was changed: %s", privacy)
	}
}

func TestMigrateRemovedGeoIPSettingsFromDropIns(t *testing.T) {
	dir := t.TempDir()
	dropInDir := filepath.Join(dir, "yaml.d")
	if err := os.MkdirAll(dropInDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(serverPath, []byte("server:\n  port: 64580\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dropInDir, "security.yml"), []byte("server:\n  geoip:\n    enabled: true\n    dir: /old/geoip\n    databases:\n      country: true\n    allow_countries: [US]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dropInDir, "privacy.yml"), []byte("server:\n  compliance:\n    enabled: true\n    hipaa: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := migrateConfigAreas(serverPath)
	if err != nil || !changed {
		t.Fatalf("migrateConfigAreas() = %v, %v", changed, err)
	}

	security, err := os.ReadFile(filepath.Join(dropInDir, "security.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(security), "dir:") || strings.Contains(string(security), "databases:") {
		t.Fatalf("removed GeoIP settings remained in drop-in: %s", security)
	}
	if !strings.Contains(string(security), "allow_countries") {
		t.Fatalf("supported GeoIP settings were lost from drop-in: %s", security)
	}
	privacy, err := os.ReadFile(filepath.Join(dropInDir, "privacy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(privacy), "enabled: true") || !strings.Contains(string(privacy), "hipaa: false") {
		t.Fatalf("unmapped compliance key or standard changed in drop-in: %s", privacy)
	}
}

func TestMigrateTopLevelCLIConfigToMaintenanceArea(t *testing.T) {
	dir := t.TempDir()
	dropInDir := filepath.Join(dir, "yaml.d")
	if err := os.MkdirAll(dropInDir, 0o700); err != nil {
		t.Fatal(err)
	}
	serverPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(serverPath, []byte("server:\n  cluster:\n    enabled: true\ncli:\n  binary_download:\n    dir: /data/search/cli\n    require_auth: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := migrateConfigAreas(serverPath)
	if err != nil || !changed {
		t.Fatalf("migrateConfigAreas() = %v, %v", changed, err)
	}
	server, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(server), "binary_download:") {
		t.Fatalf("CLI settings remained in server.yml: %s", server)
	}
	if strings.Contains(string(server), "cluster:") {
		t.Fatalf("unsupported cluster setting remained in server.yml: %s", server)
	}
	app, err := os.ReadFile(filepath.Join(dropInDir, "app.yml"))
	if err == nil && strings.Contains(string(app), "cluster:") {
		t.Fatalf("unsupported cluster setting was routed to app.yml: %s", app)
	}
	maintenance, err := os.ReadFile(filepath.Join(dropInDir, "maintenance.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(maintenance), "cli:") || !strings.Contains(string(maintenance), "binary_download:") || !strings.Contains(string(maintenance), "require_auth: true") {
		t.Fatalf("CLI settings not migrated to maintenance.yml: %s", maintenance)
	}
}

func TestImportLegacyAppSecretStoresBeforeRemovingConfigValue(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.yml")
	const legacySecret = "legacy-installation-secret"
	const legacyPreviousKey = "legacy-previous-encryption-key"
	if err := os.WriteFile(configPath, []byte("server:\n  security:\n    installation_secret: "+legacySecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secrets.yml"), []byte("server:\n  security:\n    encryption_key_previous: "+legacyPreviousKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbManager, err := database.NewDatabaseManager(&database.Config{
		Driver:  "sqlite",
		DataDir: filepath.Join(dir, "db"),
		MaxOpen: 1,
		MaxIdle: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dbManager.Close()
	if err := database.InitSchema(context.Background(), dbManager); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ImportLegacyAppSecrets(context.Background(), dbManager.ServerDB()); err != nil {
		t.Fatal(err)
	}
	stored, err := database.GetSecret(context.Background(), dbManager.ServerDB(), "installation_secret")
	if err != nil || stored.Value != legacySecret {
		t.Fatalf("stored installation secret = %#v, err %v", stored, err)
	}
	storedPrevious, err := database.GetSecret(context.Background(), dbManager.ServerDB(), "encryption_key_previous")
	if err != nil || storedPrevious.Value != legacyPreviousKey {
		t.Fatalf("stored previous encryption key = %#v, err %v", storedPrevious, err)
	}
	remaining, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(remaining), legacySecret) {
		t.Fatalf("legacy secret remained in config after database import: %s", remaining)
	}
	remaining, err = os.ReadFile(filepath.Join(dir, "secrets.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(remaining), legacyPreviousKey) {
		t.Fatalf("legacy secret remained in secrets.yml after database import: %s", remaining)
	}
}

func TestMigrateLegacyHTTPSPort(t *testing.T) {
	for _, test := range []struct {
		name, input, want string
	}{
		{name: "folds single port", input: "server:\n  port: 8080\n  https_port: 8443\n", want: "port: \"8080,8443\""},
		{name: "keeps dual port", input: "server:\n  port: \"8080,9443\"\n  https_port: 8443\n", want: "port: \"8080,9443\""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document yaml.Node
			if err := yaml.Unmarshal([]byte(test.input), &document); err != nil {
				t.Fatal(err)
			}
			if !migrateLegacyHTTPSPort(&document) {
				t.Fatal("expected legacy port migration")
			}
			encoded, err := yaml.Marshal(&document)
			if err != nil {
				t.Fatal(err)
			}
			port := mappingValue(mappingValue(configYAMLRoot(&document), "server"), "port")
			if port == nil || port.Value != strings.Trim(test.want[len("port: "):], `"`) || port.Tag != "!!str" || strings.Contains(string(encoded), "https_port") {
				t.Fatalf("unexpected migrated config: %s", encoded)
			}
		})
	}
}
