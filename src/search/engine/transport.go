package engine

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SharedTransport is a single http.Transport shared across all engines.
// Sharing one transport enables TCP connection reuse across engines,
// prevents file-descriptor exhaustion under load, and avoids the
// TIME_WAIT accumulation that causes intermittent ERR_CONNECTION_TIMED_OUT.
var SharedTransport = &http.Transport{
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   10,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	DisableCompression:    false,
}

// maxBodyBytes is the upper bound for reading an engine response body.
// Responses larger than this are truncated (parsing handles truncation).
// 4 MB
const maxBodyBytes = 4 * 1024 * 1024

// ReadBody fully reads an HTTP response body up to maxBodyBytes and
// returns it as a byte slice. Fully draining the body (to EOF) allows
// Go's HTTP transport to reuse the underlying TCP connection.
func ReadBody(resp *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
}

// blockPageMarkers holds engine-specific substrings that only appear on that
// engine's anti-bot / CAPTCHA / JS-challenge block page, never on a real
// results page. Live-confirmed via manual verification against real block
// responses.
var blockPageMarkers = map[string][]string{
	"google": {"/httpservice/retry/enablejs", "Our systems have detected unusual traffic"},
	"yandex": {"showCaptcha", "Are you not a robot", "SmartCaptcha"},
	"baidu":  {"百度安全验证", "Baidu Security Verification", "wappass.baidu.com/static/captcha"},
}

// detectBlockPage returns an error if body matches a known anti-bot/CAPTCHA
// block-page signature for the named engine, so callers can surface a real
// failure instead of silently returning an empty result slice with nil error.
func detectBlockPage(engineName, body string) error {
	for _, marker := range blockPageMarkers[engineName] {
		if strings.Contains(body, marker) {
			return fmt.Errorf("%s returned an anti-bot/CAPTCHA block page (marker: %q)", engineName, marker)
		}
	}
	return nil
}
