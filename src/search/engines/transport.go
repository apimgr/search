package engines

import (
	"io"
	"net/http"
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
