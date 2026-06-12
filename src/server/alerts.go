package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/i18n"
	"github.com/apimgr/search/src/model"
)

type AlertNewPageData struct {
	PageData
	Query            string
	Category         string
	Language         string
	Region           string
	SafeSearch       int
	Frequency        string
	EmailEnabled     bool
	DefaultRSS       bool
	DefaultWebhook   bool
	AvailableEngines []AlertEngineOption
	Error            string
	Success          string
}

type AlertCreatedPageData struct {
	PageData
	Alert            *alert.Alert
	ManageURL        string
	RSSURL           string
	VerificationSent bool
}

type AlertManagePageData struct {
	PageData
	Alert            *alert.Alert
	ManagePath       string
	FeedURL          string
	AvailableEngines []AlertEngineOption
	Error            string
	Success          string
}

type AlertEngineOption struct {
	Name        string
	DisplayName string
	Selected    bool
}

func (s *Server) handleAlertNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	if s.alertManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
		return
	}

	safeSearch, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("safe_search")))
	if safeSearch < 0 || safeSearch > 2 {
		safeSearch = 1
	}
	baseData := s.newPageData(w, r, "", "alerts-new")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "alerts.create_title")
	data := &AlertNewPageData{
		PageData:         *baseData,
		Query:            strings.TrimSpace(r.URL.Query().Get("q")),
		Category:         model.ParseCategory(r.URL.Query().Get("category")).String(),
		Language:         strings.TrimSpace(r.URL.Query().Get("language")),
		Region:           strings.TrimSpace(r.URL.Query().Get("region")),
		SafeSearch:       safeSearch,
		Frequency:        s.config.Search.Alerts.DefaultFrequency,
		EmailEnabled:     s.mailer != nil && s.mailer.IsEnabled(),
		DefaultRSS:       s.config.Search.Alerts.DefaultDeliverRSS,
		DefaultWebhook:   s.config.Search.Alerts.DefaultDeliverWebhook,
		AvailableEngines: s.alertEngineOptions(splitCSVParam(r.URL.Query()["engines"])),
		Error:            strings.TrimSpace(r.URL.Query().Get("error")),
		Success:          strings.TrimSpace(r.URL.Query().Get("success")),
	}
	if !model.ParseCategory(data.Category).IsValid() {
		data.Category = model.CategoryGeneral.String()
	}
	if data.Language == "" {
		data.Language = "en"
	}
	if data.Frequency == "" {
		data.Frequency = string(alert.FrequencyDaily)
	}
	if err := s.renderer.Render(w, "alerts-new", data); err != nil {
		log.Printf("[Server] template render error: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	if s.alertManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
		return
	}
	if err := r.ParseForm(); err != nil {
		alertRedirectWithMessage(w, r, "/alerts/new", "error", "alerts.invalid_form_data")
		return
	}
	safeSearch, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("safe_search")))
	created, err := s.alertManager.Create(r.Context(), alert.CreateRequest{
		Query:          r.FormValue("query"),
		Category:       r.FormValue("category"),
		Language:       r.FormValue("language"),
		Region:         r.FormValue("region"),
		Engines:        r.Form["engines"],
		SafeSearch:     safeSearch,
		Frequency:      alert.Frequency(strings.TrimSpace(r.FormValue("frequency"))),
		Email:          r.FormValue("email"),
		DeliverEmail:   config.IsTruthy(r.FormValue("deliver_email")),
		DeliverRSS:     config.IsTruthy(r.FormValue("deliver_rss")),
		DeliverWebhook: config.IsTruthy(r.FormValue("deliver_webhook")),
		WebhookURL:     r.FormValue("webhook_url"),
		BaseURL:        s.getBaseURL(r),
		CreatedFromIP:  getClientIPSimple(r),
	})
	if err != nil {
		params := fmt.Sprintf("/alerts/new?q=%s&category=%s&language=%s&region=%s&safe_search=%s&error=%s%s",
			urlQueryEscape(r.FormValue("query")),
			urlQueryEscape(r.FormValue("category")),
			urlQueryEscape(r.FormValue("language")),
			urlQueryEscape(r.FormValue("region")),
			urlQueryEscape(r.FormValue("safe_search")),
			urlQueryEscape(localizeAlertUserError(r, err)),
			buildAlertEnginesQuery(r.Form["engines"]),
		)
		http.Redirect(w, r, params, http.StatusSeeOther)
		return
	}

	baseData := s.newPageData(w, r, "", "alerts-created")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "alerts.created_title")
	data := &AlertCreatedPageData{
		PageData:         *baseData,
		Alert:            created.Alert,
		ManageURL:        s.getBaseURL(r) + "/alerts/manage/" + created.ManageToken,
		RSSURL:           s.getBaseURL(r) + "/alerts/" + created.RSSToken + ".rss",
		VerificationSent: created.VerificationSent,
	}
	if err := s.renderer.Render(w, "alerts-created", data); err != nil {
		log.Printf("[Server] template render error: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (s *Server) handleAlertConfirm(w http.ResponseWriter, r *http.Request) {
	if s.alertManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		s.renderAlertError(w, r, http.StatusBadRequest, "alerts.error_invalid_token_title", "alerts.error_missing_verification_token")
		return
	}
	if _, _, _, err := s.alertManager.Verify(r.Context(), token); err != nil {
		s.renderAlertError(w, r, http.StatusBadRequest, "alerts.error_invalid_token_title", "alerts.error_invalid_token")
		return
	}
	baseData := s.newPageData(w, r, "", "alerts-created")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "alerts.confirmed_title")
	data := &AlertCreatedPageData{
		PageData:         *baseData,
		VerificationSent: false,
	}
	if err := s.renderer.Render(w, "alerts-confirmed", data); err != nil {
		log.Printf("[Server] template render error: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (s *Server) handleAlertAction(w http.ResponseWriter, r *http.Request) {
	if s.alertManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/alerts/")
	switch {
	case strings.HasSuffix(path, ".rss"):
		token := strings.TrimSuffix(path, ".rss")
		xmlData, err := s.alertManager.FeedXML(r.Context(), token, 50)
		if err != nil {
			s.renderAlertError(w, r, http.StatusNotFound, "alerts.error_not_found_title", "alerts.error_not_found")
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(xmlData)
	case strings.HasPrefix(path, "manage/"):
		token := strings.TrimPrefix(path, "manage/")
		s.renderManageAlert(w, r, token)
	default:
		token, action := splitAlertAction(path)
		if token == "" {
			http.NotFound(w, r)
			return
		}
		switch action {
		case "update":
			s.handleAlertUpdate(w, r, token)
		case "pause":
			s.handleAlertPause(w, r, token)
		case "delete":
			s.handleAlertDelete(w, r, token)
		default:
			http.NotFound(w, r)
		}
	}
}

func (s *Server) renderManageAlert(w http.ResponseWriter, r *http.Request, token string) {
	alertInfo, err := s.alertManager.GetByManageToken(r.Context(), token)
	if err != nil {
		s.renderAlertError(w, r, http.StatusNotFound, "alerts.error_not_found_title", "alerts.error_not_found")
		return
	}
	baseData := s.newPageData(w, r, "", "alerts-manage")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "alerts.manage_title")
	feedURL := ""
	if alertInfo.DeliverRSS {
		rssToken, err := s.alertManager.RSSTokenForManageToken(r.Context(), token)
		if err != nil {
			s.renderAlertError(w, r, http.StatusInternalServerError, "alerts.error_unavailable_title", "alerts.error_unavailable")
			return
		}
		if rssToken != "" {
			feedURL = s.getBaseURL(r) + "/alerts/" + rssToken + ".rss"
		}
	}
	data := &AlertManagePageData{
		PageData:         *baseData,
		Alert:            alertInfo,
		ManagePath:       "/alerts/" + token,
		FeedURL:          feedURL,
		AvailableEngines: s.alertEngineOptions(alertInfo.Engines),
		Error:            strings.TrimSpace(r.URL.Query().Get("error")),
		Success:          strings.TrimSpace(r.URL.Query().Get("success")),
	}
	if err := s.renderer.Render(w, "alerts-manage", data); err != nil {
		log.Printf("[Server] template render error: %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (s *Server) handleAlertUpdate(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method != http.MethodPost {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	if err := r.ParseForm(); err != nil {
		alertRedirectWithMessage(w, r, "/alerts/manage/"+token, "error", "alerts.invalid_form_data")
		return
	}
	safeSearch, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("safe_search")))
	if _, err := s.alertManager.Update(r.Context(), token, alert.UpdateRequest{
		Query:          r.FormValue("query"),
		Category:       r.FormValue("category"),
		Language:       r.FormValue("language"),
		Region:         r.FormValue("region"),
		Engines:        r.Form["engines"],
		Frequency:      alert.Frequency(strings.TrimSpace(r.FormValue("frequency"))),
		SafeSearch:     safeSearch,
		DeliverEmail:   config.IsTruthy(r.FormValue("deliver_email")),
		DeliverRSS:     config.IsTruthy(r.FormValue("deliver_rss")),
		DeliverWebhook: config.IsTruthy(r.FormValue("deliver_webhook")),
		WebhookURL:     r.FormValue("webhook_url"),
	}); err != nil {
		http.Redirect(w, r, "/alerts/manage/"+token+"?error="+urlQueryEscape(localizeAlertUserError(r, err)), http.StatusSeeOther)
		return
	}
	alertRedirectWithMessage(w, r, "/alerts/manage/"+token, "success", "alerts.updated_success")
}

