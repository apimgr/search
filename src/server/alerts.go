package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
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
	Alert         *alert.Alert
	ManageURL     string
	RSSURL        string
	OwnerToken    string
	WebhookSecret string
}

const alertCreateResultCookie = "search_alert_created"

type alertCreateResult struct {
	Issued       int64  `json:"issued"`
	OwnerToken   string `json:"owner_token,omitempty"`
	ManageToken  string `json:"manage_token"`
	RSSToken     string `json:"rss_token"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

func (s *Server) clearAlertCreateResultCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: alertCreateResultCookie, Value: "", Path: "/alerts/created", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) setAlertCreateResultCookie(w http.ResponseWriter, r *http.Request, result alertCreateResult) error {
	if s.dbManager == nil || s.dbManager.ServerDB() == nil {
		return errors.New("server database unavailable for alert result encryption")
	}
	secret, err := database.EnsureSecret(r.Context(), s.dbManager.ServerDB(), database.SecretCookieSigning)
	if err != nil {
		return err
	}
	encodedKey, err := base64.StdEncoding.DecodeString(secret.Value)
	if err != nil {
		return err
	}
	key := sha256.Sum256(encodedKey)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	result.Issued = time.Now().UTC().Unix()
	plain, err := json.Marshal(result)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := aead.Seal(nil, nonce, plain, []byte("search-alert-created-v1"))
	value := "v1." + base64.RawURLEncoding.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString(ciphertext)
	http.SetCookie(w, &http.Cookie{Name: alertCreateResultCookie, Value: value, Path: "/alerts/created", MaxAge: 60, HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
	return nil
}

func (s *Server) takeAlertCreateResultCookie(w http.ResponseWriter, r *http.Request) (*alertCreateResult, error) {
	cookie, err := r.Cookie(alertCreateResultCookie)
	if err != nil {
		return nil, nil
	}
	s.clearAlertCreateResultCookie(w, r)
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 || parts[0] != "v1" || s.dbManager == nil || s.dbManager.ServerDB() == nil {
		return nil, errors.New("invalid alert result cookie")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	secret, err := database.GetSecret(r.Context(), s.dbManager.ServerDB(), database.SecretCookieSigning)
	if err != nil {
		return nil, err
	}
	for _, encodedKey := range []string{secret.Value, secret.PreviousValue} {
		if encodedKey == "" {
			continue
		}
		decoded, decodeErr := base64.StdEncoding.DecodeString(encodedKey)
		if decodeErr != nil {
			continue
		}
		key := sha256.Sum256(decoded)
		block, cipherErr := aes.NewCipher(key[:])
		if cipherErr != nil {
			continue
		}
		aead, cipherErr := cipher.NewGCM(block)
		if cipherErr != nil || len(nonce) != aead.NonceSize() {
			continue
		}
		plain, openErr := aead.Open(nil, nonce, ciphertext, []byte("search-alert-created-v1"))
		if openErr != nil {
			continue
		}
		var result alertCreateResult
		if json.Unmarshal(plain, &result) != nil || result.ManageToken == "" || result.RSSToken == "" {
			return nil, errors.New("invalid alert result payload")
		}
		now := time.Now().UTC().Unix()
		if result.Issued > now || now-result.Issued > 60 {
			return nil, errors.New("expired alert result cookie")
		}
		return &result, nil
	}
	return nil, errors.New("alert result cookie authentication failed")
}

type AlertManagePageData struct {
	PageData
	Alert            *alert.Alert
	ManagePath       string
	FeedURL          string
	WebhookSecret    string
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
	if s.ownerManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
		return
	}

	safeSearch, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("safe_search")))
	if safeSearch < 0 || safeSearch > 2 {
		safeSearch = 1
	}
	baseData := s.newPageData(w, r, "", "alerts-new")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "alerts.create_title")
	mailer := s.currentMailer()
	data := &AlertNewPageData{
		PageData:         *baseData,
		Query:            strings.TrimSpace(r.URL.Query().Get("q")),
		Category:         model.ParseCategory(r.URL.Query().Get("category")).String(),
		Language:         strings.TrimSpace(r.URL.Query().Get("language")),
		Region:           strings.TrimSpace(r.URL.Query().Get("region")),
		SafeSearch:       safeSearch,
		Frequency:        s.config.Search.Alerts.DefaultFrequency,
		EmailEnabled:     mailer != nil && mailer.IsEnabled(),
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
		slog.Error("template render error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (s *Server) handleAlertCreated(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	result, err := s.takeAlertCreateResultCookie(w, r)
	if err != nil {
		slog.Warn("invalid one-use alert result cookie", "error", err)
	}
	if result == nil || err != nil {
		http.Redirect(w, r, "/alerts/new", http.StatusSeeOther)
		return
	}
	baseData := s.newPageData(w, r, "", "alerts-created")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "alerts.created_title")
	data := &AlertCreatedPageData{
		PageData:      *baseData,
		ManageURL:     s.getBaseURL(r) + "/alerts/manage/" + result.ManageToken,
		RSSURL:        s.getBaseURL(r) + "/alerts/" + result.RSSToken + ".rss",
		OwnerToken:    result.OwnerToken,
		WebhookSecret: result.WebhookSecret,
	}
	if err := s.renderer.Render(w, "alerts-created", data); err != nil {
		slog.Error("template render error", "err", err)
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
	if s.ownerManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
		return
	}
	if err := r.ParseForm(); err != nil {
		alertRedirectWithMessage(w, r, "/alerts/new", "error", "alerts.invalid_form_data")
		return
	}
	safeSearch, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("safe_search")))
	createRequest := alert.CreateRequest{
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
	}
	if err := s.alertManager.ValidateCreate(createRequest); err != nil {
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
	ownerToken := ""
	if cookie, err := r.Cookie(ownerTokenCookie); err == nil {
		ownerToken = cookie.Value
	}
	identity, issuedToken, err := s.ownerManager.IssueOrResolve(r.Context(), ownerToken)
	if err != nil {
		s.renderAlertError(w, r, http.StatusInternalServerError, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
		return
	}
	createRequest.OwnerID = identity.OwnerID
	created, err := s.alertManager.Create(r.Context(), createRequest)
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
	result := alertCreateResult{OwnerToken: issuedToken, ManageToken: created.ManageToken, RSSToken: created.RSSToken, WebhookSecret: created.WebhookSecret}
	if err := s.setAlertCreateResultCookie(w, r, result); err != nil {
		if cleanupErr := s.alertManager.Delete(r.Context(), created.ManageToken); cleanupErr != nil {
			slog.Error("failed to remove alert after result-cookie failure", "error", cleanupErr)
		}
		s.handleInternalError(w, r, "alert result cookie encryption", err)
		return
	}
	if issuedToken != "" {
		s.setOwnerCookie(w, r, issuedToken)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/alerts/created", http.StatusSeeOther)
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
	webhookSecret, err := s.alertManager.WebhookSecretForManageToken(r.Context(), token)
	if err != nil {
		s.renderAlertError(w, r, http.StatusInternalServerError, "alerts.error_unavailable_title", "alerts.error_unavailable")
		return
	}
	data := &AlertManagePageData{
		PageData:         *baseData,
		Alert:            alertInfo,
		ManagePath:       "/alerts/" + token,
		FeedURL:          feedURL,
		WebhookSecret:    webhookSecret,
		AvailableEngines: s.alertEngineOptions(alertInfo.Engines),
		Error:            strings.TrimSpace(r.URL.Query().Get("error")),
		Success:          strings.TrimSpace(r.URL.Query().Get("success")),
	}
	w.Header().Set("Cache-Control", "no-store")
	if err := s.renderer.Render(w, "alerts-manage", data); err != nil {
		slog.Error("template render error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (s *Server) handleAlertUpdate(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method != http.MethodPost {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	if s.alertManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
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
	if s.alertManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
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
	if s.alertManager == nil {
		s.renderAlertError(w, r, http.StatusServiceUnavailable, "alerts.error_unavailable_title", "alerts.error_storage_unavailable")
		return
	}
	if err := s.alertManager.Delete(r.Context(), token); err != nil {
		http.Redirect(w, r, "/alerts/manage/"+token+"?error="+urlQueryEscape(localizeAlertUserError(r, err)), http.StatusSeeOther)
		return
	}
	// Deleting an alert revokes its accountless access token. Per AI.md PART 11
	// emit Clear-Site-Data on token revocation so any cached alert data and stored
	// state for this origin is wiped from the browser.
	if s.config.Server.Security.Headers.ClearSiteData.OnTokenRevocation {
		s.setClearSiteData(w, true)
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
	// Guard against nil renderer (e.g. during tests or early startup)
	if s.renderer == nil {
		http.Error(w, message, status)
		return
	}
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
