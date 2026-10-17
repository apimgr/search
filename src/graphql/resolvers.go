package graphql

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/widget"
	"github.com/graphql-go/graphql"
)

var processStartedAt = time.Now()

func dependenciesFromContext(ctx context.Context) Dependencies {
	if ctx == nil {
		return Dependencies{}
	}
	dependencies, _ := ctx.Value(dependenciesContextKey{}).(Dependencies)
	return dependencies
}

// resolveSearch executes a search through the same aggregator used by REST.
func resolveSearch(p graphql.ResolveParams) (interface{}, error) {
	dependencies := dependenciesFromContext(p.Context)
	if dependencies.Aggregator == nil {
		return nil, errors.New("search service is unavailable")
	}
	queryText, _ := p.Args["q"].(string)
	query := model.NewQuery(queryText)
	if category, ok := p.Args["category"].(string); ok && category != "" {
		query.Category = model.ParseCategory(category)
	}
	if page, ok := p.Args["page"].(int); ok {
		if page < 1 || page > 1000 {
			return nil, errors.New("page must be between 1 and 1000")
		}
		query.Page = page
	}
	if language, ok := p.Args["lang"].(string); ok && language != "" {
		query.Language = language
	}
	if limit, ok := p.Args["limit"].(int); ok {
		if limit < 1 || limit > 100 {
			return nil, errors.New("limit must be between 1 and 100")
		}
		query.PerPage = limit
	}
	if safeSearch, ok := p.Args["safeSearch"].(int); ok {
		if safeSearch < 0 || safeSearch > 2 {
			return nil, errors.New("safeSearch must be 0, 1, or 2")
		}
		query.SafeSearch = safeSearch
	}
	if timeRange, ok := p.Args["timeRange"].(string); ok {
		query.TimeRange = timeRange
	}
	if timeFrom, ok := p.Args["timeFrom"].(string); ok {
		if _, err := time.Parse("2006-01-02", timeFrom); err != nil {
			return nil, errors.New("timeFrom must use YYYY-MM-DD format")
		}
		query.DateAfter = timeFrom
	}
	if timeTo, ok := p.Args["timeTo"].(string); ok {
		if _, err := time.Parse("2006-01-02", timeTo); err != nil {
			return nil, errors.New("timeTo must use YYYY-MM-DD format")
		}
		query.DateBefore = timeTo
	}
	if query.TimeRange == "custom" && (query.DateAfter == "" || query.DateBefore == "") {
		return nil, errors.New("custom timeRange requires timeFrom and timeTo")
	}
	if query.DateAfter != "" && query.DateBefore != "" {
		from, _ := time.Parse("2006-01-02", query.DateAfter)
		to, _ := time.Parse("2006-01-02", query.DateBefore)
		if from.After(to) {
			return nil, errors.New("timeFrom must not be after timeTo")
		}
	}
	if region, ok := p.Args["region"].(string); ok {
		query.Region = region
	}
	switch engines := p.Args["engines"].(type) {
	case []interface{}:
		for _, engineName := range engines {
			if name, valid := engineName.(string); valid {
				query.Engines = append(query.Engines, name)
			}
		}
	case []string:
		query.Engines = append(query.Engines, engines...)
	}
	results, err := dependencies.Aggregator.Search(p.Context, query)
	if err != nil {
		return nil, err
	}
	pageResults := results.GetPage(query.Page)
	items := make([]map[string]interface{}, 0, len(pageResults))
	for _, result := range pageResults {
		thumbnail := result.Thumbnail
		if dependencies.ThumbnailRewriter != nil {
			thumbnail = dependencies.ThumbnailRewriter(thumbnail)
		}
		imageURL := result.ImageURL
		if imageURL == "" && result.Category == model.CategoryImages && result.Metadata != nil {
			imageURL, _ = result.Metadata["full_image"].(string)
		}
		if dependencies.ThumbnailRewriter != nil {
			imageURL = dependencies.ThumbnailRewriter(imageURL)
		}
		published := ""
		if !result.PublishedAt.IsZero() {
			published = result.PublishedAt.UTC().Format(time.RFC3339)
		}
		items = append(items, map[string]interface{}{
			"title": result.Title, "url": result.URL, "content": result.Content,
			"engine": result.Engine, "score": result.Score,
			"imageUrl": imageURL, "thumbnail": thumbnail,
			"imageWidth": result.ImageWidth, "imageHeight": result.ImageHeight,
			"imageFormat": result.ImageFormat, "fileSize": result.FileSize,
			"duration": result.Duration, "viewCount": float64(result.ViewCount), "embedUrl": result.EmbedURL,
			"published": published, "author": result.Author, "domain": result.Domain,
			"category": result.Category.String(),
		})
	}
	return map[string]interface{}{
		"query": results.Query, "category": results.Category.String(), "results": items,
		"totalResults": results.TotalResults, "page": results.Page, "perPage": results.PerPage,
		"totalPages": results.TotalPages, "searchTime": results.SearchTime,
		"engines": results.Engines, "suggestions": results.Suggestions,
	}, nil
}

