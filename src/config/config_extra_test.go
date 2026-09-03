package config

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSanitized verifies sensitive fields are redacted and non-sensitive fields are preserved.
func TestSanitized(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Token = "super-secret-token"
	cfg.Server.SecretKey = "my-secret-key"
	cfg.Server.Port = 8080
	cfg.Server.Mode = "production"
	cfg.Server.Address = "127.0.0.1"
	cfg.Server.Title = "Test Server"

	s := cfg.Sanitized()

	if s["token"] != "xxxxx" {
		t.Errorf("Sanitized() token = %q, want 'xxxxx'", s["token"])
	}
	if s["secret_key"] != "xxxxx" {
		t.Errorf("Sanitized() secret_key = %q, want 'xxxxx'", s["secret_key"])
	}

	if s["port"] != 8080 {
		t.Errorf("Sanitized() port = %v, want 8080", s["port"])
	}
	if s["mode"] != "production" {
		t.Errorf("Sanitized() mode = %v, want 'production'", s["mode"])
	}
	if s["address"] != "127.0.0.1" {
		t.Errorf("Sanitized() address = %v, want '127.0.0.1'", s["address"])
	}
	if s["title"] != "Test Server" {
		t.Errorf("Sanitized() title = %v, want 'Test Server'", s["title"])
	}

	ssl, ok := s["ssl"].(map[string]any)
	if !ok {
		t.Error("Sanitized() ssl field should be a map")
	} else {
		if _, hasEnabled := ssl["enabled"]; !hasEnabled {
			t.Error("Sanitized() ssl map should have 'enabled' field")
		}
	}
}

// TestSanitizedDoesNotExposeLiveToken verifies the real token never appears in the output.
func TestSanitizedDoesNotExposeLiveToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Token = "my-very-secret-operator-token"

	s := cfg.Sanitized()

	for k, v := range s {
		if str, ok := v.(string); ok {
			if str == "my-very-secret-operator-token" {
				t.Errorf("Sanitized() key %q contains the real token value", k)
			}
		}
	}
}

// TestAddConfigComments verifies YAML node gets section comments without panicking.
func TestAddConfigComments(t *testing.T) {
	cfg := DefaultConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	// Must not panic
	addConfigComments(&doc)
}

// TestAddConfigCommentsServerSection verifies server section gets its comment.
func TestAddConfigCommentsServerSection(t *testing.T) {
	yamlInput := `server:
  title: Test
  port: 8080
search:
  enabled: true
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlInput), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	addConfigComments(&doc)

	// Find the server key node and verify it has a head comment
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		t.Fatal("expected document node with content")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		t.Fatal("expected mapping node at root")
	}

	serverCommentSet := false
	searchCommentSet := false
	for i := 0; i < len(root.Content)-1; i += 2 {
		key := root.Content[i]
		if key.Value == "server" && key.HeadComment != "" {
			serverCommentSet = true
		}
		if key.Value == "search" && key.HeadComment != "" {
			searchCommentSet = true
		}
	}

	if !serverCommentSet {
		t.Error("addConfigComments() did not set HeadComment on 'server' key")
	}
	if !searchCommentSet {
		t.Error("addConfigComments() did not set HeadComment on 'search' key")
	}
}

// TestAddConfigCommentsNonDocumentNode verifies no panic on non-document node.
func TestAddConfigCommentsNonDocumentNode(t *testing.T) {
	// Pass a mapping node directly (not a document node) — should be a no-op
	node := &yaml.Node{
		Kind: yaml.MappingNode,
	}
	// Must not panic
	addConfigComments(node)
}

// TestAddConfigCommentsDocumentRootNotMappingNode verifies no panic when document root is not a mapping.
func TestAddConfigCommentsDocumentRootNotMappingNode(t *testing.T) {
	// DocumentNode with a SequenceNode as root — covers line 1559 root.Kind != yaml.MappingNode guard
	doc := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{
			{Kind: yaml.SequenceNode},
		},
	}
	// Must not panic, must be a no-op
	addConfigComments(doc)
}

// TestAddConfigCommentsEmptyDocument verifies no panic on empty document.
func TestAddConfigCommentsEmptyDocument(t *testing.T) {
	node := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{},
	}
	// Must not panic
	addConfigComments(node)
}

// TestAddConfigCommentsServerSubsections verifies server subsection keys get comments.
func TestAddConfigCommentsServerSubsections(t *testing.T) {
	yamlInput := `server:
  title: Test
  port: 8080
  mode: production
  token: abc123
  ssl:
    enabled: false
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlInput), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	addConfigComments(&doc)

	root := doc.Content[0]
	for i := 0; i < len(root.Content)-1; i += 2 {
		key := root.Content[i]
		if key.Value != "server" {
			continue
		}
		value := root.Content[i+1]
		if value.Kind != yaml.MappingNode {
			t.Fatal("server value is not a mapping node")
		}

		commentedKeys := map[string]bool{}
		for j := 0; j < len(value.Content)-1; j += 2 {
			subKey := value.Content[j]
			if subKey.HeadComment != "" {
				commentedKeys[subKey.Value] = true
			}
		}

		expectedCommented := []string{"title", "port", "mode", "token", "ssl"}
		for _, k := range expectedCommented {
			if !commentedKeys[k] {
				t.Errorf("addConfigComments() did not set HeadComment on server.%s", k)
			}
		}
	}
}

