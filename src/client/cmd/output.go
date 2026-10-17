package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// view is the format-agnostic description of a command result.
// Per AI.md PART 32 cli.yml reference, output.format supports
// table, json, yaml, plain and csv; every command renders through this type so
// all five formats behave identically across the whole CLI.
type view struct {
	// Data is the structured payload rendered by the json and yaml formats.
	Data any
	// Headers are the column names for the table and csv formats.
	Headers []string
	// Rows are the tabular cells for the table and csv formats.
	Rows [][]string
	// Plain holds one pre-rendered line per record for the plain format.
	// When empty, plain falls back to tab-joined Rows.
	Plain []string
	// Footer is an optional summary line appended by the table format only.
	Footer string
}

// render writes v in the requested format to stdout.
func render(format string, v view) error {
	return renderTo(os.Stdout, format, v)
}

// renderTo writes v in the requested format to w.
func renderTo(w io.Writer, format string, v view) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v.Data)
	case "yaml", "yml":
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		if err := enc.Encode(v.Data); err != nil {
			return err
		}
		return enc.Close()
	case "csv":
		cw := csv.NewWriter(w)
		if len(v.Headers) > 0 {
			if err := cw.Write(v.Headers); err != nil {
				return err
			}
		}
		for _, row := range v.Rows {
			if err := cw.Write(row); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	case "plain":
		if len(v.Plain) > 0 {
			for _, line := range v.Plain {
				if _, err := fmt.Fprintln(w, line); err != nil {
					return err
				}
			}
			return nil
		}
		for _, row := range v.Rows {
			if _, err := fmt.Fprintln(w, strings.Join(row, "\t")); err != nil {
				return err
			}
		}
		return nil
	default:
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		if len(v.Headers) > 0 {
			if _, err := fmt.Fprintln(tw, strings.Join(v.Headers, "\t")); err != nil {
				return err
			}
		}
		for _, row := range v.Rows {
			if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if v.Footer != "" {
			if _, err := fmt.Fprintf(w, "\n%s\n", v.Footer); err != nil {
				return err
			}
		}
		return nil
	}
}

// truncateCell shortens a table cell to width runes with an ellipsis.
func truncateCell(s string, width int) string {
	r := []rune(s)
	if width <= 3 || len(r) <= width {
		return s
	}
	return string(r[:width-3]) + "..."
}

// boolCell renders a boolean as a stable, machine-parseable cell value.
func boolCell(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