func resolveAutocomplete(p graphql.ResolveParams) (interface{}, error) {
	dependencies := dependenciesFromContext(p.Context)
	if dependencies.Autocomplete == nil {
		return []string{}, nil
	}
	query, _ := p.Args["q"].(string)
	return dependencies.Autocomplete(p.Context, query), nil
}

func resolveRelatedSearches(p graphql.ResolveParams) (interface{}, error) {
	dependencies := dependenciesFromContext(p.Context)
	if dependencies.RelatedSearches == nil {
		return []string{}, nil
	}
	query, _ := p.Args["q"].(string)
	limit := 8
	if requested, ok := p.Args["limit"].(int); ok {
		if requested < 1 || requested > 20 {
			return nil, errors.New("limit must be between 1 and 20")
		}
		limit = requested
	}
	return dependencies.RelatedSearches(p.Context, query, limit)
}

func resolvePreferences(p graphql.ResolveParams) (interface{}, error) {
	resolver := dependenciesFromContext(p.Context).Preferences
	if resolver == nil {
		return nil, errors.New("preferences are unavailable")
	}
	return resolver(), nil
}

func resolveSavePreferences(p graphql.ResolveParams) (interface{}, error) {
	resolver := dependenciesFromContext(p.Context).SavePreferences
	if resolver == nil {
		return nil, errors.New("preferences are unavailable")
	}
	return resolver(), nil
}

func resolveOperatorStatus(p graphql.ResolveParams) (interface{}, error) {
	resolver := dependenciesFromContext(p.Context).OperatorStatus
	request := requestFromContext(p.Context)
	if resolver == nil || request == nil {
		return nil, errors.New("operator authorization is unavailable")
	}
	return resolver(request)
}

func resolveOperatorConfig(p graphql.ResolveParams) (interface{}, error) {
	resolver := dependenciesFromContext(p.Context).OperatorConfig
	request := requestFromContext(p.Context)
	if resolver == nil || request == nil {
		return nil, errors.New("operator authorization is unavailable")
	}
	return resolver(request)
}

func resolveGeoIPLookup(p graphql.ResolveParams) (interface{}, error) {
	lookup := dependenciesFromContext(p.Context).GeoIPLookup
	if lookup == nil {
		return nil, errors.New("GeoIP lookup is unavailable")
	}
	ip, _ := p.Args["ip"].(string)
	if ip == "" {
		request := requestFromContext(p.Context)
		if request == nil {
			return nil, errors.New("request is unavailable for caller IP lookup")
		}
		ip = httputil.GetClientIP(request)
	}
	return lookup(ip)
}

func resolveServerInfo(p graphql.ResolveParams) (interface{}, error) {
	info := dependenciesFromContext(p.Context).ServerInfo
	if info == nil {
		return nil, errors.New("server info is unavailable")
	}
	return info(), nil
}

