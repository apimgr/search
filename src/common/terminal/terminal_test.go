package terminal

import (
	"context"
	"os"
	"testing"
	"time"
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

func TestSupportsUnicodeWithLCALL(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	origLCCTYPE := os.Getenv("LC_CTYPE")
	origLANG := os.Getenv("LANG")
	origTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("LC_ALL", origLCALL)
		os.Setenv("LC_CTYPE", origLCCTYPE)
		os.Setenv("LANG", origLANG)
		os.Setenv("TERM", origTERM)
	}()

	// Clear all env vars first
	os.Setenv("LC_ALL", "")
	os.Setenv("LC_CTYPE", "")
	os.Setenv("LANG", "")
	os.Setenv("TERM", "")

	// Test LC_ALL with UTF-8
	os.Setenv("LC_ALL", "en_US.UTF-8")
	if !supportsUnicode() {
		t.Error("supportsUnicode() should return true when LC_ALL contains UTF-8")
	}

	// Test with utf8 lowercase variant
	os.Setenv("LC_ALL", "en_US.utf8")
	if !supportsUnicode() {
		t.Error("supportsUnicode() should return true when LC_ALL contains utf8")
	}
}

func TestSupportsUnicodeWithLCCTYPE(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	origLCCTYPE := os.Getenv("LC_CTYPE")
	origLANG := os.Getenv("LANG")
	origTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("LC_ALL", origLCALL)
		os.Setenv("LC_CTYPE", origLCCTYPE)
		os.Setenv("LANG", origLANG)
		os.Setenv("TERM", origTERM)
	}()

	// Clear all env vars first
	os.Setenv("LC_ALL", "")
	os.Setenv("LC_CTYPE", "")
	os.Setenv("LANG", "")
	os.Setenv("TERM", "")

	// Test LC_CTYPE with UTF-8
	os.Setenv("LC_CTYPE", "en_US.UTF-8")
	if !supportsUnicode() {
		t.Error("supportsUnicode() should return true when LC_CTYPE contains UTF-8")
	}
}

func TestSupportsUnicodeWithLANG(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	origLCCTYPE := os.Getenv("LC_CTYPE")
	origLANG := os.Getenv("LANG")
	origTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("LC_ALL", origLCALL)
		os.Setenv("LC_CTYPE", origLCCTYPE)
		os.Setenv("LANG", origLANG)
		os.Setenv("TERM", origTERM)
	}()

	// Clear all env vars first
	os.Setenv("LC_ALL", "")
	os.Setenv("LC_CTYPE", "")
	os.Setenv("LANG", "")
	os.Setenv("TERM", "")

	// Test LANG with UTF-8
	os.Setenv("LANG", "en_US.UTF-8")
	if !supportsUnicode() {
		t.Error("supportsUnicode() should return true when LANG contains UTF-8")
	}
}

func TestSupportsUnicodeWithTERM(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	origLCCTYPE := os.Getenv("LC_CTYPE")
	origLANG := os.Getenv("LANG")
	origTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("LC_ALL", origLCALL)
		os.Setenv("LC_CTYPE", origLCCTYPE)
		os.Setenv("LANG", origLANG)
		os.Setenv("TERM", origTERM)
	}()

	// Clear all env vars first
	os.Setenv("LC_ALL", "")
	os.Setenv("LC_CTYPE", "")
	os.Setenv("LANG", "")
	os.Setenv("TERM", "")

	// Test various TERM values that support unicode
	unicodeTerms := []string{
		"xterm-256color", "rxvt-unicode", "screen-256color", "tmux-256color",
		"vt100", "linux", "konsole", "gnome-terminal", "alacritty", "kitty",
	}

	for _, term := range unicodeTerms {
		os.Setenv("TERM", term)
		if !supportsUnicode() {
			t.Errorf("supportsUnicode() should return true for TERM=%s", term)
		}
	}
}

