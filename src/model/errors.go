// Package models defines core data structures and errors
// Per AI.md PART 31: Standard error definitions and data models
package model

import "errors"

// Standard Error Codes per AI.md PART 16: Unified Response Format
// These map to HTTP status codes for consistent API responses
const (
	// 400 Bad Request
	// Malformed request syntax
	ErrCodeBadRequest = "BAD_REQUEST"
	// Input validation failed
	ErrCodeValidation = "VALIDATION_FAILED"

	// 401 Unauthorized
	// Authentication required
	ErrCodeUnauthorized = "UNAUTHORIZED"
	// Token has expired
	ErrCodeTokenExpired = "TOKEN_EXPIRED"
	// Invalid token
	ErrCodeTokenInvalid = "TOKEN_INVALID"
	// Two-factor authentication required
	ErrCode2FARequired = "2FA_REQUIRED"
	// Invalid 2FA code
	ErrCode2FAInvalid = "2FA_INVALID"

	// 403 Forbidden
	// Permission denied
	ErrCodeForbidden = "FORBIDDEN"
	// Account temporarily locked
	ErrCodeAccountLocked = "ACCOUNT_LOCKED"

	// 404 Not Found
	// Resource not found
	ErrCodeNotFound = "NOT_FOUND"

	// 405 Method Not Allowed
	// HTTP method not supported
	ErrCodeMethodNotAllowed = "METHOD_NOT_ALLOWED"

	// 409 Conflict
	// Resource already exists or version conflict
	ErrCodeConflict = "CONFLICT"

	// 422 Unprocessable Entity (uses same code as 400 validation - semantic validation)
	// Semantic validation error
	ErrCodeUnprocessable = "UNPROCESSABLE"

	// 429 Too Many Requests
	// Rate limit exceeded
	ErrCodeRateLimit = "RATE_LIMITED"

	// 500 Internal Server Error
	// Server error
	ErrCodeInternal = "SERVER_ERROR"

	// 503 Service Unavailable
	// Maintenance mode or overloaded
	ErrCodeMaintenance = "MAINTENANCE"
)

// ErrorCodeToHTTP maps error codes to HTTP status codes
var ErrorCodeToHTTP = map[string]int{
	ErrCodeBadRequest:       400,
	ErrCodeValidation:       400,
	ErrCodeUnauthorized:     401,
	ErrCodeTokenExpired:     401,
	ErrCodeTokenInvalid:     401,
	ErrCode2FARequired:      401,
	ErrCode2FAInvalid:       401,
	ErrCodeForbidden:        403,
	ErrCodeAccountLocked:    403,
	ErrCodeNotFound:         404,
	ErrCodeMethodNotAllowed: 405,
	ErrCodeConflict:         409,
	ErrCodeUnprocessable:    422,
	ErrCodeRateLimit:        429,
	ErrCodeInternal:         500,
	ErrCodeMaintenance:      503,
}

// HTTPStatusCode returns the HTTP status code for an error code
func HTTPStatusCode(code string) int {
	if status, ok := ErrorCodeToHTTP[code]; ok {
		return status
	}
	// Default to internal error
	return 500
}

// HTTPToErrorCode maps HTTP status codes to default error codes
// Per AI.md PART 16: Unified Response Format
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
	503: ErrCodeMaintenance,
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
