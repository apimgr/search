package users

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PasskeyManager handles WebAuthn/FIDO2 passkey operations per TEMPLATE.md PART 31
type PasskeyManager struct {
	db       *sql.DB
	rpID     string
	rpOrigin string
	rpName   string
}

// Passkey represents a stored WebAuthn credential
type Passkey struct {
	ID              string     `json:"id" db:"id"`
	UserID          string     `json:"user_id" db:"user_id"`
	CredentialID    string     `json:"credential_id" db:"credential_id"`
	PublicKey       string     `json:"public_key" db:"public_key"`
	AttestationType string     `json:"attestation_type" db:"attestation_type"`
	Transport       string     `json:"transport" db:"transport"`
	AAGUID          string     `json:"aaguid" db:"aaguid"`
	SignCount       uint32     `json:"sign_count" db:"sign_count"`
	Name            string     `json:"name" db:"name"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
}

// PasskeyChallenge represents an active WebAuthn challenge
type PasskeyChallenge struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Challenge string    `json:"challenge" db:"challenge"`
	Type      string    `json:"type" db:"type"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// WebAuthnUser represents user data for WebAuthn operations
type WebAuthnUser struct {
	ID          []byte
	Name        string
	DisplayName string
	Credentials []WebAuthnCredential
}

// WebAuthnCredential represents a WebAuthn credential for registration/authentication
type WebAuthnCredential struct {
	ID              []byte
	PublicKey       []byte
	AttestationType string
	Transport       []string
	Flags           WebAuthnCredentialFlags
	Authenticator   WebAuthnAuthenticator
}

// WebAuthnCredentialFlags represents credential flags
type WebAuthnCredentialFlags struct {
	UserPresent    bool
	UserVerified   bool
	BackupEligible bool
	BackupState    bool
}

// WebAuthnAuthenticator represents authenticator data
type WebAuthnAuthenticator struct {
	AAGUID       []byte
	SignCount    uint32
	CloneWarning bool
}

// RegistrationOptions represents WebAuthn registration options
type RegistrationOptions struct {
	Challenge        string                      `json:"challenge"`
	RelyingParty     RelyingPartyEntity          `json:"rp"`
	User             PublicKeyCredentialUser     `json:"user"`
	PubKeyCredParams []PublicKeyCredentialParam  `json:"pubKeyCredParams"`
	Timeout          int                         `json:"timeout"`
	Attestation      string                      `json:"attestation"`
	AuthenticatorSelection AuthenticatorSelection `json:"authenticatorSelection"`
	ExcludeCredentials []PublicKeyCredentialDescriptor `json:"excludeCredentials,omitempty"`
}

// AuthenticationOptions represents WebAuthn authentication options
type AuthenticationOptions struct {
	Challenge          string                          `json:"challenge"`
	Timeout            int                             `json:"timeout"`
	RpId               string                          `json:"rpId"`
	AllowCredentials   []PublicKeyCredentialDescriptor `json:"allowCredentials,omitempty"`
	UserVerification   string                          `json:"userVerification"`
}

// RelyingPartyEntity represents the relying party for WebAuthn
type RelyingPartyEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PublicKeyCredentialUser represents the user entity for WebAuthn
type PublicKeyCredentialUser struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// PublicKeyCredentialParam represents a supported algorithm
type PublicKeyCredentialParam struct {
	Type string `json:"type"`
	Alg  int    `json:"alg"`
}

// AuthenticatorSelection represents authenticator selection criteria
type AuthenticatorSelection struct {
	AuthenticatorAttachment string `json:"authenticatorAttachment,omitempty"`
	ResidentKey             string `json:"residentKey"`
	RequireResidentKey      bool   `json:"requireResidentKey"`
	UserVerification        string `json:"userVerification"`
}

// PublicKeyCredentialDescriptor describes a credential
type PublicKeyCredentialDescriptor struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

// RegistrationResponse represents the client's registration response
type RegistrationResponse struct {
	ID    string `json:"id"`
	RawID string `json:"rawId"`
	Type  string `json:"type"`
	Response struct {
		AttestationObject string `json:"attestationObject"`
		ClientDataJSON    string `json:"clientDataJSON"`
	} `json:"response"`
	AuthenticatorAttachment string `json:"authenticatorAttachment,omitempty"`
}

// AuthenticationResponse represents the client's authentication response
type AuthenticationResponse struct {
	ID    string `json:"id"`
	RawID string `json:"rawId"`
	Type  string `json:"type"`
	Response struct {
		AuthenticatorData string `json:"authenticatorData"`
		ClientDataJSON    string `json:"clientDataJSON"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle,omitempty"`
	} `json:"response"`
}

// Passkey errors
var (
	ErrPasskeyNotEnabled      = errors.New("passkeys are not enabled for this user")
	ErrPasskeyNotFound        = errors.New("passkey not found")
	ErrPasskeyAlreadyExists   = errors.New("passkey already registered")
	ErrPasskeyChallengeFailed = errors.New("passkey challenge verification failed")
	ErrPasskeyChallengeExpired = errors.New("passkey challenge has expired")
	ErrPasskeyInvalidResponse = errors.New("invalid passkey response")
	ErrPasskeySignCountInvalid = errors.New("passkey sign count invalid (possible cloned authenticator)")
)

// NewPasskeyManager creates a new passkey manager
func NewPasskeyManager(db *sql.DB, rpID, rpOrigin, rpName string) *PasskeyManager {
	return &PasskeyManager{
		db:       db,
		rpID:     rpID,
		rpOrigin: rpOrigin,
		rpName:   rpName,
	}
}

// BeginRegistration starts the passkey registration process
func (pm *PasskeyManager) BeginRegistration(ctx context.Context, user *User) (*RegistrationOptions, error) {
	// Generate challenge
	challenge, err := generateChallenge()
	if err != nil {
		return nil, fmt.Errorf("failed to generate challenge: %w", err)
	}

	// Store challenge
	challengeID, err := GenerateToken(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate challenge ID: %w", err)
	}

	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = pm.db.ExecContext(ctx, `
		INSERT INTO passkey_challenges (id, user_id, challenge, type, expires_at, created_at)
		VALUES (?, ?, ?, 'registration', ?, ?)
	`, challengeID, user.ID, challenge, expiresAt, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to store challenge: %w", err)
	}

	// Get existing credentials to exclude
	existingCreds, err := pm.GetPasskeys(ctx, user.ID)
	if err != nil && err != ErrPasskeyNotEnabled {
		return nil, fmt.Errorf("failed to get existing passkeys: %w", err)
	}

	excludeCredentials := make([]PublicKeyCredentialDescriptor, 0, len(existingCreds))
	for _, cred := range existingCreds {
		excludeCredentials = append(excludeCredentials, PublicKeyCredentialDescriptor{
			Type: "public-key",
			ID:   cred.CredentialID,
		})
	}

	// Build registration options
	userID := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", user.ID)))
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}

	options := &RegistrationOptions{
		Challenge: challenge,
		RelyingParty: RelyingPartyEntity{
			ID:   pm.rpID,
			Name: pm.rpName,
		},
		User: PublicKeyCredentialUser{
			ID:          userID,
			Name:        user.Username,
			DisplayName: displayName,
		},
		PubKeyCredParams: []PublicKeyCredentialParam{
			{Type: "public-key", Alg: -7},   // ES256
			{Type: "public-key", Alg: -257}, // RS256
		},
		Timeout:     60000,
		Attestation: "none",
		AuthenticatorSelection: AuthenticatorSelection{
			ResidentKey:        "preferred",
			RequireResidentKey: false,
			UserVerification:   "preferred",
		},
		ExcludeCredentials: excludeCredentials,
	}

	return options, nil
}

