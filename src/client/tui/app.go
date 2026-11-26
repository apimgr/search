package tui

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/client/surface"
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
	client        *api.Client
	input         textinput.Model
	commandInputs []textinput.Model
	viewport      viewport.Model
	results       []api.SearchResult
	err           error
	searching     bool
	width         int
	height        int
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
	debug          bool
	commands       []surface.Command
	commandIndex   int
	commandMode    bool
	commandView    bool
	commandFocus   int
	commandRunning bool
	commandOutput  string
	commandErr     error
}

type searchResultMsg struct {
	results []api.SearchResult
	err     error
}

type commandResultMsg struct {
	output string
	err    error
}

// Views returns TUI views in CLI help order.
func Views() []surface.Command { return surface.Commands() }

func initialModel(client *api.Client, debug bool) model {
	ti := textinput.New()
	ti.Placeholder = surface.Text("client.surface.enter_value")
	ti.Focus()
	ti.Width = 50

	mode := terminal.GetSize().Mode
	return model{
		client:       client,
		input:        ti,
		commands:     Views(),
		commandMode:  true,
		debug:        debug,
		sizeMode:     mode,
		layout:       GetLayoutConfig(mode),
		symbols:      GetTUISymbols(display.Detect()),
		inputFocused: false,
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
		m.viewport.SetContent(m.renderContent())
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

	case commandResultMsg:
		m.commandRunning = false
		m.commandOutput = msg.output
		m.commandErr = msg.err
		m.viewport.SetContent(m.renderContent())
	}

	var cmd tea.Cmd
	if m.commandMode && m.commandView && m.commandFocus >= 0 && m.commandFocus < len(m.commandInputs) {
		m.commandInputs[m.commandFocus], cmd = m.commandInputs[m.commandFocus].Update(msg)
		cmds = append(cmds, cmd)
	} else if !m.commandMode && m.inputFocused {
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
	if key == "ctrl+c" {
		return m, tea.Quit, true
	}

	if m.showHelp {
		// Any key closes the help screen.
		m.showHelp = false
		m.viewport.SetContent(m.renderContent())
		return m, nil, true
	}

	if m.commandMode {
		if key == "?" {
			m.showHelp = true
			return m, nil, true
		}
		if !m.commandView {
			switch key {
			case "/":
				m.commandMode = false
				m.inputFocused = true
				m.input.Focus()
				m.viewport.SetContent(m.renderContent())
				return m, textinput.Blink, true
			case "q", "esc":
				return m, tea.Quit, true
			case "j", "down", "l", "right":
				if len(m.commands) > 0 {
					m.commandIndex = (m.commandIndex + 1) % len(m.commands)
					m.viewport.SetContent(m.renderContent())
				}
				return m, nil, true
			case "k", "up", "h", "left":
				if len(m.commands) > 0 {
					m.commandIndex = (m.commandIndex + len(m.commands) - 1) % len(m.commands)
					m.viewport.SetContent(m.renderContent())
				}
				return m, nil, true
			case "enter":
				m = m.openCommand()
				m.viewport.SetContent(m.renderContent())
				return m, nil, true
			}
			return m, nil, false
		}

		fieldCount := len(m.commandInputs)
		switch key {
		case "q":
			return m, tea.Quit, true
		case "esc", "b":
			m.commandView = false
			m.commandInputs = nil
			m.commandOutput = ""
			m.commandErr = nil
			m.viewport.SetContent(m.renderContent())
			return m, nil, true
		case "tab", "shift+tab":
			if key == "shift+tab" {
				if m.commandFocus == 0 {
					m.commandFocus = fieldCount
				} else {
					m.commandFocus--
				}
			} else if m.commandFocus >= fieldCount {
				m.commandFocus = 0
			} else {
				m.commandFocus++
			}
			m.focusCommandInput()
			m.viewport.SetContent(m.renderContent())
			return m, textinput.Blink, true
		case "enter":
			if m.commandFocus < fieldCount {
				m.commandFocus++
				m.focusCommandInput()
				m.viewport.SetContent(m.renderContent())
				return m, textinput.Blink, true
			}
			if !m.commandRunning {
				m.commandRunning = true
				m.commandOutput = ""
				m.commandErr = nil
				return m, m.doCommand, true
			}
			return m, nil, true
		case "?":
			m.showHelp = true
			return m, nil, true
		}
		if m.commandFocus < fieldCount {
			return m, nil, false
		}
		return m, nil, false
	}

	if key == "?" && !m.inputFocused {
		m.showHelp = true
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
	if key == "esc" {
		m.input.SetValue("")
		m.inputFocused = true
		m.input.Focus()
		m.searching = false
		m.results = nil
		m.err = nil
		m.selected = 0
		m.offset = 0
		m.viewport.SetContent(m.renderContent())
		return m, textinput.Blink, true
	}
	if !m.commandMode {
		switch key {
		case "/":
			m.inputFocused = true
			m.input.Focus()
			m.viewport.SetContent(m.renderContent())
			return m, textinput.Blink, true
		case "j", "down":
			m = m.moveSelection(1)
			return m, nil, true
		case "k", "up":
			m = m.moveSelection(-1)
			return m, nil, true
		case "g":
			m.selected = 0
			m.offset = 0
			m.viewport.SetContent(m.renderResults())
			return m, nil, true
		case "G":
			if len(m.results) > 0 {
				m.selected = len(m.results) - 1
				m.offset = ensureVisible(m.selected, m.offset, m.pageRows())
				m.viewport.SetContent(m.renderResults())
			}
			return m, nil, true
		}
	}
	if key == "tab" {
		m.commandMode = true
		m.viewport.SetContent(m.renderContent())
		return m, nil, true
	}
	return m, nil, false
}

func (m model) openCommand() model {
	if len(m.commands) == 0 {
		return m
	}
	m.commandView = true
	m.commandOutput = ""
	m.commandErr = nil
	m.commandInputs = make([]textinput.Model, len(m.commands[m.commandIndex].Arguments))
	for i := range m.commands[m.commandIndex].Arguments {
		field := textinput.New()
		field.Placeholder = surface.Text("client.surface.enter_value")
		field.Width = 50
		m.commandInputs[i] = field
	}
	m.commandFocus = 0
	m.focusCommandInput()
	return m
}

func (m *model) focusCommandInput() {
	for i := range m.commandInputs {
		m.commandInputs[i].Blur()
	}
	if m.commandFocus < len(m.commandInputs) {
		m.commandInputs[m.commandFocus].Focus()
	}
}

func (m model) commandValues() map[string]string {
	values := make(map[string]string, len(m.commandInputs))
	if len(m.commands) == 0 || m.commandIndex >= len(m.commands) {
		return values
	}
	for i, argument := range m.commands[m.commandIndex].Arguments {
		if i < len(m.commandInputs) {
			values[argument.Name] = m.commandInputs[i].Value()
		}
	}
	return values
}

func (m model) doCommand() tea.Msg {
	if len(m.commands) == 0 {
		return commandResultMsg{err: fmt.Errorf("%s", surface.Text("client.surface.no_commands"))}
	}
	output, err := surface.Run(context.Background(), m.client, m.commands[m.commandIndex], m.commandValues())
	return commandResultMsg{output: output, err: err}
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
		return errorStyle.Render(fmt.Sprintf("%s %s %v", m.symbols.Error, surface.Text("client.surface.error_label"), m.err))
	}

	if len(m.results) == 0 {
		return helpStyle.Render(surface.Text("client.surface.no_results"))
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

// renderContent renders the selected command screen or the ordered command menu.
func (m model) renderContent() string {
	if !m.commandMode {
		return m.renderResults()
	}
	var sb strings.Builder
	if !m.commandView {
		sb.WriteString(titleStyle.Render(surface.Text("client.surface.menu_title")))
		sb.WriteString("\n\n")
		start, end := visibleCommandRange(m.commandIndex, len(m.commands), m.viewport.Height)
		for i := start; i < end; i++ {
			command := m.commands[i]
			line := fmt.Sprintf("%s %-24s %s", " ", command.Name(), surface.DescriptionText(command))
			if i == m.commandIndex {
				line = fmt.Sprintf("%s %-24s %s", m.symbols.Arrow, command.Name(), surface.DescriptionText(command))
				sb.WriteString(selectedStyle.Render(line))
			} else {
				sb.WriteString(resultStyle.Render(line))
			}
			sb.WriteByte('\n')
		}
		return sb.String()
	}
	if m.commandIndex < 0 || m.commandIndex >= len(m.commands) {
		return errorStyle.Render(surface.Text("client.surface.no_selection"))
	}
	command := m.commands[m.commandIndex]
	sb.WriteString(titleStyle.Render(command.Name()))
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(surface.DescriptionText(command)))
	sb.WriteString("\n\n")
	for i, argument := range command.Arguments {
		label := surface.Text(argument.Label)
		if argument.Required {
			label = surface.Text("client.surface.required", label)
		}
		sb.WriteString(resultStyle.Render(label))
		sb.WriteByte('\n')
		if i < len(m.commandInputs) {
			if m.layout.ShowBorders {
				sb.WriteString(inputStyle.Render(m.commandInputs[i].View()))
			} else {
				sb.WriteString(m.commandInputs[i].View())
			}
		}
		sb.WriteString("\n")
	}
	runLabel := "  " + surface.Text("client.surface.run")
	if m.commandFocus == len(m.commandInputs) {
		runLabel = m.symbols.Arrow + " " + surface.Text("client.surface.run")
	}
	if m.commandRunning {
		runLabel = "  " + surface.Text("client.surface.running")
	}
	sb.WriteString(selectedStyle.Render(runLabel))
	if m.commandErr != nil {
		sb.WriteString("\n\n")
		sb.WriteString(errorStyle.Render(surface.Text("client.surface.command_failed", m.commandErr.Error())))
	}
	if m.commandOutput != "" {
		sb.WriteString("\n\n")
		sb.WriteString(resultStyle.Render(strings.TrimSpace(m.commandOutput)))
	}
	return sb.String()
}

func visibleCommandRange(selected, count, height int) (int, int) {
	if height <= 0 || height >= count {
		return 0, count
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= count {
		selected = count - 1
	}
	start := selected - height/2
	if start < 0 {
		start = 0
	}
	if start+height > count {
		start = count - height
	}
	return start, start + height
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
		{"j / down", "client.surface.help.menu_move"},
		{"k / up", "client.surface.help.menu_previous"},
		{"enter", "client.surface.help.open_run"},
		{"tab / shift+tab", "client.surface.help.focus"},
		{"esc / b", "client.surface.help.back"},
		{"?", "client.surface.help.help"},
		{"q / ctrl+c", "client.surface.help.quit"},
	}

	keyWidth := 0
	for _, b := range bindings {
		if len(b.keys) > keyWidth {
			keyWidth = len(b.keys)
		}
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(surface.Text("client.surface.help.title")))
	sb.WriteString("\n\n")

	compact := m.sizeMode <= terminal.SizeModeMinimal
	for _, b := range bindings {
		if compact {
			sb.WriteString(resultStyle.Render(b.keys))
			sb.WriteString("\n")
			sb.WriteString(helpStyle.Render("  " + surface.Text(b.desc)))
			sb.WriteString("\n")
			continue
		}
		sb.WriteString(resultStyle.Render(padTo(b.keys, keyWidth+2)))
		sb.WriteString(helpStyle.Render(surface.Text(b.desc)))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(surface.Text("client.surface.help.return")))
	return sb.String()
}

// renderFooter renders the key hint bar for the current size mode.
func (m model) renderFooter() string {
	if m.commandMode {
		if m.commandView {
			return helpStyle.Render(surface.Text("client.surface.footer.form"))
		}
		return helpStyle.Render(surface.Text("client.surface.footer.menu"))
	}
	return helpStyle.Render(surface.Text("client.surface.footer.menu"))
}

func (m model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}

	var sb strings.Builder
	if m.commandMode {
		if m.layout.ShowHeader {
			sb.WriteString(titleStyle.Render(surface.Text("client.surface.title")))
			sb.WriteString("\n\n")
		}
		if m.commandRunning {
			sb.WriteString(helpStyle.Render(surface.Text("client.surface.running")))
			sb.WriteString("\n")
		}
		sb.WriteString(m.viewport.View())
		if m.layout.ShowFooter {
			sb.WriteString("\n")
			sb.WriteString(m.renderFooter())
		}
		return sb.String()
	}

	if m.layout.ShowHeader {
		sb.WriteString(titleStyle.Render(surface.Text("client.surface.title")))
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
