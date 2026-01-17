package model

import "testing"

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{ErrCodeBadRequest, "BAD_REQUEST"},
		{ErrCodeValidation, "VALIDATION_FAILED"},
		{ErrCodeUnauthorized, "UNAUTHORIZED"},
		{ErrCodeTokenExpired, "TOKEN_EXPIRED"},
		{ErrCodeTokenInvalid, "TOKEN_INVALID"},
		{ErrCode2FARequired, "2FA_REQUIRED"},
		{ErrCode2FAInvalid, "2FA_INVALID"},
		{ErrCodeForbidden, "FORBIDDEN"},
		{ErrCodeAccountLocked, "ACCOUNT_LOCKED"},
		{ErrCodeNotFound, "NOT_FOUND"},
		{ErrCodeMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{ErrCodeConflict, "CONFLICT"},
		{ErrCodeUnprocessable, "UNPROCESSABLE"},
		{ErrCodeRateLimit, "RATE_LIMITED"},
		{ErrCodeInternal, "SERVER_ERROR"},
		{ErrCodeMaintenance, "MAINTENANCE"},
	}

	for _, tt := range tests {
		if tt.code != tt.want {
			t.Errorf("Error code = %q, want %q", tt.code, tt.want)
		}
	}
}

func TestHTTPStatusCode(t *testing.T) {
	tests := []struct {
		code   string
		status int
	}{
		{ErrCodeBadRequest, 400},
		{ErrCodeValidation, 400},
		{ErrCodeUnauthorized, 401},
		{ErrCodeTokenExpired, 401},
		{ErrCodeTokenInvalid, 401},
		{ErrCode2FARequired, 401},
		{ErrCode2FAInvalid, 401},
		{ErrCodeForbidden, 403},
		{ErrCodeAccountLocked, 403},
		{ErrCodeNotFound, 404},
		{ErrCodeMethodNotAllowed, 405},
		{ErrCodeConflict, 409},
		{ErrCodeUnprocessable, 422},
		{ErrCodeRateLimit, 429},
		{ErrCodeInternal, 500},
		{ErrCodeMaintenance, 503},
		{"UNKNOWN_CODE", 500}, // Should default to 500
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := HTTPStatusCode(tt.code)
			if got != tt.status {
				t.Errorf("HTTPStatusCode(%q) = %d, want %d", tt.code, got, tt.status)
			}
		})
	}
}

func TestErrorCodeFromHTTP(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{400, ErrCodeBadRequest},
		{401, ErrCodeUnauthorized},
		{403, ErrCodeForbidden},
		{404, ErrCodeNotFound},
		{405, ErrCodeMethodNotAllowed},
		{409, ErrCodeConflict},
		{422, ErrCodeUnprocessable},
		{429, ErrCodeRateLimit},
		{500, ErrCodeInternal},
		{503, ErrCodeMaintenance},
		{418, ErrCodeBadRequest},    // Unknown 4xx defaults to BAD_REQUEST
		{502, ErrCodeInternal},      // Unknown 5xx defaults to SERVER_ERROR
		{501, ErrCodeInternal},      // Unknown 5xx defaults to SERVER_ERROR
		{200, ErrCodeInternal},      // Non-error status defaults to SERVER_ERROR
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.status)), func(t *testing.T) {
			got := ErrorCodeFromHTTP(tt.status)
			if got != tt.code {
				t.Errorf("ErrorCodeFromHTTP(%d) = %q, want %q", tt.status, got, tt.code)
			}
		})
	}
}

func TestErrorCodeToHTTPMap(t *testing.T) {
	// Verify all error codes have mappings
	codes := []string{
		ErrCodeBadRequest,
		ErrCodeValidation,
		ErrCodeUnauthorized,
		ErrCodeTokenExpired,
		ErrCodeTokenInvalid,
		ErrCode2FARequired,
		ErrCode2FAInvalid,
		ErrCodeForbidden,
		ErrCodeAccountLocked,
		ErrCodeNotFound,
		ErrCodeMethodNotAllowed,
		ErrCodeConflict,
		ErrCodeUnprocessable,
		ErrCodeRateLimit,
		ErrCodeInternal,
		ErrCodeMaintenance,
	}

	for _, code := range codes {
		if _, ok := ErrorCodeToHTTP[code]; !ok {
			t.Errorf("ErrorCodeToHTTP missing mapping for %q", code)
		}
	}
}

func TestHTTPToErrorCodeMap(t *testing.T) {
	// Verify expected HTTP statuses have mappings
	statuses := []int{400, 401, 403, 404, 405, 409, 422, 429, 500, 503}

	for _, status := range statuses {
		if _, ok := HTTPToErrorCode[status]; !ok {
			t.Errorf("HTTPToErrorCode missing mapping for %d", status)
		}
	}
}

func TestDomainErrors(t *testing.T) {
	// Verify error variables are properly defined
	errors := []error{
		ErrEmptyQuery,
		ErrInvalidCategory,
		ErrEngineNotFound,
		ErrEngineDisabled,
		ErrEngineUnavailable,
		ErrEngineTimeout,
		ErrEngineRateLimit,
		ErrNoResults,
		ErrNoEngines,
		ErrSearchTimeout,
		ErrInvalidConfig,
		ErrMissingConfig,
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

func TestDomainErrorMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{ErrEmptyQuery, "query text cannot be empty"},
		{ErrInvalidCategory, "invalid category"},
		{ErrEngineNotFound, "engine not found"},
		{ErrEngineDisabled, "engine is disabled"},
		{ErrEngineUnavailable, "engine is unavailable"},
		{ErrEngineTimeout, "engine request timed out"},
		{ErrEngineRateLimit, "engine rate limit exceeded"},
		{ErrNoResults, "no results found"},
		{ErrNoEngines, "no engines available"},
		{ErrSearchTimeout, "search request timed out"},
		{ErrInvalidConfig, "invalid configuration"},
		{ErrMissingConfig, "missing required configuration"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if tt.err.Error() != tt.want {
				t.Errorf("Error message = %q, want %q", tt.err.Error(), tt.want)
			}
		})
	}
}
