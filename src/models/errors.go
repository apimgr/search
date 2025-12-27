// Package models defines core data structures and errors
// Per AI.md PART 31: Standard error definitions and data models
package models

import "errors"

// Standard Error Codes per AI.md PART 31
// These map to HTTP status codes for consistent API responses
const (
	// 400 Bad Request
	ErrCodeBadRequest = "ERR_BAD_REQUEST" // Malformed request syntax
	ErrCodeValidation = "ERR_VALIDATION"   // Input validation failed

	// 401 Unauthorized
	ErrCodeUnauthorized    = "ERR_UNAUTHORIZED"     // Authentication required
	ErrCodeSessionExpired  = "ERR_SESSION_EXPIRED"  // Session has expired
	ErrCodeSessionInvalid  = "ERR_SESSION_INVALID"  // Invalid session token
	ErrCode2FARequired     = "ERR_2FA_REQUIRED"     // Two-factor authentication required
	ErrCode2FAInvalid      = "ERR_2FA_INVALID"      // Invalid 2FA code

	// 403 Forbidden
	ErrCodeForbidden      = "ERR_FORBIDDEN"       // Permission denied
	ErrCodeAccountLocked  = "ERR_ACCOUNT_LOCKED"  // Account temporarily locked

	// 404 Not Found
	ErrCodeNotFound = "ERR_NOT_FOUND" // Resource not found

	// 405 Method Not Allowed
	ErrCodeMethodNotAllowed = "ERR_METHOD_NOT_ALLOWED" // HTTP method not supported

	// 409 Conflict
	ErrCodeConflict = "ERR_CONFLICT" // Resource already exists or version conflict

	// 422 Unprocessable Entity
	ErrCodeUnprocessable = "ERR_UNPROCESSABLE" // Semantic validation error

	// 429 Too Many Requests
	ErrCodeRateLimit = "ERR_RATE_LIMIT" // Rate limit exceeded

	// 500 Internal Server Error
	ErrCodeInternal = "ERR_INTERNAL" // Server error

	// 503 Service Unavailable
	ErrCodeServiceUnavailable = "ERR_SERVICE_UNAVAILABLE" // Maintenance mode or overloaded
)

// ErrorCodeToHTTP maps error codes to HTTP status codes
var ErrorCodeToHTTP = map[string]int{
	ErrCodeBadRequest:         400,
	ErrCodeValidation:         400,
	ErrCodeUnauthorized:       401,
	ErrCodeSessionExpired:     401,
	ErrCodeSessionInvalid:     401,
	ErrCode2FARequired:        401,
	ErrCode2FAInvalid:         401,
	ErrCodeForbidden:          403,
	ErrCodeAccountLocked:      403,
	ErrCodeNotFound:           404,
	ErrCodeMethodNotAllowed:   405,
	ErrCodeConflict:           409,
	ErrCodeUnprocessable:      422,
	ErrCodeRateLimit:          429,
	ErrCodeInternal:           500,
	ErrCodeServiceUnavailable: 503,
}

// HTTPStatusCode returns the HTTP status code for an error code
func HTTPStatusCode(code string) int {
	if status, ok := ErrorCodeToHTTP[code]; ok {
		return status
	}
	return 500 // Default to internal error
}

// HTTPToErrorCode maps HTTP status codes to default error codes
// Used for backwards compatibility when error code is not explicitly provided
var HTTPToErrorCode = map[int]string{
	400: ErrCodeBadRequest,
	401: ErrCodeUnauthorized,
	403: ErrCodeForbidden,
	404: ErrCodeNotFound,
	405: ErrCodeMethodNotAllowed,
	409: ErrCodeConflict,
	422: ErrCodeUnprocessable,
	429: ErrCodeRateLimit,
	500: ErrCodeInternal,
	503: ErrCodeServiceUnavailable,
}

// ErrorCodeFromHTTP returns the default error code for an HTTP status
func ErrorCodeFromHTTP(status int) string {
	if code, ok := HTTPToErrorCode[status]; ok {
		return code
	}
	if status >= 400 && status < 500 {
		return ErrCodeBadRequest
	}
	return ErrCodeInternal
}

// Domain-specific errors
var (
	// Query errors
	ErrEmptyQuery      = errors.New("query text cannot be empty")
	ErrInvalidCategory = errors.New("invalid category")

	// Engine errors
	ErrEngineNotFound    = errors.New("engine not found")
	ErrEngineDisabled    = errors.New("engine is disabled")
	ErrEngineUnavailable = errors.New("engine is unavailable")
	ErrEngineTimeout     = errors.New("engine request timed out")
	ErrEngineRateLimit   = errors.New("engine rate limit exceeded")

	// Search errors
	ErrNoResults     = errors.New("no results found")
	ErrNoEngines     = errors.New("no engines available")
	ErrSearchTimeout = errors.New("search request timed out")

	// Configuration errors
	ErrInvalidConfig = errors.New("invalid configuration")
	ErrMissingConfig = errors.New("missing required configuration")
)
