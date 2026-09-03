package tui

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/common/terminal"
)

// styles are derived from CurrentTUITheme (see theme.go) rather than
// hardcoded colors, so switching themes restyles the whole TUI.
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(CurrentTUITheme.Primary).
			Bold(true).
			Padding(0, 1)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(CurrentTUITheme.Secondary).
			Padding(0, 1)

	resultStyle = lipgloss.NewStyle().
			Foreground(CurrentTUITheme.Foreground)

	urlStyle = lipgloss.NewStyle().
			Foreground(CurrentTUITheme.Accent)

	helpStyle = lipgloss.NewStyle().
			Foreground(CurrentTUITheme.Secondary)

	errorStyle = lipgloss.NewStyle().
			Foreground(CurrentTUITheme.Error)
)

// refreshStyles rebuilds theme-derived styles from CurrentTUITheme.
// Called after SetTUITheme changes the active theme.
func refreshStyles() {
	titleStyle = titleStyle.Foreground(CurrentTUITheme.Primary)
	inputStyle = inputStyle.BorderForeground(CurrentTUITheme.Secondary)
	resultStyle = resultStyle.Foreground(CurrentTUITheme.Foreground)
	urlStyle = urlStyle.Foreground(CurrentTUITheme.Accent)
	helpStyle = helpStyle.Foreground(CurrentTUITheme.Secondary)
	errorStyle = errorStyle.Foreground(CurrentTUITheme.Error)
}

type model struct {
	client    *api.Client
	input     textinput.Model
	viewport  viewport.Model
	results   []api.SearchResult
	err       error
	searching bool
	width     int
	height    int
	// sizeMode is the responsive breakpoint tier per AI.md PART 32.
	sizeMode terminal.SizeMode
	// debug enables verbose diagnostics (window resize, etc) to stderr.
	// Per AI.md PART 32 window resize example.
	debug bool
}

type searchResultMsg struct {
	results []api.SearchResult
	err     error
}

func initialModel(client *api.Client, debug bool) model {
	ti := textinput.New()
	ti.Placeholder = "Enter search query..."
	ti.Focus()
	ti.Width = 50

	return model{
		client: client,
		input:  ti,
		debug:  debug,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if m.input.Value() != "" {
				m.searching = true
				return m, m.doSearch
			}
		case "esc":
			m.input.SetValue("")
			m.results = nil
			m.err = nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport = viewport.New(msg.Width, msg.Height-6)
		// Per AI.md PART 32: responsive breakpoint tier for the current size.
		m.sizeMode = terminal.GetSize().Mode
		// Per AI.md PART 32 window resize example: debug-mode diagnostics.
		if m.debug {
			log.Printf("Window resize: %dx%d, mode: %s", msg.Width, msg.Height, m.sizeMode)
		}

	case searchResultMsg:
		m.searching = false
		m.results = msg.results
		m.err = msg.err
		m.viewport.SetContent(m.renderResults())
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) doSearch() tea.Msg {
	resp, err := m.client.Search(m.input.Value(), 1, 20)
	if err != nil {
		return searchResultMsg{err: err}
	}
	return searchResultMsg{results: resp.Results}
}

func (m model) renderResults() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	if len(m.results) == 0 {
		return helpStyle.Render("No results found")
	}

	var sb strings.Builder
	for i, r := range m.results {
		sb.WriteString(resultStyle.Render(fmt.Sprintf("%d. %s", i+1, r.Title)))
		sb.WriteString("\n")
		sb.WriteString(urlStyle.Render("   " + r.URL))
		sb.WriteString("\n")
		if r.Description != "" {
			sb.WriteString(helpStyle.Render("   " + r.Description))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m model) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Search"))
	sb.WriteString("\n\n")

	sb.WriteString(inputStyle.Render(m.input.View()))
	sb.WriteString("\n\n")

	if m.searching {
		sb.WriteString(helpStyle.Render("Searching..."))
	} else {
		sb.WriteString(m.viewport.View())
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("Enter: search • Esc: clear • q: quit"))

	return sb.String()
}

// RunTUIApp starts the TUI application.
// themeName selects the active TUITheme (dark, light, or system - see SetTUITheme).
// debug enables verbose window-resize/diagnostic logging per AI.md PART 32.
func RunTUIApp(client *api.Client, themeName string, debug bool) error {
	SetTUITheme(themeName)
	refreshStyles()

	p := tea.NewProgram(initialModel(client, debug), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