func resolveHealth(p graphql.ResolveParams) (interface{}, error) {
	dependencies := dependenciesFromContext(p.Context)
	if dependencies.API != nil {
		health := dependencies.API.HealthForGraphQL()
		return map[string]interface{}{
			"project": map[string]interface{}{"name": health.Project.Name, "tagline": health.Project.Tagline, "description": health.Project.Description},
			"status":  health.Status, "pendingRestart": health.PendingRestart, "restartReason": health.RestartReason,
			"version": health.Version, "goVersion": health.GoVersion,
			"build":  map[string]interface{}{"commit": health.Build.Commit, "date": health.Build.Date},
			"uptime": health.Uptime, "mode": health.Mode, "timestamp": health.Timestamp,
			"features": map[string]interface{}{
				"tor":   map[string]interface{}{"enabled": health.Features.Tor.Enabled, "running": health.Features.Tor.Running, "status": health.Features.Tor.Status, "hostname": health.Features.Tor.Hostname},
				"i2p":   map[string]interface{}{"enabled": health.Features.I2P.Enabled, "running": health.Features.I2P.Running, "status": health.Features.I2P.Status, "hostname": health.Features.I2P.Hostname, "provider": health.Features.I2P.Provider},
				"geoip": health.Features.GeoIP,
			},
			"checks": map[string]interface{}{
				"database": health.Checks.Database, "cache": health.Checks.Cache, "disk": health.Checks.Disk,
				"scheduler": health.Checks.Scheduler, "tor": health.Checks.Tor, "i2p": health.Checks.I2P,
			},
			"stats": map[string]interface{}{"requestsTotal": health.Stats.RequestsTotal, "requests24h": health.Stats.Requests24h, "activeConns": health.Stats.ActiveConns},
		}, nil
	}
	startedAt := dependencies.StartedAt
	if startedAt.IsZero() {
		startedAt = processStartedAt
	}
	status := "healthy"
	if dependencies.Aggregator == nil || len(dependencies.Aggregator.EngineNames()) == 0 {
		status = "degraded"
	}
	return map[string]interface{}{
		"status": status, "version": config.Version,
		"uptime": time.Since(startedAt).Round(time.Second).String(), "mode": "production",
	}, nil
}

func resolveDirectAnswer(p graphql.ResolveParams) (interface{}, error) {
	dependencies := dependenciesFromContext(p.Context)
	if dependencies.Direct == nil {
		return nil, errors.New("direct answer service is unavailable")
	}
	answerType, _ := p.Args["type"].(string)
	term, _ := p.Args["term"].(string)
	answer, err := dependencies.Direct.ProcessType(p.Context, direct.AnswerType(answerType), term)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok": answer.Error == "",
		"data": map[string]interface{}{
			"type": string(answer.Type), "term": answer.Term, "title": answer.Title,
			"description": answer.Description, "content": answer.Content,
			"source": answer.Source, "sourceUrl": answer.SourceURL,
			"cacheTtlSeconds": int(answer.CacheTTL.Seconds()), "found": answer.Error == "",
			"details": answer.Data,
		},
	}, nil
}

func resolveInstant(p graphql.ResolveParams) (interface{}, error) {
	dependencies := dependenciesFromContext(p.Context)
	if dependencies.Instant == nil {
		return nil, errors.New("instant answer service is unavailable")
	}
	query, _ := p.Args["q"].(string)
	answer, err := dependencies.Instant.Process(p.Context, query)
	if err != nil {
		return nil, err
	}
	if answer == nil {
		return map[string]interface{}{"ok": true, "data": map[string]interface{}{
			"query": query, "type": "", "title": "", "content": "", "source": "", "found": false,
		}}, nil
	}
	return map[string]interface{}{"ok": true, "data": map[string]interface{}{
		"query": answer.Query, "type": string(answer.Type), "title": answer.Title,
		"content": answer.Content, "source": answer.Source, "found": true,
		"details": answer.Data,
	}}, nil
}

func resolveWidgets(p graphql.ResolveParams) (interface{}, error) {
	manager := dependenciesFromContext(p.Context).Widgets
	if manager == nil {
		return map[string]interface{}{"enabled": false, "widgets": []interface{}{}, "defaults": []string{}}, nil
	}
	widgets := manager.GetAllWidgets()
	category, _ := p.Args["category"].(string)
	if category != "" {
		filtered := make([]*widget.Widget, 0, len(widgets))
		for _, item := range widgets {
			if string(item.Category) == category {
				filtered = append(filtered, item)
			}
		}
		widgets = filtered
	}
	return map[string]interface{}{"enabled": manager.IsEnabled(), "widgets": widgets, "defaults": manager.GetDefaultWidgets()}, nil
}

