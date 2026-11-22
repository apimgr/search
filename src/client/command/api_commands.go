package cmd

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/config"
)

// registerAPICommands wires every server API operation to a subcommand.
// Per AI.md PART 32: "CLI must work 100% with entire server functionality" —
// each operation exposed by src/client/api has a matching command here.
func registerAPICommands() {
	rootCmd.addCommand("search", newSearchCommand())
	rootCmd.addCommand("health", newHealthCommand())
	rootCmd.addCommand("info", newInfoCommand())
	rootCmd.addCommand("engines", newEnginesCommand())
	rootCmd.addCommand("categories", newCategoriesCommand())
	rootCmd.addCommand("bangs", newBangsCommand())
	rootCmd.addCommand("widgets", newWidgetsCommand())
	rootCmd.addCommand("widget", newWidgetCommand())
	rootCmd.addCommand("instant", newInstantCommand())
	rootCmd.addCommand("answer", newAnswerCommand())
	rootCmd.addCommand("related", newRelatedCommand())
	rootCmd.addCommand("autocomplete", newAutocompleteCommand())
	rootCmd.addCommand("preferences", newPreferencesCommand())
	rootCmd.addCommand("alerts", newAlertsCommand())
	rootCmd.addCommand("metrics", newMetricsCommand())
	rootCmd.addCommand("server-config", newServerConfigCommand())
	rootCmd.addCommand("scheduler", newSchedulerCommand())
	rootCmd.addCommand("autodiscover", newAutodiscoverCommand())
}

func init() {
	registerAPICommands()
}

// withClient initializes the shared API client before running fn.
func withClient(fn func(c *api.Client) error) error {
	if apiClient == nil {
		if err := initClient(); err != nil {
			return err
		}
	}
	return fn(apiClient)
}

// requireArg returns the joined positional arguments or a usage error.
func requireArg(args []string, what string) (string, error) {
	joined := strings.TrimSpace(strings.Join(args, " "))
	if joined == "" {
		return "", fmt.Errorf("%w: %s is required", ErrUsage, what)
	}
	return joined, nil
}

// stringListView renders a flat list of strings under a single column.
func stringListView(header string, items []string) view {
	rows := make([][]string, 0, len(items))
	for _, s := range items {
		rows = append(rows, []string{s})
	}
	return view{Data: items, Headers: []string{header}, Rows: rows, Plain: items}
}

func newSearchCommand() *command {
	return &command{
		Use:   "search [query]",
		Short: "Search the index",
		Long: `Search the index and print the results.

Examples:
  ` + getBinaryName() + ` search golang generics
  ` + getBinaryName() + ` search golang --output json --limit 25`,
		run: func(args []string) error {
			query, err := requireArg(args, "a search query")
			if err != nil {
				return err
			}
			return runSearch(query)
		},
	}
}

func newHealthCommand() *command {
	return &command{
		Use:   "health",
		Short: "Show the server health report",
		Long: `Show the extended server health report, including build info,
enabled features, dependency checks and request statistics.`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				h, err := c.Health()
				if err != nil {
					return err
				}
				rows := [][]string{
					{"status", h.Status},
					{"version", h.Version},
					{"go_version", h.GoVersion},
					{"mode", h.Mode},
					{"uptime", h.Uptime},
				}
				if h.Build != nil {
					rows = append(rows, []string{"commit", h.Build.Commit}, []string{"build_date", h.Build.Date})
				}
				flattenHealthFields(&rows, "feature", h.Features)
				flattenHealthFields(&rows, "check", h.Checks)
				if h.Stats != nil {
					rows = append(rows,
						[]string{"requests_total", strconv.FormatInt(h.Stats.RequestsTotal, 10)},
						[]string{"active_connections", strconv.Itoa(h.Stats.ActiveConnections)},
					)
				}
				plain := make([]string, 0, len(rows))
				for _, r := range rows {
					plain = append(plain, r[0]+"="+r[1])
				}
				return render(getOutputFormat(), view{
					Data:    h,
					Headers: []string{"FIELD", "VALUE"},
					Rows:    rows,
					Plain:   plain,
				})
			})
		},
	}
}

