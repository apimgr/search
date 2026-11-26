package cmd

import (
	"sort"
	"strings"
	"testing"

	"github.com/apimgr/search/src/client/gui"
	"github.com/apimgr/search/src/client/surface"
	"github.com/apimgr/search/src/client/tui"
)

func TestEveryServerCommandHasGUIAndTUIViewInCLIHelpOrder(t *testing.T) {
	want := cliLeafHelpOrder(t)
	assertSurfaceOrder(t, "shared catalog", surface.Commands(), want)
	assertSurfaceOrder(t, "GUI", gui.Screens(), want)
	assertSurfaceOrder(t, "TUI", tui.Views(), want)
}

func cliLeafHelpOrder(t *testing.T) []string {
	t.Helper()
	names := make([]string, 0, len(rootCmd.subcommands))
	for name := range rootCmd.subcommands {
		if name != "shell" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var leaves []string
	for _, name := range names {
		command := rootCmd.subcommands[name]
		if command.run == nil {
			t.Errorf("root command %q has no implementation", name)
			continue
		}
		actions := cliActionsFromUsage(name, command.Use)
		if name == "preferences" {
			leaves = append(leaves, name)
		}
		if len(actions) == 0 {
			leaves = append(leaves, name)
			continue
		}
		for _, action := range actions {
			leaves = append(leaves, name+" "+action)
		}
	}
	return leaves
}

func cliActionsFromUsage(name, usage string) []string {
	if name == "preferences" {
		if strings.Contains(usage, "set") {
			return []string{"set"}
		}
	}
	open, close := "[", "]"
	if name == "scheduler" {
		open, close = "<", ">"
	}
	start := strings.Index(usage, open)
	if start < 0 {
		return nil
	}
	end := strings.Index(usage[start+1:], close)
	if end < 0 {
		return nil
	}
	options := usage[start+1 : start+1+end]
	if !strings.Contains(options, "|") {
		return nil
	}
	parts := strings.Split(options, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func assertSurfaceOrder(t *testing.T, label string, surfaces []surface.Command, want []string) {
	t.Helper()
	got := make([]string, 0, len(surfaces))
	seen := make(map[string]bool, len(surfaces))
	for _, screen := range surfaces {
		name := screen.Name()
		got = append(got, name)
		if seen[name] {
			t.Errorf("%s registers duplicate screen/view %q", label, name)
		}
		seen[name] = true
		if len(screen.Path) == 0 || strings.TrimSpace(screen.Description) == "" {
			t.Errorf("%s screen/view %q lacks a path or useful description", label, name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s has %d screens/views; CLI help has %d leaves", label, len(got), len(want))
	}
	max := len(got)
	if len(want) < max {
		max = len(want)
	}
	for i := 0; i < max; i++ {
		if got[i] != want[i] {
			t.Errorf("%s position %d is %q; CLI help order requires %q", label, i, got[i], want[i])
		}
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("%s is missing CLI leaf %q", label, name)
		}
	}
	for name := range seen {
		found := false
		for _, expected := range want {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s registers extra non-CLI screen/view %q", label, name)
		}
	}
}