func resolveWidgetData(p graphql.ResolveParams) (interface{}, error) {
	manager := dependenciesFromContext(p.Context).Widgets
	if manager == nil {
		return map[string]interface{}{"type": p.Args["type"], "error": "Widget data not available - configure in widget settings"}, nil
	}
	widgetType := widget.WidgetType(strings.TrimSpace(p.Args["type"].(string)))
	if !manager.IsWidgetEnabled(widgetType) {
		return nil, errors.New("widget not available")
	}
	params := make(map[string]string)
	if raw, ok := p.Args["params"].(map[string]interface{}); ok {
		for key, value := range raw {
			text, valid := value.(string)
			if !valid {
				return nil, errors.New("widget parameters must be strings")
			}
			params[key] = text
		}
	}
	ctx, cancel := context.WithTimeout(p.Context, 10*time.Second)
	defer cancel()
	return manager.FetchWidgetData(ctx, widgetType, params)
}

func resolveCategories(graphql.ResolveParams) (interface{}, error) {
	return api.AvailableCategories(), nil
}

func resolveBangs(params graphql.ResolveParams) (interface{}, error) {
	category, _ := params.Args["category"].(string)
	searchTerm, _ := params.Args["search"].(string)
	searchTerm = strings.ToLower(strings.TrimSpace(searchTerm))
	bangs := api.AvailableBangs()
	filtered := make([]api.BangInfo, 0, len(bangs))
	for _, bang := range bangs {
		if category != "" && bang.Category != category {
			continue
		}
		if searchTerm != "" && !strings.Contains(strings.ToLower(bang.Shortcut), searchTerm) && !strings.Contains(strings.ToLower(bang.Name), searchTerm) {
			continue
		}
		filtered = append(filtered, bang)
	}
	return filtered, nil
}

func resolveEngines(params graphql.ResolveParams) (interface{}, error) {
	dependencies := dependenciesFromContext(params.Context)
	if dependencies.EngineInventory == nil {
		return []api.EngineInfo{}, nil
	}
	return dependencies.EngineInventory(), nil
}

func resolveEngine(params graphql.ResolveParams) (interface{}, error) {
	dependencies := dependenciesFromContext(params.Context)
	if dependencies.EngineInventory == nil {
		return nil, nil
	}
	id, _ := params.Args["id"].(string)
	for _, engineInfo := range dependencies.EngineInventory() {
		if engineInfo.ID == id {
			return engineInfo, nil
		}
	}
	return nil, nil
}

func alertInputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}

func alertInputBool(input map[string]interface{}, key string) bool {
	value, _ := input[key].(bool)
	return value
}

func alertInputInt(input map[string]interface{}, key string) int {
	switch value := input[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	}
	return 0
}

func alertInputStrings(input map[string]interface{}, key string) []string {
	values := make([]string, 0)
	switch list := input[key].(type) {
	case []interface{}:
		for _, value := range list {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
	case []string:
		values = append(values, list...)
	}
	return values
}

func graphQLAlertError(err error) error {
	switch {
	case errors.Is(err, alert.ErrNotFound), errors.Is(err, alert.ErrInvalidToken):
		return errors.New("alert not found")
	case errors.Is(err, alert.ErrInvalidInput), errors.Is(err, alert.ErrEmailRequired):
		return errors.New(err.Error())
	case strings.Contains(err.Error(), "operator token") || strings.Contains(err.Error(), "operator authorization"):
		return errors.New("operator authorization required")
	default:
		return errors.New("alert operation failed")
	}
}

func resolveAlerts(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("alert service is unavailable")
	}
	items, err := deps.API.AlertListForGraphQL(p.Context, request)
	if err != nil {
		return nil, graphQLAlertError(err)
	}
	return items, nil
}

func resolveAlert(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("alert service is unavailable")
	}
	token, _ := p.Args["manageToken"].(string)
	item, err := deps.API.AlertGetForGraphQL(p.Context, request, token)
	if err != nil {
		return nil, graphQLAlertError(err)
	}
	return item, nil
}

