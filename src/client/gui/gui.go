//go:build gui && !freebsd && !netbsd && !openbsd

// Package gui implements the native client interface for local displays.
package gui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/ui/app"
	"github.com/gogpu/ui/core/button"
	"github.com/gogpu/ui/core/scrollview"
	"github.com/gogpu/ui/core/textfield"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/render"
	"github.com/gogpu/ui/state"
	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/client/clicfg"
	"github.com/apimgr/search/src/client/surface"
	clientpath "github.com/apimgr/search/src/client/path"
)

// Available reports whether this build includes native GUI support.
func Available() bool { return true }

// Screens returns GUI command screens in the same order as CLI root help.
func Screens() []surface.Command { return surface.Commands() }

// Launch opens the native Search client window.
func Launch(client *api.Client) error {
	if client == nil {
		return fmt.Errorf("%s", surface.Text("client.surface.api_client_missing"))
	}
	commands := Screens()
	selected := state.NewSignal(0)
	status := state.NewSignal(surface.Text("client.surface.choose_command"))
	theme := systemTheme()
	maxArguments := 0
	for _, command := range commands {
		if len(command.Arguments) > maxArguments {
			maxArguments = len(command.Arguments)
		}
	}
	values := make([]state.Signal[string], maxArguments)
	labels := make([]state.Signal[string], maxArguments)
	fields := make([]*primitives.BoxWidget, 0, len(values))
	for i := range values {
		values[i] = state.NewSignal("")
		labels[i] = state.NewSignal("")
		field := textfield.New(
			textfield.A11yLabel(surface.Text("client.surface.argument_accessibility")),
			textfield.Placeholder(surface.Text("client.surface.enter_value")),
			textfield.ValueSignal(values[i]),
			textfield.PainterOpt(material3.TextFieldPainter{Theme: theme}),
		).Padding(10)
		fields = append(fields, primitives.Box(primitives.TextFn(func() string { return labels[i].Get() }), field))
	}
	var runID atomic.Uint64
	selectCommand := func(index int) {
		if index < 0 || index >= len(commands) {
			return
		}
		selected.Set(index)
		for i := range values {
			values[i].Set("")
			labels[i].Set("")
		}
		command := commands[index]
		for i, argument := range command.Arguments {
			label := surface.Text(argument.Label)
			if argument.Required {
				label = surface.Text("client.surface.required", label)
			}
		labels[i].Set(label)
		}
		status.Set(surface.DescriptionText(command))
	}
	menuItems := make([]*primitives.BoxWidget, 0, len(commands))
	for i, command := range commands {
		index := i
		menuItems = append(menuItems, primitives.Box(button.New(
			button.TextOpt(command.Name()),
			button.A11yHint(surface.DescriptionText(command)),
			button.OnClick(func() { selectCommand(index) }),
			button.PainterOpt(material3.ButtonPainter{Theme: theme}),
		), primitives.Text(command.Description).FontSize(12)).Gap(5))
	}
	menu := scrollview.New(primitives.Box(menuItems...).Gap(8), scrollview.DirectionOpt(scrollview.Vertical))
	run := func() {
		index := selected.Get()
		command := commands[index]
		arguments := make(map[string]string, len(command.Arguments))
		for i, argument := range command.Arguments {
			arguments[argument.Name] = values[i].Get()
		}
		request := runID.Add(1)
		status.Set(surface.Text("client.surface.running_command", command.Name()))
		go func() {
			output, err := surface.Run(context.Background(), client, command, arguments)
			if runID.Load() != request {
				return
			}
			if err != nil {
				status.Set(surface.Text("client.surface.command_failed", err.Error()) + "\n\n" + output)
				return
			}
			status.Set(output)
		}()
	}
	form := primitives.Box(fields...).Gap(8)
	result := scrollview.New(primitives.TextFn(func() string { return status.Get() }).FontSize(14), scrollview.DirectionOpt(scrollview.Vertical))
	root := primitives.Box(
		primitives.Text(surface.Text("client.surface.title")).FontSize(28).Bold(),
		primitives.Text(surface.Text("client.surface.instructions")).FontSize(15),
		primitives.Box(menu, primitives.Box(
			primitives.TextFn(func() string { return commands[selected.Get()].Name() }).FontSize(20).Bold(),
			form,
			button.New(button.TextOpt(surface.Text("client.surface.run")), button.A11yHint(surface.Text("client.surface.run_accessibility")), button.OnClick(run), button.PainterOpt(material3.ButtonPainter{Theme: theme})),
			result,
		).Gap(12)).Gap(18),
	).Padding(28).Gap(16)

	return runWindow(surface.Text("client.surface.title"), 1000, 720, root, status)
}