func (s *Server) handleAlertPause(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method != http.MethodPost {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	pause, err := config.ParseBool(r.FormValue("paused"), true)
	if err != nil {
		http.Redirect(w, r, "/alerts/manage/"+token+"?error="+urlQueryEscape(localizeAlertUserError(r, err)), http.StatusSeeOther)
		return
	}
	if err := s.alertManager.SetPaused(r.Context(), token, pause); err != nil {
		http.Redirect(w, r, "/alerts/manage/"+token+"?error="+urlQueryEscape(localizeAlertUserError(r, err)), http.StatusSeeOther)
		return
	}
	messageKey := "alerts.paused_success"
	if !pause {
		messageKey = "alerts.resumed_success"
	}
	alertRedirectWithMessage(w, r, "/alerts/manage/"+token, "success", messageKey)
}

func (s *Server) handleAlertDelete(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method != http.MethodPost {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	if err := s.alertManager.Delete(r.Context(), token); err != nil {
		http.Redirect(w, r, "/alerts/manage/"+token+"?error="+urlQueryEscape(localizeAlertUserError(r, err)), http.StatusSeeOther)
		return
	}
	alertRedirectWithMessage(w, r, "/alerts/new", "success", "alerts.deleted_success")
}

func splitAlertAction(path string) (string, string) {
	token, action, found := strings.Cut(path, "/")
	if !found {
		return "", ""
	}
	return token, action
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}

func buildAlertEnginesQuery(engines []string) string {
	if len(engines) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, engine := range engines {
		engine = strings.TrimSpace(engine)
		if engine == "" {
			continue
		}
		builder.WriteString("&engines=")
		builder.WriteString(urlQueryEscape(engine))
	}
	return builder.String()
}

func splitCSVParam(values []string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				parts = append(parts, item)
			}
		}
	}
	return parts
}

