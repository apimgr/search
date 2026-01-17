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