func newInfoCommand() *command {
	return &command{
		Use:   "info",
		Short: "Show server information",
		Long:  `Show the server name, version, mode, uptime, engine counts and feature flags.`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				info, err := c.GetInfo()
				if err != nil {
					return err
				}
				rows := [][]string{
					{"name", info.Name},
					{"version", info.Version},
					{"description", info.Description},
					{"mode", info.Mode},
					{"uptime", info.Uptime},
					{"engines_total", strconv.Itoa(info.Engines.Total)},
					{"engines_enabled", strconv.Itoa(info.Engines.Enabled)},
				}
				for name, enabled := range info.Features {
					rows = append(rows, []string{"feature." + name, boolCell(enabled)})
				}
				plain := make([]string, 0, len(rows))
				for _, r := range rows {
					plain = append(plain, r[0]+"="+r[1])
				}
				return render(getOutputFormat(), view{
					Data:    info,
					Headers: []string{"FIELD", "VALUE"},
					Rows:    rows,
					Plain:   plain,
				})
			})
		},
	}
}

func newEnginesCommand() *command {
	return &command{
		Use:   "engines [id]",
		Short: "List search engines or show one engine",
		Long: `List the configured search engines, or show a single engine by id.

Examples:
  ` + getBinaryName() + ` engines
  ` + getBinaryName() + ` engines duckduckgo --output yaml`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				if len(args) > 0 && args[0] != "" {
					e, err := c.GetEngineByID(args[0])
					if err != nil {
						return err
					}
					rows := [][]string{
						{"id", e.ID},
						{"name", e.Name},
						{"enabled", boolCell(e.Enabled)},
						{"priority", strconv.Itoa(e.Priority)},
						{"categories", strings.Join(e.Categories, ",")},
						{"description", e.Description},
						{"homepage", e.Homepage},
					}
					plain := make([]string, 0, len(rows))
					for _, r := range rows {
						plain = append(plain, r[0]+"="+r[1])
					}
					return render(getOutputFormat(), view{
						Data:    e,
						Headers: []string{"FIELD", "VALUE"},
						Rows:    rows,
						Plain:   plain,
					})
				}

				engines, err := c.GetEngines()
				if err != nil {
					return err
				}
				rows := make([][]string, 0, len(engines))
				plain := make([]string, 0, len(engines))
				for _, e := range engines {
					rows = append(rows, []string{
						e.ID, e.Name, boolCell(e.Enabled),
						strconv.Itoa(e.Priority), strings.Join(e.Categories, ","),
					})
					plain = append(plain, e.ID)
				}
				return render(getOutputFormat(), view{
					Data:    engines,
					Headers: []string{"ID", "NAME", "ENABLED", "PRIORITY", "CATEGORIES"},
					Rows:    rows,
					Plain:   plain,
					Footer:  fmt.Sprintf("Total: %d engines", len(engines)),
				})
			})
		},
	}
}

func newCategoriesCommand() *command {
	return &command{
		Use:   "categories",
		Short: "List search categories",
		Long:  `List the search categories supported by the server.`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				cats, err := c.GetCategories()
				if err != nil {
					return err
				}
				rows := make([][]string, 0, len(cats))
				plain := make([]string, 0, len(cats))
				for _, cat := range cats {
					rows = append(rows, []string{cat.ID, cat.Name, cat.Description})
					plain = append(plain, cat.ID)
				}
				return render(getOutputFormat(), view{
					Data:    cats,
					Headers: []string{"ID", "NAME", "DESCRIPTION"},
					Rows:    rows,
					Plain:   plain,
					Footer:  fmt.Sprintf("Total: %d categories", len(cats)),
				})
			})
		},
	}
}