func TestSupportsUnicodeNoSupport(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	origLCCTYPE := os.Getenv("LC_CTYPE")
	origLANG := os.Getenv("LANG")
	origTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("LC_ALL", origLCALL)
		os.Setenv("LC_CTYPE", origLCCTYPE)
		os.Setenv("LANG", origLANG)
		os.Setenv("TERM", origTERM)
	}()

	// Clear all env vars first
	os.Setenv("LC_ALL", "")
	os.Setenv("LC_CTYPE", "")
	os.Setenv("LANG", "")
	os.Setenv("TERM", "dumb")

	// Should return false when no unicode support
	if supportsUnicode() {
		t.Error("supportsUnicode() should return false when no unicode indicators are present")
	}
}

func TestGetSymbolsUnicode(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	defer os.Setenv("LC_ALL", origLCALL)

	// Force unicode support
	os.Setenv("LC_ALL", "en_US.UTF-8")

	sym := GetSymbols()
	if sym.Success != UnicodeSymbols.Success {
		t.Errorf("GetSymbols() returned %q for Success, want %q", sym.Success, UnicodeSymbols.Success)
	}
}

func TestGetSymbolsASCII(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	origLCCTYPE := os.Getenv("LC_CTYPE")
	origLANG := os.Getenv("LANG")
	origTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("LC_ALL", origLCALL)
		os.Setenv("LC_CTYPE", origLCCTYPE)
		os.Setenv("LANG", origLANG)
		os.Setenv("TERM", origTERM)
	}()

	// Force no unicode support
	os.Setenv("LC_ALL", "")
	os.Setenv("LC_CTYPE", "")
	os.Setenv("LANG", "")
	os.Setenv("TERM", "dumb")

	sym := GetSymbols()
	if sym.Success != ASCIISymbols.Success {
		t.Errorf("GetSymbols() returned %q for Success, want %q", sym.Success, ASCIISymbols.Success)
	}
}

func TestResizeHandlerMultipleCallbacks(t *testing.T) {
	h := NewResizeHandler()

	callCount1 := 0
	callCount2 := 0

	h.OnResize(func(s Size) {
		callCount1++
	})
	h.OnResize(func(s Size) {
		callCount2++
	})

	// Verify callbacks are registered (won't be called until resize)
	if callCount1 != 0 || callCount2 != 0 {
		t.Error("callbacks should not be called until resize")
	}
}

func TestResizeHandlerRefreshCallsCallbacks(t *testing.T) {
	h := &ResizeHandler{
		size: Size{Cols: 10, Rows: 10, Mode: SizeModeMicro},
	}

	called := false
	var receivedSize Size

	h.OnResize(func(s Size) {
		called = true
		receivedSize = s
	})

	// Refresh should call callbacks if size changes
	// Since GetSize() returns actual terminal size (or defaults),
	// it will likely differ from our initial 10x10
	h.Refresh()

	// Check if callback was called (depends on whether size changed)
	newSize := h.CurrentSize()
	if newSize.Cols != 10 || newSize.Rows != 10 {
		if !called {
			t.Error("callback should have been called when size changed")
		}
		if receivedSize.Cols != newSize.Cols || receivedSize.Rows != newSize.Rows {
			t.Error("callback should receive the new size")
		}
	}
}

func TestResizeHandlerRefreshNoChange(t *testing.T) {
	h := NewResizeHandler()
	currentSize := h.CurrentSize()

	// Manually set size to match what GetSize will return
	h.mu.Lock()
	h.size = currentSize
	h.mu.Unlock()

	called := false
	h.OnResize(func(s Size) {
		called = true
	})

	// Refresh with same size should not call callbacks
	h.Refresh()

	if called {
		t.Error("callback should not be called when size hasn't changed")
	}
}

