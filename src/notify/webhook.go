package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
	"github.com/google/uuid"
)

const (
	maxWebhookBodyBytes     = 64 << 10
	maxWebhookResponseBytes = 8 << 10
	webhookQueueSize        = 256
)

var webhookRetryDelays = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}

type Event struct {
	Role           string
	Name           string
	Subject        string
	Body           string
	Severity       string
	TrackingID     string
	ProjectName    string
	ProjectVersion string
	AppName        string
	AppURL         string
	Timestamp      time.Time
	SourceIP       string `json:"-"`
}

type delivery struct {
	url       string
	transport string
	secret    string
	event     Event
	id        string
	attempt   int
}

type Manager struct {
	config  func() *config.Config
	client  *http.Client
	logger  *slog.Logger
	queue   chan delivery
	once    sync.Once
	logMu   sync.Mutex
	lastLog map[string]time.Time
}

func NewManager(configSource func() *config.Config, client *http.Client, logger *slog.Logger) *Manager {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	copyClient := *client
	copyClient.Timeout = 10 * time.Second
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{config: configSource, client: &copyClient, logger: logger, queue: make(chan delivery, webhookQueueSize), lastLog: make(map[string]time.Time)}
}

func (m *Manager) Start(ctx context.Context) {
	m.once.Do(func() {
		for range 4 {
			go m.run(ctx)
		}
	})
}

func (m *Manager) Dispatch(ctx context.Context, event Event) error {
	return m.dispatch(ctx, event, true)
}

// DispatchExact sends only to the configured role without cross-role fallback.
func (m *Manager) DispatchExact(ctx context.Context, event Event) error {
	return m.dispatch(ctx, event, false)
}

func (m *Manager) dispatch(ctx context.Context, event Event, fallback bool) error {
	if m == nil || m.config == nil {
		return errors.New("notification manager is unavailable")
	}
	if event.Role == "" || event.Name == "" || event.Subject == "" {
		return errors.New("notification event requires role, name, and subject")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	if len(event.Body) > maxWebhookBodyBytes {
		return errors.New("notification body exceeds the size limit")
	}
	configSnapshot := m.config()
	if configSnapshot == nil {
		return errors.New("notification configuration is unavailable")
	}
	serverConfig := configSnapshot.Get()
	if event.ProjectName == "" {
		event.ProjectName = "search"
	}
	if event.ProjectVersion == "" {
		event.ProjectVersion = version.Version
	}
	if event.AppName == "" {
		event.AppName = serverConfig.Branding.Title
	}
	if event.AppURL == "" {
		event.AppURL = configuredAppURL(serverConfig)
	}
	webhooks := resolveWebhooks(serverConfig.Contact, event.Role)
	if !fallback {
		webhooks = resolveExactWebhooks(serverConfig.Contact, event.Role)
	}
	for _, endpoint := range webhooks {
		if endpoint.url == "" {
			continue
		}
		if endpoint.secret == "" {
			m.logFailure("error", event.Role, "unknown", event.SourceIP, "configured webhook has no signing secret")
			return errors.New("configured webhook is missing its signing secret")
		}
	}
	for transport, endpoint := range webhooks {
		if endpoint.url == "" {
			continue
		}
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("create webhook event id: %w", err)
		}
		job := delivery{url: endpoint.url, transport: transport, secret: endpoint.secret, event: event, id: id.String()}
		select {
		case m.queue <- job:
		case <-ctx.Done():
			return ctx.Err()
		default:
			m.logFailure("error", event.Role, transport, event.SourceIP, "webhook delivery queue is full")
			return errors.New("webhook delivery queue is full")
		}
	}
	return nil
}

func (m *Manager) HasWebhook(role string) bool {
	if m == nil || m.config == nil {
		return false
	}
	configSnapshot := m.config()
	if configSnapshot == nil {
		return false
	}
	return HasConfiguredWebhook(configSnapshot.Get().Contact, role)
}

