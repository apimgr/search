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
	"github.com/apimgr/search/src/common/display"
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

	selectedStyle = lipgloss.NewStyle().
			Foreground(CurrentTUITheme.Primary).
			Bold(true)
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
	selectedStyle = selectedStyle.Foreground(CurrentTUITheme.Primary)
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
	// layout is the responsive layout derived from sizeMode.
	layout LayoutConfig
	// symbols is the Unicode or ASCII glyph set for the current terminal.
	symbols SymbolSet
	// selected is the index of the highlighted result.
	selected int
	// offset is the index of the first visible result.
	offset int
	// showHelp toggles the help screen bound to "?".
	showHelp bool
	// inputFocused reports whether typing edits the query or drives navigation.
	inputFocused bool
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

	mode := terminal.GetSize().Mode
	return model{
		client:       client,
		input:        ti,
		debug:        debug,
		sizeMode:     mode,
		layout:       GetLayoutConfig(mode),
		symbols:      GetTUISymbols(display.Detect()),
		inputFocused: true,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		next, cmd, handled := m.handleKey(msg)
		if handled {
			return next, cmd
		}
		m = next

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Per AI.md PART 32: responsive breakpoint tier for the current size.
		m.sizeMode = terminal.GetSize().Mode
		m.layout = GetLayoutConfig(m.sizeMode)
		m.viewport = viewport.New(msg.Width, m.contentHeight())
		m.viewport.SetContent(m.renderResults())
		// Per AI.md PART 32 window resize example: debug-mode diagnostics.
		if m.debug {
			log.Printf("Window resize: %dx%d, mode: %s", msg.Width, msg.Height, m.sizeMode)
		}

	case searchResultMsg:
		m.searching = false
		m.results = msg.results
		m.err = msg.err
		m.selected = 0
		m.offset = 0
		m.viewport.SetContent(m.renderResults())
	}

	var cmd tea.Cmd
	if m.inputFocused {
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleKey implements the phone-friendly key bindings from AI.md PART 32.
// It returns the updated model, an optional command, and whether the key was
// fully handled (in which case Update returns immediately).
func (m model) handleKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	key := msg.String()

	// Quit and help work in every mode.
	switch key {
	case "ctrl+c":
		return m, tea.Quit, true
	case "?":
		if !m.inputFocused {
			m.showHelp = !m.showHelp
			return m, nil, true
		}
	}

	if m.showHelp {
		// Any key closes the help screen.
		m.showHelp = false
		return m, nil, true
	}

	if m.inputFocused {
		switch key {
		case "enter":
			if m.input.Value() != "" {
				m.searching = true
				return m, m.doSearch, true
			}
			return m, nil, true
		case "esc":
			m.inputFocused = false
			m.input.Blur()
			return m, nil, true
		}
		return m, nil, false
	}

	switch key {
	case "q":
		return m, tea.Quit, true
	case "/":
		m.inputFocused = true
		m.input.Focus()
		return m, textinput.Blink, true
	case "j", "down":
		m = m.moveSelection(1)
		return m, nil, true
	case "k", "up":
		m = m.moveSelection(-1)
		return m, nil, true
	case "l", "right", "pgdown", " ":
		m = m.moveSelection(m.pageRows())
		return m, nil, true
	case "h", "left", "pgup":
		m = m.moveSelection(-m.pageRows())
		return m, nil, true
	case "g", "home":
		m.selected = 0
		m.offset = 0
		m.viewport.SetContent(m.renderResults())
		return m, nil, true
	case "G", "end":
		m = m.moveSelection(len(m.results))
		return m, nil, true
	case "esc", "b":
		m.input.SetValue("")
		m.results = nil
		m.err = nil
		m.selected = 0
		m.offset = 0
		m.inputFocused = true
		m.input.Focus()
		m.viewport.SetContent(m.renderResults())
		return m, textinput.Blink, true
	case "enter":
		m.viewport.SetContent(m.renderResults())
		return m, nil, true
	}
	return m, nil, false
}