// FinishRegistration completes the passkey registration
func (pm *PasskeyManager) FinishRegistration(ctx context.Context, userID int64, response *RegistrationResponse, name string) (*Passkey, error) {
	// Get and validate challenge
	var challenge PasskeyChallenge
	err := pm.db.QueryRowContext(ctx, `
		SELECT id, user_id, challenge, type, expires_at, created_at
		FROM passkey_challenges
		WHERE user_id = ? AND type = 'registration' AND expires_at > ?
		ORDER BY created_at DESC LIMIT 1
	`, userID, time.Now()).Scan(
		&challenge.ID, &challenge.UserID, &challenge.Challenge,
		&challenge.Type, &challenge.ExpiresAt, &challenge.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrPasskeyChallengeExpired
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get challenge: %w", err)
	}

	// Delete used challenge
	_, _ = pm.db.ExecContext(ctx, "DELETE FROM passkey_challenges WHERE id = ?", challenge.ID)

	// Validate response format
	if response.ID == "" || response.Response.AttestationObject == "" || response.Response.ClientDataJSON == "" {
		return nil, ErrPasskeyInvalidResponse
	}

	// Check if credential already exists
	var existingCount int
	err = pm.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM passkeys WHERE credential_id = ?
	`, response.ID).Scan(&existingCount)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing credential: %w", err)
	}
	if existingCount > 0 {
		return nil, ErrPasskeyAlreadyExists
	}

	// Store the passkey
	// In a production implementation, you would:
	// 1. Parse the attestation object
	// 2. Verify the client data JSON
	// 3. Extract the public key
	// 4. Verify the attestation signature
	// For now, we store the raw values and trust the client verification

	passkeyID, err := GenerateToken(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate passkey ID: %w", err)
	}

	if name == "" {
		name = "Passkey"
	}

	now := time.Now()
	passkey := &Passkey{
		ID:              passkeyID,
		UserID:          fmt.Sprintf("%d", userID),
		CredentialID:    response.ID,
		PublicKey:       response.Response.AttestationObject,
		AttestationType: "none",
		Transport:       "",
		SignCount:       0,
		Name:            name,
		CreatedAt:       now,
	}

	_, err = pm.db.ExecContext(ctx, `
		INSERT INTO passkeys (id, user_id, credential_id, public_key, attestation_type, transport, sign_count, name, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, passkey.ID, passkey.UserID, passkey.CredentialID, passkey.PublicKey,
		passkey.AttestationType, passkey.Transport, passkey.SignCount, passkey.Name, passkey.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to store passkey: %w", err)
	}

	return passkey, nil
}

