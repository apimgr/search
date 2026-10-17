//go:build gui && !freebsd && !netbsd && !openbsd

// Package gui implements the native client interface for local displays.
package gui

import (
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
	clientpath "github.com/apimgr/search/src/client/path"
)

// Available reports whether this build includes native GUI support.
func Available() bool { return true }

// Launch opens the native Search client window.
func Launch(client *api.Client) error {
	if client == nil {
		return fmt.Errorf("API client is not initialized")
	}
	query := state.NewSignal("")
	status := state.NewSignal("Enter a search query.")
	page := state.NewSignal(1)
	pages := state.NewSignal(1)
	theme := systemTheme()
	var requestID atomic.Uint64
	activeQuery := ""
	search := func() {
		q := strings.TrimSpace(query.Get())
		if q == "" {
			status.Set("Enter a search query.")
			return
		}
		if q != activeQuery {
			activeQuery = q
			page.Set(1)
		}
		currentPage := page.Get()
		request := requestID.Add(1)
		status.Set("Searching…")
		go func() {
			result, err := client.Search(q, currentPage, 20)
			if requestID.Load() != request {
				return
			}
			if err != nil {
				status.Set("Search failed: " + err.Error())
				return
			}
			pages.Set(max(result.Pagination.Pages, 1))
			var output strings.Builder
			fmt.Fprintf(&output, "%d results for %q — page %d of %d\n\n", result.Pagination.Total, q, result.Pagination.Page, max(result.Pagination.Pages, 1))
			for i, item := range result.Results {
				if i == 20 {
					break
				}
				fmt.Fprintf(&output, "%d. %s\n   %s\n   %s\n\n", i+1, item.Title, item.URL, item.Description)
			}
			if result.Pagination.Total == 0 {
				output.WriteString("No results found.")
			}
			status.Set(output.String())
		}()
	}

	field := textfield.New(
		textfield.A11yLabel("Search query"),
		textfield.Placeholder("Search the web"),
		textfield.InputTypeOpt(textfield.TypeSearch),
		textfield.ValueSignal(query),
		textfield.OnSubmit(func(string) { search() }),
		textfield.PainterOpt(material3.TextFieldPainter{Theme: theme}),
	).Padding(12)
	searchButton := button.New(
		button.TextOpt("Search"),
		button.A11yHint("Run the web search"),
		button.OnClick(search),
		button.PainterOpt(material3.ButtonPainter{Theme: theme}),
	)
	previous := button.New(
		button.TextOpt("Previous page"),
		button.OnClick(func() {
			if page.Get() > 1 {
				page.Set(page.Get() - 1)
				search()
			}
		}),
		button.PainterOpt(material3.ButtonPainter{Theme: theme}),
	)
	next := button.New(
		button.TextOpt("Next page"),
		button.OnClick(func() {
			if page.Get() < pages.Get() {
				page.Set(page.Get() + 1)
				search()
			}
		}),
		button.PainterOpt(material3.ButtonPainter{Theme: theme}),
	)
	results := scrollview.New(
		primitives.TextFn(func() string { return status.Get() }).FontSize(14),
		scrollview.DirectionOpt(scrollview.Vertical),
	)
	root := primitives.Box(
		primitives.Text("Search").FontSize(28).Bold(),
		primitives.Text("Search the web across configured search engines.").FontSize(15),
		primitives.Box(field, searchButton).Gap(12),
		results,
		primitives.Box(previous, next).Gap(12),
	).Padding(28).Gap(18)

	return runWindow("Search", 1000, 720, root, status)
}

// Configure opens the first-run wizard, validates the server, and stores
// credentials with owner-only permissions.
func Configure() error {
	server := state.NewSignal("https://")
	token := state.NewSignal("")
	status := state.NewSignal("Connect to a Search server to continue.")
	var connected atomic.Bool
	theme := systemTheme()
	connect := func() {
		address := strings.TrimSpace(server.Get())
		parsed, err := url.Parse(address)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			status.Set("Enter a valid http:// or https:// server URL.")
			return
		}
		address = strings.TrimRight(address, "/")
		client := api.NewClient(address, strings.TrimSpace(token.Get()), 10)
		if _, err := client.GetVersion(); err != nil {
			status.Set("Connection failed: " + err.Error())
			return
		}
		clicfg.Set("server.primary", address)
		if value := strings.TrimSpace(token.Get()); value != "" {
			clicfg.Set("auth.token", value)
		}
		if err := clientpath.EnsureDirs(); err != nil {
			status.Set("Unable to create configuration directory: " + err.Error())
			return
		}
		configPath, err := clientpath.ConfigFile()
		if err != nil {
			status.Set("Unable to locate configuration file: " + err.Error())
			return
		}
		if err := clicfg.WriteConfigAs(configPath); err != nil {
			status.Set("Unable to save configuration: " + err.Error())
			return
		}
		if err := os.Chmod(configPath, 0600); err != nil {
			status.Set("Unable to secure configuration file: " + err.Error())
			return
		}
		connected.Store(true)
		status.Set("Connected and saved. Close this window to continue.")
	}
	root := primitives.Box(
		primitives.Text("Connect to Search").FontSize(28).Bold(),
		primitives.Text("Enter the URL of the Search server you want to use.").FontSize(15),
		primitives.Text("Server URL").FontSize(14),
		textfield.New(
			textfield.A11yLabel("Server URL"),
			textfield.Placeholder("https://search.example.com"),
			textfield.ValueSignal(server),
			textfield.PainterOpt(material3.TextFieldPainter{Theme: theme}),
		).Padding(12),
		primitives.Text("API token (optional)").FontSize(14),
		textfield.New(
			textfield.A11yLabel("API token"),
			textfield.Placeholder("Leave blank for anonymous access"),
			textfield.InputTypeOpt(textfield.TypePassword),
			textfield.ValueSignal(token),
			textfield.PainterOpt(material3.TextFieldPainter{Theme: theme}),
		).Padding(12),
		button.New(
			button.TextOpt("Test connection and save"),
			button.A11yHint("Validate the server and save configuration"),
			button.OnClick(connect),
			button.PainterOpt(material3.ButtonPainter{Theme: theme}),
		),
		primitives.TextFn(func() string { return status.Get() }).FontSize(14).MaxLines(5),
	).Padding(28).Gap(14)
	if err := runWindow("Search CLI Setup", 800, 640, root, status); err != nil {
		return err
	}
	if !connected.Load() {
		return fmt.Errorf("setup window closed before a server was configured")
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
				status.Set("Unable to initialize the display: " + err.Error())
				return
			}
		}
		ui.Frame()
		cw, ch := canvas.Size()
		if cw != w || ch != h {
			if err := canvas.Resize(w, h); err != nil {
				status.Set("Unable to resize the display: " + err.Error())
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
			status.Set("Unable to render the window: " + err.Error())
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