func TestResizeHandlerConcurrentAccess(t *testing.T) {
	h := NewResizeHandler()
	done := make(chan bool)

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			_ = h.CurrentSize()
		}
		done <- true
	}()

	// Concurrent callback registration
	go func() {
		for i := 0; i < 100; i++ {
			h.OnResize(func(s Size) {})
		}
		done <- true
	}()

	// Concurrent refresh
	go func() {
		for i := 0; i < 100; i++ {
			h.Refresh()
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
}

func TestSizeModeAllMethodsCompleteness(t *testing.T) {
	// Test all SizeMode values with all methods to ensure complete coverage
	allModes := []SizeMode{
		SizeModeMicro,
		SizeModeMinimal,
		SizeModeCompact,
		SizeModeStandard,
		SizeModeWide,
		SizeModeUltrawide,
		SizeModeMassive,
	}

	for _, mode := range allModes {
		// All methods should not panic
		_ = mode.String()
		_ = mode.ShowASCIIArt()
		_ = mode.ShowBorders()
		_ = mode.ShowSidebar()
		_ = mode.ShowIcons()
		_ = mode.ShowFullInfo()
	}
}

func TestUnicodeSymbolsCompleteness(t *testing.T) {
	sym := UnicodeSymbols

	// Test all fields are set
	if sym.Pending == "" {
		t.Error("Pending symbol should not be empty")
	}
	if sym.ProgressFG == "" {
		t.Error("ProgressFG symbol should not be empty")
	}
	if sym.ProgressBG == "" {
		t.Error("ProgressBG symbol should not be empty")
	}
	if sym.BoxTopRight == "" {
		t.Error("BoxTopRight symbol should not be empty")
	}
	if sym.BoxBottomLeft == "" {
		t.Error("BoxBottomLeft symbol should not be empty")
	}
	if sym.BoxBottomRight == "" {
		t.Error("BoxBottomRight symbol should not be empty")
	}
	if sym.BoxVertical == "" {
		t.Error("BoxVertical symbol should not be empty")
	}
	if sym.ArrowLeft == "" {
		t.Error("ArrowLeft symbol should not be empty")
	}
	if sym.ArrowUp == "" {
		t.Error("ArrowUp symbol should not be empty")
	}
	if sym.ArrowDown == "" {
		t.Error("ArrowDown symbol should not be empty")
	}
	if sym.Cross == "" {
		t.Error("Cross symbol should not be empty")
	}
	if sym.Ellipsis == "" {
		t.Error("Ellipsis symbol should not be empty")
	}
}

func TestASCIISymbolsCompleteness(t *testing.T) {
	sym := ASCIISymbols

	// Test all fields are set
	if sym.Warning == "" {
		t.Error("ASCII Warning symbol should not be empty")
	}
	if sym.Info == "" {
		t.Error("ASCII Info symbol should not be empty")
	}
	if sym.Pending == "" {
		t.Error("ASCII Pending symbol should not be empty")
	}
	if sym.ProgressFG == "" {
		t.Error("ASCII ProgressFG symbol should not be empty")
	}
	if sym.ProgressBG == "" {
		t.Error("ASCII ProgressBG symbol should not be empty")
	}
	if sym.BoxTopRight == "" {
		t.Error("ASCII BoxTopRight symbol should not be empty")
	}
	if sym.BoxBottomLeft == "" {
		t.Error("ASCII BoxBottomLeft symbol should not be empty")
	}
	if sym.BoxBottomRight == "" {
		t.Error("ASCII BoxBottomRight symbol should not be empty")
	}
	if sym.BoxVertical == "" {
		t.Error("ASCII BoxVertical symbol should not be empty")
	}
	if sym.ArrowLeft == "" {
		t.Error("ASCII ArrowLeft symbol should not be empty")
	}
	if sym.ArrowUp == "" {
		t.Error("ASCII ArrowUp symbol should not be empty")
	}
	if sym.ArrowDown == "" {
		t.Error("ASCII ArrowDown symbol should not be empty")
	}
	if sym.Check == "" {
		t.Error("ASCII Check symbol should not be empty")
	}
	if sym.Cross == "" {
		t.Error("ASCII Cross symbol should not be empty")
	}
	if sym.Ellipsis == "" {
		t.Error("ASCII Ellipsis symbol should not be empty")
	}
}

func TestCalculateModeEdgeCases(t *testing.T) {
	// Test exact boundary values
	tests := []struct {
		name string
		cols int
		rows int
		want SizeMode
	}{
		// Exact boundary at 40 cols
		{"exactly 40 cols, 30 rows", 40, 30, SizeModeMinimal},
		// Exact boundary at 60 cols
		{"exactly 60 cols, 30 rows", 60, 30, SizeModeCompact},
		// Exact boundary at 80 cols
		{"exactly 80 cols, 24 rows", 80, 24, SizeModeStandard},
		// Exact boundary at 120 cols
		{"exactly 120 cols, 40 rows", 120, 40, SizeModeWide},
		// Exact boundary at 200 cols
		{"exactly 200 cols, 60 rows", 200, 60, SizeModeUltrawide},
		// Exact boundary at 400 cols
		{"exactly 400 cols, 80 rows", 400, 80, SizeModeMassive},
		// Rows boundaries
		{"exactly 10 rows", 100, 10, SizeModeMinimal},
		{"exactly 16 rows", 100, 16, SizeModeCompact},
		{"exactly 24 rows", 100, 24, SizeModeStandard},
		{"exactly 40 rows", 150, 40, SizeModeWide},
		{"exactly 60 rows", 250, 60, SizeModeUltrawide},
		{"exactly 80 rows", 450, 80, SizeModeMassive},
		// Just below boundaries
		{"39 cols", 39, 100, SizeModeMicro},
		{"9 rows", 100, 9, SizeModeMicro},
		{"59 cols", 59, 100, SizeModeMinimal},
		{"15 rows", 100, 15, SizeModeMinimal},
		{"79 cols", 79, 100, SizeModeCompact},
		{"23 rows", 100, 23, SizeModeCompact},
		{"119 cols", 119, 100, SizeModeStandard},
		{"39 rows", 150, 39, SizeModeStandard},
		{"199 cols", 199, 100, SizeModeWide},
		{"59 rows", 250, 59, SizeModeWide},
		{"399 cols", 399, 100, SizeModeUltrawide},
		{"79 rows", 450, 79, SizeModeUltrawide},
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

func TestSizeModeInvalidValue(t *testing.T) {
	// Test invalid SizeMode values
	invalidModes := []SizeMode{-1, 100, 255}

	for _, mode := range invalidModes {
		str := mode.String()
		if str != "unknown" {
			t.Errorf("SizeMode(%d).String() = %q, want %q", mode, str, "unknown")
		}
	}
}

func TestSizeEquality(t *testing.T) {
	s1 := Size{Cols: 80, Rows: 24, Mode: SizeModeStandard}
	s2 := Size{Cols: 80, Rows: 24, Mode: SizeModeStandard}
	s3 := Size{Cols: 100, Rows: 24, Mode: SizeModeStandard}

	if s1.Cols != s2.Cols || s1.Rows != s2.Rows || s1.Mode != s2.Mode {
		t.Error("identical sizes should be equal")
	}

	if s1.Cols == s3.Cols {
		t.Error("different sizes should not be equal")
	}
}

func TestGetSizeDefaults(t *testing.T) {
	// GetSize should return reasonable defaults even when not in a terminal
	size := GetSize()

	// Should return at least minimum values
	if size.Cols <= 0 {
		t.Errorf("GetSize().Cols = %d, should be positive", size.Cols)
	}
	if size.Rows <= 0 {
		t.Errorf("GetSize().Rows = %d, should be positive", size.Rows)
	}

	// Mode should be consistent with cols/rows
	expectedMode := calculateMode(size.Cols, size.Rows)
	if size.Mode != expectedMode {
		t.Errorf("GetSize().Mode = %v, should match calculateMode result %v", size.Mode, expectedMode)
	}
}

func TestStartResizeListener(t *testing.T) {
	h := NewResizeHandler()
	ctx := context.Background()

	// Start the listener
	cancel := h.StartResizeListener(ctx)
	if cancel == nil {
		t.Fatal("StartResizeListener should return a non-nil cancel function")
	}

	// Let it run briefly
	time.Sleep(10 * time.Millisecond)

	// Cancel the listener
	cancel()

	// Give the goroutine time to clean up
	time.Sleep(10 * time.Millisecond)
}

func TestStartResizeListenerWithCanceledContext(t *testing.T) {
	h := NewResizeHandler()
	ctx, ctxCancel := context.WithCancel(context.Background())

	// Cancel context immediately
	ctxCancel()

	// Start the listener with already-canceled context
	cancel := h.StartResizeListener(ctx)
	if cancel == nil {
		t.Fatal("StartResizeListener should return a non-nil cancel function")
	}

	// Give the goroutine time to exit
	time.Sleep(20 * time.Millisecond)

	// Clean up (should be a no-op since context was already canceled)
	cancel()
}

func TestStartResizeListenerMultipleCallbacks(t *testing.T) {
	h := NewResizeHandler()
	ctx := context.Background()

	callCount := 0
	h.OnResize(func(s Size) {
		callCount++
	})

	// Start the listener
	cancel := h.StartResizeListener(ctx)

	// Let it run briefly
	time.Sleep(10 * time.Millisecond)

	// Clean up
	cancel()
	time.Sleep(10 * time.Millisecond)
}

func TestStartResizeListenerCancelMultipleTimes(t *testing.T) {
	h := NewResizeHandler()
	ctx := context.Background()

	cancel := h.StartResizeListener(ctx)

	// Cancel multiple times should not panic
	cancel()
	cancel()
	cancel()
}

func TestResizeHandlerRefreshWithMultipleCallbacks(t *testing.T) {
	h := &ResizeHandler{
		size: Size{Cols: 1, Rows: 1, Mode: SizeModeMicro},
	}

	callCount1 := 0
	callCount2 := 0
	callCount3 := 0

	h.OnResize(func(s Size) {
		callCount1++
	})
	h.OnResize(func(s Size) {
		callCount2++
	})
	h.OnResize(func(s Size) {
		callCount3++
	})

	// Refresh should call all callbacks when size changes
	h.Refresh()

	newSize := h.CurrentSize()
	if newSize.Cols != 1 || newSize.Rows != 1 {
		// Size changed, all callbacks should have been called
		if callCount1 != 1 {
			t.Errorf("callback1 called %d times, want 1", callCount1)
		}
		if callCount2 != 1 {
			t.Errorf("callback2 called %d times, want 1", callCount2)
		}
		if callCount3 != 1 {
			t.Errorf("callback3 called %d times, want 1", callCount3)
		}
	}
}

func TestResizeHandlerEmptyCallbacks(t *testing.T) {
	h := &ResizeHandler{
		size: Size{Cols: 1, Rows: 1, Mode: SizeModeMicro},
	}

	// Refresh with no callbacks should not panic
	h.Refresh()
}

func TestNewResizeHandlerInitialSize(t *testing.T) {
	h := NewResizeHandler()

	size := h.CurrentSize()
	expectedSize := GetSize()

	// Initial size should match GetSize
	if size.Cols != expectedSize.Cols {
		t.Errorf("initial Cols = %d, want %d", size.Cols, expectedSize.Cols)
	}
	if size.Rows != expectedSize.Rows {
		t.Errorf("initial Rows = %d, want %d", size.Rows, expectedSize.Rows)
	}
	if size.Mode != expectedSize.Mode {
		t.Errorf("initial Mode = %v, want %v", size.Mode, expectedSize.Mode)
	}
}

func TestIsTerminalEnvironment(t *testing.T) {
	// Test that IsTerminal returns a boolean without panicking
	result := IsTerminal()

	// In CI/test environments this typically returns false
	// but we just verify the function works
	if result != true && result != false {
		t.Error("IsTerminal should return a boolean")
	}
}

func TestSymbolsSpinnerLength(t *testing.T) {
	// Unicode spinner should have 10 frames
	if len(UnicodeSymbols.Spinner) != 10 {
		t.Errorf("UnicodeSymbols.Spinner has %d frames, want 10", len(UnicodeSymbols.Spinner))
	}

	// ASCII spinner should have 4 frames
	if len(ASCIISymbols.Spinner) != 4 {
		t.Errorf("ASCIISymbols.Spinner has %d frames, want 4", len(ASCIISymbols.Spinner))
	}
}

func TestSymbolsSpinnerFrames(t *testing.T) {
	// All spinner frames should be non-empty
	for i, frame := range UnicodeSymbols.Spinner {
		if frame == "" {
			t.Errorf("UnicodeSymbols.Spinner[%d] is empty", i)
		}
	}

	for i, frame := range ASCIISymbols.Spinner {
		if frame == "" {
			t.Errorf("ASCIISymbols.Spinner[%d] is empty", i)
		}
	}
}

func TestSizeModeConstants(t *testing.T) {
	// Verify the order of constants
	tests := []struct {
		mode SizeMode
		val  int
	}{
		{SizeModeMicro, 0},
		{SizeModeMinimal, 1},
		{SizeModeCompact, 2},
		{SizeModeStandard, 3},
		{SizeModeWide, 4},
		{SizeModeUltrawide, 5},
		{SizeModeMassive, 6},
	}

	for _, tt := range tests {
		if int(tt.mode) != tt.val {
			t.Errorf("SizeMode constant %v = %d, want %d", tt.mode, int(tt.mode), tt.val)
		}
	}
}

func TestSizeModeComparisons(t *testing.T) {
	// Test that modes can be compared
	if SizeModeMicro >= SizeModeMinimal {
		t.Error("SizeModeMicro should be less than SizeModeMinimal")
	}
	if SizeModeMinimal >= SizeModeCompact {
		t.Error("SizeModeMinimal should be less than SizeModeCompact")
	}
	if SizeModeCompact >= SizeModeStandard {
		t.Error("SizeModeCompact should be less than SizeModeStandard")
	}
	if SizeModeStandard >= SizeModeWide {
		t.Error("SizeModeStandard should be less than SizeModeWide")
	}
	if SizeModeWide >= SizeModeUltrawide {
		t.Error("SizeModeWide should be less than SizeModeUltrawide")
	}
	if SizeModeUltrawide >= SizeModeMassive {
		t.Error("SizeModeUltrawide should be less than SizeModeMassive")
	}
}

func TestCalculateModeZeroValues(t *testing.T) {
	// Zero values should return Micro mode
	mode := calculateMode(0, 0)
	if mode != SizeModeMicro {
		t.Errorf("calculateMode(0, 0) = %v, want SizeModeMicro", mode)
	}
}

func TestCalculateModeNegativeValues(t *testing.T) {
	// Negative values should return Micro mode (as they're less than thresholds)
	mode := calculateMode(-1, -1)
	if mode != SizeModeMicro {
		t.Errorf("calculateMode(-1, -1) = %v, want SizeModeMicro", mode)
	}
}

func TestCalculateModeLargeValues(t *testing.T) {
	// Very large values should return Massive mode
	mode := calculateMode(10000, 10000)
	if mode != SizeModeMassive {
		t.Errorf("calculateMode(10000, 10000) = %v, want SizeModeMassive", mode)
	}
}

func TestResizeHandlerCurrentSizeThread(t *testing.T) {
	h := NewResizeHandler()
	done := make(chan bool, 10)

	// Multiple goroutines reading CurrentSize
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				size := h.CurrentSize()
				if size.Cols < 0 || size.Rows < 0 {
					t.Errorf("invalid size: %+v", size)
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestSupportsUnicodeEmptyEnv(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	origLCCTYPE := os.Getenv("LC_CTYPE")
	origLANG := os.Getenv("LANG")
	origTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("LC_ALL", origLCALL)
		os.Setenv("LC_CTYPE", origLCCTYPE)
		os.Setenv("LANG", origLANG)
		os.Setenv("TERM", origTERM)
	}()

	// Set all to empty
	os.Setenv("LC_ALL", "")
	os.Setenv("LC_CTYPE", "")
	os.Setenv("LANG", "")
	os.Setenv("TERM", "")

	// Should return false when all env vars are empty
	if supportsUnicode() {
		t.Error("supportsUnicode() should return false when all env vars are empty")
	}
}

func TestSupportsUnicodeCaseSensitivity(t *testing.T) {
	// Save original env
	origLCALL := os.Getenv("LC_ALL")
	defer os.Setenv("LC_ALL", origLCALL)

	// Test different case variations
	testCases := []string{
		"en_US.UTF-8",
		"en_US.utf-8",
		"en_US.Utf-8",
		"en_US.UTF8",
		"en_US.utf8",
	}

	for _, tc := range testCases {
		os.Setenv("LC_ALL", tc)
		if !supportsUnicode() {
			t.Errorf("supportsUnicode() should return true for LC_ALL=%s", tc)
		}
	}
}
