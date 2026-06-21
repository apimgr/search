package alert

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/ssl"
)

type Frequency string

const (
	FrequencyImmediate Frequency = "immediate"
	FrequencyDaily     Frequency = "daily"
	FrequencyWeekly    Frequency = "weekly"
)

const (
	StatusPending = "pending"
	StatusActive  = "active"
	StatusPaused  = "paused"
)

var (
	ErrNotFound      = errors.New("alert not found")
	ErrInvalidToken  = errors.New("invalid alert token")
	ErrInvalidInput  = errors.New("invalid alert input")
	ErrEmailRequired = errors.New("email delivery requires SMTP configuration")
)

type Manager struct {
	db           *sql.DB
	serverConfig *config.Config
	aggregator   *search.Aggregator
	mailer       *email.Mailer
	templates    *email.EmailTemplate
	client       *http.Client
}

type Alert struct {
	ID             string
	Email          string
	Query          string
	Category       string
	Language       string
	Region         string
	Engines        []string
	SafeSearch     int
	Frequency      Frequency
	DeliverEmail   bool
	DeliverRSS     bool
	DeliverWebhook bool
	WebhookURL     string
	EmailVerified  bool
	Status         string
	BaseURL        string
	LastCheckedAt  *time.Time
	LastSentAt     *time.Time
	LastError      string
	CreatedFromIP  string
	CreatedAt      time.Time
	VerifiedAt     *time.Time
	PausedAt       *time.Time
}

type AlertResult struct {
	ID                string
	Title             string
	URL               string
	Content           string
	Engine            string
	PublishedAt       *time.Time
	FirstSeenAt       time.Time
	NotifiedEmailAt   *time.Time
	NotifiedWebhookAt *time.Time
}

type CreateRequest struct {
	Query          string
	Category       string
	Language       string
	Region         string
	Engines        []string
	SafeSearch     int
	Frequency      Frequency
	Email          string
	DeliverEmail   bool
	DeliverRSS     bool
	DeliverWebhook bool
	WebhookURL     string
	BaseURL        string
	CreatedFromIP  string
}

type CreateResponse struct {
	Alert       *Alert
	ManageToken string
	RSSToken    string
}

type UpdateRequest struct {
	Query          string
	Category       string
	Language       string
	Region         string
	Engines        []string
	Frequency      Frequency
	DeliverEmail   bool
	DeliverRSS     bool
	DeliverWebhook bool
	WebhookURL     string
	SafeSearch     int
}

type Feed struct {
	Alert   *Alert
	Items   []AlertResult
	FeedURL string
}