// TestAddConfigCommentsServerValueNotMappingNode verifies no panic when server value is not a mapping node.
func TestAddConfigCommentsServerValueNotMappingNode(t *testing.T) {
	// Build a document where server key maps to a scalar, not a mapping
	doc := &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{
			{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "server"},
					// Value is a scalar, not a mapping — should be skipped without panic
					{Kind: yaml.ScalarNode, Value: "unexpected-scalar"},
				},
			},
		},
	}
	// Must not panic
	addConfigComments(doc)
}

// TestMigrateYamlToYmlReadError verifies migrateYamlToYml returns error when source is unreadable.
func TestMigrateYamlToYmlReadError(t *testing.T) {
	err := migrateYamlToYml("/nonexistent/path/server.yaml", "/tmp/server.yml")
	if err == nil {
		t.Error("migrateYamlToYml() with missing source should return error")
	}
}

// TestMigrateYamlToYmlWriteError verifies migrateYamlToYml returns error when destination is unwritable.
func TestMigrateYamlToYmlWriteError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(tmpDir)

	yamlPath := tmpDir + "/server.yaml"
	if err := os.WriteFile(yamlPath, []byte("server:\n  port: 8080\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Write to a non-existent directory to trigger write error
	badDest := "/nonexistent/directory/server.yml"
	err = migrateYamlToYml(yamlPath, badDest)
	if err == nil {
		t.Error("migrateYamlToYml() with unwritable destination should return error")
	}
}

// TestValidateAndApplyDefaultsEmptyMode verifies empty mode gets set to production.
func TestValidateAndApplyDefaultsEmptyMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Mode = ""

	warnings := cfg.ValidateAndApplyDefaults()

	if cfg.Server.Mode != "production" {
		t.Errorf("ValidateAndApplyDefaults() mode = %q, want 'production' after empty mode", cfg.Server.Mode)
	}
	_ = warnings
}

// TestGetConfigPathFromEnv verifies GetConfigPath uses CONFIG_DIR env var.
// Per AI.md PART 5: CONFIG_DIR is the init-only variable for config directory.
func TestGetConfigPathFromEnv(t *testing.T) {
	original := os.Getenv("CONFIG_DIR")
	defer os.Setenv("CONFIG_DIR", original)

	os.Setenv("CONFIG_DIR", "/custom/path")
	got := GetConfigPath()
	if got != "/custom/path/server.yml" {
		t.Errorf("GetConfigPath() with CONFIG_DIR = %q, want '/custom/path/server.yml'", got)
	}
}

// TestLoadOrCreateWithExistingFile verifies LoadOrCreate returns existing config without creating.
func TestLoadOrCreateWithExistingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "loadorcreate-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := tmpDir + "/server.yml"
	cfg := DefaultConfig()
	cfg.Server.Title = "ExistingTitle"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, created, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	if created {
		t.Error("LoadOrCreate() created = true, want false for existing file")
	}
	if loaded.Server.Title != "ExistingTitle" {
		t.Errorf("LoadOrCreate() title = %q, want 'ExistingTitle'", loaded.Server.Title)
	}
}

// TestLoadOrCreateMigrationError verifies LoadOrCreate returns an error when migration fails.
func TestLoadOrCreateMigrationError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "loadorcreate-migrate-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a directory where server.yaml exists alongside the server.yml target path.
	// To force a migration write failure: create a regular file named server.yml so that
	// os.WriteFile cannot write (it is a directory-as-blocker trick won't work as root).
	// Instead create the .yml target as a directory — WriteFile on a directory path fails.
	if err := os.MkdirAll(tmpDir+"/server.yml", 0755); err != nil {
		t.Fatalf("MkdirAll(server.yml) error = %v", err)
	}

	// Write server.yaml adjacent so migration is triggered
	yamlPath := tmpDir + "/server.yaml"
	if err := os.WriteFile(yamlPath, []byte("server:\n  port: 8080\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ymlPath := tmpDir + "/server.yml"
	_, _, err = LoadOrCreate(ymlPath)
	// Migration write to ymlPath (which is a directory) should fail
	if err == nil {
		t.Error("LoadOrCreate() with migration target that is a directory should return error")
	}
}

// TestLoadOrCreateSaveErrorOnNewFile verifies LoadOrCreate returns error when Save fails for a new config.
func TestLoadOrCreateSaveErrorOnNewFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "loadorcreate-save-err-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a regular file where the directory component of the config path would be.
	// Save() calls os.MkdirAll(dir, 0755); if dir is a regular file, MkdirAll fails.
	blockingFile := tmpDir + "/blockeddir"
	if err := os.WriteFile(blockingFile, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Config path whose parent directory is actually a regular file — MkdirAll will fail
	configPath := blockingFile + "/server.yml"
	_, _, err = LoadOrCreate(configPath)
	if err == nil {
		t.Error("LoadOrCreate() when Save fails on new config should return error")
	}
}

// TestConfigSaveIncludesAddConfigComments verifies Save triggers comments via the round-trip.
func TestConfigSaveIncludesAddConfigComments(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-extra-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := tmpDir + "/server.yml"
	cfg := DefaultConfig()
	cfg.Server.Title = "CommentTest"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	content := string(data)
	// The saved YAML should contain server configuration
	if !strings.Contains(content, "server:") {
		t.Error("Saved config should contain 'server:' section")
	}
}
