package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/apimgr/search/src/client/api"
)

// Dracula colors per AI.md PART 33
var (
	background = lipgloss.Color("#282a36")
	foreground = lipgloss.Color("#f8f8f2")
	selection  = lipgloss.Color("#44475a")
	comment    = lipgloss.Color("#6272a4")
	cyan       = lipgloss.Color("#8be9fd")
	green      = lipgloss.Color("#50fa7b")
	orange     = lipgloss.Color("#ffb86c")
	pink       = lipgloss.Color("#ff79c6")
	purple     = lipgloss.Color("#bd93f9")
	red        = lipgloss.Color("#ff5555")
	yellow     = lipgloss.Color("#f1fa8c")
)

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true).
			Padding(0, 1)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(comment).
			Padding(0, 1)

	resultStyle = lipgloss.NewStyle().
			Foreground(foreground)

	urlStyle = lipgloss.NewStyle().
			Foreground(cyan)

	helpStyle = lipgloss.NewStyle().
			Foreground(comment)

	errorStyle = lipgloss.NewStyle().
			Foreground(red)
)

type model struct {
	client    *api.Client
	input     textinput.Model
	viewport  viewport.Model
	results   []api.SearchResult
	err       error
	searching bool
	width     int
	height    int
}

type searchResultMsg struct {
	results []api.SearchResult
	err     error
}

func initialModel(client *api.Client) model {
	ti := textinput.New()
	ti.Placeholder = "Enter search query..."
	ti.Focus()
	ti.Width = 50

	return model{
		client: client,
		input:  ti,
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
		if r.Snippet != "" {
			sb.WriteString(helpStyle.Render("   " + r.Snippet))
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

// Run starts the TUI application
func Run(client *api.Client) error {
	p := tea.NewProgram(initialModel(client), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
