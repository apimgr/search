package instant

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math"
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
			regexp.MustCompile(`(?i)^password\s+(\d+)(?:\s+characters?)?\s*$`),
			regexp.MustCompile(`(?i)^random\s+password\s*$`),
			regexp.MustCompile(`(?i)^strong\s+password\s*$`),
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
		if length > 64 {
			length = 64
		}
	}

	password, err := generatePassword(length)
	if err != nil {
		return nil, err
	}

	strength := passwordStrength(length, 26+26+10+len("!@#$%^&*()-_=+[]{}|;:,.<>?"))
	return &Answer{
		Type:  AnswerTypePassword,
		Query: query,
		Title: "Password Generator",
		Content: fmt.Sprintf(`<div class="password-result" data-password-widget>
<label>Length <input class="password-length" type="number" min="8" max="64" value="%d"></label>
<fieldset><legend>Character types</legend>
<label><input type="checkbox" data-password-set="lowercase" checked> Lowercase</label>
<label><input type="checkbox" data-password-set="uppercase" checked> Uppercase</label>
<label><input type="checkbox" data-password-set="numbers" checked> Numbers</label>
<label><input type="checkbox" data-password-set="symbols" checked> Symbols</label>
</fieldset>
<strong>Secure Password:</strong><br>
<code class="password-value">%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>
<strong>Strength:</strong> <meter class="password-strength-meter" min="0" max="100" value="%d"></meter> <span class="password-strength-label">%s</span>
<button type="button" data-action="regenerate-password">Regenerate</button>
</div>`, length, escapeHTML(password), escapeHTML(password), minInt(strength, 100), passwordStrengthLabel(strength)),
		Data: map[string]interface{}{
			"password": password,
			"length":   length,
			"strength": passwordStrengthLabel(strength),
		},
	}, nil
}

func passwordStrength(length, alphabetSize int) int {
	if length <= 0 || alphabetSize <= 1 {
		return 0
	}
	return int(float64(length) * math.Log2(float64(alphabetSize)))
}

func passwordStrengthLabel(entropy int) string {
	switch {
	case entropy < 40:
		return "Weak"
	case entropy < 70:
		return "Fair"
	case entropy < 100:
		return "Strong"
	default:
		return "Very strong"
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func generatePassword(length int) (string, error) {
	return generatePasswordWithReader(length, rand.Reader)
}

func generatePasswordWithReader(length int, reader io.Reader) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}|;:,.<>?"
	return generateFromCharset(length, charset, reader)
}

func generateFromCharset(length int, charset string, reader io.Reader) (string, error) {
	if length < 1 || length > 64 {
		return "", fmt.Errorf("password length must be between 1 and 64")
	}
	if charset == "" {
		return "", fmt.Errorf("password character set is empty")
	}
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("generate secure password: %w", err)
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

func generatePasswordNoSpecial(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	return generateFromCharset(length, charset, rand.Reader)
}
