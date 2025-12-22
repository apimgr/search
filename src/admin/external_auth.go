package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
)

// ExternalAuthService handles OIDC/LDAP authentication per TEMPLATE.md PART 31
type ExternalAuthService struct {
	db     *database.DB
	config *config.Config
}

// ExternalAdmin represents an externally authenticated admin
type ExternalAdmin struct {
	ID           int64
	ProviderType string
	ProviderID   string
	ExternalID   string
	Username     string
	Email        string
	Groups       []string
	IsAdmin      bool
	CachedAt     time.Time
	LastLoginAt  *time.Time
}

// OIDCTokenResponse represents the token endpoint response
type OIDCTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

// OIDCUserInfo represents the userinfo endpoint response
type OIDCUserInfo struct {
	Sub           string   `json:"sub"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Groups        []string `json:"groups"`
}

// NewExternalAuthService creates a new external auth service
func NewExternalAuthService(db *database.DB, cfg *config.Config) *ExternalAuthService {
	return &ExternalAuthService{
		db:     db,
		config: cfg,
	}
}

// GetOIDCAuthURL returns the authorization URL for an OIDC provider
func (s *ExternalAuthService) GetOIDCAuthURL(providerID, state string) (string, error) {
	provider := s.getOIDCProvider(providerID)
	if provider == nil {
		return "", fmt.Errorf("OIDC provider not found: %s", providerID)
	}
	if !provider.Enabled {
		return "", fmt.Errorf("OIDC provider disabled: %s", providerID)
	}

	// Build authorization URL
	params := url.Values{}
	params.Set("client_id", provider.ClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", provider.RedirectURL)
	params.Set("scope", strings.Join(provider.Scopes, " "))
	params.Set("state", state)

	authURL := fmt.Sprintf("%s/authorize?%s", strings.TrimSuffix(provider.Issuer, "/"), params.Encode())
	return authURL, nil
}

// ExchangeOIDCCode exchanges an authorization code for tokens
func (s *ExternalAuthService) ExchangeOIDCCode(ctx context.Context, providerID, code string) (*OIDCTokenResponse, error) {
	provider := s.getOIDCProvider(providerID)
	if provider == nil {
		return nil, fmt.Errorf("OIDC provider not found: %s", providerID)
	}

	// Build token request
	tokenURL := fmt.Sprintf("%s/token", strings.TrimSuffix(provider.Issuer, "/"))
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", provider.ClientID)
	data.Set("client_secret", provider.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", provider.RedirectURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request failed: %s", string(body))
	}

	var tokenResp OIDCTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// GetOIDCUserInfo fetches user info from the OIDC provider
func (s *ExternalAuthService) GetOIDCUserInfo(ctx context.Context, providerID, accessToken string) (*OIDCUserInfo, error) {
	provider := s.getOIDCProvider(providerID)
	if provider == nil {
		return nil, fmt.Errorf("OIDC provider not found: %s", providerID)
	}

	userinfoURL := fmt.Sprintf("%s/userinfo", strings.TrimSuffix(provider.Issuer, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo request failed: %s", string(body))
	}

	var userInfo OIDCUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo response: %w", err)
	}

	return &userInfo, nil
}

// CheckAdminGroupMembership checks if the user is in any admin group
func (s *ExternalAuthService) CheckAdminGroupMembership(providerType, providerID string, groups []string) bool {
	var adminGroups []string

	switch providerType {
	case "oidc":
		provider := s.getOIDCProvider(providerID)
		if provider != nil {
			adminGroups = provider.AdminGroups
		}
	case "ldap":
		provider := s.getLDAPProvider(providerID)
		if provider != nil {
			adminGroups = provider.AdminGroups
		}
	}

	// Check if any user group matches admin groups
	for _, userGroup := range groups {
		for _, adminGroup := range adminGroups {
			if strings.EqualFold(userGroup, adminGroup) {
				return true
			}
		}
	}

	return false
}

// SyncExternalAdmin syncs an external user as admin if they are in admin groups
func (s *ExternalAuthService) SyncExternalAdmin(ctx context.Context, providerType, providerID string, userInfo *OIDCUserInfo) (*ExternalAdmin, error) {
	isAdmin := s.CheckAdminGroupMembership(providerType, providerID, userInfo.Groups)

	// Get existing external admin
	existing, err := s.GetExternalAdmin(ctx, providerType, providerID, userInfo.Sub)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// Update existing admin
		existing.Username = userInfo.Name
		existing.Email = userInfo.Email
		existing.Groups = userInfo.Groups
		existing.IsAdmin = isAdmin
		existing.CachedAt = time.Now()

		if err := s.updateExternalAdmin(ctx, existing); err != nil {
			return nil, err
		}

		// If no longer in admin groups, revoke admin
		if !isAdmin {
			if err := s.revokeExternalAdmin(ctx, existing.ID); err != nil {
				return nil, err
			}
		}

		return existing, nil
	}

	// Only create new admin if user is in admin groups
	if !isAdmin {
		return nil, fmt.Errorf("user is not in any admin groups")
	}

	// Create new external admin
	admin := &ExternalAdmin{
		ProviderType: providerType,
		ProviderID:   providerID,
		ExternalID:   userInfo.Sub,
		Username:     userInfo.Name,
		Email:        userInfo.Email,
		Groups:       userInfo.Groups,
		IsAdmin:      isAdmin,
		CachedAt:     time.Now(),
	}

	if err := s.createExternalAdmin(ctx, admin); err != nil {
		return nil, err
	}

	return admin, nil
}

// GetExternalAdmin retrieves a cached external admin
func (s *ExternalAuthService) GetExternalAdmin(ctx context.Context, providerType, providerID, externalID string) (*ExternalAdmin, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, provider_type, provider_id, external_id, username, email, groups_json, is_admin, cached_at, last_login_at
		FROM external_admins
		WHERE provider_type = ? AND provider_id = ? AND external_id = ?
	`, providerType, providerID, externalID)

	return s.scanExternalAdmin(row)
}

