package tui

import (
	"strings"
	"testing"

	"github.com/apimgr/search/src/client/surface"
)

func TestCommandMenuAndScreensRenderEveryCommand(t *testing.T) {
	m := initialModel(nil, false)
	menu := m.renderContent()
	for _, command := range m.commands {
		if !strings.Contains(menu, command.Name()) {
			t.Errorf("command menu does not render %q", command.Name())
		}
	}
	for i, command := range m.commands {
		m.commandIndex = i
		m = m.openCommand()
		view := m.renderContent()
		if !strings.Contains(view, command.Name()) || !strings.Contains(view, surface.Text("client.surface.run")) || !strings.Contains(view, surface.DescriptionText(command)) {
			t.Errorf("command view %q is not reachable or does not render its action", command.Name())
		}
		for _, argument := range command.Arguments {
			if !strings.Contains(view, surface.Text(argument.Label)) {
				t.Errorf("command view %q does not render argument %q", command.Name(), argument.Label)
			}
		}
		m.commandView = false
	}
}