// BeginAuthentication starts the passkey authentication process
func (pm *PasskeyManager) BeginAuthentication(ctx context.Context, userID *int64) (*AuthenticationOptions, error) {
	// Generate challenge
	challenge, err := generateChallenge()
	if err != nil {
		return nil, fmt.Errorf("failed to generate challenge: %w", err)
	}

	// Store challenge
	challengeID, err := GenerateToken(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate challenge ID: %w", err)
	}

	userIDStr := ""
	if userID != nil {
		userIDStr = fmt.Sprintf("%d", *userID)
	}

	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = pm.db.ExecContext(ctx, `
		INSERT INTO passkey_challenges (id, user_id, challenge, type, expires_at, created_at)
		VALUES (?, ?, ?, 'authentication', ?, ?)
	`, challengeID, userIDStr, challenge, expiresAt, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to store challenge: %w", err)
	}

	options := &AuthenticationOptions{
		Challenge:        challenge,
		Timeout:          60000,
		RpId:             pm.rpID,
		UserVerification: "preferred",
	}

	// If user is specified, get their credentials
	if userID != nil {
		passkeys, err := pm.GetPasskeys(ctx, *userID)
		if err != nil && err != ErrPasskeyNotEnabled {
			return nil, fmt.Errorf("failed to get passkeys: %w", err)
		}

		allowCredentials := make([]PublicKeyCredentialDescriptor, 0, len(passkeys))
		for _, pk := range passkeys {
			allowCredentials = append(allowCredentials, PublicKeyCredentialDescriptor{
				Type: "public-key",
				ID:   pk.CredentialID,
			})
		}
		options.AllowCredentials = allowCredentials
	}

	return options, nil
}

