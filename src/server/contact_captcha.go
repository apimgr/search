package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/config"
)

const maxCaptchaResponseBytes = 16 << 10

type contactCaptchaVerifier struct {
	client    *http.Client
	recaptcha string
	hcaptcha  string
}

func newContactCaptchaVerifier(client *http.Client, recaptchaURL, hcaptchaURL string) contactCaptchaVerifier {
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	clientCopy := *client
	clientCopy.Timeout = 6 * time.Second
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return contactCaptchaVerifier{client: &clientCopy, recaptcha: recaptchaURL, hcaptcha: hcaptchaURL}
}

func (v contactCaptchaVerifier) verify(ctx context.Context, provider, secret, token, remoteIP string) bool {
	endpoint := ""
	form := url.Values{"secret": {secret}, "response": {token}}
	switch provider {
	case "recaptcha":
		endpoint = v.recaptcha
	case "hcaptcha":
		endpoint = v.hcaptcha
	default:
		return false
	}
	if endpoint == "" || strings.TrimSpace(secret) == "" || strings.TrimSpace(token) == "" {
		return false
	}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.client.Do(req)
	if err != nil {
		return false
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCaptchaResponseBytes+1))
	if err != nil || len(data) > maxCaptchaResponseBytes {
		return false
	}
	var result struct {
		Success bool `json:"success"`
	}
	if json.Unmarshal(data, &result) != nil {
		return false
	}
	return result.Success
}

func (s *Server) configuredContactCaptcha() (string, string, string) {
	return contactCaptchaConfig(s.config)
}

func contactCaptchaConfig(cfg *config.Config) (string, string, string) {
	if cfg == nil {
		return "simple", "", ""
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Server.Pages.Contact.Captcha))
	if mode == "" {
		mode = "simple"
	}
	return mode, strings.TrimSpace(cfg.Server.Pages.Contact.CaptchaSiteKey), strings.TrimSpace(cfg.Server.Pages.Contact.CaptchaSecret)
}

func (s *Server) verifyConfiguredContactCaptcha(r *http.Request) bool {
	mode, siteKey, secretKey := s.configuredContactCaptcha()
	answer := r.PostFormValue("captcha")
	id := r.PostFormValue("captcha_id")
	if mode == "simple" || isOnionRequest(s.config, r) {
		return s.validCaptcha(id, answer)
	}
	if mode != "recaptcha" && mode != "hcaptcha" {
		return false
	}
	if siteKey == "" || secretKey == "" {
		return false
	}
	tokenField := "g-recaptcha-response"
	if mode == "hcaptcha" {
		tokenField = "h-captcha-response"
	}
	verifier := newContactCaptchaVerifier(nil, "https://www.google.com/recaptcha/api/siteverify", "https://api.hcaptcha.com/siteverify")
	return verifier.verify(r.Context(), mode, secretKey, r.PostFormValue(tokenField), "")
}

func contactCaptchaCSP(csp, mode string) string {
	sources := map[string][]string{}
	switch mode {
	case "recaptcha":
		sources["script-src"] = []string{"https://www.google.com/recaptcha/", "https://www.gstatic.com/recaptcha/"}
		sources["frame-src"] = []string{"https://www.google.com/recaptcha/"}
		sources["connect-src"] = []string{"https://www.google.com/recaptcha/"}
	case "hcaptcha":
		sources["script-src"] = []string{"https://js.hcaptcha.com"}
		sources["frame-src"] = []string{"https://hcaptcha.com", "https://*.hcaptcha.com"}
		sources["connect-src"] = []string{"https://hcaptcha.com", "https://*.hcaptcha.com"}
	default:
		return csp
	}
	directives := strings.Split(csp, ";")
	for directive, additions := range sources {
		found := false
		for index, value := range directives {
			fields := strings.Fields(value)
			if len(fields) == 0 || fields[0] != directive {
				continue
			}
			found = true
			for _, source := range additions {
				if !containsString(fields[1:], source) {
					fields = append(fields, source)
				}
			}
			directives[index] = strings.Join(fields, " ")
			break
		}
		if !found {
			directives = append(directives, " "+directive+" 'self' "+strings.Join(additions, " "))
		}
	}
	return strings.Join(directives, ";")
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