func resolveAlertFeed(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	if deps.API == nil {
		return nil, errors.New("alert service is unavailable")
	}
	token, _ := p.Args["rssToken"].(string)
	feed, err := deps.API.AlertFeedForGraphQL(p.Context, token)
	if err != nil {
		return nil, graphQLAlertError(err)
	}
	return map[string]interface{}{"items": feed.Items, "feedUrl": feed.FeedURL}, nil
}

func resolveCreateAlert(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("alert service is unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	created, err := deps.API.AlertCreateForGraphQL(p.Context, request, alert.CreateRequest{Query: alertInputString(input, "query"), Category: alertInputString(input, "category"), Language: alertInputString(input, "language"), Region: alertInputString(input, "region"), Engines: alertInputStrings(input, "engines"), SafeSearch: alertInputInt(input, "safe_search"), Frequency: alert.Frequency(alertInputString(input, "frequency")), Email: alertInputString(input, "email"), DeliverEmail: alertInputBool(input, "deliver_email"), DeliverRSS: alertInputBool(input, "deliver_rss"), DeliverWebhook: alertInputBool(input, "deliver_webhook"), WebhookURL: alertInputString(input, "webhook_url")})
	if err != nil {
		return nil, graphQLAlertError(err)
	}
	return created, nil
}

func resolveUpdateAlert(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("alert service is unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	token, _ := p.Args["manageToken"].(string)
	updated, err := deps.API.AlertUpdateForGraphQL(p.Context, request, token, alert.UpdateRequest{Query: alertInputString(input, "query"), Category: alertInputString(input, "category"), Language: alertInputString(input, "language"), Region: alertInputString(input, "region"), Engines: alertInputStrings(input, "engines"), Frequency: alert.Frequency(alertInputString(input, "frequency")), SafeSearch: alertInputInt(input, "safe_search"), DeliverEmail: alertInputBool(input, "deliver_email"), DeliverRSS: alertInputBool(input, "deliver_rss"), DeliverWebhook: alertInputBool(input, "deliver_webhook"), WebhookURL: alertInputString(input, "webhook_url")})
	if err != nil {
		return nil, graphQLAlertError(err)
	}
	return updated, nil
}

func resolvePauseAlert(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	if deps.API == nil {
		return nil, errors.New("alert service is unavailable")
	}
	token, _ := p.Args["manageToken"].(string)
	paused, _ := p.Args["paused"].(bool)
	if err := deps.API.AlertPauseForGraphQL(p.Context, token, paused); err != nil {
		return nil, graphQLAlertError(err)
	}
	return map[string]bool{"paused": paused}, nil
}

func resolveDeleteAlert(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	if deps.API == nil {
		return nil, errors.New("alert service is unavailable")
	}
	token, _ := p.Args["manageToken"].(string)
	if err := deps.API.AlertDeleteForGraphQL(p.Context, token); err != nil {
		return nil, graphQLAlertError(err)
	}
	return map[string]bool{"deleted": true}, nil
}

func graphQLSchedulerError(err error) error {
	if strings.Contains(err.Error(), "operator token") || strings.Contains(err.Error(), "operator authorization") {
		return errors.New("operator authorization required")
	}
	return errors.New("scheduler operation failed")
}

func resolveSchedulerTasks(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("scheduler service is unavailable")
	}
	result, err := deps.API.SchedulerTasksForGraphQL(request)
	if err != nil {
		return nil, graphQLSchedulerError(err)
	}
	return result, nil
}

func resolveSchedulerTask(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("scheduler service is unavailable")
	}
	id, _ := p.Args["task"].(string)
	result, err := deps.API.SchedulerTaskForGraphQL(request, id)
	if err != nil {
		return nil, graphQLSchedulerError(err)
	}
	return result, nil
}

func resolveSchedulerHistory(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("scheduler service is unavailable")
	}
	id, _ := p.Args["task"].(string)
	result, err := deps.API.SchedulerHistoryForGraphQL(request, id)
	if err != nil {
		return nil, graphQLSchedulerError(err)
	}
	return result, nil
}

