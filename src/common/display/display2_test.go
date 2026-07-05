package display

import (
	"os"
	"testing"
)

// resetState calls InitOutput("auto") with a neutral TERM so every test starts
// from a known baseline.  Individual tests override env vars as needed and
// restore them in t.Cleanup.
func resetState(t *testing.T) {
	t.Helper()
	// Clear env vars that affect auto-detection before calling InitOutput.
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	InitOutput("auto")
}

// setEnv is a helper that sets an env var and registers a cleanup to restore it.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv(key)
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
	t.Cleanup(func() {
		if hadPrev {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestInitOutputAlways(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	InitOutput("always")

	if !ColorEnabled() {
		t.Error("ColorEnabled() = false after InitOutput(always), want true")
	}
	if !EmojiEnabled() {
		t.Error("EmojiEnabled() = false after InitOutput(always), want true")
	}
}

func TestInitOutputNever(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	InitOutput("never")

	if ColorEnabled() {
		t.Error("ColorEnabled() = true after InitOutput(never), want false")
	}
	if EmojiEnabled() {
		t.Error("EmojiEnabled() = true after InitOutput(never), want false")
	}
}

func TestInitOutputAutoNoColor(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	setEnv(t, "NO_COLOR", "1")
	setEnv(t, "TERM", "xterm-256color")
	InitOutput("")

	if ColorEnabled() {
		t.Error("ColorEnabled() = true with NO_COLOR=1, want false")
	}
	if EmojiEnabled() {
		t.Error("EmojiEnabled() = true with NO_COLOR=1, want false")
	}
}

func TestInitOutputAutoDumbTerm(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	setEnv(t, "NO_COLOR", "")
	setEnv(t, "TERM", "dumb")
	InitOutput("")

	if ColorEnabled() {
		t.Error("ColorEnabled() = true with TERM=dumb, want false")
	}
	if EmojiEnabled() {
		t.Error("EmojiEnabled() = true with TERM=dumb, want false")
	}
}

func TestInitOutputAutoEmptyTerm(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	setEnv(t, "NO_COLOR", "")
	setEnv(t, "TERM", "")
	InitOutput("")

	if ColorEnabled() {
		t.Error("ColorEnabled() = true with TERM unset, want false")
	}
}

func TestInitOutputAutoNormalTerm(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	setEnv(t, "TERM", "xterm-256color")
	setEnv(t, "NO_COLOR", "")
	InitOutput("auto")

	if !ColorEnabled() {
		t.Error("ColorEnabled() = false with TERM=xterm-256color and no NO_COLOR, want true")
	}
	if !EmojiEnabled() {
		t.Error("EmojiEnabled() = false with TERM=xterm-256color and no NO_COLOR, want true")
	}
}

func TestColorEnabledWithForce(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	InitOutput("always")
	if !ColorEnabled() {
		t.Error("ColorEnabled() = false after always, want true")
	}

	InitOutput("never")
	if ColorEnabled() {
		t.Error("ColorEnabled() = true after never, want false")
	}

	// auto with a real terminal environment
	setEnv(t, "TERM", "xterm-256color")
	setEnv(t, "NO_COLOR", "")
	InitOutput("auto")
	if !ColorEnabled() {
		t.Error("ColorEnabled() = false after auto with xterm-256color, want true")
	}
}

func TestEmojiFunction(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	InitOutput("always")
	got := Emoji("🔍", "search")
	if got != "🔍" {
		t.Errorf("Emoji() = %q with emojis enabled, want 🔍", got)
	}

	InitOutput("never")
	got = Emoji("🔍", "search")
	if got != "search" {
		t.Errorf("Emoji() = %q with emojis disabled, want search", got)
	}
}

func TestColorEnabledReflectsForceFlag(t *testing.T) {
	t.Cleanup(func() { resetState(t) })

	// colorForce=always overrides env
	setEnv(t, "NO_COLOR", "1")
	InitOutput("always")
	if !ColorEnabled() {
		t.Error("ColorEnabled() = false: always flag should override NO_COLOR")
	}

	// colorForce=never overrides a good terminal
	setEnv(t, "TERM", "xterm-256color")
	setEnv(t, "NO_COLOR", "")
	InitOutput("never")
	if ColorEnabled() {
		t.Error("ColorEnabled() = true: never flag should override good TERM")
	}
}
