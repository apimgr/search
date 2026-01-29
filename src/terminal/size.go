// Package terminal provides terminal utilities for all binaries
// Per AI.md PART 7: BINARY REQUIREMENTS - Common Go Modules
package terminal

import (
	"os"

	"golang.org/x/term"
)

// SizeMode represents terminal size breakpoints
type SizeMode int

const (
	// SizeModeMicro - <40 cols or <10 rows
	SizeModeMicro SizeMode = iota
	// SizeModeMinimal - 40-59 cols or 10-15 rows
	SizeModeMinimal
	// SizeModeCompact - 60-79 cols or 16-23 rows
	SizeModeCompact
	// SizeModeStandard - 80-119 cols and 24-39 rows
	SizeModeStandard
	// SizeModeWide - 120-199 cols and 40-59 rows
	SizeModeWide
	// SizeModeUltrawide - 200-399 cols and 60-79 rows
	SizeModeUltrawide
	// SizeModeMassive - 400+ cols and 80+ rows
	SizeModeMassive
)

// String returns the string representation of the size mode
func (s SizeMode) String() string {
	switch s {
	case SizeModeMicro:
		return "micro"
	case SizeModeMinimal:
		return "minimal"
	case SizeModeCompact:
		return "compact"
	case SizeModeStandard:
		return "standard"
	case SizeModeWide:
		return "wide"
	case SizeModeUltrawide:
		return "ultrawide"
	case SizeModeMassive:
		return "massive"
	default:
		return "unknown"
	}
}

// Size represents terminal dimensions
type Size struct {
	Cols int
	Rows int
	Mode SizeMode
}

// GetSize returns the current terminal size
func GetSize() Size {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols == 0 {
		cols = 80
	}
	if err != nil || rows == 0 {
		rows = 24
	}

	return Size{
		Cols: cols,
		Rows: rows,
		Mode: calculateMode(cols, rows),
	}
}

// calculateMode determines the size mode based on dimensions
func calculateMode(cols, rows int) SizeMode {
	switch {
	case cols < 40 || rows < 10:
		return SizeModeMicro
	case cols < 60 || rows < 16:
		return SizeModeMinimal
	case cols < 80 || rows < 24:
		return SizeModeCompact
	case cols < 120 || rows < 40:
		return SizeModeStandard
	case cols < 200 || rows < 60:
		return SizeModeWide
	case cols < 400 || rows < 80:
		return SizeModeUltrawide
	default:
		return SizeModeMassive
	}
}

// ShowASCIIArt returns true if terminal is large enough for ASCII art
func (s SizeMode) ShowASCIIArt() bool {
	return s >= SizeModeStandard
}

// ShowBorders returns true if terminal supports bordered elements
func (s SizeMode) ShowBorders() bool {
	return s >= SizeModeCompact
}

// ShowSidebar returns true if terminal is wide enough for sidebar
func (s SizeMode) ShowSidebar() bool {
	return s >= SizeModeWide
}

// ShowIcons returns true if terminal supports icons/symbols
func (s SizeMode) ShowIcons() bool {
	return s >= SizeModeMinimal
}

// ShowFullInfo returns true if terminal can show full information
func (s SizeMode) ShowFullInfo() bool {
	return s >= SizeModeStandard
}

// IsTerminal returns true if stdout is a terminal
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
