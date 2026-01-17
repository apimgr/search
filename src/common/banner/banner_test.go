package banner

import (
	"testing"
)

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		AppName:     "search",
		Version:     "1.0.0",
		Mode:        "production",
		Debug:       false,
		URLs:        []string{"http://localhost:8080"},
		ShowSetup:   true,
		SetupToken:  "abc123",
		AdminPath:   "admin",
		Description: "Test app",
		SMTPStatus:  "Configured",
	}

	if cfg.AppName != "search" {
		t.Errorf("AppName = %q, want %q", cfg.AppName, "search")
	}
	if cfg.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", cfg.Version, "1.0.0")
	}
	if len(cfg.URLs) != 1 {
		t.Errorf("URLs length = %d, want 1", len(cfg.URLs))
	}
}

func TestPrint(t *testing.T) {
	// Just test that Print doesn't panic
	cfg := Config{
		AppName: "test",
		Version: "1.0.0",
		URLs:    []string{"http://localhost:8080"},
	}
	Print(cfg)
}

func TestPrintFull(t *testing.T) {
	cfg := Config{
		AppName:    "search",
		Version:    "1.0.0",
		Mode:       "production",
		URLs:       []string{"http://localhost:8080"},
		ShowSetup:  true,
		SetupToken: "test-token",
		AdminPath:  "admin",
		SMTPStatus: "Auto-detected",
	}
	// Just verify it doesn't panic
	printFull(cfg)
}

func TestPrintCompact(t *testing.T) {
	cfg := Config{
		AppName:    "search",
		Version:    "1.0.0",
		Mode:       "production",
		URLs:       []string{"http://localhost:8080"},
		ShowSetup:  true,
		SetupToken: "test-token",
	}
	// Just verify it doesn't panic
	printCompact(cfg)

	// Test development mode
	cfg.Mode = "development"
	printCompact(cfg)
}

func TestPrintMinimal(t *testing.T) {
	cfg := Config{
		AppName:    "search",
		Version:    "1.0.0",
		URLs:       []string{"http://localhost:8080"},
		ShowSetup:  true,
		SetupToken: "test-token",
	}
	// Just verify it doesn't panic
	printMinimal(cfg)

	// Without URLs
	cfg.URLs = nil
	printMinimal(cfg)
}

func TestPrintMicro(t *testing.T) {
	cfg := Config{
		AppName: "search",
		URLs:    []string{"http://localhost:8080"},
	}
	// Just verify it doesn't panic
	printMicro(cfg)

	// Without URLs
	cfg.URLs = nil
	printMicro(cfg)
}

func TestPrintBoxLine(t *testing.T) {
	// Just verify it doesn't panic with various inputs
	printBoxLine("║", "test content", 72)
	printBoxLine("║", "", 72)
	printBoxLine("║", "very long content that might exceed the box width maybe", 30)
}

func TestExtractHostPort(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"http://localhost:8080", "localhost:8080"},
		{"https://example.com:443", "example.com:443"},
		{"http://localhost:8080/path", "localhost:8080"},
		{"https://example.com/path/to/page", "example.com"},
		{"localhost:3000", "localhost:3000"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := extractHostPort(tt.url)
			if got != tt.want {
				t.Errorf("extractHostPort(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestPrintShutdown(t *testing.T) {
	// Just verify it doesn't panic
	PrintShutdown("test-app")
}

func TestPrintError(t *testing.T) {
	// Just verify it doesn't panic
	PrintError(nil)
	PrintError(&testError{msg: "test error"})
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestPrintSuccess(t *testing.T) {
	// Just verify it doesn't panic
	PrintSuccess("operation completed")
}

func TestPrintWarning(t *testing.T) {
	// Just verify it doesn't panic
	PrintWarning("something might be wrong")
}

func TestPrintInfo(t *testing.T) {
	// Just verify it doesn't panic
	PrintInfo("some information")
}

func TestBoxWidth(t *testing.T) {
	if boxWidth != 72 {
		t.Errorf("boxWidth = %d, want 72", boxWidth)
	}
}
