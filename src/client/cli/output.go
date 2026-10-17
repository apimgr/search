// Package cli provides colorized terminal output helpers for the client.
// Per AI.md PART 32: CLI output uses the ANSI TerminalPalette, never the
// literal hex ThemePalette used by the web frontend.
package cli

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/apimgr/search/src/common/display"
	"github.com/apimgr/search/src/common/theme"
)

// Output writes colorized messages to a stream.
type Output struct {
	palette theme.TerminalPalette
	colors  bool
	out     io.Writer
	errOut  io.Writer
}

// NewOutput builds an Output for the named theme, honoring the NO_COLOR and
// --color resolution already applied by the command layer.
func NewOutput(themeName string) *Output {
	return &Output{
		palette: theme.GetTerminalPalette(themeName),
		colors:  display.ColorEnabled(),
		out:     os.Stdout,
		errOut:  os.Stderr,
	}
}

// NewOutputTo builds an Output writing to explicit streams, used by tests and
// by callers that capture client output.
func NewOutputTo(themeName string, out, errOut io.Writer, colors bool) *Output {
	return &Output{
		palette: theme.GetTerminalPalette(themeName),
		colors:  colors,
		out:     out,
		errOut:  errOut,
	}
}

// Palette returns the ANSI palette backing this Output.
func (o *Output) Palette() theme.TerminalPalette { return o.palette }

// ColorsEnabled reports whether ANSI color codes are emitted.
func (o *Output) ColorsEnabled() bool { return o.colors }

// colorize wraps msg in a 256-color ANSI sequence when colors are enabled.
func (o *Output) colorize(code, msg string) string {
	if !o.colors {
		return msg
	}
	return fmt.Sprintf("\033[38;5;%sm%s\033[0m", code, msg)
}

// PrintSuccess writes a success message to stdout.
func (o *Output) PrintSuccess(msg string) {
	if _, err := fmt.Fprintln(o.out, o.colorize(o.palette.Success, msg)); err != nil {
		log.Printf("write CLI success message: %v", err)
	}
}

// PrintError writes an error message to stderr.
func (o *Output) PrintError(msg string) {
	if _, err := fmt.Fprintln(o.errOut, o.colorize(o.palette.Error, msg)); err != nil {
		log.Printf("write CLI error message: %v", err)
	}
}

// PrintWarning writes a warning message to stderr.
func (o *Output) PrintWarning(msg string) {
	if _, err := fmt.Fprintln(o.errOut, o.colorize(o.palette.Warning, msg)); err != nil {
		log.Printf("write CLI warning message: %v", err)
	}
}

func (o *Output) writeLine(writer io.Writer, message string) {
	if _, err := fmt.Fprintln(writer, message); err != nil {
		log.Printf("write CLI output: %v", err)
	}
}

// PrintInfo writes an informational message to stdout.
func (o *Output) PrintInfo(msg string) {
	o.writeLine(o.out, o.colorize(o.palette.Accent, msg))
}

// PrintMuted writes a de-emphasized message to stdout.
func (o *Output) PrintMuted(msg string) {
	o.writeLine(o.out, o.colorize(o.palette.Muted, msg))
}

// PrintHeading writes a section heading to stdout.
func (o *Output) PrintHeading(msg string) {
	o.writeLine(o.out, o.colorize(o.palette.Primary, msg))
}

// Print writes an uncolored line to stdout.
func (o *Output) Print(msg string) {
	o.writeLine(o.out, msg)
}