func NewManager(db *sql.DB, serverConfig *config.Config, aggregator *search.Aggregator, mailer *email.Mailer) *Manager {
	return &Manager{
		db:           db,
		serverConfig: serverConfig,
		aggregator:   aggregator,
		mailer:       mailer,
		templates:    email.NewEmailTemplate(),
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (m *Manager) Create(ctx context.Context, req CreateRequest) (*CreateResponse, error) {
	if m.db == nil {
		return nil, fmt.Errorf("alert storage unavailable")
	}

	req.Query = strings.TrimSpace(req.Query)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.WebhookURL = strings.TrimSpace(req.WebhookURL)
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if req.BaseURL == "" {
		req.BaseURL = strings.TrimRight(strings.TrimSpace(m.serverConfig.Server.BaseURL), "/")
	}
	// Per AI.md PART 11: privacy — raw IPs must never be persisted.
	// Hash the IP so rate-limit buckets remain functional without storing the address.
	if req.CreatedFromIP != "" {
		h := sha256.Sum256([]byte(req.CreatedFromIP))
		req.CreatedFromIP = fmt.Sprintf("%x", h)
	}

	if req.Query == "" {
		return nil, fmt.Errorf("%w: query is required", ErrInvalidInput)
	}
	category := model.ParseCategory(req.Category)
	if !category.IsValid() {
		return nil, fmt.Errorf("%w: category is invalid", ErrInvalidInput)
	}
	if !isValidFrequency(req.Frequency) {
		req.Frequency = FrequencyDaily
	}
	req.Language = normalizeAlertLanguage(req.Language)
	req.Region = normalizeAlertRegion(req.Region)
	var err error
	req.Engines, err = m.normalizeAlertEngines(req.Engines)
	if err != nil {
		return nil, err
	}
	req.SafeSearch = normalizeSafeSearch(req.SafeSearch)
	if !req.DeliverEmail && !req.DeliverRSS && !req.DeliverWebhook {
		return nil, fmt.Errorf("%w: choose at least one delivery channel", ErrInvalidInput)
	}
	if req.DeliverEmail && (m.mailer == nil || !m.mailer.IsEnabled()) {
		return nil, ErrEmailRequired
	}
	if req.Email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrInvalidInput)
	}
	if req.DeliverWebhook {
		if req.WebhookURL == "" {
			return nil, fmt.Errorf("%w: webhook URL is required", ErrInvalidInput)
		}
		if _, err := url.ParseRequestURI(req.WebhookURL); err != nil {
			return nil, fmt.Errorf("%w: webhook URL is invalid", ErrInvalidInput)
		}
	}
	if err := m.enforceCreateRateLimit(ctx, req.CreatedFromIP); err != nil {
		return nil, err
	}

	alertID, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	manageToken, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	rssToken, err := randomToken(32)
	if err != nil {
		return nil, err
	}

	manageTokenEncrypted, err := encryptAlertToken(manageToken, m.serverConfig.Server.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt manage token: %w", err)
	}
	rssTokenEncrypted, err := encryptAlertToken(rssToken, m.serverConfig.Server.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt rss token: %w", err)
	}

	webhookSecretEncrypted := ""
	if req.DeliverWebhook {
		secret, err := randomToken(32)
		if err != nil {
			return nil, err
		}
		webhookSecretEncrypted, err = ssl.EncryptCredentials(map[string]string{"secret": secret}, m.serverConfig.Server.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt webhook secret: %w", err)
		}
	}

	now := time.Now().UTC()

	_, err = m.db.ExecContext(ctx, `
		INSERT INTO search_alerts (
			id, email, query, category, language, region, engines_json, safe_search, frequency,
			deliver_email, deliver_rss, deliver_webhook,
			webhook_url, webhook_secret_encrypted,
			email_verified, status, verify_token_hash, verify_token_expires,
			manage_token_hash, rss_token_hash, manage_token_encrypted, rss_token_encrypted, base_url, created_from_ip,
			created_at, verified_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		alertID, req.Email, req.Query, category.String(), req.Language, req.Region, marshalAlertEngines(req.Engines), req.SafeSearch, string(req.Frequency),
		boolToInt(req.DeliverEmail), boolToInt(req.DeliverRSS), boolToInt(req.DeliverWebhook),
		req.WebhookURL, webhookSecretEncrypted,
		1, StatusActive, nil, nil,
		hashTokenHex(manageToken), hashTokenHex(rssToken), manageTokenEncrypted, rssTokenEncrypted, req.BaseURL, req.CreatedFromIP,
		now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create alert: %w", err)
	}

	alert := &Alert{
		ID:             alertID,
		Email:          req.Email,
		Query:          req.Query,
		Category:       category.String(),
		Language:       req.Language,
		Region:         req.Region,
		Engines:        append([]string(nil), req.Engines...),
		SafeSearch:     req.SafeSearch,
		Frequency:      req.Frequency,
		DeliverEmail:   req.DeliverEmail,
		DeliverRSS:     req.DeliverRSS,
		DeliverWebhook: req.DeliverWebhook,
		WebhookURL:     req.WebhookURL,
		EmailVerified:  true,
		Status:         StatusActive,
		BaseURL:        req.BaseURL,
		CreatedFromIP:  req.CreatedFromIP,
		CreatedAt:      now,
		VerifiedAt:     ptrTime(now),
	}

	return &CreateResponse{
		Alert:       alert,
		ManageToken: manageToken,
		RSSToken:    rssToken,
	}, nil
}

func (m *Manager) GetByManageToken(ctx context.Context, token string) (*Alert, error) {
	return m.getAlertByTokenHash(ctx, "manage_token_hash", hashTokenHex(token))
}

func (m *Manager) GetByRSSToken(ctx context.Context, token string) (*Alert, error) {
	return m.getAlertByTokenHash(ctx, "rss_token_hash", hashTokenHex(token))
}

func (m *Manager) RSSTokenForManageToken(ctx context.Context, manageToken string) (string, error) {
	alertInfo, err := m.GetByManageToken(ctx, manageToken)
	if err != nil {
		return "", err
	}
	token, err := m.alertToken(ctx, alertInfo.ID, "rss_token_encrypted")
	if err != nil {
		return "", err
	}
	if token == "" {
		return manageToken, nil
	}
	return token, nil
}

func (m *Manager) Update(ctx context.Context, manageToken string, req UpdateRequest) (*Alert, error) {
	alert, err := m.GetByManageToken(ctx, manageToken)
	if err != nil {
		return nil, err
	}
	if !isValidFrequency(req.Frequency) {
		req.Frequency = alert.Frequency
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		req.Query = alert.Query
	}
	category := model.ParseCategory(req.Category)
	if strings.TrimSpace(req.Category) == "" {
		category = model.ParseCategory(alert.Category)
	}
	if !category.IsValid() {
		return nil, fmt.Errorf("%w: category is invalid", ErrInvalidInput)
	}
	req.Language = normalizeAlertLanguage(req.Language)
	req.Region = normalizeAlertRegion(req.Region)
	req.Engines, err = m.normalizeAlertEngines(req.Engines)
	if err != nil {
		return nil, err
	}
	req.SafeSearch = normalizeSafeSearch(req.SafeSearch)
	if req.DeliverEmail && (m.mailer == nil || !m.mailer.IsEnabled()) {
		return nil, ErrEmailRequired
	}
	if !req.DeliverEmail && !req.DeliverRSS && !req.DeliverWebhook {
		return nil, fmt.Errorf("%w: choose at least one delivery channel", ErrInvalidInput)
	}
	if req.DeliverWebhook {
		if req.WebhookURL == "" {
			req.WebhookURL = alert.WebhookURL
		}
		if _, err := url.ParseRequestURI(req.WebhookURL); err != nil {
			return nil, fmt.Errorf("%w: webhook URL is invalid", ErrInvalidInput)
		}
	}

	_, err = m.db.ExecContext(ctx, `
		UPDATE search_alerts
		SET query = ?, category = ?, language = ?, region = ?, engines_json = ?, frequency = ?, safe_search = ?, deliver_email = ?, deliver_rss = ?, deliver_webhook = ?, webhook_url = ?, paused_at = NULL
		WHERE manage_token_hash = ?
	`,
		req.Query, category.String(), req.Language, req.Region, marshalAlertEngines(req.Engines), string(req.Frequency), req.SafeSearch, boolToInt(req.DeliverEmail), boolToInt(req.DeliverRSS), boolToInt(req.DeliverWebhook), req.WebhookURL, hashTokenHex(manageToken),
	)
	if err != nil {
		return nil, fmt.Errorf("update alert: %w", err)
	}
	return m.GetByManageToken(ctx, manageToken)
}

func (m *Manager) SetPaused(ctx context.Context, manageToken string, paused bool) error {
	status := StatusActive
	var pausedAt interface{}
	if paused {
		status = StatusPaused
		pausedAt = time.Now().UTC()
	}
	_, err := m.db.ExecContext(ctx, `UPDATE search_alerts SET status = ?, paused_at = ? WHERE manage_token_hash = ?`, status, pausedAt, hashTokenHex(manageToken))
	if err != nil {
		return fmt.Errorf("update alert status: %w", err)
	}
	return nil
}

func (m *Manager) Delete(ctx context.Context, manageToken string) error {
	result, err := m.db.ExecContext(ctx, `DELETE FROM search_alerts WHERE manage_token_hash = ?`, hashTokenHex(manageToken))
	if err != nil {
		return fmt.Errorf("delete alert: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Manager) Feed(ctx context.Context, rssToken string, limit int) (*Feed, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	alert, err := m.GetByRSSToken(ctx, rssToken)
	if err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, title, url, content, engine, published_at, first_seen_at, notified_email_at, notified_webhook_at
		FROM search_alert_results
		WHERE alert_id = ?
		ORDER BY first_seen_at DESC
		LIMIT ?
	`, alert.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("load alert feed: %w", err)
	}
	defer rows.Close()

	items := make([]AlertResult, 0, limit)
	for rows.Next() {
		item, err := scanAlertResult(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	feedURL := alert.BaseURL + "/alerts/" + rssToken + ".rss"
	return &Feed{Alert: alert, Items: items, FeedURL: feedURL}, nil
}

func (m *Manager) FeedXML(ctx context.Context, rssToken string, limit int) ([]byte, error) {
	feed, err := m.Feed(ctx, rssToken, limit)
	if err != nil {
		return nil, err
	}
	type item struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		GUID        string `xml:"guid"`
		PubDate     string `xml:"pubDate,omitempty"`
	}
	type channel struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		Item        []item `xml:"item"`
	}
	type rss struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Channel channel  `xml:"channel"`
	}

	items := make([]item, 0, len(feed.Items))
	for _, result := range feed.Items {
		pubDate := result.FirstSeenAt.Format(time.RFC1123Z)
		if result.PublishedAt != nil && !result.PublishedAt.IsZero() {
			pubDate = result.PublishedAt.Format(time.RFC1123Z)
		}
		items = append(items, item{
			Title:       result.Title,
			Link:        result.URL,
			Description: result.Content,
			GUID:        result.ID,
			PubDate:     pubDate,
		})
	}

	doc := rss{
		Version: "2.0",
		Channel: channel{
			Title:       fmt.Sprintf("Search Alert: %s", feed.Alert.Query),
			Link:        feed.FeedURL,
			Description: fmt.Sprintf("Private RSS feed for %q", feed.Alert.Query),
			Item:        items,
		},
	}
	payload, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal rss: %w", err)
	}
	return append([]byte(xml.Header), payload...), nil
}