func newBangsCommand() *command {
	return &command{
		Use:   "bangs [filter]",
		Short: "List bang shortcuts",
		Long: `List the bang shortcuts, optionally filtered by a substring match on
the shortcut, name or category.`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				bangs, err := c.GetBangs()
				if err != nil {
					return err
				}
				filter := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
				matched := make([]api.Bang, 0, len(bangs))
				for _, b := range bangs {
					if filter != "" &&
						!strings.Contains(strings.ToLower(b.Shortcut), filter) &&
						!strings.Contains(strings.ToLower(b.Name), filter) &&
						!strings.Contains(strings.ToLower(b.Category), filter) {
						continue
					}
					matched = append(matched, b)
				}
				rows := make([][]string, 0, len(matched))
				plain := make([]string, 0, len(matched))
				for _, b := range matched {
					rows = append(rows, []string{b.Shortcut, b.Name, b.Category, b.URL})
					plain = append(plain, b.Shortcut)
				}
				return render(getOutputFormat(), view{
					Data:    matched,
					Headers: []string{"SHORTCUT", "NAME", "CATEGORY", "URL"},
					Rows:    rows,
					Plain:   plain,
					Footer:  fmt.Sprintf("Total: %d bangs", len(matched)),
				})
			})
		},
	}
}

func newWidgetsCommand() *command {
	return &command{
		Use:   "widgets",
		Short: "List available widgets",
		Long:  `List the widgets the server can render.`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				widgets, err := c.GetWidgets()
				if err != nil {
					return err
				}
				rows := make([][]string, 0, len(widgets))
				plain := make([]string, 0, len(widgets))
				for _, w := range widgets {
					rows = append(rows, []string{w.Type, w.Name, w.Category, strconv.Itoa(w.Order)})
					plain = append(plain, w.Type)
				}
				return render(getOutputFormat(), view{
					Data:    widgets,
					Headers: []string{"TYPE", "NAME", "CATEGORY", "ORDER"},
					Rows:    rows,
					Plain:   plain,
					Footer:  fmt.Sprintf("Total: %d widgets", len(widgets)),
				})
			})
		},
	}
}

func newWidgetCommand() *command {
	return &command{
		Use:   "widget [name] [key=value ...]",
		Short: "Fetch data for one widget",
		Long: `Fetch the rendered data for a single widget. Extra arguments in
key=value form are passed to the server as query parameters.

Examples:
  ` + getBinaryName() + ` widget weather location=Boston`,
		run: func(args []string) error {
			if len(args) == 0 || args[0] == "" {
				return fmt.Errorf("%w: a widget name is required", ErrUsage)
			}
			name := args[0]
			params := url.Values{}
			for _, arg := range args[1:] {
				key, value, found := strings.Cut(arg, "=")
				if !found {
					return fmt.Errorf("%w: widget parameter %q must be key=value", ErrUsage, arg)
				}
				params.Set(key, value)
			}
			return withClient(func(c *api.Client) error {
				data, err := c.GetWidgetData(name, params)
				if err != nil {
					return err
				}
				rows := [][]string{
					{"type", data.Type},
					{"updated_at", data.UpdatedAt},
					{"data", string(data.Data)},
				}
				if data.Error != "" {
					rows = append(rows, []string{"error", data.Error})
				}
				plain := make([]string, 0, len(rows))
				for _, r := range rows {
					plain = append(plain, r[0]+"="+r[1])
				}
				return render(getOutputFormat(), view{
					Data:    data,
					Headers: []string{"FIELD", "VALUE"},
					Rows:    rows,
					Plain:   plain,
				})
			})
		},
	}
}

func newInstantCommand() *command {
	return &command{
		Use:   "instant [query]",
		Short: "Fetch an instant answer",
		Long:  `Fetch the instant answer the server computes for a query.`,
		run: func(args []string) error {
			query, err := requireArg(args, "a query")
			if err != nil {
				return err
			}
			return withClient(func(c *api.Client) error {
				ans, err := c.GetInstantAnswer(query)
				if err != nil {
					return err
				}
				rows := [][]string{
					{"query", ans.Query},
					{"found", boolCell(ans.Found)},
					{"type", ans.Type},
					{"title", ans.Title},
					{"content", ans.Content},
					{"source", ans.Source},
				}
				return render(getOutputFormat(), view{
					Data:    ans,
					Headers: []string{"FIELD", "VALUE"},
					Rows:    rows,
					Plain:   []string{ans.Title, ans.Content},
				})
			})
		},
	}
}

