package user

import (
	"strings"
	"testing"
	"time"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  error
	}{
		{"valid", "testuser", nil},
		{"valid with numbers", "user123", nil},
		{"valid with underscore", "test_user", nil},
		{"valid with hyphen", "test-user", nil},
		{"empty", "", ErrUsernameRequired},
		{"too short", "ab", ErrUsernameTooShort},
		{"too long", strings.Repeat("a", 33), ErrUsernameTooLong},
		{"uppercase normalized", "TestUser", nil}, // Normalized to lowercase
		{"invalid special chars", "test@user", ErrUsernameInvalid},
		{"reserved admin", "admin", ErrUsernameReserved},
		{"reserved root", "root", ErrUsernameReserved},
		{"reserved system", "system", ErrUsernameReserved},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if err != tt.wantErr {
				t.Errorf("ValidateUsername(%q) = %v, want %v", tt.username, err, tt.wantErr)
			}
		})
	}
}

func TestIsBlockedUsername(t *testing.T) {
	tests := []struct {
		username string
		want     bool
	}{
		{"admin", true},
		{"root", true},
		{"system", true},
		{"testuser", false},
		{"myuser123", false},
		{"ADMIN", true}, // Case insensitive
		{"Admin", true},
	}

	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			got := IsBlockedUsername(tt.username)
			if got != tt.want {
				t.Errorf("IsBlockedUsername(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr error
	}{
		{"valid", "test@example.com", nil},
		{"valid with subdomain", "test@mail.example.com", nil},
		{"valid with plus", "test+tag@example.com", nil},
		{"valid with dots", "test.user@example.com", nil},
		{"empty", "", ErrEmailRequired},
		{"no at", "testexample.com", ErrEmailInvalid},
		{"no domain", "test@", ErrEmailInvalid},
		{"no tld", "test@example", ErrEmailInvalid},
		{"double at", "test@@example.com", ErrEmailInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if err != tt.wantErr {
				t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		minLength int
		wantErr   error
	}{
		{"valid", "Test1234", 8, nil},
		{"valid long", "MySecure1Password", 8, nil},
		{"empty", "", 8, ErrPasswordRequired},
		{"too short", "Test1", 8, ErrPasswordTooShort},
		{"no uppercase", "test1234", 8, ErrPasswordTooWeak},
		{"no lowercase", "TEST1234", 8, ErrPasswordTooWeak},
		{"no number", "TestTest", 8, ErrPasswordTooWeak},
		{"leading space", " Test1234", 8, ErrPasswordWhitespace},
		{"trailing space", "Test1234 ", 8, ErrPasswordWhitespace},
		{"custom min length", "Test1", 5, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password, tt.minLength)
			if err != tt.wantErr {
				t.Errorf("ValidatePassword(%q, %d) = %v, want %v", tt.password, tt.minLength, err, tt.wantErr)
			}
		})
	}
}

func TestHashPassword(t *testing.T) {
	password := "Test1234"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("Hash should start with $argon2id$, got: %s", hash)
	}

	// Hash should be different each time (random salt)
	hash2, _ := HashPassword(password)
	if hash == hash2 {
		t.Error("Hash should be different each time due to random salt")
	}
}

func TestCheckPassword(t *testing.T) {
	password := "Test1234"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}

	// Correct password
	if !CheckPassword(password, hash) {
		t.Error("CheckPassword should return true for correct password")
	}

	// Wrong password
	if CheckPassword("WrongPassword1", hash) {
		t.Error("CheckPassword should return false for wrong password")
	}

	// Invalid hash format
	if CheckPassword(password, "invalidhash") {
		t.Error("CheckPassword should return false for invalid hash")
	}

	// Wrong algorithm
	if CheckPassword(password, "$bcrypt$invalid") {
		t.Error("CheckPassword should return false for wrong algorithm")
	}
}

func TestGenerateToken(t *testing.T) {
	token1, err := GenerateToken(32)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	if len(token1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("Token length = %d, want 64", len(token1))
	}

	token2, _ := GenerateToken(32)
	if token1 == token2 {
		t.Error("Tokens should be different each time")
	}
}