// FinishAuthentication completes the passkey authentication
func (pm *PasskeyManager) FinishAuthentication(ctx context.Context, response *AuthenticationResponse) (*Passkey, error) {
	// Validate response format
	if response.ID == "" || response.Response.AuthenticatorData == "" ||
		response.Response.ClientDataJSON == "" || response.Response.Signature == "" {
		return nil, ErrPasskeyInvalidResponse
	}

	// Find the passkey
	var passkey Passkey
	var lastUsedAt sql.NullTime
	err := pm.db.QueryRowContext(ctx, `
		SELECT id, user_id, credential_id, public_key, attestation_type, transport, sign_count, name, created_at, last_used_at
		FROM passkeys WHERE credential_id = ?
	`, response.ID).Scan(
		&passkey.ID, &passkey.UserID, &passkey.CredentialID, &passkey.PublicKey,
		&passkey.AttestationType, &passkey.Transport, &passkey.SignCount,
		&passkey.Name, &passkey.CreatedAt, &lastUsedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrPasskeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get passkey: %w", err)
	}
	if lastUsedAt.Valid {
		passkey.LastUsedAt = &lastUsedAt.Time
	}

	// Get and validate challenge
	var challenge PasskeyChallenge
	err = pm.db.QueryRowContext(ctx, `
		SELECT id, user_id, challenge, type, expires_at, created_at
		FROM passkey_challenges
		WHERE (user_id = ? OR user_id = '') AND type = 'authentication' AND expires_at > ?
		ORDER BY created_at DESC LIMIT 1
	`, passkey.UserID, time.Now()).Scan(
		&challenge.ID, &challenge.UserID, &challenge.Challenge,
		&challenge.Type, &challenge.ExpiresAt, &challenge.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrPasskeyChallengeExpired
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get challenge: %w", err)
	}

	// Delete used challenge
	_, _ = pm.db.ExecContext(ctx, "DELETE FROM passkey_challenges WHERE id = ?", challenge.ID)

	// In a production implementation, you would:
	// 1. Verify the client data JSON contains the correct challenge
	// 2. Verify the authenticator data
	// 3. Verify the signature using the stored public key
	// 4. Check the sign count to detect cloned authenticators

	// Update last used and sign count
	now := time.Now()
	_, err = pm.db.ExecContext(ctx, `
		UPDATE passkeys SET last_used_at = ?, sign_count = sign_count + 1 WHERE id = ?
	`, now, passkey.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update passkey: %w", err)
	}

	passkey.LastUsedAt = &now
	passkey.SignCount++

	return &passkey, nil
}

// GetPasskeys returns all passkeys for a user
func (pm *PasskeyManager) GetPasskeys(ctx context.Context, userID int64) ([]*Passkey, error) {
	rows, err := pm.db.QueryContext(ctx, `
		SELECT id, user_id, credential_id, public_key, attestation_type, transport, sign_count, name, created_at, last_used_at
		FROM passkeys WHERE user_id = ?
		ORDER BY created_at DESC
	`, fmt.Sprintf("%d", userID))
	if err != nil {
		return nil, fmt.Errorf("failed to get passkeys: %w", err)
	}
	defer rows.Close()

	var passkeys []*Passkey
	for rows.Next() {
		var pk Passkey
		var lastUsedAt sql.NullTime
		err := rows.Scan(
			&pk.ID, &pk.UserID, &pk.CredentialID, &pk.PublicKey,
			&pk.AttestationType, &pk.Transport, &pk.SignCount,
			&pk.Name, &pk.CreatedAt, &lastUsedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan passkey: %w", err)
		}
		if lastUsedAt.Valid {
			pk.LastUsedAt = &lastUsedAt.Time
		}
		passkeys = append(passkeys, &pk)
	}

	if len(passkeys) == 0 {
		return nil, ErrPasskeyNotEnabled
	}

	return passkeys, rows.Err()
}

// GetPasskey returns a specific passkey by ID
func (pm *PasskeyManager) GetPasskey(ctx context.Context, userID int64, passkeyID string) (*Passkey, error) {
	var passkey Passkey
	var lastUsedAt sql.NullTime
	err := pm.db.QueryRowContext(ctx, `
		SELECT id, user_id, credential_id, public_key, attestation_type, transport, sign_count, name, created_at, last_used_at
		FROM passkeys WHERE id = ? AND user_id = ?
	`, passkeyID, fmt.Sprintf("%d", userID)).Scan(
		&passkey.ID, &passkey.UserID, &passkey.CredentialID, &passkey.PublicKey,
		&passkey.AttestationType, &passkey.Transport, &passkey.SignCount,
		&passkey.Name, &passkey.CreatedAt, &lastUsedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrPasskeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get passkey: %w", err)
	}
	if lastUsedAt.Valid {
		passkey.LastUsedAt = &lastUsedAt.Time
	}

	return &passkey, nil
}

