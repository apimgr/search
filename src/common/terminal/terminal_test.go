package terminal

import (
	"testing"
)

func TestGetSize(t *testing.T) {
	size := GetSize()
	// Size should return something reasonable
	// In non-terminal environments, defaults are used
	if size.Cols < 0 {
		t.Errorf("Cols = %d, should not be negative", size.Cols)
	}
	if size.Rows < 0 {
		t.Errorf("Rows = %d, should not be negative", size.Rows)
	}
	// Mode should be set
	if size.Mode < SizeModeMicro || size.Mode > SizeModeMassive {
		t.Errorf("Mode = %d, should be valid SizeMode", size.Mode)
	}
}

func TestNewResizeHandler(t *testing.T) {
	h := NewResizeHandler()
	if h == nil {
		t.Fatal("NewResizeHandler() returned nil")
	}
}

func TestResizeHandlerCurrentSize(t *testing.T) {
	h := NewResizeHandler()
	size := h.CurrentSize()
	if size.Cols < 0 || size.Rows < 0 {
		t.Errorf("CurrentSize() returned invalid size: %+v", size)
	}
}

func TestResizeHandlerOnResize(t *testing.T) {
	h := NewResizeHandler()
	called := false
	h.OnResize(func(s Size) {
		called = true
	})
	// Just verify it doesn't panic
	_ = called // callback is registered, won't be called unless resize happens
}

func TestResizeHandlerRefresh(t *testing.T) {
	h := NewResizeHandler()
	h.Refresh()
	// Just verify it doesn't panic
}

func TestSizeModeString(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want string
	}{
		{SizeModeMicro, "micro"},
		{SizeModeMinimal, "minimal"},
		{SizeModeCompact, "compact"},
		{SizeModeStandard, "standard"},
		{SizeModeWide, "wide"},
		{SizeModeUltrawide, "ultrawide"},
		{SizeModeMassive, "massive"},
		{SizeMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("SizeMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestCalculateMode(t *testing.T) {
	tests := []struct {
		name string
		cols int
		rows int
		want SizeMode
	}{
		{"micro cols", 30, 30, SizeModeMicro},
		{"micro rows", 100, 8, SizeModeMicro},
		{"minimal cols", 50, 30, SizeModeMinimal},
		{"minimal rows", 100, 12, SizeModeMinimal},
		{"compact cols", 70, 30, SizeModeCompact},
		{"compact rows", 100, 20, SizeModeCompact},
		{"standard", 80, 24, SizeModeStandard},
		{"standard large", 100, 35, SizeModeStandard},
		{"wide", 150, 50, SizeModeWide},
		{"ultrawide", 250, 70, SizeModeUltrawide},
		{"massive", 500, 100, SizeModeMassive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateMode(tt.cols, tt.rows)
			if got != tt.want {
				t.Errorf("calculateMode(%d, %d) = %v, want %v", tt.cols, tt.rows, got, tt.want)
			}
		})
	}
}

func TestSizeModeShowASCIIArt(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, false},
		{SizeModeCompact, false},
		{SizeModeStandard, true},
		{SizeModeWide, true},
		{SizeModeUltrawide, true},
		{SizeModeMassive, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.ShowASCIIArt()
			if got != tt.want {
				t.Errorf("SizeMode(%d).ShowASCIIArt() = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSizeModeShowBorders(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, false},
		{SizeModeCompact, true},
		{SizeModeStandard, true},
		{SizeModeWide, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.ShowBorders()
			if got != tt.want {
				t.Errorf("SizeMode(%d).ShowBorders() = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSizeModeShowSidebar(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, false},
		{SizeModeCompact, false},
		{SizeModeStandard, false},
		{SizeModeWide, true},
		{SizeModeUltrawide, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.ShowSidebar()
			if got != tt.want {
				t.Errorf("SizeMode(%d).ShowSidebar() = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSizeModeShowIcons(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, true},
		{SizeModeCompact, true},
		{SizeModeStandard, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.ShowIcons()
			if got != tt.want {
				t.Errorf("SizeMode(%d).ShowIcons() = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSizeModeShowFullInfo(t *testing.T) {
	tests := []struct {
		mode SizeMode
		want bool
	}{
		{SizeModeMicro, false},
		{SizeModeMinimal, false},
		{SizeModeCompact, false},
		{SizeModeStandard, true},
		{SizeModeWide, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.ShowFullInfo()
			if got != tt.want {
				t.Errorf("SizeMode(%d).ShowFullInfo() = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	// This test just verifies the function doesn't panic
	// In CI environments, this will return false
	result := IsTerminal()
	_ = result // Result depends on environment
}

func TestUnicodeSymbols(t *testing.T) {
	sym := UnicodeSymbols
	if sym.Success == "" {
		t.Error("Success symbol should not be empty")
	}
	if sym.Error == "" {
		t.Error("Error symbol should not be empty")
	}
	if sym.Warning == "" {
		t.Error("Warning symbol should not be empty")
	}
	if sym.Info == "" {
		t.Error("Info symbol should not be empty")
	}
	if len(sym.Spinner) == 0 {
		t.Error("Spinner should have frames")
	}
	if sym.Check == "" {
		t.Error("Check symbol should not be empty")
	}
}

func TestASCIISymbols(t *testing.T) {
	sym := ASCIISymbols
	if sym.Success == "" {
		t.Error("ASCII Success symbol should not be empty")
	}
	if sym.Error == "" {
		t.Error("ASCII Error symbol should not be empty")
	}
	if len(sym.Spinner) == 0 {
		t.Error("ASCII Spinner should have frames")
	}
	if sym.BoxTopLeft == "" {
		t.Error("ASCII BoxTopLeft should not be empty")
	}
}

func TestGetSymbols(t *testing.T) {
	sym := GetSymbols()
	// Should return either Unicode or ASCII symbols based on terminal
	if sym.Success == "" {
		t.Error("GetSymbols() Success should not be empty")
	}
	if sym.Check == "" {
		t.Error("GetSymbols() Check should not be empty")
	}
}

func TestSupportsUnicode(t *testing.T) {
	// This just verifies the function doesn't panic
	result := supportsUnicode()
	_ = result // Result depends on environment
}

func TestSizeStruct(t *testing.T) {
	size := Size{
		Cols: 80,
		Rows: 24,
		Mode: SizeModeStandard,
	}

	if size.Cols != 80 {
		t.Errorf("Cols = %d, want 80", size.Cols)
	}
	if size.Rows != 24 {
		t.Errorf("Rows = %d, want 24", size.Rows)
	}
	if size.Mode != SizeModeStandard {
		t.Errorf("Mode = %v, want SizeModeStandard", size.Mode)
	}
}

func TestSymbolsStruct(t *testing.T) {
	sym := Symbols{
		Success:       "✓",
		Error:         "✗",
		BoxHorizontal: "─",
		ArrowRight:    "→",
		Bullet:        "•",
	}

	if sym.Success != "✓" {
		t.Errorf("Success = %q, want ✓", sym.Success)
	}
	if sym.ArrowRight != "→" {
		t.Errorf("ArrowRight = %q, want →", sym.ArrowRight)
	}
}
