package clicfg

import (
	"os"
	"path/filepath"
	"testing"
)

// reset clears global state between tests so they do not leak into each other.
func reset(t *testing.T) {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
}

func TestSetAndGetString(t *testing.T) {
	reset(t)
	Set("client.name", "search")
	if got := GetString("client.name"); got != "search" {
		t.Errorf("GetString() = %q, want %q", got, "search")
	}
	// Case-insensitive key access mirrors viper.
	if got := GetString("CLIENT.NAME"); got != "search" {
		t.Errorf("GetString() case-insensitive = %q, want %q", got, "search")
	}
}

func TestGetStringUnset(t *testing.T) {
	reset(t)
	if got := GetString("missing"); got != "" {
		t.Errorf("GetString(missing) = %q, want empty", got)
	}
}

func TestDefaultsLowestPriority(t *testing.T) {
	reset(t)
	SetDefault("port", 8080)
	if got := GetInt("port"); got != 8080 {
		t.Errorf("GetInt(default) = %d, want 8080", got)
	}
	Set("port", 9090)
	if got := GetInt("port"); got != 9090 {
		t.Errorf("GetInt(override) = %d, want 9090", got)
	}
}

func TestIsSetSemantics(t *testing.T) {
	reset(t)
	SetDefault("only.default", "x")
	if IsSet("only.default") {
		t.Error("IsSet should be false for a default-only key")
	}
	Set("explicit", "y")
	if !IsSet("explicit") {
		t.Error("IsSet should be true for an explicitly set key")
	}
	if IsSet("never.touched") {
		t.Error("IsSet should be false for an untouched key")
	}
}

func TestGetBool(t *testing.T) {
	reset(t)
	tests := []struct {
		name string
		set  any
		want bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"string true", "true", true},
		{"string yes is not parsebool", "yes", false},
		{"int nonzero", 1, true},
		{"int zero", 0, false},
		{"float nonzero", 2.5, true},
		{"float zero", 0.0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Set("k", tt.set)
			if got := GetBool("k"); got != tt.want {
				t.Errorf("GetBool(%v) = %v, want %v", tt.set, got, tt.want)
			}
		})
	}
}

func TestGetBoolUnset(t *testing.T) {
	reset(t)
	if GetBool("missing") {
		t.Error("GetBool(missing) should be false")
	}
}

func TestGetInt(t *testing.T) {
	reset(t)
	tests := []struct {
		name string
		set  any
		want int
	}{
		{"int", 42, 42},
		{"int64", int64(7), 7},
		{"float", 3.9, 3},
		{"bool true", true, 1},
		{"bool false", false, 0},
		{"string number", "15", 15},
		{"string bad", "nope", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Set("k", tt.set)
			if got := GetInt("k"); got != tt.want {
				t.Errorf("GetInt(%v) = %d, want %d", tt.set, got, tt.want)
			}
		})
	}
}

func TestGetIntUnset(t *testing.T) {
	reset(t)
	if got := GetInt("missing"); got != 0 {
		t.Errorf("GetInt(missing) = %d, want 0", got)
	}
}

func TestGetStringConversions(t *testing.T) {
	reset(t)
	tests := []struct {
		name string
		set  any
		want string
	}{
		{"bool", true, "true"},
		{"int", 5, "5"},
		{"int64", int64(99), "99"},
		{"float", 1.5, "1.5"},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Set("k", tt.set)
			if got := GetString("k"); got != tt.want {
				t.Errorf("GetString(%v) = %q, want %q", tt.set, got, tt.want)
			}
		})
	}
}

func TestReadInConfigExplicitFile(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "server.yml")
	content := "client:\n  name: fromfile\n  port: 1234\nnested:\n  deep:\n    flag: true\n"
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	SetConfigFile(file)
	if err := ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig() error = %v", err)
	}
	if got := GetString("client.name"); got != "fromfile" {
		t.Errorf("GetString(client.name) = %q, want fromfile", got)
	}
	if got := GetInt("client.port"); got != 1234 {
		t.Errorf("GetInt(client.port) = %d, want 1234", got)
	}
	if !GetBool("nested.deep.flag") {
		t.Error("GetBool(nested.deep.flag) should be true")
	}
	if !IsSet("client.name") {
		t.Error("loaded keys should report IsSet true")
	}
}

func TestReadInConfigSearchPath(t *testing.T) {
	reset(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("a: b\n"), 0600); err != nil {
		t.Fatal(err)
	}
	AddConfigPath(dir)
	SetConfigName("config")
	SetConfigType("yaml")
	if err := ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig() error = %v", err)
	}
	if got := GetString("a"); got != "b" {
		t.Errorf("GetString(a) = %q, want b", got)
	}
}

func TestReadInConfigNoPathIsNoError(t *testing.T) {
	reset(t)
	if err := ReadInConfig(); err != nil {
		t.Errorf("ReadInConfig() with no path should be nil, got %v", err)
	}
}

func TestReadInConfigMissingFileErrors(t *testing.T) {
	reset(t)
	SetConfigFile(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err := ReadInConfig(); err == nil {
		t.Error("ReadInConfig() with missing explicit file should error")
	}
}

func TestReadInConfigBadYAMLErrors(t *testing.T) {
	reset(t)
	file := filepath.Join(t.TempDir(), "bad.yml")
	if err := os.WriteFile(file, []byte("::: not yaml :::\n  - broken"), 0600); err != nil {
		t.Fatal(err)
	}
	SetConfigFile(file)
	if err := ReadInConfig(); err == nil {
		t.Error("ReadInConfig() with malformed YAML should error")
	}
}

func TestWriteConfigAsRoundTrip(t *testing.T) {
	reset(t)
	SetDefault("client.timeout", 30)
	Set("client.name", "search")
	Set("client.port", 8080)
	out := filepath.Join(t.TempDir(), "sub", "out.yml")
	if err := WriteConfigAs(out); err != nil {
		t.Fatalf("WriteConfigAs() error = %v", err)
	}
	// Read it back into a fresh store and verify merged values persisted.
	Reset()
	SetConfigFile(out)
	if err := ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig() after write error = %v", err)
	}
	if got := GetString("client.name"); got != "search" {
		t.Errorf("round-trip client.name = %q, want search", got)
	}
	if got := GetInt("client.port"); got != 8080 {
		t.Errorf("round-trip client.port = %d, want 8080", got)
	}
	if got := GetInt("client.timeout"); got != 30 {
		t.Errorf("round-trip client.timeout = %d, want 30", got)
	}
}

func TestResetClearsState(t *testing.T) {
	reset(t)
	Set("k", "v")
	SetConfigFile("/tmp/x.yml")
	AddConfigPath("/tmp")
	SetConfigName("x")
	Reset()
	if IsSet("k") {
		t.Error("Reset should clear set keys")
	}
	if GetString("k") != "" {
		t.Error("Reset should clear values")
	}
}