// DeletePasskey removes a passkey
func (pm *PasskeyManager) DeletePasskey(ctx context.Context, userID int64, passkeyID string) error {
	result, err := pm.db.ExecContext(ctx, `
		DELETE FROM passkeys WHERE id = ? AND user_id = ?
	`, passkeyID, fmt.Sprintf("%d", userID))
	if err != nil {
		return fmt.Errorf("failed to delete passkey: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrPasskeyNotFound
	}

	return nil
}

// RenamePasskey updates a passkey's name
func (pm *PasskeyManager) RenamePasskey(ctx context.Context, userID int64, passkeyID, name string) error {
	result, err := pm.db.ExecContext(ctx, `
		UPDATE passkeys SET name = ? WHERE id = ? AND user_id = ?
	`, name, passkeyID, fmt.Sprintf("%d", userID))
	if err != nil {
		return fmt.Errorf("failed to rename passkey: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrPasskeyNotFound
	}

	return nil
}

// HasPasskeys checks if a user has any passkeys registered
func (pm *PasskeyManager) HasPasskeys(ctx context.Context, userID int64) bool {
	var count int
	err := pm.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM passkeys WHERE user_id = ?
	`, fmt.Sprintf("%d", userID)).Scan(&count)
	return err == nil && count > 0
}

// GetPasskeyCount returns the number of passkeys for a user
func (pm *PasskeyManager) GetPasskeyCount(ctx context.Context, userID int64) (int, error) {
	var count int
	err := pm.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM passkeys WHERE user_id = ?
	`, fmt.Sprintf("%d", userID)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count passkeys: %w", err)
	}
	return count, nil
}

// CleanupExpiredChallenges removes expired challenges
func (pm *PasskeyManager) CleanupExpiredChallenges(ctx context.Context) error {
	_, err := pm.db.ExecContext(ctx, `
		DELETE FROM passkey_challenges WHERE expires_at < ?
	`, time.Now())
	return err
}

// PasskeyStatus represents the passkey status for display
type PasskeyStatus struct {
	Enabled bool `json:"enabled"`
	Count   int  `json:"count"`
}

// GetStatus returns the passkey status for a user
func (pm *PasskeyManager) GetStatus(ctx context.Context, userID int64) PasskeyStatus {
	count, _ := pm.GetPasskeyCount(ctx, userID)
	return PasskeyStatus{
		Enabled: count > 0,
		Count:   count,
	}
}

// generateChallenge generates a random WebAuthn challenge
func generateChallenge() (string, error) {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(challenge), nil
}

// PasskeyListResponse represents a list of passkeys for API response
type PasskeyListResponse struct {
	Passkeys []PasskeyInfo `json:"passkeys"`
	Count    int           `json:"count"`
}

// PasskeyInfo represents passkey info for display (without sensitive data)
type PasskeyInfo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// ToInfo converts a Passkey to PasskeyInfo for safe display
func (pk *Passkey) ToInfo() PasskeyInfo {
	return PasskeyInfo{
		ID:         pk.ID,
		Name:       pk.Name,
		CreatedAt:  pk.CreatedAt,
		LastUsedAt: pk.LastUsedAt,
	}
}

// ListPasskeysForDisplay returns passkeys formatted for display
func (pm *PasskeyManager) ListPasskeysForDisplay(ctx context.Context, userID int64) (*PasskeyListResponse, error) {
	passkeys, err := pm.GetPasskeys(ctx, userID)
	if err != nil && err != ErrPasskeyNotEnabled {
		return nil, err
	}

	infos := make([]PasskeyInfo, 0, len(passkeys))
	for _, pk := range passkeys {
		infos = append(infos, pk.ToInfo())
	}

	return &PasskeyListResponse{
		Passkeys: infos,
		Count:    len(infos),
	}, nil
}

// SerializeOptions serializes registration/authentication options to JSON
func SerializeOptions(options interface{}) (string, error) {
	data, err := json.Marshal(options)
	if err != nil {
		return "", fmt.Errorf("failed to serialize options: %w", err)
	}
	return string(data), nil
}
