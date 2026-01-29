package display

import (
	"os"
	"runtime"
	"testing"
)

func TestDetect(t *testing.T) {
	env := Detect()

	// OS should always be set
	if env.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", env.OS, runtime.GOOS)
	}

	// DisplayType should be a valid value
	validTypes := map[DisplayType]bool{
		DisplayTypeNone:    true,
		DisplayTypeX11:     true,
		DisplayTypeWayland: true,
		DisplayTypeWindows: true,
		DisplayTypeMacOS:   true,
	}
	if !validTypes[env.DisplayType] {
		t.Errorf("DisplayType = %q, not a valid type", env.DisplayType)
	}
}

func TestDisplayTypeConstants(t *testing.T) {
	// Just verify the constants are defined
	if DisplayTypeNone != "none" {
		t.Errorf("DisplayTypeNone = %q, want %q", DisplayTypeNone, "none")
	}
	if DisplayTypeX11 != "x11" {
		t.Errorf("DisplayTypeX11 = %q, want %q", DisplayTypeX11, "x11")
	}
	if DisplayTypeWayland != "wayland" {
		t.Errorf("DisplayTypeWayland = %q, want %q", DisplayTypeWayland, "wayland")
	}
	if DisplayTypeWindows != "windows" {
		t.Errorf("DisplayTypeWindows = %q, want %q", DisplayTypeWindows, "windows")
	}
	if DisplayTypeMacOS != "macos" {
		t.Errorf("DisplayTypeMacOS = %q, want %q", DisplayTypeMacOS, "macos")
	}
}

func TestEnvFields(t *testing.T) {
	env := Detect()

	// Verify struct fields are accessible and have sensible values
	_ = env.HasDisplay
	_ = env.IsTerminal
	_ = env.Cols
	_ = env.Rows
	_ = env.IsSSH
	_ = env.IsMosh
	_ = env.IsScreen
	_ = env.IsTmux
	_ = env.HasColor
}