func newAnswerCommand() *command {
	return &command{
		Use:   "answer [slug]",
		Short: "Fetch a direct answer by slug",
		Long:  `Fetch the direct answer page the server stores for a slug.`,
		run: func(args []string) error {
			slug, err := requireArg(args, "a slug")
			if err != nil {
				return err
			}
			return withClient(func(c *api.Client) error {
				ans, err := c.GetDirectAnswer(slug)
				if err != nil {
					return err
				}
				rows := [][]string{
					{"type", ans.Type},
					{"term", ans.Term},
					{"found", boolCell(ans.Found)},
					{"title", ans.Title},
					{"description", ans.Description},
					{"content", ans.Content},
					{"source", ans.Source},
					{"source_url", ans.SourceURL},
				}
				return render(getOutputFormat(), view{
					Data:    ans,
					Headers: []string{"FIELD", "VALUE"},
					Rows:    rows,
					Plain:   []string{ans.Title, ans.Content},
				})
			})
		},
	}
}

func newRelatedCommand() *command {
	return &command{
		Use:   "related [query]",
		Short: "List related searches for a query",
		Long:  `List the related searches the server suggests for a query.`,
		run: func(args []string) error {
			query, err := requireArg(args, "a query")
			if err != nil {
				return err
			}
			return withClient(func(c *api.Client) error {
				items, err := c.GetRelatedSearches(query)
				if err != nil {
					return err
				}
				return render(getOutputFormat(), stringListView("RELATED", items))
			})
		},
	}
}

func newAutocompleteCommand() *command {
	return &command{
		Use:   "autocomplete [prefix]",
		Short: "List autocomplete suggestions",
		Long:  `List the autocomplete suggestions the server returns for a prefix.`,
		run: func(args []string) error {
			prefix, err := requireArg(args, "a prefix")
			if err != nil {
				return err
			}
			return withClient(func(c *api.Client) error {
				items, err := c.GetAutocomplete(prefix)
				if err != nil {
					return err
				}
				return render(getOutputFormat(), stringListView("SUGGESTION", items))
			})
		},
	}
}

func newPreferencesCommand() *command {
	return &command{
		Use:   "preferences [set key=value ...]",
		Short: "Show or update server-side preferences",
		Long: `Show the server-side search preferences, or update them.

Examples:
  ` + getBinaryName() + ` preferences
  ` + getBinaryName() + ` preferences set storage=cookie fields=safe_search,language`,
		run: func(args []string) error {
			if len(args) > 0 && args[0] == "set" {
				return runPreferencesSet(args[1:])
			}
			if len(args) > 0 {
				return fmt.Errorf("%w: unknown preferences action %q", ErrUsage, args[0])
			}
			return withClient(func(c *api.Client) error {
				prefs, err := c.GetPreferences()
				if err != nil {
					return err
				}
				rows := [][]string{
					{"storage", prefs.Storage},
					{"fields", strings.Join(prefs.Fields, ",")},
				}
				plain := make([]string, 0, len(rows))
				for _, r := range rows {
					plain = append(plain, r[0]+"="+r[1])
				}
				return render(getOutputFormat(), view{
					Data:    prefs,
					Headers: []string{"FIELD", "VALUE"},
					Rows:    rows,
					Plain:   plain,
				})
			})
		},
	}
}

// runPreferencesSet applies key=value pairs to the server-side preferences.
func runPreferencesSet(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: at least one key=value pair is required", ErrUsage)
	}
	prefs := &api.Preferences{}
	for _, arg := range args {
		key, value, found := strings.Cut(arg, "=")
		if !found {
			return fmt.Errorf("%w: preference %q must be key=value", ErrUsage, arg)
		}
		switch key {
		case "storage":
			prefs.Storage = value
		case "fields":
			prefs.Fields = strings.Split(value, ",")
		default:
			return fmt.Errorf("%w: unknown preference key %q", ErrUsage, key)
		}
	}
	return withClient(func(c *api.Client) error {
		if err := c.SetPreferences(prefs); err != nil {
			return err
		}
		fmt.Println("Preferences updated")
		return nil
	})
}