// Configure opens the first-run wizard, validates the server, and stores
// credentials with owner-only permissions.
func Configure() error {
	server := state.NewSignal("https://")
	token := state.NewSignal("")
	status := state.NewSignal(surface.Text("client.surface.setup.initial_status"))
	var connected atomic.Bool
	theme := systemTheme()
	connect := func() {
		address := strings.TrimSpace(server.Get())
		parsed, err := url.Parse(address)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			status.Set(surface.Text("client.surface.setup.invalid_url"))
			return
		}
		address = strings.TrimRight(address, "/")
		client := api.NewClient(address, strings.TrimSpace(token.Get()), 10)
		if _, err := client.GetVersion(); err != nil {
			status.Set(surface.Text("client.surface.setup.connection_failed", err.Error()))
			return
		}
		clicfg.Set("server.primary", address)
		if value := strings.TrimSpace(token.Get()); value != "" {
			clicfg.Set("auth.token", value)
		}
		if err := clientpath.EnsureDirs(); err != nil {
			status.Set(surface.Text("client.surface.setup.config_directory_failed", err.Error()))
			return
		}
		configPath, err := clientpath.ConfigFile()
		if err != nil {
			status.Set(surface.Text("client.surface.setup.config_file_failed", err.Error()))
			return
		}
		if err := clicfg.WriteConfigAs(configPath); err != nil {
			status.Set(surface.Text("client.surface.setup.save_failed", err.Error()))
			return
		}
		if err := os.Chmod(configPath, 0600); err != nil {
			status.Set(surface.Text("client.surface.setup.secure_failed", err.Error()))
			return
		}
		connected.Store(true)
		status.Set(surface.Text("client.surface.setup.connected"))
	}
	root := primitives.Box(
		primitives.Text(surface.Text("client.surface.setup.title")).FontSize(28).Bold(),
		primitives.Text(surface.Text("client.surface.setup.instructions")).FontSize(15),
		primitives.Text(surface.Text("client.surface.setup.server_label")).FontSize(14),
		textfield.New(
			textfield.A11yLabel(surface.Text("client.surface.setup.server_label")),
			textfield.Placeholder(surface.Text("client.surface.setup.server_placeholder")),
			textfield.ValueSignal(server),
			textfield.PainterOpt(material3.TextFieldPainter{Theme: theme}),
		).Padding(12),
		primitives.Text(surface.Text("client.surface.setup.token_label")).FontSize(14),
		textfield.New(
			textfield.A11yLabel(surface.Text("client.surface.setup.token_label")),
			textfield.Placeholder(surface.Text("client.surface.setup.token_placeholder")),
			textfield.InputTypeOpt(textfield.TypePassword),
			textfield.ValueSignal(token),
			textfield.PainterOpt(material3.TextFieldPainter{Theme: theme}),
		).Padding(12),
		button.New(
			button.TextOpt(surface.Text("client.surface.setup.connect_button")),
			button.A11yHint(surface.Text("client.surface.setup.connect_accessibility")),
			button.OnClick(connect),
			button.PainterOpt(material3.ButtonPainter{Theme: theme}),
		),
		primitives.TextFn(func() string { return status.Get() }).FontSize(14).MaxLines(5),
	).Padding(28).Gap(14)
	if err := runWindow(surface.Text("client.surface.setup.window_title"), 800, 640, root, status); err != nil {
		return err
	}
	if !connected.Load() {
		return fmt.Errorf("%s", surface.Text("client.surface.setup.closed"))
	}
	return nil
}

func runWindow(title string, width, height int, root *primitives.BoxWidget, status state.Signal[string]) error {
	window := gogpu.NewApp(gogpu.DefaultConfig().WithTitle(title).WithSize(width, height).WithMinSize(800, 600))
	ui := app.New(
		app.WithWindowProvider(window),
		app.WithPlatformProvider(window),
		app.WithEventSource(window.EventSource()),
	)
	ui.SetRoot(root)
	var canvas *ggcanvas.Canvas
	window.OnDraw(func(dc *gogpu.Context) {
		w, h := dc.Width(), dc.Height()
		if w <= 0 || h <= 0 {
			return
		}
		if canvas == nil {
			provider := window.GPUContextProvider()
			if provider == nil {
				return
			}
			var err error
			canvas, err = ggcanvas.New(provider, w, h)
			if err != nil {
				status.Set(surface.Text("client.surface.display_init_failed", err.Error()))
				return
			}
		}
		ui.Frame()
		cw, ch := canvas.Size()
		if cw != w || ch != h {
			if err := canvas.Resize(w, h); err != nil {
				status.Set(surface.Text("client.surface.display_resize_failed", err.Error()))
				return
			}
			cw, ch = canvas.Size()
		}
		canvas.Draw(func(ctx *gg.Context) {
			ctx.SetRGBA(0.08, 0.09, 0.12, 1)
			ctx.DrawRectangle(0, 0, float64(cw), float64(ch))
			ctx.Fill()
			ui.Window().DrawTo(render.NewCanvas(ctx, cw, ch))
		})
		if err := canvas.Render(dc.RenderTarget()); err != nil {
			status.Set(surface.Text("client.surface.display_render_failed", err.Error()))
		}
	})
	window.OnClose(func() { gg.CloseAccelerator() })
	return window.Run()
}

func systemTheme() *material3.Theme {
	seed := widget.Hex(0x6750A4)
	if !systemUsesDarkTheme() {
		return material3.New(seed)
	}
	return material3.NewDark(seed)
}

func systemUsesDarkTheme() bool {
	switch runtime.GOOS {
	case "linux":
		output, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
		return err != nil || strings.Contains(strings.ToLower(string(output)), "dark")
	case "darwin":
		output, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
		return err != nil || strings.Contains(strings.ToLower(string(output)), "dark")
	case "windows":
		output, err := exec.Command("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "/v", "AppsUseLightTheme").Output()
		return err != nil || strings.Contains(strings.ToLower(string(output)), "0x0")
	default:
		return true
	}
}