func resolveSetSchedulerTaskEnabled(p graphql.ResolveParams, enabled bool) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("scheduler service is unavailable")
	}
	id, _ := p.Args["task"].(string)
	result, err := deps.API.SetSchedulerTaskEnabledForGraphQL(request, id, enabled)
	if err != nil {
		return nil, graphQLSchedulerError(err)
	}
	return result, nil
}

func resolveEnableSchedulerTask(p graphql.ResolveParams) (interface{}, error) {
	return resolveSetSchedulerTaskEnabled(p, true)
}
func resolveDisableSchedulerTask(p graphql.ResolveParams) (interface{}, error) {
	return resolveSetSchedulerTaskEnabled(p, false)
}

func resolveRunSchedulerTask(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("scheduler service is unavailable")
	}
	id, _ := p.Args["task"].(string)
	result, err := deps.API.RunSchedulerTaskForGraphQL(request, id)
	if err != nil {
		return nil, graphQLSchedulerError(err)
	}
	return result, nil
}

func graphQLGeoIPPresetError(err error) error {
	if strings.Contains(err.Error(), "operator token") || strings.Contains(err.Error(), "operator authorization") {
		return errors.New("operator authorization required")
	}
	if err.Error() == "preset not found" {
		return errors.New("preset not found")
	}
	return errors.New("GeoIP preset operation failed")
}

func resolveGeoIPPresets(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("GeoIP preset service is unavailable")
	}
	result, err := deps.API.GeoIPPresetsForGraphQL(request)
	if err != nil {
		return nil, graphQLGeoIPPresetError(err)
	}
	return result, nil
}

func resolveGeoIPPreset(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("GeoIP preset service is unavailable")
	}
	name, _ := p.Args["name"].(string)
	result, err := deps.API.GeoIPPresetForGraphQL(request, name)
	if err != nil {
		return nil, graphQLGeoIPPresetError(err)
	}
	return result, nil
}

func resolveSaveGeoIPPreset(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("GeoIP preset service is unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	response, err := deps.API.SaveGeoIPPresetForGraphQL(request, api.GeoIPPresetSaveInput{Name: alertInputString(input, "name"), Codes: alertInputStrings(input, "codes"), From: alertInputString(input, "from")})
	if err != nil {
		return nil, graphQLGeoIPPresetError(err)
	}
	return response, nil
}

func resolveDeleteGeoIPPreset(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("GeoIP preset service is unavailable")
	}
	name, _ := p.Args["name"].(string)
	result, err := deps.API.DeleteGeoIPPresetForGraphQL(request, name)
	if err != nil {
		return nil, graphQLGeoIPPresetError(err)
	}
	return result, nil
}

func resolveSaveContactWebhook(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("contact webhook configuration service is unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	result, err := deps.API.ContactWebhookForGraphQL(p.Context, request, input)
	if err != nil {
		return nil, errors.New("contact webhook configuration failed; operator authorization is required")
	}
	if writer := responseWriterFromContext(p.Context); writer != nil {
		writer.Header().Set("Cache-Control", "no-store")
	}
	return result, nil
}

func resolveAPIRequest(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("REST API bridge is unavailable")
	}
	method, _ := p.Args["method"].(string)
	path, _ := p.Args["path"].(string)
	result, err := deps.API.APIRequestForGraphQL(p.Context, request, method, path, p.Args["input"])
	if err != nil {
		return nil, errors.New("REST API request is invalid or unavailable")
	}
	if body, ok := result["body"].(map[string]interface{}); ok {
		if data, ok := body["data"].(map[string]interface{}); ok {
			if _, hasSecret := data["secret"]; hasSecret {
				if writer := responseWriterFromContext(p.Context); writer != nil {
					writer.Header().Set("Cache-Control", "no-store")
				}
			}
		}
	}
	return result, nil
}