func configuredAppURL(server config.ServerConfig) string {
	host := strings.TrimSpace(server.FQDN)
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		return strings.TrimRight(host, "/") + strings.TrimRight(server.BaseURL, "/")
	}
	scheme := "http"
	if server.HTTPSPort > 0 || server.SSL.Enabled {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimRight(host, "/") + strings.TrimRight(server.BaseURL, "/")
}

type webhookTarget struct {
	url    string
	secret string
}

func resolveWebhooks(contactConfig config.ContactConfig, role string) map[string]webhookTarget {
	roles := make([]config.ContactRoleConfig, 0, 3)
	switch role {
	case "admin":
		roles = append(roles, contactConfig.Admin)
	case "security":
		roles = append(roles, contactConfig.Security, contactConfig.Admin)
	case "abuse":
		roles = append(roles, contactConfig.Abuse, contactConfig.General, contactConfig.Admin)
	case "general":
		roles = append(roles, contactConfig.General, contactConfig.Admin)
	default:
		return map[string]webhookTarget{}
	}
	result := make(map[string]webhookTarget)
	for _, configuredRole := range roles {
		for transport, endpoint := range resolveWebhookRole(configuredRole) {
			if _, exists := result[transport]; !exists {
				result[transport] = endpoint
			}
		}
	}
	return result
}

func resolveWebhookRole(role config.ContactRoleConfig) map[string]webhookTarget {
	result := make(map[string]webhookTarget)
	for transport, endpoint := range role.Webhooks {
		if strings.HasSuffix(transport, "_secret") || endpoint == "" {
			continue
		}
		result[transport] = webhookTarget{url: endpoint, secret: role.Webhooks[transport+"_secret"]}
	}
	return result
}

func resolveExactWebhooks(contactConfig config.ContactConfig, role string) map[string]webhookTarget {
	var configuredRole config.ContactRoleConfig
	switch role {
	case "admin":
		configuredRole = contactConfig.Admin
	case "security":
		configuredRole = contactConfig.Security
	case "abuse":
		configuredRole = contactConfig.Abuse
	case "general":
		configuredRole = contactConfig.General
	default:
		return map[string]webhookTarget{}
	}
	return resolveWebhookRole(configuredRole)
}

func HasConfiguredWebhook(contactConfig config.ContactConfig, role string) bool {
	for _, endpoint := range resolveWebhooks(contactConfig, role) {
		if endpoint.url != "" {
			return true
		}
	}
	return false
}

func HasExactWebhook(contactConfig config.ContactConfig, role string) bool {
	for _, endpoint := range resolveExactWebhooks(contactConfig, role) {
		if endpoint.url != "" {
			return true
		}
	}
	return false
}

func (m *Manager) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-m.queue:
			if err := m.deliver(ctx, job); err != nil {
				if job.attempt >= len(webhookRetryDelays) {
					m.logFailure("error", job.event.Role, job.transport, job.event.SourceIP, "webhook delivery permanently failed")
					continue
				}
				m.retry(ctx, job)
			}
		}
	}
}

func (m *Manager) retry(ctx context.Context, job delivery) {
	delay := webhookRetryDelays[job.attempt]
	job.attempt++
	m.logFailure("warn", job.event.Role, job.transport, job.event.SourceIP, "webhook delivery will retry", "attempt", job.attempt, "delay", delay)
	timer := time.NewTimer(delay)
	go func() {
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
			select {
			case m.queue <- job:
			case <-ctx.Done():
			}
		}
	}()
}