// moveSelection shifts the highlighted result by delta, clamping to range and
// scrolling the viewport so the selection stays visible.
func (m model) moveSelection(delta int) model {
	if len(m.results) == 0 {
		return m
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected > len(m.results)-1 {
		m.selected = len(m.results) - 1
	}
	m.offset = ensureVisible(m.selected, m.offset, m.pageRows())
	m.viewport.SetContent(m.renderResults())
	return m
}

// pageRows is how many results fit on one screen at the current size.
func (m model) pageRows() int {
	// Each result occupies a title line plus, above Micro, a URL line.
	perResult := 2
	if m.sizeMode <= terminal.SizeModeMicro {
		perResult = 1
	}
	rows := pageSize(m.contentHeight(), 0) / perResult
	if rows < 1 {
		return 1
	}
	return rows
}

// contentHeight is the viewport height after header, input and footer chrome.
func (m model) contentHeight() int {
	chrome := 2
	if m.layout.ShowHeader {
		chrome += 2
	}
	if m.layout.ShowFooter {
		chrome += 2
	}
	return pageSize(m.height, chrome)
}

// resultWidth is the column budget for a rendered result line.
func (m model) resultWidth() int {
	width := m.layout.TruncateAt
	if m.layout.ShowSidebar {
		available := m.width - m.layout.SidebarWidth - GetSpacingForMode(m.sizeMode)
		if available > 0 && (width == 0 || available < width) {
			width = available
		}
	}
	if m.width > 0 && (width == 0 || m.width < width) && m.sizeMode != terminal.SizeModeMassive {
		width = m.width - GetSpacingForMode(m.sizeMode)
	}
	if width < 0 {
		width = 0
	}
	return width
}

func (m model) doSearch() tea.Msg {
	resp, err := m.client.Search(m.input.Value(), 1, 20)
	if err != nil {
		return searchResultMsg{err: err}
	}
	return searchResultMsg{results: resp.Results}
}

// renderResults renders the visible slice of results for the current size mode.
// Per AI.md PART 32 the density, truncation and selection marker all follow the
// responsive LayoutConfig.
func (m model) renderResults() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("%s Error: %v", m.symbols.Error, m.err))
	}

	if len(m.results) == 0 {
		return helpStyle.Render("No results found")
	}

	width := m.resultWidth()
	indent := strings.Repeat(" ", GetSpacingForMode(m.sizeMode))

	end := m.offset + m.pageRows()
	if end > len(m.results) || m.layout.VerticalScroll {
		end = len(m.results)
	}
	start := m.offset
	if start > len(m.results) {
		start = len(m.results)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		r := m.results[i]

		marker := "  "
		if i == m.selected {
			marker = m.symbols.Arrow + " "
		}

		title := fmt.Sprintf("%s%d. %s", marker, i+1, truncate(r.Title, width))
		if i == m.selected {
			sb.WriteString(selectedStyle.Render(title))
		} else {
			sb.WriteString(resultStyle.Render(title))
		}
		sb.WriteString("\n")

		if m.sizeMode <= terminal.SizeModeMicro {
			continue
		}

		sb.WriteString(urlStyle.Render(indent + truncateMiddle(r.URL, width)))
		sb.WriteString("\n")

		if r.Description != "" && m.sizeMode >= terminal.SizeModeStandard {
			sb.WriteString(helpStyle.Render(indent + truncate(r.Description, width)))
			sb.WriteString("\n")
		}
		if m.sizeMode >= terminal.SizeModeCompact {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// renderHelp renders the keyboard help screen bound to "?".
// Per AI.md PART 32 the help screen is a minimum TUI feature and adapts to the
// current size mode.
func (m model) renderHelp() string {
	type binding struct {
		keys string
		desc string
	}
	bindings := []binding{
		{"/", "focus the search box"},
		{"enter", "run the search"},
		{"j / down", "next result"},
		{"k / up", "previous result"},
		{"l / right", "next page"},
		{"h / left", "previous page"},
		{"g", "first result"},
		{"G", "last result"},
		{"esc / b", "clear and go back"},
		{"?", "toggle this help"},
		{"q / ctrl+c", "quit"},
	}

	keyWidth := 0
	for _, b := range bindings {
		if len(b.keys) > keyWidth {
			keyWidth = len(b.keys)
		}
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Keyboard Help"))
	sb.WriteString("\n\n")

	compact := m.sizeMode <= terminal.SizeModeMinimal
	for _, b := range bindings {
		if compact {
			sb.WriteString(resultStyle.Render(b.keys))
			sb.WriteString("\n")
			sb.WriteString(helpStyle.Render("  " + b.desc))
			sb.WriteString("\n")
			continue
		}
		sb.WriteString(resultStyle.Render(padTo(b.keys, keyWidth+2)))
		sb.WriteString(helpStyle.Render(b.desc))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("Press any key to return"))
	return sb.String()
}

// renderFooter renders the key hint bar for the current size mode.
func (m model) renderFooter() string {
	if m.sizeMode <= terminal.SizeModeMinimal {
		return helpStyle.Render("? help • q quit")
	}
	if m.inputFocused {
		return helpStyle.Render("Enter: search • Esc: browse results • Ctrl+C: quit")
	}
	return helpStyle.Render("j/k: move • /: search • ?: help • q: quit")
}

func (m model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}

	var sb strings.Builder

	if m.layout.ShowHeader {
		sb.WriteString(titleStyle.Render("Search"))
		sb.WriteString("\n\n")
	}

	if m.layout.ShowBorders {
		sb.WriteString(inputStyle.Render(m.input.View()))
	} else {
		sb.WriteString(m.input.View())
	}
	sb.WriteString("\n")
	if m.sizeMode > terminal.SizeModeMicro {
		sb.WriteString("\n")
	}

	if m.searching {
		sb.WriteString(helpStyle.Render(m.symbols.Spinner[0] + " Searching..."))
	} else {
		sb.WriteString(m.viewport.View())
	}

	if m.layout.ShowFooter {
		sb.WriteString("\n")
		sb.WriteString(m.renderFooter())
	}

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