func newAlertsCommand() *command {
	return &command{
		Use:   "alerts [create|get|update|delete]",
		Short: "Manage saved search alerts",
		Long: `Create, inspect, update and delete saved search alerts.

Examples:
  ` + getBinaryName() + ` alerts create query="golang release" email=me@example.com frequency=daily
  ` + getBinaryName() + ` alerts get {manage_token}
  ` + getBinaryName() + ` alerts update {manage_token} frequency=weekly
  ` + getBinaryName() + ` alerts delete {manage_token}`,
		run: func(args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("%w: alerts requires an action (create, get, update, delete)", ErrUsage)
			}
			switch args[0] {
			case "create":
				return runAlertCreate(args[1:])
			case "get":
				return runAlertGet(args[1:])
			case "update":
				return runAlertUpdate(args[1:])
			case "delete":
				return runAlertDelete(args[1:])
			default:
				return fmt.Errorf("%w: unknown alerts action %q", ErrUsage, args[0])
			}
		},
	}
}

// alertFields collects key=value pairs shared by alert create and update.
type alertFields struct {
	Query          string
	Category       string
	Language       string
	Region         string
	Engines        []string
	SafeSearch     int
	Frequency      string
	Email          string
	DeliverEmail   bool
	DeliverRSS     bool
	DeliverWebhook bool
	WebhookURL     string
}

// parseAlertFields converts key=value arguments into alert fields.
// Boolean values go through config.ParseBool per AI.md PART 32.
func parseAlertFields(args []string) (*alertFields, error) {
	f := &alertFields{}
	for _, arg := range args {
		key, value, found := strings.Cut(arg, "=")
		if !found {
			return nil, fmt.Errorf("%w: alert field %q must be key=value", ErrUsage, arg)
		}
		switch key {
		case "query":
			f.Query = value
		case "category":
			f.Category = value
		case "language":
			f.Language = value
		case "region":
			f.Region = value
		case "engines":
			f.Engines = strings.Split(value, ",")
		case "frequency":
			f.Frequency = value
		case "email":
			f.Email = value
		case "webhook_url":
			f.WebhookURL = value
		case "safe_search":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("%w: safe_search must be a number", ErrUsage)
			}
			f.SafeSearch = n
		case "deliver_email", "deliver_rss", "deliver_webhook":
			b, err := config.ParseBool(value, false)
			if err != nil {
				return nil, fmt.Errorf("%w: %s must be a boolean", ErrUsage, key)
			}
			switch key {
			case "deliver_email":
				f.DeliverEmail = b
			case "deliver_rss":
				f.DeliverRSS = b
			case "deliver_webhook":
				f.DeliverWebhook = b
			}
		default:
			return nil, fmt.Errorf("%w: unknown alert field %q", ErrUsage, key)
		}
	}
	return f, nil
}

// renderAlert prints an alert in the active output format.
func renderAlert(a *api.Alert) error {
	rows := [][]string{
		{"manage_token", a.ManageToken},
		{"manage_url", a.ManageURL},
		{"rss_token", a.RSSToken},
		{"rss_url", a.RSSURL},
	}
	plain := make([]string, 0, len(rows))
	for _, r := range rows {
		plain = append(plain, r[0]+"="+r[1])
	}
	return render(getOutputFormat(), view{
		Data:    a,
		Headers: []string{"FIELD", "VALUE"},
		Rows:    rows,
		Plain:   plain,
	})
}

