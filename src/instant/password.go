package instant

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
)

// PasswordHandler generates secure passwords
type PasswordHandler struct {
	patterns []*regexp.Regexp
}

func NewPasswordHandler() *PasswordHandler {
	return &PasswordHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^password\s*$`),
			regexp.MustCompile(`(?i)^generate\s+password\s*$`),
			regexp.MustCompile(`(?i)^password\s+(\d+)\s*$`),
			regexp.MustCompile(`(?i)^random\s+password\s*$`),
			regexp.MustCompile(`(?i)^secure\s+password\s*$`),
		},
	}
}

func (h *PasswordHandler) Name() string               { return "password" }
func (h *PasswordHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *PasswordHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *PasswordHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	length := 16
	lengthPattern := regexp.MustCompile(`(\d+)`)
	if matches := lengthPattern.FindStringSubmatch(query); len(matches) > 1 {
		length, _ = strconv.Atoi(matches[1])
		if length < 8 {
			length = 8
		}
		if length > 128 {
			length = 128
		}
	}

	password := generatePassword(length)
	passwordNoSpecial := generatePasswordNoSpecial(length)

	return &Answer{
		Type:  AnswerTypePassword,
		Query: query,
		Title: "Password Generator",
		Content: fmt.Sprintf(`<div class="password-result">
<strong>Secure Password (%d chars):</strong><br>
<code>%s</code><br><br>
<strong>Alphanumeric Only:</strong><br>
<code>%s</code>
</div>`, length, password, passwordNoSpecial),
		Data: map[string]interface{}{
			"password": password,
			"length":   length,
		},
	}, nil
}

func generatePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}|;:,.<>?"
	result := make([]byte, length)
	for i := range result {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	return string(result)
}

func generatePasswordNoSpecial(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	return string(result)
}
