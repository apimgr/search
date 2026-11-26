package api

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/apimgr/search/src/common/httputil"
)

func (h *Handler) strictOwnerAction(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.config == nil || !h.config.Server.RateLimit.Enabled {
			next(w, r)
			return
		}
		limit := h.config.Server.RateLimit.Strict.Requests
		window := time.Duration(h.config.Server.RateLimit.Strict.Window) * time.Second
		if limit <= 0 {
			limit = 10
		}
		if window <= 0 {
			window = 15 * time.Minute
		}
		now := time.Now()
		ip := httputil.GetClientIP(r)
		h.strictMu.Lock()
		if h.strictAttempts == nil {
			h.strictAttempts = make(map[string][]time.Time)
		}
		attempts := h.strictAttempts[ip]
		fresh := attempts[:0]
		for _, attempt := range attempts {
			if now.Sub(attempt) < window {
				fresh = append(fresh, attempt)
			}
		}
		if len(fresh) >= limit {
			h.strictAttempts[ip] = fresh
			retryAfter := int(math.Ceil(window.Seconds() - now.Sub(fresh[0]).Seconds()))
			h.strictMu.Unlock()
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			h.errorResponse(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests", r)
			return
		}
		h.strictAttempts[ip] = append(fresh, now)
		h.strictMu.Unlock()
		next(w, r)
	}
}