func runAlertCreate(args []string) error {
	f, err := parseAlertFields(args)
	if err != nil {
		return err
	}
	if f.Query == "" {
		return fmt.Errorf("%w: query= is required to create an alert", ErrUsage)
	}
	if f.Frequency == "" {
		f.Frequency = "daily"
	}
	if f.Email != "" && !f.DeliverEmail {
		f.DeliverEmail = true
	}
	return withClient(func(c *api.Client) error {
		alert, err := c.CreateAlert(&api.CreateAlertRequest{
			Query:          f.Query,
			Category:       f.Category,
			Language:       f.Language,
			Region:         f.Region,
			Engines:        f.Engines,
			SafeSearch:     f.SafeSearch,
			Frequency:      f.Frequency,
			Email:          f.Email,
			DeliverEmail:   f.DeliverEmail,
			DeliverRSS:     f.DeliverRSS,
			DeliverWebhook: f.DeliverWebhook,
			WebhookURL:     f.WebhookURL,
		})
		if err != nil {
			return err
		}
		return renderAlert(alert)
	})
}

func runAlertGet(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("%w: a manage token is required", ErrUsage)
	}
	return withClient(func(c *api.Client) error {
		alert, err := c.GetAlert(args[0])
		if err != nil {
			return err
		}
		return renderAlert(alert)
	})
}

func runAlertUpdate(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("%w: a manage token is required", ErrUsage)
	}
	f, err := parseAlertFields(args[1:])
	if err != nil {
		return err
	}
	return withClient(func(c *api.Client) error {
		alert, err := c.UpdateAlert(args[0], &api.UpdateAlertRequest{
			Query:          f.Query,
			Category:       f.Category,
			Language:       f.Language,
			Region:         f.Region,
			Engines:        f.Engines,
			Frequency:      f.Frequency,
			SafeSearch:     f.SafeSearch,
			DeliverEmail:   f.DeliverEmail,
			DeliverRSS:     f.DeliverRSS,
			DeliverWebhook: f.DeliverWebhook,
			WebhookURL:     f.WebhookURL,
		})
		if err != nil {
			return err
		}
		return renderAlert(alert)
	})
}

func runAlertDelete(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("%w: a manage token is required", ErrUsage)
	}
	return withClient(func(c *api.Client) error {
		if err := c.DeleteAlert(args[0]); err != nil {
			return err
		}
		fmt.Println("Alert deleted")
		return nil
	})
}

func newMetricsCommand() *command {
	return &command{
		Use:   "metrics",
		Short: "Print the server Prometheus metrics",
		Long: `Print the raw Prometheus exposition text served by the server.
Requires a token with metrics access.`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				body, err := c.GetMetrics()
				if err != nil {
					return err
				}
				_, werr := os.Stdout.WriteString(body)
				return werr
			})
		},
	}
}

func newServerConfigCommand() *command {
	return &command{
		Use:   "server-config",
		Short: "Show the server's effective configuration",
		Long: `Show the effective server configuration exposed by the API.
This reads the server settings; the client's own settings live in cli.yml.`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				cfg, err := c.GetServerConfig()
				if err != nil {
					return err
				}
				keys := make([]string, 0, len(cfg.Config))
				for k := range cfg.Config {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				rows := make([][]string, 0, len(keys))
				plain := make([]string, 0, len(keys))
				for _, k := range keys {
					value := fmt.Sprintf("%v", cfg.Config[k])
					rows = append(rows, []string{k, value})
					plain = append(plain, k+"="+value)
				}
				return render(getOutputFormat(), view{
					Data:    cfg,
					Headers: []string{"KEY", "VALUE"},
					Rows:    rows,
					Plain:   plain,
				})
			})
		},
	}
}