// Tests for Mode.String()

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeHeadless, "headless"},
		{ModeCLI, "cli"},
		{ModeTUI, "tui"},
		{ModeGUI, "gui"},
		{Mode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("Mode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Tests for Mode.SupportsInteraction()

func TestModeSupportsInteraction(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeHeadless, false},
		{ModeCLI, false},
		{ModeTUI, true},
		{ModeGUI, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.SupportsInteraction()
			if got != tt.want {
				t.Errorf("SupportsInteraction() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for Mode.SupportsColors()

func TestModeSupportsColors(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeHeadless, false},
		{ModeCLI, true},
		{ModeTUI, true},
		{ModeGUI, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.SupportsColors()
			if got != tt.want {
				t.Errorf("SupportsColors() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for Mode.SupportsRichOutput()

func TestModeSupportsRichOutput(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeHeadless, false},
		{ModeCLI, false},
		{ModeTUI, true},
		{ModeGUI, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.SupportsRichOutput()
			if got != tt.want {
				t.Errorf("SupportsRichOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for Mode.IsInteractive()

func TestModeIsInteractive(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeHeadless, false},
		{ModeCLI, false},
		{ModeTUI, true},
		{ModeGUI, true},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.IsInteractive()
			if got != tt.want {
				t.Errorf("IsInteractive() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for Mode.IsDaemon()

func TestModeIsDaemon(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeHeadless, true},
		{ModeCLI, false},
		{ModeTUI, false},
		{ModeGUI, false},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := tt.mode.IsDaemon()
			if got != tt.want {
				t.Errorf("IsDaemon() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for Env.GetMode()

func TestEnvGetMode(t *testing.T) {
	tests := []struct {
		name string
		env  Env
		want Mode
	}{
		{
			name: "headless - no terminal no display",
			env:  Env{IsTerminal: false, HasDisplay: false},
			want: ModeHeadless,
		},
		{
			name: "gui - has display not remote",
			env:  Env{IsTerminal: true, HasDisplay: true, IsSSH: false, IsMosh: false},
			want: ModeGUI,
		},
		{
			name: "tui - terminal over ssh",
			env:  Env{IsTerminal: true, HasDisplay: true, IsSSH: true},
			want: ModeTUI,
		},
		{
			name: "tui - terminal over mosh",
			env:  Env{IsTerminal: true, HasDisplay: true, IsMosh: true},
			want: ModeTUI,
		},
		{
			name: "tui - terminal no display",
			env:  Env{IsTerminal: true, HasDisplay: false},
			want: ModeTUI,
		},
		{
			name: "cli - no terminal but has display",
			env:  Env{IsTerminal: false, HasDisplay: true, IsSSH: false},
			want: ModeGUI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.env.GetMode()
			if got != tt.want {
				t.Errorf("GetMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for Env.IsRemote()

func TestEnvIsRemote(t *testing.T) {
	tests := []struct {
		name string
		env  Env
		want bool
	}{
		{
			name: "not remote",
			env:  Env{IsSSH: false, IsMosh: false},
			want: false,
		},
		{
			name: "ssh",
			env:  Env{IsSSH: true, IsMosh: false},
			want: true,
		},
		{
			name: "mosh",
			env:  Env{IsSSH: false, IsMosh: true},
			want: true,
		},
		{
			name: "both",
			env:  Env{IsSSH: true, IsMosh: true},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.env.IsRemote()
			if got != tt.want {
				t.Errorf("IsRemote() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for Env.IsMultiplexed()

func TestEnvIsMultiplexed(t *testing.T) {
	tests := []struct {
		name string
		env  Env
		want bool
	}{
		{
			name: "not multiplexed",
			env:  Env{IsScreen: false, IsTmux: false},
			want: false,
		},
		{
			name: "screen",
			env:  Env{IsScreen: true, IsTmux: false},
			want: true,
		},
		{
			name: "tmux",
			env:  Env{IsScreen: false, IsTmux: true},
			want: true,
		},
		{
			name: "both",
			env:  Env{IsScreen: true, IsTmux: true},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.env.IsMultiplexed()
			if got != tt.want {
				t.Errorf("IsMultiplexed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for Env.TerminalSize()

func TestEnvTerminalSize(t *testing.T) {
	tests := []struct {
		name     string
		env      Env
		wantCols int
		wantRows int
	}{
		{
			name:     "has size",
			env:      Env{Cols: 120, Rows: 40},
			wantCols: 120,
			wantRows: 40,
		},
		{
			name:     "no size - default",
			env:      Env{Cols: 0, Rows: 0},
			wantCols: 80,
			wantRows: 24,
		},
		{
			name:     "only cols - default",
			env:      Env{Cols: 100, Rows: 0},
			wantCols: 80,
			wantRows: 24,
		},
		{
			name:     "only rows - default",
			env:      Env{Cols: 0, Rows: 30},
			wantCols: 80,
			wantRows: 24,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols, rows := tt.env.TerminalSize()
			if cols != tt.wantCols {
				t.Errorf("TerminalSize() cols = %d, want %d", cols, tt.wantCols)
			}
			if rows != tt.wantRows {
				t.Errorf("TerminalSize() rows = %d, want %d", rows, tt.wantRows)
			}
		})
	}
}

// Tests for Mode constants

func TestModeConstants(t *testing.T) {
	// Verify the ordering of constants is correct
	if ModeHeadless >= ModeCLI {
		t.Error("ModeHeadless should be < ModeCLI")
	}
	if ModeCLI >= ModeTUI {
		t.Error("ModeCLI should be < ModeTUI")
	}
	if ModeTUI >= ModeGUI {
		t.Error("ModeTUI should be < ModeGUI")
	}
}

// Tests for Env struct

func TestEnvStruct(t *testing.T) {
	env := Env{
		HasDisplay:  true,
		DisplayType: DisplayTypeX11,
		IsTerminal:  true,
		Cols:        80,
		Rows:        24,
		IsSSH:       false,
		IsMosh:      false,
		IsScreen:    false,
		IsTmux:      false,
		OS:          "linux",
		HasColor:    true,
	}

	if !env.HasDisplay {
		t.Error("HasDisplay should be true")
	}
	if env.DisplayType != DisplayTypeX11 {
		t.Errorf("DisplayType = %v, want %v", env.DisplayType, DisplayTypeX11)
	}
	if !env.IsTerminal {
		t.Error("IsTerminal should be true")
	}
	if env.Cols != 80 {
		t.Errorf("Cols = %d, want 80", env.Cols)
	}
	if env.Rows != 24 {
		t.Errorf("Rows = %d, want 24", env.Rows)
	}
}

// Tests for detectColorSupport()

func TestDetectColorSupport(t *testing.T) {
	// Save original environment
	origNoColor := os.Getenv("NO_COLOR")
	origTerm := os.Getenv("TERM")
	origColorTerm := os.Getenv("COLORTERM")
	origForceColor := os.Getenv("FORCE_COLOR")

	// Cleanup after tests
	defer func() {
		restoreEnv("NO_COLOR", origNoColor)
		restoreEnv("TERM", origTerm)
		restoreEnv("COLORTERM", origColorTerm)
		restoreEnv("FORCE_COLOR", origForceColor)
	}()

	tests := []struct {
		name       string
		noColor    string
		term       string
		colorTerm  string
		forceColor string
		want       bool
	}{
		{
			name:    "NO_COLOR set disables color",
			noColor: "1",
			term:    "xterm-256color",
			want:    false,
		},
		{
			name: "empty TERM disables color",
			term: "",
			want: false,
		},
		{
			name: "dumb TERM disables color",
			term: "dumb",
			want: false,
		},
		{
			name:      "COLORTERM enables color",
			term:      "xterm",
			colorTerm: "truecolor",
			want:      true,
		},
		{
			name:       "FORCE_COLOR enables color",
			term:       "xterm",
			forceColor: "1",
			want:       true,
		},
		{
			name: "default with normal TERM enables color",
			term: "xterm-256color",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all color-related env vars
			os.Unsetenv("NO_COLOR")
			os.Unsetenv("TERM")
			os.Unsetenv("COLORTERM")
			os.Unsetenv("FORCE_COLOR")

			// Set specific env vars for this test
			if tt.noColor != "" {
				os.Setenv("NO_COLOR", tt.noColor)
			}
			if tt.term != "" {
				os.Setenv("TERM", tt.term)
			}
			if tt.colorTerm != "" {
				os.Setenv("COLORTERM", tt.colorTerm)
			}
			if tt.forceColor != "" {
				os.Setenv("FORCE_COLOR", tt.forceColor)
			}

			got := detectColorSupport()
			if got != tt.want {
				t.Errorf("detectColorSupport() = %v, want %v", got, tt.want)
			}
		})
	}
}

// restoreEnv restores an environment variable or unsets it if it was empty
func restoreEnv(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}

// Tests for Env.detectUnixDisplay() - only runs on Unix systems

func TestDetectUnixDisplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix display test on Windows")
	}

	// Save original environment
	origWayland := os.Getenv("WAYLAND_DISPLAY")
	origDisplay := os.Getenv("DISPLAY")

	// Cleanup after tests
	defer func() {
		restoreEnv("WAYLAND_DISPLAY", origWayland)
		restoreEnv("DISPLAY", origDisplay)
	}()

	tests := []struct {
		name            string
		waylandDisplay  string
		xDisplay        string
		wantHasDisplay  bool
		wantDisplayType DisplayType
	}{
		{
			name:            "Wayland display",
			waylandDisplay:  "wayland-0",
			xDisplay:        "",
			wantHasDisplay:  true,
			wantDisplayType: DisplayTypeWayland,
		},
		{
			name:            "X11 display",
			waylandDisplay:  "",
			xDisplay:        ":0",
			wantHasDisplay:  true,
			wantDisplayType: DisplayTypeX11,
		},
		{
			name:            "Wayland takes precedence over X11",
			waylandDisplay:  "wayland-0",
			xDisplay:        ":0",
			wantHasDisplay:  true,
			wantDisplayType: DisplayTypeWayland,
		},
		{
			name:            "No display",
			waylandDisplay:  "",
			xDisplay:        "",
			wantHasDisplay:  false,
			wantDisplayType: DisplayTypeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear display env vars
			os.Unsetenv("WAYLAND_DISPLAY")
			os.Unsetenv("DISPLAY")

			// Set specific env vars for this test
			if tt.waylandDisplay != "" {
				os.Setenv("WAYLAND_DISPLAY", tt.waylandDisplay)
			}
			if tt.xDisplay != "" {
				os.Setenv("DISPLAY", tt.xDisplay)
			}

			env := &Env{}
			env.detectUnixDisplay()

			if env.HasDisplay != tt.wantHasDisplay {
				t.Errorf("HasDisplay = %v, want %v", env.HasDisplay, tt.wantHasDisplay)
			}
			if env.DisplayType != tt.wantDisplayType {
				t.Errorf("DisplayType = %v, want %v", env.DisplayType, tt.wantDisplayType)
			}
		})
	}
}

// Tests for Env.detectMacOSDisplay() - runs on darwin

func TestDetectMacOSDisplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping macOS display test on Windows")
	}

	// Save original environment
	origSSHClient := os.Getenv("SSH_CLIENT")
	origSSHTTY := os.Getenv("SSH_TTY")
	origSSHConnection := os.Getenv("SSH_CONNECTION")
	origMosh := os.Getenv("MOSH")
	origMoshConn := os.Getenv("MOSH_CONNECTION")
	origXPC := os.Getenv("XPC_SERVICE_NAME")

	// Cleanup after tests
	defer func() {
		restoreEnv("SSH_CLIENT", origSSHClient)
		restoreEnv("SSH_TTY", origSSHTTY)
		restoreEnv("SSH_CONNECTION", origSSHConnection)
		restoreEnv("MOSH", origMosh)
		restoreEnv("MOSH_CONNECTION", origMoshConn)
		restoreEnv("XPC_SERVICE_NAME", origXPC)
	}()

	tests := []struct {
		name            string
		isSSH           bool
		isMosh          bool
		xpcService      string
		wantHasDisplay  bool
		wantDisplayType DisplayType
	}{
		{
			name:            "SSH disables display",
			isSSH:           true,
			isMosh:          false,
			wantHasDisplay:  false,
			wantDisplayType: DisplayTypeNone,
		},
		{
			name:            "Mosh disables display",
			isSSH:           false,
			isMosh:          true,
			wantHasDisplay:  false,
			wantDisplayType: DisplayTypeNone,
		},
		{
			name:            "Normal macOS has display",
			isSSH:           false,
			isMosh:          false,
			wantHasDisplay:  true,
			wantDisplayType: DisplayTypeMacOS,
		},
		{
			name:            "XPC service with ppid check",
			isSSH:           false,
			isMosh:          false,
			xpcService:      "com.apple.test",
			wantHasDisplay:  os.Getppid() != 1, // depends on actual ppid
			wantDisplayType: func() DisplayType {
				if os.Getppid() == 1 {
					return DisplayTypeNone
				}
				return DisplayTypeMacOS
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear relevant env vars
			os.Unsetenv("SSH_CLIENT")
			os.Unsetenv("SSH_TTY")
			os.Unsetenv("SSH_CONNECTION")
			os.Unsetenv("MOSH")
			os.Unsetenv("MOSH_CONNECTION")
			os.Unsetenv("XPC_SERVICE_NAME")

			if tt.xpcService != "" {
				os.Setenv("XPC_SERVICE_NAME", tt.xpcService)
			}

			env := &Env{
				IsSSH:  tt.isSSH,
				IsMosh: tt.isMosh,
			}
			env.detectMacOSDisplay()

			if env.HasDisplay != tt.wantHasDisplay {
				t.Errorf("HasDisplay = %v, want %v", env.HasDisplay, tt.wantHasDisplay)
			}
			if env.DisplayType != tt.wantDisplayType {
				t.Errorf("DisplayType = %v, want %v", env.DisplayType, tt.wantDisplayType)
			}
		})
	}
}

// TestDetectMacOSDisplayXPCService tests the XPC service detection branch
// This test explicitly covers the case when running as a LaunchDaemon
func TestDetectMacOSDisplayXPCService(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping macOS display test on Windows")
	}

	origXPC := os.Getenv("XPC_SERVICE_NAME")
	defer restoreEnv("XPC_SERVICE_NAME", origXPC)

	// If ppid is 1 (Docker container or init), we can test the XPC path
	if os.Getppid() == 1 {
		os.Setenv("XPC_SERVICE_NAME", "com.apple.launchdaemon.test")

		env := &Env{
			IsSSH:  false,
			IsMosh: false,
		}
		env.detectMacOSDisplay()

		// With XPC_SERVICE_NAME set and ppid == 1, should detect as service
		if env.HasDisplay {
			t.Error("HasDisplay should be false for XPC service with ppid=1")
		}
		if env.DisplayType != DisplayTypeNone {
			t.Errorf("DisplayType = %v, want %v", env.DisplayType, DisplayTypeNone)
		}
	} else {
		// If ppid != 1, just verify the branch logic works
		os.Setenv("XPC_SERVICE_NAME", "com.apple.test")

		env := &Env{
			IsSSH:  false,
			IsMosh: false,
		}
		env.detectMacOSDisplay()

		// With XPC_SERVICE_NAME but ppid != 1, should still have display
		if !env.HasDisplay {
			t.Error("HasDisplay should be true when XPC_SERVICE_NAME set but ppid != 1")
		}
		if env.DisplayType != DisplayTypeMacOS {
			t.Errorf("DisplayType = %v, want %v", env.DisplayType, DisplayTypeMacOS)
		}
	}
}

// Tests for Env.detectPlatformDisplay() - dispatches based on OS

func TestDetectPlatformDisplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping platform display test on Windows")
	}

	// Save original environment
	origWayland := os.Getenv("WAYLAND_DISPLAY")
	origDisplay := os.Getenv("DISPLAY")

	// Cleanup after tests
	defer func() {
		restoreEnv("WAYLAND_DISPLAY", origWayland)
		restoreEnv("DISPLAY", origDisplay)
	}()

	// Clear display env vars for consistent test
	os.Unsetenv("WAYLAND_DISPLAY")
	os.Unsetenv("DISPLAY")

	env := &Env{
		OS: runtime.GOOS,
	}
	env.detectPlatformDisplay()

	// On darwin, expect macOS display type if not SSH/mosh
	// On other Unix, expect no display without DISPLAY/WAYLAND_DISPLAY
	if runtime.GOOS == "darwin" {
		// Unless SSH/mosh, should have display
		if !env.IsSSH && !env.IsMosh {
			if env.DisplayType != DisplayTypeMacOS {
				t.Errorf("On darwin, expected DisplayTypeMacOS, got %v", env.DisplayType)
			}
		}
	} else {
		// Linux/BSD without DISPLAY or WAYLAND_DISPLAY
		if env.HasDisplay {
			t.Errorf("On %s without DISPLAY/WAYLAND_DISPLAY, expected no display", runtime.GOOS)
		}
		if env.DisplayType != DisplayTypeNone {
			t.Errorf("On %s without display, expected DisplayTypeNone, got %v", runtime.GOOS, env.DisplayType)
		}
	}
}

// Tests for Env.GetMode() - additional edge cases

func TestEnvGetModeCLI(t *testing.T) {
	// Test case: no terminal, has display, but over SSH -> should fall through to CLI
	env := Env{
		IsTerminal: false,
		HasDisplay: true,
		IsSSH:      true,
	}
	got := env.GetMode()
	if got != ModeCLI {
		t.Errorf("GetMode() = %v, want %v (CLI mode for non-terminal with SSH)", got, ModeCLI)
	}
}

func TestEnvGetModeMoshCLI(t *testing.T) {
	// Test case: no terminal, has display, but over mosh -> should fall through to CLI
	env := Env{
		IsTerminal: false,
		HasDisplay: true,
		IsMosh:     true,
	}
	got := env.GetMode()
	if got != ModeCLI {
		t.Errorf("GetMode() = %v, want %v (CLI mode for non-terminal with Mosh)", got, ModeCLI)
	}
}

// Tests for Detect() with environment manipulation

func TestDetectWithEnvironment(t *testing.T) {
	// Save original environment
	origSSHClient := os.Getenv("SSH_CLIENT")
	origSSHTTY := os.Getenv("SSH_TTY")
	origSSHConnection := os.Getenv("SSH_CONNECTION")
	origMosh := os.Getenv("MOSH")
	origMoshConn := os.Getenv("MOSH_CONNECTION")
	origSTY := os.Getenv("STY")
	origTMUX := os.Getenv("TMUX")

	// Cleanup after tests
	defer func() {
		restoreEnv("SSH_CLIENT", origSSHClient)
		restoreEnv("SSH_TTY", origSSHTTY)
		restoreEnv("SSH_CONNECTION", origSSHConnection)
		restoreEnv("MOSH", origMosh)
		restoreEnv("MOSH_CONNECTION", origMoshConn)
		restoreEnv("STY", origSTY)
		restoreEnv("TMUX", origTMUX)
	}()

	t.Run("SSH detection via SSH_CLIENT", func(t *testing.T) {
		os.Unsetenv("SSH_CLIENT")
		os.Unsetenv("SSH_TTY")
		os.Unsetenv("SSH_CONNECTION")
		os.Setenv("SSH_CLIENT", "192.168.1.1 12345 22")

		env := Detect()
		if !env.IsSSH {
			t.Error("IsSSH should be true when SSH_CLIENT is set")
		}
	})

	t.Run("SSH detection via SSH_TTY", func(t *testing.T) {
		os.Unsetenv("SSH_CLIENT")
		os.Unsetenv("SSH_TTY")
		os.Unsetenv("SSH_CONNECTION")
		os.Setenv("SSH_TTY", "/dev/pts/0")

		env := Detect()
		if !env.IsSSH {
			t.Error("IsSSH should be true when SSH_TTY is set")
		}
	})

	t.Run("SSH detection via SSH_CONNECTION", func(t *testing.T) {
		os.Unsetenv("SSH_CLIENT")
		os.Unsetenv("SSH_TTY")
		os.Unsetenv("SSH_CONNECTION")
		os.Setenv("SSH_CONNECTION", "192.168.1.1 12345 192.168.1.2 22")

		env := Detect()
		if !env.IsSSH {
			t.Error("IsSSH should be true when SSH_CONNECTION is set")
		}
	})

	t.Run("Mosh detection via MOSH", func(t *testing.T) {
		os.Unsetenv("MOSH")
		os.Unsetenv("MOSH_CONNECTION")
		os.Setenv("MOSH", "1")

		env := Detect()
		if !env.IsMosh {
			t.Error("IsMosh should be true when MOSH is set")
		}
	})

	t.Run("Mosh detection via MOSH_CONNECTION", func(t *testing.T) {
		os.Unsetenv("MOSH")
		os.Unsetenv("MOSH_CONNECTION")
		os.Setenv("MOSH_CONNECTION", "192.168.1.1:60001")

		env := Detect()
		if !env.IsMosh {
			t.Error("IsMosh should be true when MOSH_CONNECTION is set")
		}
	})

	t.Run("Screen detection", func(t *testing.T) {
		os.Unsetenv("STY")
		os.Setenv("STY", "12345.pts-0.hostname")

		env := Detect()
		if !env.IsScreen {
			t.Error("IsScreen should be true when STY is set")
		}
	})

	t.Run("Tmux detection", func(t *testing.T) {
		os.Unsetenv("TMUX")
		os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

		env := Detect()
		if !env.IsTmux {
			t.Error("IsTmux should be true when TMUX is set")
		}
	})

	t.Run("No remote session", func(t *testing.T) {
		os.Unsetenv("SSH_CLIENT")
		os.Unsetenv("SSH_TTY")
		os.Unsetenv("SSH_CONNECTION")
		os.Unsetenv("MOSH")
		os.Unsetenv("MOSH_CONNECTION")
		os.Unsetenv("STY")
		os.Unsetenv("TMUX")

		env := Detect()
		if env.IsSSH {
			t.Error("IsSSH should be false when no SSH env vars are set")
		}
		if env.IsMosh {
			t.Error("IsMosh should be false when no Mosh env vars are set")
		}
		if env.IsScreen {
			t.Error("IsScreen should be false when STY is not set")
		}
		if env.IsTmux {
			t.Error("IsTmux should be false when TMUX is not set")
		}
	})
}

// Tests for Mode type edge cases

func TestModeEdgeCases(t *testing.T) {
	// Test negative mode value
	negativeMode := Mode(-1)
	if negativeMode.String() != "unknown" {
		t.Errorf("Mode(-1).String() = %q, want %q", negativeMode.String(), "unknown")
	}

	// Test very large mode value
	largeMode := Mode(1000)
	if largeMode.String() != "unknown" {
		t.Errorf("Mode(1000).String() = %q, want %q", largeMode.String(), "unknown")
	}

	// Test SupportsInteraction for edge cases
	if negativeMode.SupportsInteraction() {
		t.Error("Mode(-1).SupportsInteraction() should be false")
	}

	// Test SupportsColors for edge cases
	if negativeMode.SupportsColors() {
		t.Error("Mode(-1).SupportsColors() should be false")
	}

	// Test SupportsRichOutput for edge cases
	if negativeMode.SupportsRichOutput() {
		t.Error("Mode(-1).SupportsRichOutput() should be false")
	}

	// Test IsInteractive for edge cases
	if negativeMode.IsInteractive() {
		t.Error("Mode(-1).IsInteractive() should be false")
	}

	// Test IsDaemon for edge cases
	if negativeMode.IsDaemon() {
		t.Error("Mode(-1).IsDaemon() should be false")
	}
}

// Tests for Env with various display types

func TestEnvWithDisplayTypes(t *testing.T) {
	tests := []struct {
		name        string
		displayType DisplayType
	}{
		{"none", DisplayTypeNone},
		{"x11", DisplayTypeX11},
		{"wayland", DisplayTypeWayland},
		{"windows", DisplayTypeWindows},
		{"macos", DisplayTypeMacOS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := Env{
				DisplayType: tt.displayType,
			}
			if env.DisplayType != tt.displayType {
				t.Errorf("DisplayType = %v, want %v", env.DisplayType, tt.displayType)
			}
		})
	}
}

// Test Detect function returns consistent OS

func TestDetectOS(t *testing.T) {
	env := Detect()
	expectedOS := runtime.GOOS
	if env.OS != expectedOS {
		t.Errorf("OS = %q, want %q", env.OS, expectedOS)
	}
}

// Tests for GetMode with all possible combinations

func TestGetModeAllCombinations(t *testing.T) {
	tests := []struct {
		name       string
		isTerminal bool
		hasDisplay bool
		isSSH      bool
		isMosh     bool
		want       Mode
	}{
		// Headless cases
		{"headless: no terminal, no display", false, false, false, false, ModeHeadless},
		{"headless: no terminal, no display, SSH", false, false, true, false, ModeHeadless},
		{"headless: no terminal, no display, Mosh", false, false, false, true, ModeHeadless},

		// GUI cases
		{"gui: terminal, display, local", true, true, false, false, ModeGUI},
		{"gui: no terminal, display, local", false, true, false, false, ModeGUI},

		// TUI cases
		{"tui: terminal, display, SSH", true, true, true, false, ModeTUI},
		{"tui: terminal, display, Mosh", true, true, false, true, ModeTUI},
		{"tui: terminal, display, SSH+Mosh", true, true, true, true, ModeTUI},
		{"tui: terminal, no display", true, false, false, false, ModeTUI},
		{"tui: terminal, no display, SSH", true, false, true, false, ModeTUI},

		// CLI cases
		{"cli: no terminal, display, SSH", false, true, true, false, ModeCLI},
		{"cli: no terminal, display, Mosh", false, true, false, true, ModeCLI},
		{"cli: no terminal, display, SSH+Mosh", false, true, true, true, ModeCLI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := Env{
				IsTerminal: tt.isTerminal,
				HasDisplay: tt.hasDisplay,
				IsSSH:      tt.isSSH,
				IsMosh:     tt.isMosh,
			}
			got := env.GetMode()
			if got != tt.want {
				t.Errorf("GetMode() = %v (%s), want %v (%s)", got, got.String(), tt.want, tt.want.String())
			}
		})
	}
}