func (m *Manager) ProcessDue(ctx context.Context, frequency Frequency) error {
	if m.db == nil || m.aggregator == nil {
		return nil
	}
	if !isValidFrequency(frequency) {
		return nil
	}

	alerts, err := m.loadAlertsByFrequency(ctx, frequency)
	if err != nil {
		return err
	}
	if len(alerts) == 0 {
		return nil
	}

	var failures []string
	for _, alert := range alerts {
		if err := m.processAlert(ctx, alert); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", alert.ID, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("alert processing failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func (m *Manager) CleanupResults(ctx context.Context) error {
	if m.db == nil {
		return nil
	}
	retention := m.serverConfig.Search.Alerts.RetentionDays
	if retention < 1 {
		retention = 30
	}
	threshold := time.Now().UTC().AddDate(0, 0, -retention)
	_, err := m.db.ExecContext(ctx, `DELETE FROM search_alert_results WHERE first_seen_at < ?`, threshold)
	return err
}

func (m *Manager) processAlert(ctx context.Context, alert *Alert) error {
	query := model.NewQuery(alert.Query)
	query.Category = model.ParseCategory(alert.Category)
	query.Language = alert.Language
	query.Region = alert.Region
	query.Engines = append([]string(nil), alert.Engines...)
	query.SafeSearch = alert.SafeSearch
	query.PerPage = normalizePerPage(m.serverConfig.Search.ResultsPerPage)

	results, err := m.aggregator.Search(ctx, query)
	if err != nil && !errors.Is(err, model.ErrNoResults) {
		_, _ = m.db.ExecContext(ctx, `UPDATE search_alerts SET last_error = ? WHERE id = ?`, err.Error(), alert.ID)
		return err
	}

	if results != nil {
		for _, result := range results.Results {
			if err := m.insertResult(ctx, alert.ID, result); err != nil {
				return err
			}
		}
	}
	now := time.Now().UTC()
	if _, err := m.db.ExecContext(ctx, `UPDATE search_alerts SET last_checked_at = ?, last_error = '' WHERE id = ?`, now, alert.ID); err != nil {
		return err
	}

	if alert.DeliverEmail && m.mailer != nil && m.mailer.IsEnabled() {
		pendingEmail, err := m.pendingResults(ctx, alert.ID, "notified_email_at")
		if err != nil {
			return err
		}
		if len(pendingEmail) > 0 {
			if err := m.sendDigestEmail(ctx, alert, pendingEmail); err != nil {
				_, _ = m.db.ExecContext(ctx, `UPDATE search_alerts SET last_error = ? WHERE id = ?`, err.Error(), alert.ID)
				return err
			}
			if err := m.markResultsDelivered(ctx, pendingEmail, "notified_email_at"); err != nil {
				return err
			}
			_, _ = m.db.ExecContext(ctx, `UPDATE search_alerts SET last_sent_at = ? WHERE id = ?`, now, alert.ID)
		}
	}

	if alert.DeliverWebhook {
		pendingWebhook, err := m.pendingResults(ctx, alert.ID, "notified_webhook_at")
		if err != nil {
			return err
		}
		if len(pendingWebhook) > 0 {
			if err := m.sendWebhook(ctx, alert, pendingWebhook); err != nil {
				_, _ = m.db.ExecContext(ctx, `UPDATE search_alerts SET last_error = ? WHERE id = ?`, err.Error(), alert.ID)
				return err
			}
			if err := m.markResultsDelivered(ctx, pendingWebhook, "notified_webhook_at"); err != nil {
				return err
			}
			_, _ = m.db.ExecContext(ctx, `UPDATE search_alerts SET last_sent_at = ? WHERE id = ?`, now, alert.ID)
		}
	}

	return m.CleanupResults(ctx)
}

func (m *Manager) sendDigestEmail(ctx context.Context, alert *Alert, results []AlertResult) error {
	var body strings.Builder
	body.WriteString("<!DOCTYPE html><html><body style=\"font-family: sans-serif; max-width: 700px; margin: 0 auto; padding: 20px;\">")
	body.WriteString(fmt.Sprintf("<h1>New results for %q</h1>", alert.Query))
	body.WriteString("<ul>")
	for _, result := range results {
		body.WriteString(fmt.Sprintf("<li><a href=\"%s\">%s</a><br><small>%s</small></li>", result.URL, result.Title, result.Engine))
	}
	body.WriteString("</ul>")
	if manageURL, err := m.manageURL(ctx, alert); err == nil && manageURL != "" {
		body.WriteString(fmt.Sprintf("<p>Manage this alert: <a href=\"%s\">%s</a></p>", manageURL, manageURL))
	} else {
		body.WriteString("<p>Manage this alert from the link you saved when it was created.</p>")
	}
	body.WriteString("</body></html>")
	msg := email.NewMessage([]string{alert.Email}, fmt.Sprintf("Search alert: %s", alert.Query), "New search results are available.")
	msg.SetHTML(body.String())
	return m.mailer.Send(msg)
}

func (m *Manager) sendWebhook(ctx context.Context, alert *Alert, results []AlertResult) error {
	secret, err := m.webhookSecret(alert)
	if err != nil {
		return err
	}
	alertPayload := map[string]interface{}{
		"id":        alert.ID,
		"query":     alert.Query,
		"category":  alert.Category,
		"frequency": alert.Frequency,
	}
	if manageURL, err := m.manageURL(ctx, alert); err == nil && manageURL != "" {
		alertPayload["manage_url"] = manageURL
	}
	payload := map[string]interface{}{
		"alert": map[string]interface{}{
			"id":        alertPayload["id"],
			"query":     alertPayload["query"],
			"category":  alertPayload["category"],
			"frequency": alertPayload["frequency"],
		},
		"results": results,
		"sent_at": time.Now().UTC().Format(time.RFC3339),
	}
	if manageURL, ok := alertPayload["manage_url"]; ok {
		payload["alert"].(map[string]interface{})["manage_url"] = manageURL
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	signature := hex.EncodeToString(mac.Sum(nil))

	retries := m.serverConfig.Search.Alerts.WebhookMaxRetries
	if retries < 1 {
		retries = 3
	}
	delay := time.Duration(m.serverConfig.Search.Alerts.WebhookRetryDelayMinutes)
	if delay < 1 {
		delay = 5
	}
	delay = delay * time.Minute

	for attempt := 0; attempt < retries; attempt++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, alert.WebhookURL, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Search-Alert-Signature", "sha256="+signature)
		req.Header.Set("User-Agent", "search-alerts/1.0")
		resp, err := m.client.Do(req)
		if err == nil && resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if attempt < retries-1 {
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("webhook delivery failed")
}

func (m *Manager) enforceCreateRateLimit(ctx context.Context, ip string) error {
	limit := m.serverConfig.Search.Alerts.CreateRateLimitPerHour
	if limit < 1 || ip == "" {
		return nil
	}
	var count int
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM search_alerts WHERE created_from_ip = ? AND created_at >= ?`, ip, time.Now().UTC().Add(-1*time.Hour)).Scan(&count)
	if err != nil {
		return fmt.Errorf("check alert rate limit: %w", err)
	}
	if count >= limit {
		return fmt.Errorf("%w: rate limit exceeded", ErrInvalidInput)
	}
	return nil
}

func (m *Manager) loadAlertsByFrequency(ctx context.Context, frequency Frequency) ([]*Alert, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, email, query, category, language, region, engines_json, safe_search, frequency,
		       deliver_email, deliver_rss, deliver_webhook, webhook_url,
		       email_verified, status, base_url, last_checked_at, last_sent_at,
		       last_error, created_from_ip, created_at, verified_at, paused_at
		FROM search_alerts
		WHERE status = ? AND email_verified = 1 AND frequency = ?
	`, StatusActive, string(frequency))
	if err != nil {
		return nil, fmt.Errorf("load alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		alert, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

func (m *Manager) getAlertByTokenHash(ctx context.Context, column, tokenHash string) (*Alert, error) {
	query := fmt.Sprintf(`
		SELECT id, email, query, category, language, region, engines_json, safe_search, frequency,
		       deliver_email, deliver_rss, deliver_webhook, webhook_url,
		       email_verified, status, base_url, last_checked_at, last_sent_at,
		       last_error, created_from_ip, created_at, verified_at, paused_at
		FROM search_alerts WHERE %s = ?
	`, column)
	row := m.db.QueryRowContext(ctx, query, tokenHash)
	alert, err := scanAlert(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return alert, nil
}

func (m *Manager) insertResult(ctx context.Context, alertID string, result model.Result) error {
	fingerprint := fingerprintResult(result)
	resultID, err := randomToken(16)
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO search_alert_results (
			id, alert_id, fingerprint, title, url, content, engine, published_at, first_seen_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, resultID, alertID, fingerprint, result.Title, result.URL, result.Content, result.Engine, nullableTime(result.PublishedAt), time.Now().UTC())
	return err
}

func (m *Manager) pendingResults(ctx context.Context, alertID, column string) ([]AlertResult, error) {
	query := fmt.Sprintf(`
		SELECT id, title, url, content, engine, published_at, first_seen_at, notified_email_at, notified_webhook_at
		FROM search_alert_results
		WHERE alert_id = ? AND %s IS NULL
		ORDER BY first_seen_at ASC
	`, column)
	rows, err := m.db.QueryContext(ctx, query, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []AlertResult
	for rows.Next() {
		result, err := scanAlertResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *result)
	}
	return results, nil
}

func (m *Manager) markResultsDelivered(ctx context.Context, results []AlertResult, column string) error {
	if len(results) == 0 {
		return nil
	}
	var query string
	switch column {
	case "notified_email_at":
		query = `UPDATE search_alert_results SET notified_email_at = ? WHERE id = ?`
	case "notified_webhook_at":
		query = `UPDATE search_alert_results SET notified_webhook_at = ? WHERE id = ?`
	default:
		return fmt.Errorf("invalid delivery column: %s", column)
	}
	now := time.Now().UTC()
	for _, result := range results {
		if _, err := m.db.ExecContext(ctx, query, now, result.ID); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) webhookSecret(alert *Alert) (string, error) {
	var encrypted string
	// Per AI.md PART 10: SELECT queries must use context with 5s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.db.QueryRowContext(ctx, `SELECT webhook_secret_encrypted FROM search_alerts WHERE id = ?`, alert.ID).Scan(&encrypted); err != nil {
		return "", err
	}
	creds, err := ssl.DecryptCredentials(encrypted, m.serverConfig.Server.SecretKey)
	if err != nil {
		return "", err
	}
	return creds["secret"], nil
}

func (m *Manager) manageURL(ctx context.Context, alert *Alert) (string, error) {
	token, err := m.alertToken(ctx, alert.ID, "manage_token_encrypted")
	if err != nil || token == "" {
		return "", err
	}
	return strings.TrimRight(alert.BaseURL, "/") + "/alerts/manage/" + token, nil
}

func (m *Manager) alertToken(ctx context.Context, alertID, column string) (string, error) {
	var query string
	switch column {
	case "rss_token_encrypted":
		query = "SELECT rss_token_encrypted FROM search_alerts WHERE id = ?"
	case "manage_token_encrypted":
		query = "SELECT manage_token_encrypted FROM search_alerts WHERE id = ?"
	default:
		return "", fmt.Errorf("invalid token column: %s", column)
	}
	var encrypted string
	if err := m.db.QueryRowContext(ctx, query, alertID).Scan(&encrypted); err != nil {
		return "", err
	}
	return decryptAlertToken(encrypted, m.serverConfig.Server.SecretKey)
}

func scanAlert(scanner interface {
	Scan(dest ...interface{}) error
}) (*Alert, error) {
	var (
		alert                                              Alert
		frequency, enginesJSON                             string
		deliverEmail, deliverRSS, deliverWebhook, verified int
		lastError                                          sql.NullString
		lastChecked, lastSent, verifiedAt, pausedAt        sql.NullTime
	)
	if err := scanner.Scan(
		&alert.ID, &alert.Email, &alert.Query, &alert.Category, &alert.Language, &alert.Region, &enginesJSON, &alert.SafeSearch, &frequency,
		&deliverEmail, &deliverRSS, &deliverWebhook, &alert.WebhookURL,
		&verified, &alert.Status, &alert.BaseURL, &lastChecked, &lastSent,
		&lastError, &alert.CreatedFromIP, &alert.CreatedAt, &verifiedAt, &pausedAt,
	); err != nil {
		return nil, err
	}
	alert.Language = normalizeAlertLanguage(alert.Language)
	alert.Region = normalizeAlertRegion(alert.Region)
	alert.Engines = unmarshalAlertEngines(enginesJSON)
	alert.Frequency = Frequency(frequency)
	alert.DeliverEmail = deliverEmail == 1
	alert.DeliverRSS = deliverRSS == 1
	alert.DeliverWebhook = deliverWebhook == 1
	alert.EmailVerified = verified == 1
	if lastError.Valid {
		alert.LastError = lastError.String
	}
	if lastChecked.Valid {
		alert.LastCheckedAt = &lastChecked.Time
	}
	if lastSent.Valid {
		alert.LastSentAt = &lastSent.Time
	}
	if verifiedAt.Valid {
		alert.VerifiedAt = &verifiedAt.Time
	}
	if pausedAt.Valid {
		alert.PausedAt = &pausedAt.Time
	}
	return &alert, nil
}

func (m *Manager) normalizeAlertEngines(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return []string{}, nil
	}

	allowed := map[string]struct{}{}
	if m.aggregator != nil {
		for _, name := range m.aggregator.EngineNames() {
			allowed[name] = struct{}{}
		}
	}

	seen := map[string]struct{}{}
	engines := make([]string, 0, len(inputs))
	for _, input := range inputs {
		for _, part := range strings.Split(input, ",") {
			name := strings.ToLower(strings.TrimSpace(part))
			if name == "" {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[name]; !ok {
					return nil, fmt.Errorf("%w: unknown engine %q", ErrInvalidInput, name)
				}
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			engines = append(engines, name)
		}
	}

	return engines, nil
}

func normalizeAlertLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "en"
	}
	return value
}

func normalizeAlertRegion(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func marshalAlertEngines(engines []string) string {
	if len(engines) == 0 {
		return "[]"
	}
	data, err := json.Marshal(engines)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func unmarshalAlertEngines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}

	var engines []string
	if err := json.Unmarshal([]byte(raw), &engines); err == nil {
		return engines
	}

	parts := strings.Split(raw, ",")
	engines = make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.ToLower(strings.TrimSpace(part))
		if name != "" {
			engines = append(engines, name)
		}
	}
	return engines
}

func scanAlertResult(scanner interface {
	Scan(dest ...interface{}) error
}) (*AlertResult, error) {
	var (
		result                                      AlertResult
		publishedAt, notifiedEmail, notifiedWebhook sql.NullTime
	)
	if err := scanner.Scan(
		&result.ID, &result.Title, &result.URL, &result.Content, &result.Engine,
		&publishedAt, &result.FirstSeenAt, &notifiedEmail, &notifiedWebhook,
	); err != nil {
		return nil, err
	}
	if publishedAt.Valid {
		result.PublishedAt = &publishedAt.Time
	}
	if notifiedEmail.Valid {
		result.NotifiedEmailAt = &notifiedEmail.Time
	}
	if notifiedWebhook.Valid {
		result.NotifiedWebhookAt = &notifiedWebhook.Time
	}
	return &result, nil
}

func normalizeSafeSearch(value int) int {
	if value < 0 || value > 2 {
		return 1
	}
	return value
}

func normalizePerPage(value int) int {
	if value < 1 {
		return 20
	}
	if value > 100 {
		return 100
	}
	return value
}

func isValidFrequency(value Frequency) bool {
	switch value {
	case FrequencyImmediate, FrequencyDaily, FrequencyWeekly:
		return true
	default:
		return false
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func hashTokenHex(token string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(hash[:])
}

func encryptAlertToken(token, secretKey string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	return ssl.EncryptCredentials(map[string]string{"token": token}, secretKey)
}

func decryptAlertToken(encrypted, secretKey string) (string, error) {
	encrypted = strings.TrimSpace(encrypted)
	if encrypted == "" {
		return "", nil
	}
	creds, err := ssl.DecryptCredentials(encrypted, secretKey)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(creds["token"]), nil
}

func randomToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func fingerprintResult(result model.Result) string {
	key := strings.Join([]string{
		strings.TrimSpace(result.URL),
		strings.TrimSpace(result.Title),
		strings.TrimSpace(result.Engine),
		result.PublishedAt.UTC().Format(time.RFC3339),
	}, "|")
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

func nullableTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