func (s *Server) alertEngineOptions(selected []string) []AlertEngineOption {
	selectedSet := make(map[string]struct{}, len(selected))
	for _, name := range selected {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			selectedSet[name] = struct{}{}
		}
	}

	options := make([]AlertEngineOption, 0)
	if s.registry == nil {
		return options
	}

	for _, engine := range s.registry.GetEnabled() {
		name := strings.ToLower(strings.TrimSpace(engine.Name()))
		_, isSelected := selectedSet[name]
		options = append(options, AlertEngineOption{
			Name:        name,
			DisplayName: strings.TrimSpace(engine.DisplayName()),
			Selected:    isSelected,
		})
	}

	sort.Slice(options, func(i, j int) bool {
		return options[i].DisplayName < options[j].DisplayName
	})

	return options
}

func (s *Server) renderAlertError(w http.ResponseWriter, r *http.Request, status int, titleKey, messageKey string, args ...interface{}) {
	title := i18n.RequestString(r, titleKey)
	message := i18n.RequestString(r, messageKey, args...)
	baseData := s.newPageData(w, r, title, "error")
	data := &ErrorPageData{
		PageData:   *baseData,
		StatusCode: status,
		StatusText: title,
		Message:    message,
	}
	if s.config.IsDevelopment() {
		data.ErrorDetails = fmt.Sprintf("Request: %s %s", r.Method, r.URL.Path)
	}
	w.WriteHeader(status)
	if err := s.renderer.Render(w, "error", data); err != nil {
		http.Error(w, message, status)
	}
}

func alertRedirectWithMessage(w http.ResponseWriter, r *http.Request, path, queryKey, translationKey string, args ...interface{}) {
	http.Redirect(w, r, path+"?"+queryKey+"="+urlQueryEscape(i18n.RequestString(r, translationKey, args...)), http.StatusSeeOther)
}

func localizeAlertUserError(r *http.Request, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, alert.ErrNotFound) {
		return i18n.RequestString(r, "alerts.error_not_found")
	}
	if errors.Is(err, alert.ErrEmailRequired) {
		return i18n.RequestString(r, "alerts.error_email_required")
	}
	if errors.Is(err, alert.ErrInvalidToken) {
		return i18n.RequestString(r, "alerts.error_invalid_token")
	}
	if errors.Is(err, alert.ErrInvalidInput) {
		switch {
		case strings.Contains(err.Error(), "query is required"):
			return i18n.RequestString(r, "alerts.error_query_required")
		case strings.Contains(err.Error(), "category is invalid"):
			return i18n.RequestString(r, "alerts.error_category_invalid")
		case strings.Contains(err.Error(), "choose at least one delivery channel"):
			return i18n.RequestString(r, "alerts.error_delivery_channel_required")
		case strings.Contains(err.Error(), "email is required"):
			return i18n.RequestString(r, "alerts.error_email_required")
		case strings.Contains(err.Error(), "webhook URL is required"):
			return i18n.RequestString(r, "alerts.error_webhook_required")
		case strings.Contains(err.Error(), "webhook URL is invalid"):
			return i18n.RequestString(r, "alerts.error_webhook_invalid")
		case strings.Contains(err.Error(), "rate limit exceeded"):
			return i18n.RequestString(r, "errors.rate_limit")
		case strings.Contains(err.Error(), "unknown engine"):
			return i18n.RequestString(r, "alerts.error_unknown_engine")
		default:
			return i18n.RequestString(r, "alerts.error_invalid_input")
		}
	}
	if strings.Contains(err.Error(), "alert storage unavailable") {
		return i18n.RequestString(r, "alerts.error_storage_unavailable")
	}
	if strings.Contains(err.Error(), "invalid boolean value") {
		return i18n.RequestString(r, "alerts.error_invalid_input")
	}
	return i18n.RequestString(r, "alerts.error_unavailable")
}