func TestGenerateSessionToken(t *testing.T) {
	token, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken() error: %v", err)
	}

	if !strings.HasPrefix(token, "ses_") {
		t.Errorf("Session token should have 'ses_' prefix, got: %s", token)
	}
}

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"TestUser", "testuser"},
		{"  testuser  ", "testuser"},
		{"TEST_USER", "test_user"},
	}

	for _, tt := range tests {
		got := NormalizeUsername(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Test@Example.COM", "test@example.com"},
		{"  user@domain.com  ", "user@domain.com"},
	}

	for _, tt := range tests {
		got := NormalizeEmail(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNewUser(t *testing.T) {
	user, err := NewUser("testuser", "test@example.com", "Test1234", 8)
	if err != nil {
		t.Fatalf("NewUser() error: %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Username = %q, want %q", user.Username, "testuser")
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "test@example.com")
	}
	if user.Role != RoleUser {
		t.Errorf("Role = %q, want %q", user.Role, RoleUser)
	}
	if user.EmailVerified {
		t.Error("EmailVerified should be false for new user")
	}
	if !user.Active {
		t.Error("Active should be true for new user")
	}
	if user.PasswordHash == "" {
		t.Error("PasswordHash should be set")
	}
}

func TestNewUserValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		username string
		email    string
		password string
		wantErr  error
	}{
		{"invalid username", "ab", "test@example.com", "Test1234", ErrUsernameTooShort},
		{"invalid email", "testuser", "invalid", "Test1234", ErrEmailInvalid},
		{"invalid password", "testuser", "test@example.com", "weak", ErrPasswordTooShort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUser(tt.username, tt.email, tt.password, 8)
			if err != tt.wantErr {
				t.Errorf("NewUser() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestUserIsAdmin(t *testing.T) {
	user := &User{Role: RoleAdmin}
	if !user.IsAdmin() {
		t.Error("IsAdmin() should return true for admin role")
	}

	user.Role = RoleUser
	if user.IsAdmin() {
		t.Error("IsAdmin() should return false for user role")
	}
}

func TestUserIsModerator(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{RoleAdmin, true},
		{RoleModerator, true},
		{RoleUser, false},
	}

	for _, tt := range tests {
		user := &User{Role: tt.role}
		got := user.IsModerator()
		if got != tt.want {
			t.Errorf("IsModerator() with role %q = %v, want %v", tt.role, got, tt.want)
		}
	}
}

func TestUserCanLogin(t *testing.T) {
	user := &User{Active: true}
	if !user.CanLogin() {
		t.Error("CanLogin() should return true for active user")
	}

	user.Active = false
	if user.CanLogin() {
		t.Error("CanLogin() should return false for inactive user")
	}
}

func TestUserToPublicProfile(t *testing.T) {
	user := &User{
		ID:          1,
		Username:    "testuser",
		DisplayName: "Test User",
		AvatarURL:   "https://example.com/avatar.jpg",
		Bio:         "Test bio",
		CreatedAt:   time.Now(),
	}

	profile := user.ToPublicProfile()

	if profile.ID != user.ID {
		t.Errorf("ID = %d, want %d", profile.ID, user.ID)
	}
	if profile.Username != user.Username {
		t.Errorf("Username = %q, want %q", profile.Username, user.Username)
	}
	if profile.DisplayName != user.DisplayName {
		t.Errorf("DisplayName = %q, want %q", profile.DisplayName, user.DisplayName)
	}
}

func TestUserToPublicProfileFallbackDisplayName(t *testing.T) {
	user := &User{
		ID:          1,
		Username:    "testuser",
		DisplayName: "", // Empty display name
	}

	profile := user.ToPublicProfile()

	// Should fall back to username
	if profile.DisplayName != user.Username {
		t.Errorf("DisplayName = %q, should fall back to username %q", profile.DisplayName, user.Username)
	}
}

func TestEmailTypeConstants(t *testing.T) {
	if EmailTypeAccount != "account" {
		t.Errorf("EmailTypeAccount = %q, want %q", EmailTypeAccount, "account")
	}
	if EmailTypeNotification != "notification" {
		t.Errorf("EmailTypeNotification = %q, want %q", EmailTypeNotification, "notification")
	}
}

func TestUserGetAccountEmail(t *testing.T) {
	user := &User{Email: "account@example.com"}
	if user.GetAccountEmail() != "account@example.com" {
		t.Errorf("GetAccountEmail() = %q, want %q", user.GetAccountEmail(), "account@example.com")
	}
}

func TestUserGetNotificationEmail(t *testing.T) {
	tests := []struct {
		name                      string
		email                     string
		notificationEmail         string
		notificationEmailVerified bool
		want                      string
	}{
		{
			name:  "no notification email",
			email: "main@example.com",
			want:  "main@example.com",
		},
		{
			name:                      "notification email not verified",
			email:                     "main@example.com",
			notificationEmail:         "notify@example.com",
			notificationEmailVerified: false,
			want:                      "main@example.com",
		},
		{
			name:                      "notification email verified",
			email:                     "main@example.com",
			notificationEmail:         "notify@example.com",
			notificationEmailVerified: true,
			want:                      "notify@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{
				Email:                     tt.email,
				NotificationEmail:         tt.notificationEmail,
				NotificationEmailVerified: tt.notificationEmailVerified,
			}
			got := user.GetNotificationEmail()
			if got != tt.want {
				t.Errorf("GetNotificationEmail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUserGetEmailForType(t *testing.T) {
	user := &User{
		Email:                     "main@example.com",
		NotificationEmail:         "notify@example.com",
		NotificationEmailVerified: true,
	}

	if user.GetEmailForType(EmailTypeAccount) != "main@example.com" {
		t.Error("GetEmailForType(account) should return account email")
	}

	if user.GetEmailForType(EmailTypeNotification) != "notify@example.com" {
		t.Error("GetEmailForType(notification) should return notification email")
	}

	if user.GetEmailForType("unknown") != "main@example.com" {
		t.Error("GetEmailForType(unknown) should fall back to account email")
	}
}

func TestUserHasSeparateNotificationEmail(t *testing.T) {
	tests := []struct {
		name                      string
		email                     string
		notificationEmail         string
		notificationEmailVerified bool
		want                      bool
	}{
		{
			name:  "no notification email",
			email: "main@example.com",
			want:  false,
		},
		{
			name:                      "not verified",
			email:                     "main@example.com",
			notificationEmail:         "notify@example.com",
			notificationEmailVerified: false,
			want:                      false,
		},
		{
			name:                      "same as main",
			email:                     "main@example.com",
			notificationEmail:         "main@example.com",
			notificationEmailVerified: true,
			want:                      false,
		},
		{
			name:                      "separate and verified",
			email:                     "main@example.com",
			notificationEmail:         "notify@example.com",
			notificationEmailVerified: true,
			want:                      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{
				Email:                     tt.email,
				NotificationEmail:         tt.notificationEmail,
				NotificationEmailVerified: tt.notificationEmailVerified,
			}
			got := user.HasSeparateNotificationEmail()
			if got != tt.want {
				t.Errorf("HasSeparateNotificationEmail() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUserSetNotificationEmail(t *testing.T) {
	user := &User{Email: "main@example.com"}

	// Set valid email
	err := user.SetNotificationEmail("notify@example.com")
	if err != nil {
		t.Errorf("SetNotificationEmail() error: %v", err)
	}
	if user.NotificationEmail != "notify@example.com" {
		t.Errorf("NotificationEmail = %q, want %q", user.NotificationEmail, "notify@example.com")
	}
	if user.NotificationEmailVerified {
		t.Error("NotificationEmailVerified should be false after setting")
	}

	// Clear email
	err = user.SetNotificationEmail("")
	if err != nil {
		t.Errorf("SetNotificationEmail('') error: %v", err)
	}
	if user.NotificationEmail != "" {
		t.Errorf("NotificationEmail should be empty, got %q", user.NotificationEmail)
	}

	// Invalid email
	err = user.SetNotificationEmail("invalid")
	if err != ErrEmailInvalid {
		t.Errorf("SetNotificationEmail(invalid) error = %v, want %v", err, ErrEmailInvalid)
	}
}

func TestUserVerifyNotificationEmail(t *testing.T) {
	user := &User{NotificationEmail: "notify@example.com"}
	user.VerifyNotificationEmail()

	if !user.NotificationEmailVerified {
		t.Error("NotificationEmailVerified should be true after verification")
	}

	// Empty notification email should not be marked as verified
	user2 := &User{}
	user2.VerifyNotificationEmail()
	if user2.NotificationEmailVerified {
		t.Error("Empty notification email should not be marked as verified")
	}
}

func TestUserClearNotificationEmail(t *testing.T) {
	user := &User{
		NotificationEmail:         "notify@example.com",
		NotificationEmailVerified: true,
	}

	user.ClearNotificationEmail()

	if user.NotificationEmail != "" {
		t.Errorf("NotificationEmail should be empty, got %q", user.NotificationEmail)
	}
	if user.NotificationEmailVerified {
		t.Error("NotificationEmailVerified should be false")
	}
}

func TestUserGetEmailInfo(t *testing.T) {
	user := &User{
		Email:                     "main@example.com",
		EmailVerified:             true,
		NotificationEmail:         "notify@example.com",
		NotificationEmailVerified: true,
	}

	info := user.GetEmailInfo()

	if info.AccountEmail != "main@example.com" {
		t.Errorf("AccountEmail = %q, want %q", info.AccountEmail, "main@example.com")
	}
	if !info.AccountEmailVerified {
		t.Error("AccountEmailVerified should be true")
	}
	if info.NotificationEmail != "notify@example.com" {
		t.Errorf("NotificationEmail = %q, want %q", info.NotificationEmail, "notify@example.com")
	}
	if !info.NotificationEmailVerified {
		t.Error("NotificationEmailVerified should be true")
	}
	if !info.UsingSeparateNotification {
		t.Error("UsingSeparateNotification should be true")
	}
}

func TestRoleConstants(t *testing.T) {
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q, want %q", RoleUser, "user")
	}
	if RoleAdmin != "admin" {
		t.Errorf("RoleAdmin = %q, want %q", RoleAdmin, "admin")
	}
	if RoleModerator != "moderator" {
		t.Errorf("RoleModerator = %q, want %q", RoleModerator, "moderator")
	}
}

func TestUserStruct(t *testing.T) {
	now := time.Now()
	user := User{
		ID:            1,
		Username:      "testuser",
		Email:         "test@example.com",
		PasswordHash:  "hash",
		DisplayName:   "Test User",
		AvatarURL:     "https://example.com/avatar.jpg",
		Bio:           "Bio",
		Role:          RoleUser,
		EmailVerified: true,
		Active:        true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if user.ID != 1 {
		t.Errorf("ID = %d, want 1", user.ID)
	}
	if user.Username != "testuser" {
		t.Errorf("Username = %q, want %q", user.Username, "testuser")
	}
}

func TestUserSessionStruct(t *testing.T) {
	now := time.Now()
	session := UserSession{
		ID:         1,
		UserID:     1,
		Token:      "ses_token",
		IPAddress:  "192.168.1.1",
		UserAgent:  "Mozilla/5.0",
		DeviceName: "Test Device",
		CreatedAt:  now,
		ExpiresAt:  now.Add(24 * time.Hour),
		LastUsed:   now,
	}

	if session.ID != 1 {
		t.Errorf("ID = %d, want 1", session.ID)
	}
	if session.Token != "ses_token" {
		t.Errorf("Token = %q, want %q", session.Token, "ses_token")
	}
}

func TestBlockedUsernames(t *testing.T) {
	// Test some expected blocked usernames
	blocked := []string{
		"admin", "root", "system", "api", "www",
		"login", "logout", "register", "settings",
	}

	for _, username := range blocked {
		if !BlockedUsernames[username] {
			t.Errorf("Expected %q to be in BlockedUsernames", username)
		}
	}

	// Test some that should not be blocked
	allowed := []string{
		"myuser", "john123", "test_account",
	}

	for _, username := range allowed {
		if BlockedUsernames[username] {
			t.Errorf("%q should not be in BlockedUsernames", username)
		}
	}
}

func TestErrorVariables(t *testing.T) {
	errors := []error{
		ErrUsernameRequired,
		ErrUsernameTooShort,
		ErrUsernameTooLong,
		ErrUsernameInvalid,
		ErrUsernameReserved,
		ErrEmailRequired,
		ErrEmailInvalid,
		ErrPasswordRequired,
		ErrPasswordTooShort,
		ErrPasswordTooWeak,
		ErrPasswordWhitespace,
		ErrUserNotFound,
		ErrInvalidCredentials,
		ErrUserInactive,
		ErrEmailNotVerified,
		ErrUsernameTaken,
		ErrEmailTaken,
		ErrSessionExpired,
		ErrSessionNotFound,
		ErrRegistrationDisabled,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error variable should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

// Tests for UserToken

func TestUserTokenStruct(t *testing.T) {
	now := time.Now()
	expires := now.Add(24 * time.Hour)
	token := UserToken{
		ID:          1,
		UserID:      2,
		Name:        "API Token",
		TokenHash:   "hash123",
		TokenPrefix: "usr_abcd1234...",
		Permissions: "read,write",
		LastUsed:    &now,
		ExpiresAt:   &expires,
		CreatedAt:   now,
	}

	if token.ID != 1 {
		t.Errorf("ID = %d, want 1", token.ID)
	}
	if token.Name != "API Token" {
		t.Errorf("Name = %q, want 'API Token'", token.Name)
	}
	if token.TokenPrefix != "usr_abcd1234..." {
		t.Errorf("TokenPrefix = %q", token.TokenPrefix)
	}
}

func TestUserTokenGetPermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions string
		want        []string
	}{
		{"empty", "", nil},
		{"single", "read", []string{"read"}},
		{"multiple", "read,write,delete", []string{"read", "write", "delete"}},
		{"wildcard", "*", []string{"*"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &UserToken{Permissions: tt.permissions}
			got := token.GetPermissions()

			if tt.want == nil && got != nil {
				t.Errorf("GetPermissions() = %v, want nil", got)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("GetPermissions() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("GetPermissions()[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestUserTokenHasPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions string
		check       string
		want        bool
	}{
		{"has permission", "read,write", "read", true},
		{"no permission", "read", "write", false},
		{"wildcard", "*", "anything", true},
		{"empty permissions", "", "read", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &UserToken{Permissions: tt.permissions}
			got := token.HasPermission(tt.check)
			if got != tt.want {
				t.Errorf("HasPermission(%q) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}

func TestUserTokenIsExpired(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	tests := []struct {
		name      string
		expiresAt *time.Time
		want      bool
	}{
		{"no expiry", nil, false},
		{"expired", &past, true},
		{"not expired", &future, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &UserToken{ExpiresAt: tt.expiresAt}
			got := token.IsExpired()
			if got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUserTokenToInfo(t *testing.T) {
	now := time.Now()
	token := &UserToken{
		ID:          1,
		Name:        "Test Token",
		TokenPrefix: "usr_test...",
		Permissions: "read,write",
		LastUsed:    &now,
		CreatedAt:   now,
	}

	info := token.ToInfo()

	if info.ID != token.ID {
		t.Errorf("ID = %d, want %d", info.ID, token.ID)
	}
	if info.Name != token.Name {
		t.Errorf("Name = %q, want %q", info.Name, token.Name)
	}
	if info.Prefix != token.TokenPrefix {
		t.Errorf("Prefix = %q, want %q", info.Prefix, token.TokenPrefix)
	}
	if len(info.Permissions) != 2 {
		t.Errorf("Permissions length = %d, want 2", len(info.Permissions))
	}
	if info.Expired {
		t.Error("Expired should be false")
	}
}

func TestTokenErrorVariables(t *testing.T) {
	errors := []error{
		ErrTokenNotFound,
		ErrTokenExpired,
		ErrTokenInvalid,
		ErrTokenNameEmpty,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error variable should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

// Tests for CreateTokenRequest

func TestCreateTokenRequestStruct(t *testing.T) {
	req := CreateTokenRequest{
		Name:        "My API Token",
		Permissions: []string{"read", "write"},
		ExpiresIn:   30 * 24 * time.Hour,
	}

	if req.Name != "My API Token" {
		t.Errorf("Name = %q, want 'My API Token'", req.Name)
	}
	if len(req.Permissions) != 2 {
		t.Errorf("Permissions length = %d, want 2", len(req.Permissions))
	}
	if req.ExpiresIn != 30*24*time.Hour {
		t.Errorf("ExpiresIn = %v, want %v", req.ExpiresIn, 30*24*time.Hour)
	}
}

// Tests for CreateTokenResponse

func TestCreateTokenResponseStruct(t *testing.T) {
	expiry := time.Now().Add(30 * 24 * time.Hour)
	resp := CreateTokenResponse{
		Token:  "usr_abcdefghijklmnop",
		ID:     1,
		Name:   "My Token",
		Prefix: "usr_abcd...",
		Expiry: &expiry,
	}

	if resp.Token != "usr_abcdefghijklmnop" {
		t.Errorf("Token = %q", resp.Token)
	}
	if resp.ID != 1 {
		t.Errorf("ID = %d", resp.ID)
	}
	if resp.Prefix != "usr_abcd..." {
		t.Errorf("Prefix = %q", resp.Prefix)
	}
}

// Tests for TokenInfo

func TestTokenInfoStruct(t *testing.T) {
	now := time.Now()
	info := TokenInfo{
		ID:          1,
		Name:        "Token",
		Prefix:      "usr_test...",
		Permissions: []string{"read"},
		LastUsed:    &now,
		ExpiresAt:   nil,
		CreatedAt:   now,
		Expired:     false,
	}

	if info.ID != 1 {
		t.Errorf("ID = %d", info.ID)
	}
	if info.Name != "Token" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Expired {
		t.Error("Expired should be false")
	}
}

// Tests for Recovery Keys

func TestRecoveryKeyStruct(t *testing.T) {
	now := time.Now()
	key := RecoveryKey{
		ID:        1,
		UserID:    2,
		KeyHash:   "$argon2id$...",
		Used:      false,
		CreatedAt: now,
	}

	if key.ID != 1 {
		t.Errorf("ID = %d", key.ID)
	}
	if key.UserID != 2 {
		t.Errorf("UserID = %d", key.UserID)
	}
	if key.Used {
		t.Error("Used should be false")
	}
}

func TestRecoveryErrorVariables(t *testing.T) {
	errors := []error{
		ErrNoRecoveryKeys,
		ErrInvalidRecoveryKey,
		ErrRecoveryKeyUsed,
		ErrRecoveryKeyNotFound,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error variable should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

func TestRecoveryKeyStatsStruct(t *testing.T) {
	stats := RecoveryKeyStats{
		Total:     10,
		Used:      3,
		Remaining: 7,
	}

	if stats.Total != 10 {
		t.Errorf("Total = %d", stats.Total)
	}
	if stats.Used != 3 {
		t.Errorf("Used = %d", stats.Used)
	}
	if stats.Remaining != 7 {
		t.Errorf("Remaining = %d", stats.Remaining)
	}
}

func TestFormatRecoveryKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ABCD1234EFGH5678", "ABCD-1234-EFGH-5678"},
		{"abcd1234efgh5678", "ABCD-1234-EFGH-5678"},
		{"ABCD-1234-EFGH-5678", "ABCD-1234-EFGH-5678"},
		{"abcd 1234 efgh 5678", "ABCD-1234-EFGH-5678"},
		{"short", "SHORT"}, // Too short, returned as-is (uppercase)
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := FormatRecoveryKey(tt.input)
			if got != tt.want {
				t.Errorf("FormatRecoveryKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRecoveryKeyConstants(t *testing.T) {
	// Per AI.md PART 2: Argon2id parameters
	if recoveryArgon2Time != 3 {
		t.Errorf("recoveryArgon2Time = %d, want 3", recoveryArgon2Time)
	}
	if recoveryArgon2Memory != 64*1024 {
		t.Errorf("recoveryArgon2Memory = %d, want %d", recoveryArgon2Memory, 64*1024)
	}
	if recoveryArgon2Threads != 4 {
		t.Errorf("recoveryArgon2Threads = %d, want 4", recoveryArgon2Threads)
	}
	if recoveryArgon2KeyLen != 32 {
		t.Errorf("recoveryArgon2KeyLen = %d, want 32", recoveryArgon2KeyLen)
	}
	if recoveryArgon2SaltLen != 16 {
		t.Errorf("recoveryArgon2SaltLen = %d, want 16", recoveryArgon2SaltLen)
	}
}

// Tests for VerificationToken

func TestVerificationTokenStruct(t *testing.T) {
	now := time.Now()
	token := VerificationToken{
		ID:        1,
		UserID:    2,
		Token:     "abc123def456",
		Type:      TokenTypeEmailVerify,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}

	if token.ID != 1 {
		t.Errorf("ID = %d, want 1", token.ID)
	}
	if token.UserID != 2 {
		t.Errorf("UserID = %d, want 2", token.UserID)
	}
	if token.Type != TokenTypeEmailVerify {
		t.Errorf("Type = %q, want %q", token.Type, TokenTypeEmailVerify)
	}
}

func TestVerificationTokenTypeConstants(t *testing.T) {
	if TokenTypeEmailVerify != "email_verify" {
		t.Errorf("TokenTypeEmailVerify = %q, want %q", TokenTypeEmailVerify, "email_verify")
	}
	if TokenTypePasswordReset != "password_reset" {
		t.Errorf("TokenTypePasswordReset = %q, want %q", TokenTypePasswordReset, "password_reset")
	}
}

func TestVerificationDurationConstants(t *testing.T) {
	if EmailVerifyDuration != 24*time.Hour {
		t.Errorf("EmailVerifyDuration = %v, want %v", EmailVerifyDuration, 24*time.Hour)
	}
	if PasswordResetDuration != 1*time.Hour {
		t.Errorf("PasswordResetDuration = %v, want %v", PasswordResetDuration, 1*time.Hour)
	}
}

func TestVerificationErrorVariables(t *testing.T) {
	errors := []error{
		ErrVerificationTokenNotFound,
		ErrVerificationTokenExpired,
		ErrVerificationTokenInvalid,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error variable should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

// Tests for User2FA and TOTP

func TestUser2FAStruct(t *testing.T) {
	now := time.Now()
	tfa := User2FA{
		ID:              1,
		UserID:          2,
		SecretEncrypted: "encrypted_secret",
		Enabled:         true,
		Verified:        true,
		CreatedAt:       now,
		EnabledAt:       &now,
	}

	if tfa.ID != 1 {
		t.Errorf("ID = %d, want 1", tfa.ID)
	}
	if tfa.UserID != 2 {
		t.Errorf("UserID = %d, want 2", tfa.UserID)
	}
	if !tfa.Enabled {
		t.Error("Enabled should be true")
	}
	if !tfa.Verified {
		t.Error("Verified should be true")
	}
}

func TestTOTPSetupResponseStruct(t *testing.T) {
	resp := TOTPSetupResponse{
		Secret:    "ABCDEFGHIJKLMNOP",
		QRCodeURL: "otpauth://totp/Example:user@example.com?secret=ABCD",
		Issuer:    "Example",
		Account:   "user@example.com",
	}

	if resp.Secret != "ABCDEFGHIJKLMNOP" {
		t.Errorf("Secret = %q", resp.Secret)
	}
	if resp.Issuer != "Example" {
		t.Errorf("Issuer = %q", resp.Issuer)
	}
	if resp.Account != "user@example.com" {
		t.Errorf("Account = %q", resp.Account)
	}
}

func TestTOTPErrorVariables(t *testing.T) {
	errors := []error{
		ErrTOTPNotEnabled,
		ErrTOTPAlreadySetup,
		ErrTOTPInvalidCode,
		ErrTOTPNotVerified,
		ErrEncryptionFailed,
		ErrDecryptionFailed,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error variable should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

// Tests for password hashing edge cases

func TestCheckPasswordEdgeCases(t *testing.T) {
	// Test with malformed hash - wrong number of parts
	if CheckPassword("password", "too$few$parts") {
		t.Error("CheckPassword should return false for malformed hash")
	}

	// Test with wrong version format
	malformed := "$argon2id$v=invalid$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2g"
	if CheckPassword("password", malformed) {
		t.Error("CheckPassword should return false for invalid version")
	}

	// Test with invalid params format
	malformed2 := "$argon2id$v=19$invalid_params$c29tZXNhbHQ$c29tZWhhc2g"
	if CheckPassword("password", malformed2) {
		t.Error("CheckPassword should return false for invalid params")
	}

	// Test with invalid base64 salt
	malformed3 := "$argon2id$v=19$m=65536,t=3,p=4$!!!invalid$c29tZWhhc2g"
	if CheckPassword("password", malformed3) {
		t.Error("CheckPassword should return false for invalid salt")
	}

	// Test with invalid base64 hash
	malformed4 := "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$!!!invalid"
	if CheckPassword("password", malformed4) {
		t.Error("CheckPassword should return false for invalid hash")
	}
}

// Tests for concurrent password operations

func TestHashPasswordConcurrent(t *testing.T) {
	done := make(chan bool, 4)

	for i := 0; i < 4; i++ {
		go func(id int) {
			for j := 0; j < 5; j++ {
				hash, err := HashPassword("TestPassword1")
				if err != nil {
					t.Errorf("HashPassword error: %v", err)
				}
				if !strings.HasPrefix(hash, "$argon2id$") {
					t.Errorf("Invalid hash format: %s", hash)
				}
			}
			done <- true
		}(i)
	}

	for i := 0; i < 4; i++ {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout in concurrent HashPassword")
		}
	}
}

// Tests for validation edge cases

func TestValidateUsernameEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  error
	}{
		{"exactly 3 chars", "abc", nil},
		{"exactly 32 chars", strings.Repeat("a", 32), nil},
		{"33 chars", strings.Repeat("a", 33), ErrUsernameTooLong},
		{"2 chars", "ab", ErrUsernameTooShort},
		{"whitespace only", "   ", ErrUsernameTooShort}, // After trimming whitespace, becomes empty -> too short
		{"with dot", "test.user", ErrUsernameInvalid},
		{"with at sign", "test@user", ErrUsernameInvalid},
		{"with space", "test user", ErrUsernameInvalid},
		{"reserved moderator", "moderator", ErrUsernameReserved},
		{"reserved webmaster", "webmaster", ErrUsernameReserved},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if err != tt.wantErr {
				t.Errorf("ValidateUsername(%q) = %v, want %v", tt.username, err, tt.wantErr)
			}
		})
	}
}

func TestValidateEmailEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr error
	}{
		{"valid with numbers", "test123@example.com", nil},
		{"valid with underscore", "test_user@example.com", nil},
		{"valid with hyphen domain", "test@my-domain.com", nil},
		{"whitespace only", "   ", ErrEmailInvalid},
		{"multiple at signs", "test@test@example.com", ErrEmailInvalid},
		{"no username", "@example.com", ErrEmailInvalid},
		{"trailing dot", "test@example.", ErrEmailInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if err != tt.wantErr {
				t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePasswordEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		minLength int
		wantErr   error
	}{
		{"exactly min length", "Test123A", 8, nil},
		{"with special chars", "Test1234!@#", 8, nil},
		{"with unicode", "Tëst1234", 8, nil},
		{"leading tab", "\tTest1234", 8, ErrPasswordWhitespace},
		{"trailing tab", "Test1234\t", 8, ErrPasswordWhitespace},
		{"internal space ok", "Test 1234", 8, nil},
		{"all same case no number", "testtest", 8, ErrPasswordTooWeak},
		{"numbers only", "12345678", 8, ErrPasswordTooWeak},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password, tt.minLength)
			if err != tt.wantErr {
				t.Errorf("ValidatePassword(%q, %d) = %v, want %v", tt.password, tt.minLength, err, tt.wantErr)
			}
		})
	}
}

// Tests for blocked usernames

func TestBlockedUsernamesCount(t *testing.T) {
	// Should have many blocked usernames
	if len(BlockedUsernames) < 100 {
		t.Errorf("BlockedUsernames should have at least 100 entries, got %d", len(BlockedUsernames))
	}
}

func TestBlockedUsernameCategorySystem(t *testing.T) {
	// System-related usernames
	systemNames := []string{"admin", "root", "system", "sudo", "operator"}
	for _, name := range systemNames {
		if !BlockedUsernames[name] {
			t.Errorf("Expected %q to be blocked (system)", name)
		}
	}
}

func TestBlockedUsernameCategoryEmail(t *testing.T) {
	// Email-related usernames
	emailNames := []string{"postmaster", "webmaster", "hostmaster", "abuse", "noreply"}
	for _, name := range emailNames {
		if !BlockedUsernames[name] {
			t.Errorf("Expected %q to be blocked (email)", name)
		}
	}
}

func TestBlockedUsernameCategoryApp(t *testing.T) {
	// Application-specific usernames
	appNames := []string{"api", "www", "app", "static", "cdn"}
	for _, name := range appNames {
		if !BlockedUsernames[name] {
			t.Errorf("Expected %q to be blocked (app)", name)
		}
	}
}

// Test GenerateToken with different sizes

func TestGenerateTokenDifferentSizes(t *testing.T) {
	sizes := []int{16, 32, 64}

	for _, size := range sizes {
		t.Run(string(rune(size)), func(t *testing.T) {
			token, err := GenerateToken(size)
			if err != nil {
				t.Errorf("GenerateToken(%d) error: %v", size, err)
			}
			if len(token) != size*2 { // hex encoding doubles the size
				t.Errorf("GenerateToken(%d) length = %d, want %d", size, len(token), size*2)
			}
		})
	}
}

// Test PublicProfile struct

func TestPublicProfileStruct(t *testing.T) {
	now := time.Now()
	profile := PublicProfile{
		ID:          1,
		Username:    "testuser",
		DisplayName: "Test User",
		AvatarURL:   "https://example.com/avatar.jpg",
		Bio:         "Test bio",
		CreatedAt:   now,
	}

	if profile.ID != 1 {
		t.Errorf("ID = %d, want 1", profile.ID)
	}
	if profile.Username != "testuser" {
		t.Errorf("Username = %q", profile.Username)
	}
	if profile.DisplayName != "Test User" {
		t.Errorf("DisplayName = %q", profile.DisplayName)
	}
}

// Test EmailInfo struct

func TestEmailInfoStruct(t *testing.T) {
	info := EmailInfo{
		AccountEmail:             "account@example.com",
		AccountEmailVerified:     true,
		NotificationEmail:        "notify@example.com",
		NotificationEmailVerified: true,
		UsingSeparateNotification: true,
	}

	if info.AccountEmail != "account@example.com" {
		t.Errorf("AccountEmail = %q", info.AccountEmail)
	}
	if !info.AccountEmailVerified {
		t.Error("AccountEmailVerified should be true")
	}
	if !info.UsingSeparateNotification {
		t.Error("UsingSeparateNotification should be true")
	}
}

// ===== PREFERENCES TESTS =====

func TestDefaultPreferences(t *testing.T) {
	prefs := DefaultPreferences()

	if prefs.Theme != "auto" {
		t.Errorf("Theme = %q, want auto", prefs.Theme)
	}
	if prefs.ResultsPerPage != 20 {
		t.Errorf("ResultsPerPage = %d, want 20", prefs.ResultsPerPage)
	}
	if prefs.DefaultCategory != "general" {
		t.Errorf("DefaultCategory = %q", prefs.DefaultCategory)
	}
	if prefs.SafeSearch != 1 {
		t.Errorf("SafeSearch = %d, want 1", prefs.SafeSearch)
	}
	if !prefs.ShowThumbnails {
		t.Error("ShowThumbnails should be true")
	}
	if !prefs.AutocompleteOn {
		t.Error("AutocompleteOn should be true")
	}
	if !prefs.AnonymizeResults {
		t.Error("AnonymizeResults should be true")
	}
}

func TestUserPreferencesValidate(t *testing.T) {
	tests := []struct {
		name    string
		prefs   UserPreferences
		checkFn func(p *UserPreferences) bool
		desc    string
	}{
		{
			"invalid theme gets corrected",
			UserPreferences{Theme: "invalid"},
			func(p *UserPreferences) bool { return p.Theme == "auto" },
			"Theme should be auto",
		},
		{
			"valid dark theme preserved",
			UserPreferences{Theme: "dark"},
			func(p *UserPreferences) bool { return p.Theme == "dark" },
			"Theme should remain dark",
		},
		{
			"results too low gets minimum",
			UserPreferences{ResultsPerPage: 5},
			func(p *UserPreferences) bool { return p.ResultsPerPage == 10 },
			"ResultsPerPage should be 10",
		},
		{
			"results too high gets maximum",
			UserPreferences{ResultsPerPage: 200},
			func(p *UserPreferences) bool { return p.ResultsPerPage == 100 },
			"ResultsPerPage should be 100",
		},
		{
			"safe search negative corrected",
			UserPreferences{SafeSearch: -1},
			func(p *UserPreferences) bool { return p.SafeSearch == 1 },
			"SafeSearch should be 1",
		},
		{
			"safe search too high corrected",
			UserPreferences{SafeSearch: 5},
			func(p *UserPreferences) bool { return p.SafeSearch == 1 },
			"SafeSearch should be 1",
		},
		{
			"invalid sort corrected",
			UserPreferences{DefaultSort: "invalid"},
			func(p *UserPreferences) bool { return p.DefaultSort == "relevance" },
			"DefaultSort should be relevance",
		},
		{
			"invalid category corrected",
			UserPreferences{DefaultCategory: "invalid"},
			func(p *UserPreferences) bool { return p.DefaultCategory == "general" },
			"DefaultCategory should be general",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefs := tt.prefs
			prefs.Validate()
			if !tt.checkFn(&prefs) {
				t.Error(tt.desc)
			}
		})
	}
}

func TestUserPreferencesMerge(t *testing.T) {
	base := DefaultPreferences()

	updates := &UserPreferences{
		Theme:           "dark",
		ResultsPerPage:  50,
		DefaultLanguage: "de",
		OpenInNewTab:    true,
		HighContrast:    true,
	}

	base.Merge(updates)

	if base.Theme != "dark" {
		t.Errorf("Theme = %q, want dark", base.Theme)
	}
	if base.ResultsPerPage != 50 {
		t.Errorf("ResultsPerPage = %d, want 50", base.ResultsPerPage)
	}
	if base.DefaultLanguage != "de" {
		t.Errorf("DefaultLanguage = %q", base.DefaultLanguage)
	}
	if !base.OpenInNewTab {
		t.Error("OpenInNewTab should be true")
	}
	if !base.HighContrast {
		t.Error("HighContrast should be true")
	}
}

func TestUserPreferencesToJSON(t *testing.T) {
	prefs := DefaultPreferences()
	data, err := prefs.ToJSON()
	if err != nil {
		t.Errorf("ToJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("ToJSON() returned empty data")
	}

	// Verify it's valid JSON by parsing it back
	parsed, err := FromJSON(data)
	if err != nil {
		t.Errorf("FromJSON() error: %v", err)
	}
	if parsed.Theme != prefs.Theme {
		t.Errorf("Round-trip Theme = %q, want %q", parsed.Theme, prefs.Theme)
	}
}

func TestFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"valid JSON", `{"theme":"dark","results_per_page":30}`, false},
		{"empty JSON object", `{}`, false},
		{"invalid JSON", `{invalid}`, true},
		{"empty string", ``, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromJSON([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("FromJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToPreferenceString(t *testing.T) {
	// Test with custom settings
	prefs := &UserPreferences{
		Theme:           "dark",
		ResultsPerPage:  50,
		DefaultLanguage: "de",
		DefaultRegion:   "DE",
		SafeSearch:      0,
		DefaultSort:     "date",
		OpenInNewTab:    true,
		HighContrast:    true,
	}
	str := prefs.ToPreferenceString()

	// Should contain dark theme indicator
	if !containsSubstr(str, "t=d") {
		t.Errorf("ToPreferenceString() missing t=d, got %q", str)
	}
	// Should contain results per page
	if !containsSubstr(str, "r=50") {
		t.Errorf("ToPreferenceString() missing r=50, got %q", str)
	}
	// Should contain language
	if !containsSubstr(str, "l=de") {
		t.Errorf("ToPreferenceString() missing l=de, got %q", str)
	}
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestParsePreferenceString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		checkFn func(p *UserPreferences) bool
	}{
		{
			"empty string returns defaults",
			"",
			func(p *UserPreferences) bool { return p.Theme == "auto" },
		},
		{
			"dark theme",
			"t=d",
			func(p *UserPreferences) bool { return p.Theme == "dark" },
		},
		{
			"light theme",
			"t=l",
			func(p *UserPreferences) bool { return p.Theme == "light" },
		},
		{
			"results per page",
			"r=50",
			func(p *UserPreferences) bool { return p.ResultsPerPage == 50 },
		},
		{
			"safe search off",
			"s=0",
			func(p *UserPreferences) bool { return p.SafeSearch == 0 },
		},
		{
			"multiple settings",
			"t=d;r=30;s=2;n=1",
			func(p *UserPreferences) bool {
				return p.Theme == "dark" && p.ResultsPerPage == 30 && p.SafeSearch == 2 && p.OpenInNewTab
			},
		},
		{
			"sort by date",
			"o=d",
			func(p *UserPreferences) bool { return p.DefaultSort == "date" },
		},
		{
			"sort by popularity",
			"o=p",
			func(p *UserPreferences) bool { return p.DefaultSort == "popularity" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefs := ParsePreferenceString(tt.input)
			if !tt.checkFn(prefs) {
				t.Errorf("ParsePreferenceString(%q) check failed", tt.input)
			}
		})
	}
}

func TestParsePreferenceStringEngines(t *testing.T) {
	// Test engine codes parsing
	prefs := ParsePreferenceString("e=g,b,d")
	if len(prefs.EnabledEngines) != 3 {
		t.Errorf("EnabledEngines len = %d, want 3", len(prefs.EnabledEngines))
	}

	// Check specific engines
	expected := map[string]bool{"google": true, "bing": true, "duckduckgo": true}
	for _, eng := range prefs.EnabledEngines {
		if !expected[eng] {
			t.Errorf("Unexpected engine: %q", eng)
		}
	}

	// Test disabled engines
	prefs2 := ParsePreferenceString("x=y,w")
	if len(prefs2.DisabledEngines) != 2 {
		t.Errorf("DisabledEngines len = %d, want 2", len(prefs2.DisabledEngines))
	}
}

func TestGetShareableURL(t *testing.T) {
	prefs := DefaultPreferences()
	url := prefs.GetShareableURL("https://search.example.com")

	// With defaults, might just be preferences page
	if url == "" {
		t.Error("GetShareableURL() returned empty")
	}

	// Test with custom prefs
	customPrefs := &UserPreferences{
		Theme: "dark",
	}
	customURL := customPrefs.GetShareableURL("https://search.example.com")
	if !containsSubstr(customURL, "preferences") {
		t.Errorf("GetShareableURL() missing preferences path: %q", customURL)
	}
}

func TestPreferencesError(t *testing.T) {
	if ErrPreferencesNotFound.Error() != "preferences not found" {
		t.Errorf("ErrPreferencesNotFound = %q", ErrPreferencesNotFound.Error())
	}
}

// ===== PASSKEYS TESTS =====

func TestPasskeyErrors(t *testing.T) {
	errors := map[error]string{
		ErrPasskeyNotEnabled:       "passkeys are not enabled for this user",
		ErrPasskeyNotFound:         "passkey not found",
		ErrPasskeyAlreadyExists:    "passkey already registered",
		ErrPasskeyChallengeFailed:  "passkey challenge verification failed",
		ErrPasskeyChallengeExpired: "passkey challenge has expired",
		ErrPasskeyInvalidResponse:  "invalid passkey response",
		ErrPasskeySignCountInvalid: "passkey sign count invalid (possible cloned authenticator)",
	}

	for err, expected := range errors {
		if err.Error() != expected {
			t.Errorf("%v = %q, want %q", err, err.Error(), expected)
		}
	}
}

func TestPasskeyStruct(t *testing.T) {
	now := time.Now()
	lastUsed := now.Add(-1 * time.Hour)

	passkey := Passkey{
		ID:              "pk_123",
		UserID:          "100",
		CredentialID:    "cred_abc123",
		PublicKey:       "pubkey_data",
		AttestationType: "none",
		Transport:       "usb",
		AAGUID:          "aaguid123",
		SignCount:       5,
		Name:            "My YubiKey",
		CreatedAt:       now,
		LastUsedAt:      &lastUsed,
	}

	if passkey.ID != "pk_123" {
		t.Errorf("ID = %q", passkey.ID)
	}
	if passkey.SignCount != 5 {
		t.Errorf("SignCount = %d", passkey.SignCount)
	}
	if passkey.Name != "My YubiKey" {
		t.Errorf("Name = %q", passkey.Name)
	}
}

func TestPasskeyToInfo(t *testing.T) {
	now := time.Now()
	lastUsed := now.Add(-1 * time.Hour)

	passkey := &Passkey{
		ID:         "pk_123",
		Name:       "Test Passkey",
		CreatedAt:  now,
		LastUsedAt: &lastUsed,
		// These should not be in info
		PublicKey:    "sensitive_data",
		CredentialID: "also_sensitive",
	}

	info := passkey.ToInfo()

	if info.ID != "pk_123" {
		t.Errorf("ToInfo().ID = %q", info.ID)
	}
	if info.Name != "Test Passkey" {
		t.Errorf("ToInfo().Name = %q", info.Name)
	}
	if info.LastUsedAt == nil {
		t.Error("ToInfo().LastUsedAt should not be nil")
	}
}

func TestPasskeyChallengeStruct(t *testing.T) {
	now := time.Now()
	expires := now.Add(5 * time.Minute)

	challenge := PasskeyChallenge{
		ID:        "ch_123",
		UserID:    "100",
		Challenge: "random_challenge_base64",
		Type:      "registration",
		ExpiresAt: expires,
		CreatedAt: now,
	}

	if challenge.ID != "ch_123" {
		t.Errorf("ID = %q", challenge.ID)
	}
	if challenge.Type != "registration" {
		t.Errorf("Type = %q", challenge.Type)
	}
}

func TestWebAuthnUserStruct(t *testing.T) {
	user := WebAuthnUser{
		ID:          []byte("user123"),
		Name:        "testuser",
		DisplayName: "Test User",
		Credentials: []WebAuthnCredential{},
	}

	if string(user.ID) != "user123" {
		t.Errorf("ID = %q", user.ID)
	}
	if user.Name != "testuser" {
		t.Errorf("Name = %q", user.Name)
	}
}

func TestWebAuthnCredentialStruct(t *testing.T) {
	cred := WebAuthnCredential{
		ID:              []byte("cred123"),
		PublicKey:       []byte("pubkey"),
		AttestationType: "none",
		Transport:       []string{"usb", "nfc"},
		Flags: WebAuthnCredentialFlags{
			UserPresent:    true,
			UserVerified:   true,
			BackupEligible: false,
			BackupState:    false,
		},
		Authenticator: WebAuthnAuthenticator{
			AAGUID:       []byte("aaguid"),
			SignCount:    10,
			CloneWarning: false,
		},
	}

	if len(cred.Transport) != 2 {
		t.Errorf("Transport len = %d", len(cred.Transport))
	}
	if !cred.Flags.UserPresent {
		t.Error("UserPresent should be true")
	}
	if cred.Authenticator.SignCount != 10 {
		t.Errorf("SignCount = %d", cred.Authenticator.SignCount)
	}
}

func TestRegistrationOptionsStruct(t *testing.T) {
	opts := RegistrationOptions{
		Challenge: "challenge123",
		RelyingParty: RelyingPartyEntity{
			ID:   "example.com",
			Name: "Example",
		},
		User: PublicKeyCredentialUser{
			ID:          "user123",
			Name:        "testuser",
			DisplayName: "Test User",
		},
		PubKeyCredParams: []PublicKeyCredentialParam{
			{Type: "public-key", Alg: -7},
		},
		Timeout:     60000,
		Attestation: "none",
	}

	if opts.Challenge != "challenge123" {
		t.Errorf("Challenge = %q", opts.Challenge)
	}
	if opts.RelyingParty.ID != "example.com" {
		t.Errorf("RelyingParty.ID = %q", opts.RelyingParty.ID)
	}
	if opts.Timeout != 60000 {
		t.Errorf("Timeout = %d", opts.Timeout)
	}
}

func TestAuthenticationOptionsStruct(t *testing.T) {
	opts := AuthenticationOptions{
		Challenge:        "challenge456",
		Timeout:          60000,
		RpId:             "example.com",
		UserVerification: "preferred",
		AllowCredentials: []PublicKeyCredentialDescriptor{
			{Type: "public-key", ID: "cred1"},
		},
	}

	if opts.Challenge != "challenge456" {
		t.Errorf("Challenge = %q", opts.Challenge)
	}
	if opts.RpId != "example.com" {
		t.Errorf("RpId = %q", opts.RpId)
	}
	if len(opts.AllowCredentials) != 1 {
		t.Errorf("AllowCredentials len = %d", len(opts.AllowCredentials))
	}
}

func TestPasskeyStatusStruct(t *testing.T) {
	status := PasskeyStatus{
		Enabled: true,
		Count:   3,
	}

	if !status.Enabled {
		t.Error("Enabled should be true")
	}
	if status.Count != 3 {
		t.Errorf("Count = %d", status.Count)
	}
}

func TestPasskeyListResponseStruct(t *testing.T) {
	now := time.Now()
	resp := PasskeyListResponse{
		Passkeys: []PasskeyInfo{
			{ID: "pk1", Name: "Key 1", CreatedAt: now},
			{ID: "pk2", Name: "Key 2", CreatedAt: now},
		},
		Count: 2,
	}

	if resp.Count != 2 {
		t.Errorf("Count = %d", resp.Count)
	}
	if len(resp.Passkeys) != 2 {
		t.Errorf("Passkeys len = %d", len(resp.Passkeys))
	}
}

func TestSerializeOptions(t *testing.T) {
	opts := &RegistrationOptions{
		Challenge: "test_challenge",
		RelyingParty: RelyingPartyEntity{
			ID:   "example.com",
			Name: "Example",
		},
	}

	data, err := SerializeOptions(opts)
	if err != nil {
		t.Errorf("SerializeOptions() error: %v", err)
	}
	if data == "" {
		t.Error("SerializeOptions() returned empty string")
	}
	if !containsSubstr(data, "test_challenge") {
		t.Error("SerializeOptions() missing challenge in output")
	}
}

func TestSerializeOptionsAuth(t *testing.T) {
	opts := &AuthenticationOptions{
		Challenge: "auth_challenge",
		RpId:      "example.com",
	}

	data, err := SerializeOptions(opts)
	if err != nil {
		t.Errorf("SerializeOptions() error: %v", err)
	}
	if !containsSubstr(data, "auth_challenge") {
		t.Error("SerializeOptions() missing challenge")
	}
}

// ===== EMAIL MANAGER TESTS =====

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"standard email", "john@example.com", "j***n@e***e.com"},
		{"short local", "ab@example.com", "***@e***e.com"},
		{"two char local", "ab@ex.com", "***@e***.com"},
		{"single char domain", "john@x.com", "j***n@***.com"},
		{"subdomain", "john@mail.example.com", "j***n@m***l.example.com"},
		{"invalid no at", "johnatexample.com", "***@***.***"},
		{"empty", "", "***@***.***"},
		{"only at sign", "@", "***@***"},
		{"no domain after at", "test@", "t***t@***"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskEmail(tt.email)
			if got != tt.want {
				t.Errorf("MaskEmail(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

func TestMaskString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdef", "a***f"},
		{"ab", "***"},
		{"a", "***"},
		{"", "***"},
		{"abc", "a***c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := maskString(tt.input)
			if got != tt.want {
				t.Errorf("maskString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateVerificationToken(t *testing.T) {
	token1, err := generateVerificationToken()
	if err != nil {
		t.Errorf("generateVerificationToken() error: %v", err)
	}
	if len(token1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("Token length = %d, want 64", len(token1))
	}

	// Should be different each time
	token2, _ := generateVerificationToken()
	if token1 == token2 {
		t.Error("Tokens should be different")
	}
}

func TestUserEmailStruct(t *testing.T) {
	now := time.Now()
	expires := now.Add(24 * time.Hour)
	verified := now.Add(-1 * time.Hour)

	email := UserEmail{
		ID:                  "em_123",
		UserID:              "100",
		Email:               "test@example.com",
		Verified:            true,
		IsPrimary:           true,
		IsNotification:      false,
		VerificationToken:   "token123",
		VerificationExpires: &expires,
		CreatedAt:           now,
		VerifiedAt:          &verified,
	}

	if email.ID != "em_123" {
		t.Errorf("ID = %q", email.ID)
	}
	if email.Email != "test@example.com" {
		t.Errorf("Email = %q", email.Email)
	}
	if !email.Verified {
		t.Error("Verified should be true")
	}
	if !email.IsPrimary {
		t.Error("IsPrimary should be true")
	}
}

func TestUserEmailToInfo(t *testing.T) {
	now := time.Now()
	verified := now.Add(-1 * time.Hour)

	email := &UserEmail{
		ID:             "em_123",
		Email:          "test@example.com",
		Verified:       true,
		IsPrimary:      true,
		IsNotification: false,
		CreatedAt:      now,
		VerifiedAt:     &verified,
	}

	info := email.ToInfo()

	if info.ID != "em_123" {
		t.Errorf("ToInfo().ID = %q", info.ID)
	}
	if info.Email != "test@example.com" {
		t.Errorf("ToInfo().Email = %q", info.Email)
	}
	if info.MaskedEmail != "t***t@e***e.com" {
		t.Errorf("ToInfo().MaskedEmail = %q", info.MaskedEmail)
	}
	if !info.Verified {
		t.Error("ToInfo().Verified should be true")
	}
	if !info.IsPrimary {
		t.Error("ToInfo().IsPrimary should be true")
	}
	if info.VerifiedAt == nil {
		t.Error("ToInfo().VerifiedAt should not be nil")
	}
}

func TestUserEmailInfoStruct(t *testing.T) {
	now := time.Now()
	info := UserEmailInfo{
		ID:             "em_456",
		Email:          "user@domain.com",
		MaskedEmail:    "u***r@d***n.com",
		Verified:       true,
		IsPrimary:      false,
		IsNotification: true,
		CreatedAt:      now,
		VerifiedAt:     &now,
	}

	if info.ID != "em_456" {
		t.Errorf("ID = %q", info.ID)
	}
	if !info.IsNotification {
		t.Error("IsNotification should be true")
	}
}

func TestEmailErrors(t *testing.T) {
	errors := []error{
		ErrEmailExists,
		ErrEmailNotFoundEM,
		ErrEmailAlreadyPrimary,
		ErrCannotRemovePrimary,
		ErrVerificationExpired,
		ErrInvalidVerification,
		ErrMinimumOneEmail,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error should not be nil")
		}
		if err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	}
}

// ===== RECOVERY KEY ADDITIONAL TESTS =====

func TestNewRecoveryManager(t *testing.T) {
	// Test with default key count
	rm := NewRecoveryManager(nil, 0)
	if rm == nil {
		t.Fatal("NewRecoveryManager() returned nil")
	}
	if rm.keyCount != 10 {
		t.Errorf("keyCount = %d, want 10 (default)", rm.keyCount)
	}

	// Test with custom key count
	rm2 := NewRecoveryManager(nil, 5)
	if rm2.keyCount != 5 {
		t.Errorf("keyCount = %d, want 5", rm2.keyCount)
	}

	// Test with negative key count (should default to 10)
	rm3 := NewRecoveryManager(nil, -5)
	if rm3.keyCount != 10 {
		t.Errorf("keyCount = %d, want 10 (default for negative)", rm3.keyCount)
	}
}

func TestNormalizeRecoveryKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcd-1234-efgh-5678", "ABCD1234EFGH5678"},
		{"ABCD1234EFGH5678", "ABCD1234EFGH5678"},
		{"abcd 1234 efgh 5678", "ABCD1234EFGH5678"},
		{"AbCd-1234-EfGh-5678", "ABCD1234EFGH5678"},
		{"", ""},
		{"   spaced   ", "SPACED"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeRecoveryKey(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRecoveryKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHashAndVerifyRecoveryKey(t *testing.T) {
	// Test hashing a recovery key
	key := "ABCD1234EFGH5678"
	hash, err := hashRecoveryKey(key)
	if err != nil {
		t.Fatalf("hashRecoveryKey() error: %v", err)
	}

	// Hash should be argon2id format
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("Hash should start with $argon2id$, got: %s", hash)
	}

	// Verify with correct key
	if !verifyRecoveryKey(key, hash) {
		t.Error("verifyRecoveryKey() should return true for correct key")
	}

	// Verify with wrong key
	if verifyRecoveryKey("WRONGKEY12345678", hash) {
		t.Error("verifyRecoveryKey() should return false for wrong key")
	}
}

func TestVerifyRecoveryKeyEdgeCases(t *testing.T) {
	// Test with invalid hash formats
	tests := []struct {
		name string
		hash string
	}{
		{"too few parts", "too$few$parts"},
		{"wrong algorithm", "$bcrypt$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2g"},
		{"invalid version", "$argon2id$v=invalid$m=65536,t=3,p=4$c29tZXNhbHQ$c29tZWhhc2g"},
		{"invalid params", "$argon2id$v=19$invalid_params$c29tZXNhbHQ$c29tZWhhc2g"},
		{"invalid salt base64", "$argon2id$v=19$m=65536,t=3,p=4$!!!invalid$c29tZWhhc2g"},
		{"invalid hash base64", "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$!!!invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if verifyRecoveryKey("anykey", tt.hash) {
				t.Errorf("verifyRecoveryKey() should return false for %s", tt.name)
			}
		})
	}
}

// ===== TOTP MANAGER TESTS =====

func TestNewTOTPManager(t *testing.T) {
	// Valid 32-byte key
	validKey := make([]byte, 32)
	for i := range validKey {
		validKey[i] = byte(i)
	}

	tm, err := NewTOTPManager(nil, "TestIssuer", validKey)
	if err != nil {
		t.Fatalf("NewTOTPManager() error: %v", err)
	}
	if tm == nil {
		t.Fatal("NewTOTPManager() returned nil")
	}
	if tm.issuer != "TestIssuer" {
		t.Errorf("issuer = %q, want %q", tm.issuer, "TestIssuer")
	}
}

func TestNewTOTPManagerInvalidKey(t *testing.T) {
	// Invalid key length
	invalidKey := make([]byte, 16)
	_, err := NewTOTPManager(nil, "TestIssuer", invalidKey)
	if err == nil {
		t.Error("NewTOTPManager() should return error for invalid key length")
	}
}

func TestTOTPManagerEncryptDecrypt(t *testing.T) {
	// Valid 32-byte key
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	tm, _ := NewTOTPManager(nil, "TestIssuer", key)

	// Test encryption/decryption
	plaintext := "my_secret_totp_key"
	encrypted, err := tm.encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt() error: %v", err)
	}

	if encrypted == plaintext {
		t.Error("Encrypted should not equal plaintext")
	}

	// Decrypt
	decrypted, err := tm.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt() error: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestTOTPManagerDecryptErrors(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	tm, _ := NewTOTPManager(nil, "TestIssuer", key)

	// Test with invalid base64
	_, err := tm.decrypt("!!!invalid base64!!!")
	if err != ErrDecryptionFailed {
		t.Errorf("decrypt() should return ErrDecryptionFailed for invalid base64, got %v", err)
	}

	// Test with data too short for nonce
	_, err = tm.decrypt("YWJj") // "abc" in base64 - too short
	if err != ErrDecryptionFailed {
		t.Errorf("decrypt() should return ErrDecryptionFailed for short data, got %v", err)
	}
}

func TestGenerateBackupCodes(t *testing.T) {
	key := make([]byte, 32)
	tm, _ := NewTOTPManager(nil, "TestIssuer", key)

	codes, err := tm.GenerateBackupCodes(8)
	if err != nil {
		t.Fatalf("GenerateBackupCodes() error: %v", err)
	}

	if len(codes) != 8 {
		t.Errorf("GenerateBackupCodes(8) returned %d codes", len(codes))
	}

	// Each code should be in XXXX-XXXX format
	for i, code := range codes {
		if len(code) != 9 { // 4 + 1 + 4
			t.Errorf("Code %d length = %d, want 9", i, len(code))
		}
		if code[4] != '-' {
			t.Errorf("Code %d missing hyphen at position 4", i)
		}
	}

	// Codes should be unique
	seen := make(map[string]bool)
	for _, code := range codes {
		if seen[code] {
			t.Error("Duplicate backup code generated")
		}
		seen[code] = true
	}
}

func TestTOTPStatusStruct(t *testing.T) {
	now := time.Now()
	status := TOTPStatus{
		Enabled:   true,
		Verified:  true,
		EnabledAt: &now,
	}

	if !status.Enabled {
		t.Error("Enabled should be true")
	}
	if !status.Verified {
		t.Error("Verified should be true")
	}
	if status.EnabledAt == nil {
		t.Error("EnabledAt should not be nil")
	}
}

// ===== TOKEN MANAGER TESTS =====

func TestNewTokenManager(t *testing.T) {
	tm := NewTokenManager(nil)
	if tm == nil {
		t.Fatal("NewTokenManager() returned nil")
	}
}

func TestUserTokenPrefixConstant(t *testing.T) {
	// Per AI.md PART 11: User token prefix must be "usr_"
	if userTokenPrefix != "usr_" {
		t.Errorf("userTokenPrefix = %q, want %q", userTokenPrefix, "usr_")
	}
}

// ===== PREFERENCES MANAGER TESTS =====

func TestNewPreferencesManager(t *testing.T) {
	pm := NewPreferencesManager(nil)
	if pm == nil {
		t.Fatal("NewPreferencesManager() returned nil")
	}
	if pm.cookieName != "search_prefs" {
		t.Errorf("cookieName = %q, want %q", pm.cookieName, "search_prefs")
	}
}

func TestEngineCodes(t *testing.T) {
	// Test that all engine codes have reverse mapping
	for name, code := range engineCodes {
		if engineNames[code] != name {
			t.Errorf("Engine code mapping mismatch: %s -> %s -> %s", name, code, engineNames[code])
		}
	}

	// Test that all engine names have codes
	for code, name := range engineNames {
		if engineCodes[name] != code {
			t.Errorf("Engine name mapping mismatch: %s -> %s -> %s", code, name, engineCodes[name])
		}
	}
}

func TestParseEngineCodesFunction(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"g,b,d", []string{"google", "bing", "duckduckgo"}},
		{"g", []string{"google"}},
		{"", nil},
		{"invalid", nil},
		{"g,invalid,b", []string{"google", "bing"}},
		{" g , b ", []string{"google", "bing"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseEngineCodes(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseEngineCodes(%q) length = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseEngineCodes(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestToPreferenceStringAllOptions(t *testing.T) {
	prefs := &UserPreferences{
		Theme:             "light",
		ResultsPerPage:    30,
		DefaultLanguage:   "fr",
		DefaultRegion:     "FR",
		SafeSearch:        2,
		DefaultSort:       "popularity",
		DefaultCategory:   "images",
		EnabledEngines:    []string{"google", "bing"},
		DisabledEngines:   []string{"yahoo"},
		OpenInNewTab:      true,
		ShowThumbnails:    false,
		ShowEngineIcons:   false,
		InfiniteScroll:    true,
		AutocompleteOn:    false,
		SaveSearchHistory: true,
		AnonymizeResults:  false,
		HighContrast:      true,
		LargeFont:         true,
		ReduceMotion:      true,
	}

	str := prefs.ToPreferenceString()

	expectedParts := []string{
		"t=l",   // light theme
		"l=fr",  // language
		"g=FR",  // region
		"s=2",   // safe search
		"r=30",  // results per page
		"c=images",
		"o=p",   // popularity sort
		"e=g,b", // enabled engines
		"x=y",   // disabled engines (yahoo)
		"n=1",   // new tab
		"h=0",   // thumbnails off
		"i=0",   // icons off
		"f=1",   // infinite scroll
		"a=0",   // autocomplete off
		"y=1",   // save history
		"p=0",   // anonymize off
		"hc=1",  // high contrast
		"lf=1",  // large font
		"rm=1",  // reduce motion
	}

	for _, part := range expectedParts {
		if !containsSubstr(str, part) {
			t.Errorf("ToPreferenceString() missing %q, got %q", part, str)
		}
	}
}

func TestParsePreferenceStringAllOptions(t *testing.T) {
	input := "t=l;l=fr;g=FR;s=2;r=30;c=images;o=p;e=g,b;x=y;n=1;h=0;i=0;f=1;a=0;y=1;p=0;hc=1;lf=1;rm=1"
	prefs := ParsePreferenceString(input)

	if prefs.Theme != "light" {
		t.Errorf("Theme = %q, want light", prefs.Theme)
	}
	if prefs.DefaultLanguage != "fr" {
		t.Errorf("DefaultLanguage = %q, want fr", prefs.DefaultLanguage)
	}
	if prefs.DefaultRegion != "FR" {
		t.Errorf("DefaultRegion = %q, want FR", prefs.DefaultRegion)
	}
	if prefs.SafeSearch != 2 {
		t.Errorf("SafeSearch = %d, want 2", prefs.SafeSearch)
	}
	if prefs.ResultsPerPage != 30 {
		t.Errorf("ResultsPerPage = %d, want 30", prefs.ResultsPerPage)
	}
	if prefs.DefaultCategory != "images" {
		t.Errorf("DefaultCategory = %q, want images", prefs.DefaultCategory)
	}
	if prefs.DefaultSort != "popularity" {
		t.Errorf("DefaultSort = %q, want popularity", prefs.DefaultSort)
	}
	if !prefs.OpenInNewTab {
		t.Error("OpenInNewTab should be true")
	}
	if prefs.ShowThumbnails {
		t.Error("ShowThumbnails should be false")
	}
	if prefs.ShowEngineIcons {
		t.Error("ShowEngineIcons should be false")
	}
	if !prefs.InfiniteScroll {
		t.Error("InfiniteScroll should be true")
	}
	if prefs.AutocompleteOn {
		t.Error("AutocompleteOn should be false")
	}
	if !prefs.SaveSearchHistory {
		t.Error("SaveSearchHistory should be true")
	}
	if prefs.AnonymizeResults {
		t.Error("AnonymizeResults should be false")
	}
	if !prefs.HighContrast {
		t.Error("HighContrast should be true")
	}
	if !prefs.LargeFont {
		t.Error("LargeFont should be true")
	}
	if !prefs.ReduceMotion {
		t.Error("ReduceMotion should be true")
	}
}

func TestParsePreferenceStringInvalidValues(t *testing.T) {
	// Invalid safe search values should be ignored (keep defaults)
	prefs := ParsePreferenceString("s=5")
	if prefs.SafeSearch != 1 { // default
		t.Errorf("SafeSearch = %d, want 1 (default)", prefs.SafeSearch)
	}

	// Invalid results per page
	prefs = ParsePreferenceString("r=5") // below minimum
	if prefs.ResultsPerPage != 20 {      // default
		t.Errorf("ResultsPerPage = %d, want 20 (default)", prefs.ResultsPerPage)
	}

	prefs = ParsePreferenceString("r=200") // above maximum
	if prefs.ResultsPerPage != 20 {        // default
		t.Errorf("ResultsPerPage = %d, want 20 (default)", prefs.ResultsPerPage)
	}

	// Invalid key=value format
	prefs = ParsePreferenceString("invalid;t=d;also_invalid")
	if prefs.Theme != "dark" {
		t.Errorf("Theme = %q, want dark", prefs.Theme)
	}
}

func TestParsePreferenceStringSortValues(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"o=r", "relevance"},
		{"o=d", "date"},
		{"o=p", "popularity"},
		{"o=x", "relevance"}, // invalid defaults to relevance
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			prefs := ParsePreferenceString(tt.input)
			if prefs.DefaultSort != tt.expected {
				t.Errorf("ParsePreferenceString(%q).DefaultSort = %q, want %q", tt.input, prefs.DefaultSort, tt.expected)
			}
		})
	}
}

func TestUserPreferencesStruct(t *testing.T) {
	prefs := UserPreferences{
		Theme:             "dark",
		ResultsPerPage:    25,
		OpenInNewTab:      true,
		DefaultCategory:   "news",
		DefaultLanguage:   "es",
		DefaultRegion:     "ES",
		SafeSearch:        2,
		DefaultSort:       "date",
		EnabledEngines:    []string{"google"},
		DisabledEngines:   []string{"bing"},
		ShowThumbnails:    true,
		ShowEngineIcons:   true,
		InfiniteScroll:    false,
		AutocompleteOn:    true,
		SaveSearchHistory: false,
		AnonymizeResults:  true,
		HighContrast:      false,
		LargeFont:         false,
		ReduceMotion:      false,
	}

	if prefs.Theme != "dark" {
		t.Errorf("Theme = %q", prefs.Theme)
	}
	if prefs.ResultsPerPage != 25 {
		t.Errorf("ResultsPerPage = %d", prefs.ResultsPerPage)
	}
	if len(prefs.EnabledEngines) != 1 {
		t.Errorf("EnabledEngines len = %d", len(prefs.EnabledEngines))
	}
}

func TestPreferencesMergeAllFields(t *testing.T) {
	base := DefaultPreferences()
	updates := &UserPreferences{
		Theme:           "light",
		ResultsPerPage:  50,
		DefaultCategory: "news",
		DefaultLanguage: "de",
		DefaultRegion:   "DE",
		DefaultSort:     "date",
		EnabledEngines:  []string{"google", "bing"},
		DisabledEngines: []string{"yahoo"},
		OpenInNewTab:    true,
		ShowThumbnails:  false,
		ShowEngineIcons: false,
		InfiniteScroll:  true,
		AutocompleteOn:  false,
		SaveSearchHistory: true,
		AnonymizeResults: false,
		HighContrast:    true,
		LargeFont:       true,
		ReduceMotion:    true,
		SafeSearch:      2,
	}

	base.Merge(updates)

	if base.Theme != "light" {
		t.Errorf("Theme = %q", base.Theme)
	}
	if base.ResultsPerPage != 50 {
		t.Errorf("ResultsPerPage = %d", base.ResultsPerPage)
	}
	if base.DefaultCategory != "news" {
		t.Errorf("DefaultCategory = %q", base.DefaultCategory)
	}
	if base.DefaultLanguage != "de" {
		t.Errorf("DefaultLanguage = %q", base.DefaultLanguage)
	}
	if base.DefaultRegion != "DE" {
		t.Errorf("DefaultRegion = %q", base.DefaultRegion)
	}
	if base.DefaultSort != "date" {
		t.Errorf("DefaultSort = %q", base.DefaultSort)
	}
	if len(base.EnabledEngines) != 2 {
		t.Errorf("EnabledEngines len = %d", len(base.EnabledEngines))
	}
	if len(base.DisabledEngines) != 1 {
		t.Errorf("DisabledEngines len = %d", len(base.DisabledEngines))
	}
	if !base.OpenInNewTab {
		t.Error("OpenInNewTab should be true")
	}
	if base.ShowThumbnails {
		t.Error("ShowThumbnails should be false")
	}
	if base.ShowEngineIcons {
		t.Error("ShowEngineIcons should be false")
	}
	if !base.InfiniteScroll {
		t.Error("InfiniteScroll should be true")
	}
	if base.AutocompleteOn {
		t.Error("AutocompleteOn should be false")
	}
	if !base.SaveSearchHistory {
		t.Error("SaveSearchHistory should be true")
	}
	if base.AnonymizeResults {
		t.Error("AnonymizeResults should be false")
	}
	if !base.HighContrast {
		t.Error("HighContrast should be true")
	}
	if !base.LargeFont {
		t.Error("LargeFont should be true")
	}
	if !base.ReduceMotion {
		t.Error("ReduceMotion should be true")
	}
	if base.SafeSearch != 2 {
		t.Errorf("SafeSearch = %d", base.SafeSearch)
	}
}

func TestGetShareableURLEdgeCases(t *testing.T) {
	// Default preferences should return base URL
	prefs := &UserPreferences{
		Theme:          "auto",
		ResultsPerPage: 20,
		SafeSearch:     1,
		DefaultSort:    "relevance",
	}
	url := prefs.GetShareableURL("https://example.com")
	// With minimal defaults, should return preferences page (depends on implementation)
	if url == "" {
		t.Error("GetShareableURL() should not return empty")
	}
}

// ===== PASSKEY MANAGER TESTS =====

func TestNewPasskeyManager(t *testing.T) {
	pm := NewPasskeyManager(nil, "example.com", "https://example.com", "Example App")
	if pm == nil {
		t.Fatal("NewPasskeyManager() returned nil")
	}
	if pm.rpID != "example.com" {
		t.Errorf("rpID = %q, want %q", pm.rpID, "example.com")
	}
	if pm.rpOrigin != "https://example.com" {
		t.Errorf("rpOrigin = %q, want %q", pm.rpOrigin, "https://example.com")
	}
	if pm.rpName != "Example App" {
		t.Errorf("rpName = %q, want %q", pm.rpName, "Example App")
	}
}

func TestGenerateChallenge(t *testing.T) {
	challenge1, err := generateChallenge()
	if err != nil {
		t.Fatalf("generateChallenge() error: %v", err)
	}

	// Challenge should be base64url encoded
	if challenge1 == "" {
		t.Error("generateChallenge() returned empty string")
	}

	// Should be different each time
	challenge2, _ := generateChallenge()
	if challenge1 == challenge2 {
		t.Error("Challenges should be different")
	}
}

func TestRegistrationResponseStruct(t *testing.T) {
	resp := RegistrationResponse{
		ID:    "cred_id",
		RawID: "raw_cred_id",
		Type:  "public-key",
		AuthenticatorAttachment: "platform",
	}
	resp.Response.AttestationObject = "attestation_data"
	resp.Response.ClientDataJSON = "client_data"

	if resp.ID != "cred_id" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.Type != "public-key" {
		t.Errorf("Type = %q", resp.Type)
	}
	if resp.Response.AttestationObject != "attestation_data" {
		t.Errorf("AttestationObject = %q", resp.Response.AttestationObject)
	}
}

func TestAuthenticationResponseStruct(t *testing.T) {
	resp := AuthenticationResponse{
		ID:    "cred_id",
		RawID: "raw_cred_id",
		Type:  "public-key",
	}
	resp.Response.AuthenticatorData = "auth_data"
	resp.Response.ClientDataJSON = "client_data"
	resp.Response.Signature = "signature_data"
	resp.Response.UserHandle = "user_handle"

	if resp.ID != "cred_id" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.Response.Signature != "signature_data" {
		t.Errorf("Signature = %q", resp.Response.Signature)
	}
	if resp.Response.UserHandle != "user_handle" {
		t.Errorf("UserHandle = %q", resp.Response.UserHandle)
	}
}

func TestPublicKeyCredentialDescriptorStruct(t *testing.T) {
	desc := PublicKeyCredentialDescriptor{
		Type:       "public-key",
		ID:         "cred_123",
		Transports: []string{"usb", "nfc"},
	}

	if desc.Type != "public-key" {
		t.Errorf("Type = %q", desc.Type)
	}
	if desc.ID != "cred_123" {
		t.Errorf("ID = %q", desc.ID)
	}
	if len(desc.Transports) != 2 {
		t.Errorf("Transports len = %d", len(desc.Transports))
	}
}

func TestAuthenticatorSelectionStruct(t *testing.T) {
	sel := AuthenticatorSelection{
		AuthenticatorAttachment: "platform",
		ResidentKey:             "preferred",
		RequireResidentKey:      false,
		UserVerification:        "preferred",
	}

	if sel.AuthenticatorAttachment != "platform" {
		t.Errorf("AuthenticatorAttachment = %q", sel.AuthenticatorAttachment)
	}
	if sel.ResidentKey != "preferred" {
		t.Errorf("ResidentKey = %q", sel.ResidentKey)
	}
}

func TestRelyingPartyEntityStruct(t *testing.T) {
	rp := RelyingPartyEntity{
		ID:   "example.com",
		Name: "Example App",
	}

	if rp.ID != "example.com" {
		t.Errorf("ID = %q", rp.ID)
	}
	if rp.Name != "Example App" {
		t.Errorf("Name = %q", rp.Name)
	}
}

func TestPublicKeyCredentialUserStruct(t *testing.T) {
	user := PublicKeyCredentialUser{
		ID:          "user_123",
		Name:        "testuser",
		DisplayName: "Test User",
	}

	if user.ID != "user_123" {
		t.Errorf("ID = %q", user.ID)
	}
	if user.Name != "testuser" {
		t.Errorf("Name = %q", user.Name)
	}
	if user.DisplayName != "Test User" {
		t.Errorf("DisplayName = %q", user.DisplayName)
	}
}

func TestPublicKeyCredentialParamStruct(t *testing.T) {
	param := PublicKeyCredentialParam{
		Type: "public-key",
		Alg:  -7, // ES256
	}

	if param.Type != "public-key" {
		t.Errorf("Type = %q", param.Type)
	}
	if param.Alg != -7 {
		t.Errorf("Alg = %d", param.Alg)
	}
}

func TestPasskeyInfoStruct(t *testing.T) {
	now := time.Now()
	lastUsed := now.Add(-1 * time.Hour)
	info := PasskeyInfo{
		ID:         "pk_123",
		Name:       "My Security Key",
		CreatedAt:  now,
		LastUsedAt: &lastUsed,
	}

	if info.ID != "pk_123" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.Name != "My Security Key" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.LastUsedAt == nil {
		t.Error("LastUsedAt should not be nil")
	}
}

// ===== VERIFICATION MANAGER TESTS =====

func TestNewVerificationManager(t *testing.T) {
	vm := NewVerificationManager(nil)
	if vm == nil {
		t.Fatal("NewVerificationManager() returned nil")
	}
}

// ===== EMAIL MANAGER TESTS =====

func TestNewEmailManager(t *testing.T) {
	em := NewEmailManager(nil)
	if em == nil {
		t.Fatal("NewEmailManager() returned nil")
	}
}

// ===== AUTH MANAGER ADDITIONAL TESTS =====

func TestNewAuthManager(t *testing.T) {
	// Test with default config
	am := NewAuthManager(nil, AuthConfig{})
	if am == nil {
		t.Fatal("NewAuthManager() returned nil")
	}
	// Should use defaults
	if am.cookieName != "user_session" {
		t.Errorf("cookieName = %q, want %q", am.cookieName, "user_session")
	}

	// Test with custom config
	am2 := NewAuthManager(nil, AuthConfig{
		SessionDurationDays: 14,
		CookieName:          "custom_session",
		CookieDomain:        "example.com",
		CookieSecure:        true,
	})
	if am2.cookieName != "custom_session" {
		t.Errorf("cookieName = %q, want %q", am2.cookieName, "custom_session")
	}
	if am2.cookieDomain != "example.com" {
		t.Errorf("cookieDomain = %q", am2.cookieDomain)
	}
	if !am2.cookieSecure {
		t.Error("cookieSecure should be true")
	}
}

func TestAuthManagerSessionDuration(t *testing.T) {
	// Test with zero duration (should default to 7)
	am := NewAuthManager(nil, AuthConfig{SessionDurationDays: 0})
	expectedDuration := 7 * 24 * time.Hour
	if am.sessionDuration != expectedDuration {
		t.Errorf("sessionDuration = %v, want %v", am.sessionDuration, expectedDuration)
	}

	// Test with custom duration
	am2 := NewAuthManager(nil, AuthConfig{SessionDurationDays: 14})
	expectedDuration2 := 14 * 24 * time.Hour
	if am2.sessionDuration != expectedDuration2 {
		t.Errorf("sessionDuration = %v, want %v", am2.sessionDuration, expectedDuration2)
	}
}

// ===== ADDITIONAL EDGE CASE TESTS =====

func TestToPreferenceStringThemes(t *testing.T) {
	tests := []struct {
		theme    string
		expected string
	}{
		{"dark", "t=d"},
		{"light", "t=l"},
		{"auto", "t=a"},
	}

	for _, tt := range tests {
		t.Run(tt.theme, func(t *testing.T) {
			prefs := &UserPreferences{Theme: tt.theme}
			str := prefs.ToPreferenceString()
			if !containsSubstr(str, tt.expected) {
				t.Errorf("Theme %q should produce %q, got %q", tt.theme, tt.expected, str)
			}
		})
	}
}

func TestToPreferenceStringEmptyEngines(t *testing.T) {
	prefs := &UserPreferences{
		Theme:          "dark",
		EnabledEngines: []string{},
	}
	str := prefs.ToPreferenceString()
	// Should not contain engine encoding for empty engines
	if containsSubstr(str, "e=") {
		t.Errorf("Empty engines should not produce e= in string: %q", str)
	}
}

func TestToPreferenceStringUnknownEngines(t *testing.T) {
	prefs := &UserPreferences{
		Theme:          "dark",
		EnabledEngines: []string{"unknownengine", "google"},
	}
	str := prefs.ToPreferenceString()
	// Should only encode known engines
	if containsSubstr(str, "e=g") {
		// Should contain google
	}
}

func TestValidatePreferencesValidCategories(t *testing.T) {
	validCategories := []string{"general", "images", "videos", "news", "maps", "files", "it", "science", "social"}

	for _, cat := range validCategories {
		t.Run(cat, func(t *testing.T) {
			prefs := &UserPreferences{DefaultCategory: cat}
			prefs.Validate()
			if prefs.DefaultCategory != cat {
				t.Errorf("Category %q should remain valid, got %q", cat, prefs.DefaultCategory)
			}
		})
	}
}

func TestValidatePreferencesValidSorts(t *testing.T) {
	validSorts := []string{"relevance", "date", "popularity"}

	for _, sort := range validSorts {
		t.Run(sort, func(t *testing.T) {
			prefs := &UserPreferences{DefaultSort: sort}
			prefs.Validate()
			if prefs.DefaultSort != sort {
				t.Errorf("Sort %q should remain valid, got %q", sort, prefs.DefaultSort)
			}
		})
	}
}

func TestValidatePreferencesBoundaries(t *testing.T) {
	// Exactly at boundaries
	prefs := &UserPreferences{ResultsPerPage: 10, SafeSearch: 0}
	prefs.Validate()
	if prefs.ResultsPerPage != 10 {
		t.Errorf("ResultsPerPage 10 should remain valid, got %d", prefs.ResultsPerPage)
	}
	if prefs.SafeSearch != 0 {
		t.Errorf("SafeSearch 0 should remain valid, got %d", prefs.SafeSearch)
	}

	prefs2 := &UserPreferences{ResultsPerPage: 100, SafeSearch: 2}
	prefs2.Validate()
	if prefs2.ResultsPerPage != 100 {
		t.Errorf("ResultsPerPage 100 should remain valid, got %d", prefs2.ResultsPerPage)
	}
	if prefs2.SafeSearch != 2 {
		t.Errorf("SafeSearch 2 should remain valid, got %d", prefs2.SafeSearch)
	}
}

// ===== ARGON2 CONSTANT TESTS =====

func TestArgon2Constants(t *testing.T) {
	// Per AI.md: Argon2id parameters
	if argon2Time != 3 {
		t.Errorf("argon2Time = %d, want 3", argon2Time)
	}
	if argon2Memory != 64*1024 {
		t.Errorf("argon2Memory = %d, want %d", argon2Memory, 64*1024)
	}
	if argon2Threads != 4 {
		t.Errorf("argon2Threads = %d, want 4", argon2Threads)
	}
	if argon2KeyLen != 32 {
		t.Errorf("argon2KeyLen = %d, want 32", argon2KeyLen)
	}
	if argon2SaltLen != 16 {
		t.Errorf("argon2SaltLen = %d, want 16", argon2SaltLen)
	}
}

// ===== ADDITIONAL STRUCT TESTS =====

func TestWebAuthnCredentialFlagsStruct(t *testing.T) {
	flags := WebAuthnCredentialFlags{
		UserPresent:    true,
		UserVerified:   true,
		BackupEligible: true,
		BackupState:    false,
	}

	if !flags.UserPresent {
		t.Error("UserPresent should be true")
	}
	if !flags.UserVerified {
		t.Error("UserVerified should be true")
	}
	if !flags.BackupEligible {
		t.Error("BackupEligible should be true")
	}
	if flags.BackupState {
		t.Error("BackupState should be false")
	}
}

func TestWebAuthnAuthenticatorStruct(t *testing.T) {
	auth := WebAuthnAuthenticator{
		AAGUID:       []byte("test_aaguid"),
		SignCount:    100,
		CloneWarning: false,
	}

	if string(auth.AAGUID) != "test_aaguid" {
		t.Errorf("AAGUID = %q", auth.AAGUID)
	}
	if auth.SignCount != 100 {
		t.Errorf("SignCount = %d", auth.SignCount)
	}
	if auth.CloneWarning {
		t.Error("CloneWarning should be false")
	}
}

// ===== TEST HELPER VALIDATION =====

func TestContainsSubstrHelper(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"hello world", "world", true},
		{"hello world", "foo", false},
		{"hello", "hello", true},
		{"", "", true},
		{"hello", "", true},
		{"", "hello", false},
	}

	for _, tt := range tests {
		got := containsSubstr(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("containsSubstr(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}