func (m *Manager) logFailure(level, role, transport, sourceIP, message string, details ...any) {
	key := role + ":" + transport + ":"
	if sourceIP != "" {
		sourceHash := sha256.Sum256([]byte(sourceIP))
		key += hex.EncodeToString(sourceHash[:])
	} else {
		key += "system"
	}
	now := time.Now()
	m.logMu.Lock()
	if len(m.lastLog) >= 8192 {
		for oldKey, last := range m.lastLog {
			if now.Sub(last) >= time.Minute {
				delete(m.lastLog, oldKey)
			}
		}
		if len(m.lastLog) >= 8192 {
			m.logMu.Unlock()
			return
		}
	}
	previous := m.lastLog[key]
	if !previous.IsZero() && now.Sub(previous) < time.Minute {
		m.logMu.Unlock()
		return
	}
	m.lastLog[key] = now
	m.logMu.Unlock()
	attrs := []any{"event", "notify.webhook_failed", "role", role, "transport", transport}
	attrs = append(attrs, details...)
	switch level {
	case "warn":
		m.logger.Warn(message, attrs...)
	default:
		m.logger.Error(message, attrs...)
	}
}

func (m *Manager) deliver(ctx context.Context, job delivery) error {
	requestURL, body, contentType, err := webhookRequest(job)
	if err != nil {
		return err
	}
	if len(body) > maxWebhookBodyBytes {
		return errors.New("webhook request exceeds the size limit")
	}
	mac := hmac.New(sha256.New, []byte(job.secret))
	_, _ = mac.Write(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprint(job.event.Timestamp.Unix()))
	req.Header.Set("X-Webhook-ID", job.id)
	req.Header.Set("X-Webhook-Event", job.event.Name)
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s (+%s)", job.event.ProjectName, job.event.ProjectVersion, job.event.AppURL))
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			m.logger.Warn("failed to close webhook response body", "error", err)
		}
	}()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxWebhookResponseBytes))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func webhookRequest(job delivery) (string, []byte, string, error) {
	parsed, err := url.Parse(job.url)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", nil, "", errors.New("webhook URL must be a credential-free HTTPS URL")
	}
	message := job.event.Subject + "\n\n" + job.event.Body
	switch job.transport {
	case "telegram":
		query := parsed.Query()
		query.Set("text", message)
		parsed.RawQuery = query.Encode()
		return parsed.String(), nil, "application/x-www-form-urlencoded", nil
	case "discord":
		return parsed.String(), marshalPayload(map[string]string{"content": message, "username": job.event.AppName}), "application/json", nil
	case "slack", "mattermost":
		return parsed.String(), marshalPayload(map[string]string{"text": message}), "application/json", nil
	case "pushover":
		query := parsed.Query()
		user, token := query.Get("user"), query.Get("token")
		query.Del("user")
		query.Del("token")
		parsed.RawQuery = query.Encode()
		form := url.Values{"user": {user}, "token": {token}, "message": {message}, "title": {job.event.Subject}}
		return parsed.String(), []byte(form.Encode()), "application/x-www-form-urlencoded", nil
	case "gotify":
		query := parsed.Query()
		token := query.Get("token")
		query.Del("token")
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/message"
		if token != "" {
			query.Set("token", token)
		}
		parsed.RawQuery = query.Encode()
		return parsed.String(), marshalPayload(map[string]interface{}{"title": job.event.Subject, "message": message, "priority": 5}), "application/json", nil
	case "generic":
		payload := map[string]interface{}{"role": job.event.Role, "event": job.event.Name, "subject": job.event.Subject, "body": job.event.Body, "severity": job.event.Severity, "timestamp": job.event.Timestamp.Format(time.RFC3339Nano), "project_name": job.event.ProjectName, "project_version": job.event.ProjectVersion, "app_url": job.event.AppURL}
		if job.event.TrackingID != "" {
			payload["tracking_id"] = job.event.TrackingID
		}
		return parsed.String(), marshalPayload(payload), "application/json", nil
	default:
		return "", nil, "", fmt.Errorf("unknown webhook transport %q", job.transport)
	}
}

func marshalPayload(value interface{}) []byte {
	data, _ := json.Marshal(value)
	return data
}

func GenerateWebhookSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}