func resolveOwnerTokenOperation(p graphql.ResolveParams, operation string) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("owner token service is unavailable")
	}
	id, _ := p.Args["id"].(string)
	input, _ := p.Args["input"].(map[string]interface{})
	result, err := deps.API.OwnerTokenForGraphQL(p.Context, request, operation, id, input)
	if err != nil {
		return nil, errors.New("owner token operation failed; primary bearer authorization is required")
	}
	if operation == "revealPrimary" || operation == "rotatePrimary" || operation == "create" || operation == "reveal" || operation == "rotate" {
		if writer := responseWriterFromContext(p.Context); writer != nil {
			writer.Header().Set("Cache-Control", "no-store")
		}
	}
	return result, nil
}

func resolveOwnerTokens(p graphql.ResolveParams) (interface{}, error) {
	return resolveOwnerTokenOperation(p, "list")
}

func resolveRevealPrimaryToken(p graphql.ResolveParams) (interface{}, error) {
	return resolveOwnerTokenOperation(p, "revealPrimary")
}

func resolveRotatePrimaryToken(p graphql.ResolveParams) (interface{}, error) {
	return resolveOwnerTokenOperation(p, "rotatePrimary")
}

func resolveCreateOwnerToken(p graphql.ResolveParams) (interface{}, error) {
	return resolveOwnerTokenOperation(p, "create")
}

func resolveRevealOwnerToken(p graphql.ResolveParams) (interface{}, error) {
	return resolveOwnerTokenOperation(p, "reveal")
}

func resolveRotateOwnerToken(p graphql.ResolveParams) (interface{}, error) {
	return resolveOwnerTokenOperation(p, "rotate")
}

func resolveUpdateOwnerToken(p graphql.ResolveParams) (interface{}, error) {
	return resolveOwnerTokenOperation(p, "update")
}

func resolveRevokeOwnerToken(p graphql.ResolveParams) (interface{}, error) {
	return resolveOwnerTokenOperation(p, "revoke")
}

func resolveServerPage(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("server page service is unavailable")
	}
	name, _ := p.Args["name"].(string)
	page, err := deps.API.ServerPageForGraphQL(p.Context, request, name)
	if err != nil {
		return nil, errors.New("server page is unavailable")
	}
	return page, nil
}

func resolveContactInfo(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("contact service is unavailable")
	}
	contact, err := deps.API.ContactForGraphQL(p.Context, request, http.MethodGet, nil)
	if err != nil {
		return nil, errors.New("contact information is unavailable")
	}
	return contact, nil
}

func resolveSubmitContact(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("contact service is unavailable")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	result, err := deps.API.ContactForGraphQL(p.Context, request, http.MethodPost, input)
	if err != nil {
		return nil, errors.New("contact submission could not be accepted")
	}
	return result, nil
}

func resolveFavicon(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("favicon service is unavailable")
	}
	target, _ := p.Args["url"].(string)
	result, err := deps.API.FaviconForGraphQL(p.Context, request, target)
	if err != nil {
		return nil, errors.New("favicon is unavailable")
	}
	return result, nil
}

func resolveExportPreferences(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("preferences are unavailable")
	}
	result, err := deps.API.PreferencesExportForGraphQL(p.Context, request)
	if err != nil {
		return nil, errors.New("preferences are unavailable")
	}
	return result, nil
}

func resolveImportPreferences(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("preferences are unavailable")
	}
	values := make(url.Values)
	for _, key := range []string{"theme", "lang", "code"} {
		if value, ok := p.Args[key].(string); ok && value != "" {
			values.Set(key, value)
		}
	}
	result, cookies, err := deps.API.PreferencesImportForGraphQL(p.Context, request, values)
	if err != nil {
		return nil, errors.New("preferences could not be imported")
	}
	if writer := responseWriterFromContext(p.Context); writer != nil {
		for _, cookie := range cookies {
			writer.Header().Add("Set-Cookie", cookie)
		}
	}
	return result, nil
}

func resolveAutodiscover(p graphql.ResolveParams) (interface{}, error) {
	deps := dependenciesFromContext(p.Context)
	request := requestFromContext(p.Context)
	if deps.API == nil || request == nil {
		return nil, errors.New("service discovery is unavailable")
	}
	result, err := deps.API.AutodiscoverForGraphQL(p.Context, request)
	if err != nil {
		return nil, errors.New("service discovery is unavailable")
	}
	return result, nil
}