func newAutodiscoverCommand() *command {
	return &command{
		Use:   "autodiscover",
		Short: "Show the server autodiscover document",
		Long: `Show the /api/autodiscover document, including the API base path and
the CLI binary versions published for each platform.`,
		run: func(args []string) error {
			return withClient(func(c *api.Client) error {
				info, err := c.Autodiscover()
				if err != nil {
					return err
				}
				rows := [][]string{
					{"server.name", info.Server.Name},
					{"server.version", info.Server.Version},
					{"server.url", info.Server.URL},
					{"server.features.auth", boolCell(info.Server.Features.Auth)},
					{"server.features.search", boolCell(info.Server.Features.Search)},
					{"server.features.register", boolCell(info.Server.Features.Register)},
					{"api.version", info.API.Version},
					{"api.base_path", info.API.BasePath},
					{"cli_min_version", info.CLIMinVersion},
				}
				platforms := make([]string, 0, len(info.CLIVersions))
				for k := range info.CLIVersions {
					platforms = append(platforms, k)
				}
				sort.Strings(platforms)
				for _, p := range platforms {
					rows = append(rows, []string{"cli_versions." + p, info.CLIVersions[p].Version})
				}
				plain := make([]string, 0, len(rows))
				for _, r := range rows {
					plain = append(plain, r[0]+"="+r[1])
				}
				return render(getOutputFormat(), view{
					Data:    info,
					Headers: []string{"FIELD", "VALUE"},
					Rows:    rows,
					Plain:   plain,
				})
			})
		},
	}
}

func newSchedulerCommand() *command {
	return &command{
		Use:   "scheduler <list|show|run|enable|disable|history> [task]",
		Short: "Inspect and control scheduled tasks",
		Long:  "List scheduler tasks, inspect history, and trigger or toggle skippable tasks. Requires an operator token.",
		run: func(args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("%w: scheduler action is required", ErrUsage)
			}
			action := args[0]
			if action != "list" && len(args) < 2 {
				return fmt.Errorf("%w: scheduler task id is required", ErrUsage)
			}
			taskID := ""
			if len(args) > 1 {
				taskID = args[1]
			}
			return withClient(func(c *api.Client) error {
				switch action {
				case "list":
					tasks, err := c.GetSchedulerTasks()
					if err != nil {
						return err
					}
					rows := make([][]string, 0, len(tasks))
					for _, task := range tasks {
						rows = append(rows, []string{task.ID, task.Schedule, task.LastStatus, task.NextRun.Format(time.RFC3339), boolCell(task.Enabled)})
					}
					return render(getOutputFormat(), view{Data: tasks, Headers: []string{"TASK", "SCHEDULE", "STATUS", "NEXT RUN", "ENABLED"}, Rows: rows})
				case "show":
					task, err := c.GetSchedulerTask(taskID)
					if err != nil {
						return err
					}
					rows := [][]string{{"id", task.ID}, {"name", task.Name}, {"schedule", task.Schedule}, {"last_status", task.LastStatus}, {"last_run", task.LastRun.Format(time.RFC3339)}, {"next_run", task.NextRun.Format(time.RFC3339)}, {"enabled", boolCell(task.Enabled)}, {"run_count", strconv.FormatInt(task.RunCount, 10)}, {"fail_count", strconv.FormatInt(task.FailCount, 10)}}
					return render(getOutputFormat(), view{Data: task, Headers: []string{"FIELD", "VALUE"}, Rows: rows})
				case "history":
					history, err := c.GetSchedulerHistory(taskID)
					if err != nil {
						return err
					}
					rows := make([][]string, 0, len(history))
					for _, item := range history {
						rows = append(rows, []string{item.StartedAt.Format(time.RFC3339), item.Status, item.Duration.String(), item.Error})
					}
					return render(getOutputFormat(), view{Data: history, Headers: []string{"STARTED", "STATUS", "DURATION", "ERROR"}, Rows: rows})
				case "run":
					if err := c.RunSchedulerTask(taskID); err != nil {
						return err
					}
					return render(getOutputFormat(), view{Data: map[string]interface{}{"task": taskID, "triggered": true}, Rows: [][]string{{"task", taskID}, {"triggered", "true"}}, Headers: []string{"FIELD", "VALUE"}})
				case "enable", "disable":
					if err := c.SetSchedulerTaskEnabled(taskID, action == "enable"); err != nil {
						return err
					}
					return render(getOutputFormat(), view{Data: map[string]interface{}{"task": taskID, "enabled": action == "enable"}, Rows: [][]string{{"task", taskID}, {"enabled", action}}, Headers: []string{"FIELD", "VALUE"}})
				default:
					return fmt.Errorf("%w: unknown scheduler action %q", ErrUsage, action)
				}
			})
		},
	}
}