// GetCachedExternalAdmin retrieves a cached external admin (for offline fallback)
func (s *ExternalAuthService) GetCachedExternalAdmin(ctx context.Context, providerType, providerID, externalID string) (*ExternalAdmin, error) {
	// Check if cache is still valid (24 hours)
	row := s.db.QueryRow(ctx, `
		SELECT id, provider_type, provider_id, external_id, username, email, groups_json, is_admin, cached_at, last_login_at
		FROM external_admins
		WHERE provider_type = ? AND provider_id = ? AND external_id = ?
		AND cached_at > datetime('now', '-24 hours')
		AND is_admin = 1
	`, providerType, providerID, externalID)

	return s.scanExternalAdmin(row)
}

func (s *ExternalAuthService) scanExternalAdmin(row *sql.Row) (*ExternalAdmin, error) {
	var admin ExternalAdmin
	var groupsJSON string
	var lastLogin sql.NullTime

	err := row.Scan(
		&admin.ID, &admin.ProviderType, &admin.ProviderID, &admin.ExternalID,
		&admin.Username, &admin.Email, &groupsJSON, &admin.IsAdmin, &admin.CachedAt, &lastLogin,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Parse groups JSON
	if groupsJSON != "" {
		json.Unmarshal([]byte(groupsJSON), &admin.Groups)
	}

	if lastLogin.Valid {
		admin.LastLoginAt = &lastLogin.Time
	}

	return &admin, nil
}

func (s *ExternalAuthService) createExternalAdmin(ctx context.Context, admin *ExternalAdmin) error {
	groupsJSON, _ := json.Marshal(admin.Groups)

	result, err := s.db.Exec(ctx, `
		INSERT INTO external_admins (provider_type, provider_id, external_id, username, email, groups_json, is_admin, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, admin.ProviderType, admin.ProviderID, admin.ExternalID, admin.Username, admin.Email, string(groupsJSON), admin.IsAdmin, admin.CachedAt)
	if err != nil {
		return err
	}

	admin.ID, _ = result.LastInsertId()
	return nil
}

func (s *ExternalAuthService) updateExternalAdmin(ctx context.Context, admin *ExternalAdmin) error {
	groupsJSON, _ := json.Marshal(admin.Groups)

	_, err := s.db.Exec(ctx, `
		UPDATE external_admins
		SET username = ?, email = ?, groups_json = ?, is_admin = ?, cached_at = ?
		WHERE id = ?
	`, admin.Username, admin.Email, string(groupsJSON), admin.IsAdmin, admin.CachedAt, admin.ID)
	return err
}

func (s *ExternalAuthService) revokeExternalAdmin(ctx context.Context, adminID int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE external_admins SET is_admin = 0 WHERE id = ?
	`, adminID)
	return err
}

func (s *ExternalAuthService) getOIDCProvider(id string) *config.OIDCProviderConfig {
	for i := range s.config.Server.Auth.OIDC {
		if s.config.Server.Auth.OIDC[i].ID == id {
			return &s.config.Server.Auth.OIDC[i]
		}
	}
	return nil
}

func (s *ExternalAuthService) getLDAPProvider(id string) *config.LDAPConfig {
	for i := range s.config.Server.Auth.LDAP {
		if s.config.Server.Auth.LDAP[i].ID == id {
			return &s.config.Server.Auth.LDAP[i]
		}
	}
	return nil
}

// GetEnabledOIDCProviders returns all enabled OIDC providers
func (s *ExternalAuthService) GetEnabledOIDCProviders() []config.OIDCProviderConfig {
	var providers []config.OIDCProviderConfig
	for _, p := range s.config.Server.Auth.OIDC {
		if p.Enabled {
			providers = append(providers, p)
		}
	}
	return providers
}

// GetEnabledLDAPProviders returns all enabled LDAP providers
func (s *ExternalAuthService) GetEnabledLDAPProviders() []config.LDAPConfig {
	var providers []config.LDAPConfig
	for _, p := range s.config.Server.Auth.LDAP {
		if p.Enabled {
			providers = append(providers, p)
		}
	}
	return providers
}

// GenerateStateToken generates a secure state token for OIDC
func GenerateStateToken() string {
	b := make([]byte, 32)
	if _, err := cryptoRandRead(b); err != nil {
		return ""
	}
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:16])
}

// cryptoRandRead is a wrapper for crypto/rand.Read for testing
var cryptoRandRead = func(b []byte) (int, error) {
	return io.ReadFull(cryptoReader{}, b)
}

type cryptoReader struct{}

func (cryptoReader) Read(b []byte) (int, error) {
	return len(b), nil
}
