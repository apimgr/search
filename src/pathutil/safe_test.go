package pathutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"simple", "path/to/file", "path/to/file"},
		{"leading slash", "/path/to/file", "path/to/file"},
		{"trailing slash", "path/to/file/", "path/to/file"},
		{"both slashes", "/path/to/file/", "path/to/file"},
		{"double slashes", "path//to//file", "path/to/file"},
		{"dot", "path/./to/file", "path/to/file"},
		{"dotdot cleaned", "path/../to/file", "to/file"}, // path.Clean resolves this
		{"only dots", "..", ""},                          // Returns empty because it still contains ..
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePath(tt.input)
			if got != tt.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidatePathSegment(t *testing.T) {
	tests := []struct {
		segment string
		wantErr error
	}{
		{"valid", nil},
		{"with-hyphen", nil},
		{"with_underscore", nil},
		{"abc123", nil},
		{"", ErrInvalidPath},
		{".", ErrInvalidPath},  // Fails regex before traversal check
		{"..", ErrInvalidPath}, // Fails regex before traversal check
		{"InvalidUpper", ErrInvalidPath},
		{"has.dot", ErrInvalidPath},
		{"has/slash", ErrInvalidPath},
		{"has@special", ErrInvalidPath},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			err := validatePathSegment(tt.segment)
			if err != tt.wantErr {
				t.Errorf("validatePathSegment(%q) = %v, want %v", tt.segment, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePathSegmentTooLong(t *testing.T) {
	// Create a segment longer than 64 chars
	longSegment := ""
	for i := 0; i < 65; i++ {
		longSegment += "a"
	}

	err := validatePathSegment(longSegment)
	if err != ErrPathTooLong {
		t.Errorf("validatePathSegment(long) = %v, want %v", err, ErrPathTooLong)
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{"valid simple", "path/to/file", nil},
		{"valid with leading slash", "/path/to/file", nil},
		{"valid with double slash", "path//to//file", nil},
		{"contains dotdot", "path/../file", ErrPathTraversal},
		{"starts with dotdot", "../path/file", ErrPathTraversal},
		{"ends with dotdot", "path/file/..", ErrPathTraversal},
		{"invalid chars", "Path/To/File", ErrInvalidPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if err != tt.wantErr {
				t.Errorf("validatePath(%q) = %v, want %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePathTooLong(t *testing.T) {
	// Create a path longer than 2048 chars
	longPath := ""
	for i := 0; i < 2049; i++ {
		longPath += "a"
	}

	err := validatePath(longPath)
	if err != ErrPathTooLong {
		t.Errorf("validatePath(long) = %v, want %v", err, ErrPathTooLong)
	}
}

func TestSafePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid", "path/to/file", "path/to/file", false},
		{"with slashes", "/path/to/file/", "path/to/file", false},
		{"traversal", "../etc/passwd", "", true},
		{"uppercase", "Path/To/File", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SafePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSafeFilePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid", "file.txt", "file.txt", false},
		{"with path", "path/to/file.txt", "path/to/file.txt", false},
		{"with slashes", "/path/to/file.txt/", "path/to/file.txt", false},
		{"traversal", "../etc/passwd", "", true},
		{"dot alone", ".", ".", false}, // path.Clean returns "."
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeFilePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeFilePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SafeFilePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSafeFilePathTooLong(t *testing.T) {
	// Create a path longer than 2048 chars
	longPath := ""
	for i := 0; i < 2049; i++ {
		longPath += "a"
	}

	_, err := SafeFilePath(longPath)
	if err != ErrPathTooLong {
		t.Errorf("SafeFilePath(long) error = %v, want %v", err, ErrPathTooLong)
	}
}

func TestSafeFilePathWithBase(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		userPath string
		want     string
		wantErr  bool
	}{
		{"valid", "/data", "uploads/file", "/data/uploads/file", false},
		{"simple file", "/data", "file", "/data/file", false},
		{"traversal attempt", "/data", "../etc/passwd", "", true},
		{"traversal hidden", "/data", "uploads/../../../etc/passwd", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeFilePathWithBase(tt.baseDir, tt.userPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeFilePathWithBase(%q, %q) error = %v, wantErr %v", tt.baseDir, tt.userPath, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SafeFilePathWithBase(%q, %q) = %q, want %q", tt.baseDir, tt.userPath, got, tt.want)
			}
		})
	}
}

func TestErrorVariables(t *testing.T) {
	errors := []error{
		ErrPathTraversal,
		ErrInvalidPath,
		ErrPathTooLong,
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

// Tests for PathSecurityMiddleware (middleware.go)

func TestPathSecurityMiddleware_NormalRequest(t *testing.T) {
	handler := PathSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantPath   string
	}{
		{"simple path", "/api/users", http.StatusOK, "/api/users"},
		{"root path", "/", http.StatusOK, "/"},
		{"path with trailing slash", "/api/users/", http.StatusOK, "/api/users/"},
		{"path needing normalization", "/api//users", http.StatusOK, "/api/users"},
		{"path with single dot", "/api/./users", http.StatusOK, "/api/users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestPathSecurityMiddleware_BlocksTraversal(t *testing.T) {
	handler := PathSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name string
		path string
	}{
		{"dotdot in path", "/api/../etc/passwd"},
		{"dotdot at start", "/../etc/passwd"},
		{"dotdot at end", "/api/.."},
		{"multiple dotdot", "/api/../../etc/passwd"},
		{"hidden dotdot", "/api/users/../../../etc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestPathSecurityMiddleware_BlocksEncodedTraversal(t *testing.T) {
	handler := PathSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name    string
		rawPath string
	}{
		{"encoded dot lowercase", "/api/%2e%2e/etc/passwd"},
		{"encoded dot uppercase", "/api/%2E%2E/etc/passwd"},
		{"mixed case encoded", "/api/%2e%2E/etc/passwd"},
		{"single encoded dot", "/api/%2e/etc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.URL.RawPath = tt.rawPath
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestPathSecurityMiddleware_RawPathTraversal(t *testing.T) {
	handler := PathSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test when RawPath contains .. but decoded Path doesn't
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.URL.RawPath = "/api/../etc/passwd"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for RawPath traversal", rec.Code, http.StatusBadRequest)
	}
}

func TestPathSecurityMiddleware_EmptyRawPath(t *testing.T) {
	// Test the case where RawPath is empty (line 20-22 in middleware.go)
	handler := PathSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	// Ensure RawPath is empty - this is the default
	req.URL.RawPath = ""
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestPathSecurityMiddleware_TrailingSlashPreserved(t *testing.T) {
	var receivedPath string
	handler := PathSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{"trailing slash preserved", "/api/users/", "/api/users/"},
		{"no trailing slash", "/api/users", "/api/users"},
		{"root with slash", "/", "/"},
		{"double slash with trailing", "/api//users/", "/api/users/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedPath = ""
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if receivedPath != tt.wantPath {
				t.Errorf("path = %q, want %q", receivedPath, tt.wantPath)
			}
		})
	}
}

func TestPathSecurityMiddleware_LeadingSlashAdded(t *testing.T) {
	// Test that leading slash is added when missing after Clean
	// path.Clean("") returns "." and path.Clean("/") returns "/"
	// This tests the edge case on lines 37-39
	var receivedPath string
	handler := PathSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Manually modify the path to test edge case
	req.URL.Path = "api/users" // No leading slash
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if receivedPath != "/api/users" {
		t.Errorf("path = %q, want %q", receivedPath, "/api/users")
	}
}

// Additional edge case tests for safe.go

func TestSafeFilePath_EmptyAfterClean(t *testing.T) {
	// Test empty input which becomes empty after clean (line 113-114)
	_, err := SafeFilePath("")
	if err != ErrInvalidPath {
		t.Errorf("SafeFilePath(\"\") error = %v, want %v", err, ErrInvalidPath)
	}

	// Test input that becomes empty after trim
	_, err = SafeFilePath("/")
	if err != ErrInvalidPath {
		t.Errorf("SafeFilePath(\"/\") error = %v, want %v", err, ErrInvalidPath)
	}
}

func TestSafeFilePathWithBase_PathEqualsBase(t *testing.T) {
	// Test case where absPath equals absBase (line 137)
	// This should fail because the path equals the base directory
	got, err := SafeFilePathWithBase("/data", "")
	// Empty path through SafePath returns empty, so join results in just "/data"
	// absPath == absBase check should allow this
	if err != nil {
		t.Errorf("SafeFilePathWithBase(\"/data\", \"\") error = %v, want nil", err)
	}
	if got != "/data" {
		t.Errorf("SafeFilePathWithBase(\"/data\", \"\") = %q, want %q", got, "/data")
	}
}

func TestSafeFilePathWithBase_PathOutsideBase(t *testing.T) {
	// Test various ways path could escape base directory
	tests := []struct {
		name     string
		baseDir  string
		userPath string
	}{
		{"simple escape", "/data", "../other"},
		{"double escape", "/data/uploads", "../../other"},
		{"sibling directory", "/data", "../data2/file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeFilePathWithBase(tt.baseDir, tt.userPath)
			if err == nil {
				t.Errorf("SafeFilePathWithBase(%q, %q) should return error", tt.baseDir, tt.userPath)
			}
		})
	}
}

func TestValidatePathSegment_ExactMatch(t *testing.T) {
	// Test the exact . and .. check (lines 57-58)
	// Note: These fail the regex check first, so we can't easily reach lines 57-58
	// The regex `^[a-z0-9_-]+$` fails for "." and ".." because they don't match

	// Test that single dot fails (caught by regex)
	err := validatePathSegment(".")
	if err != ErrInvalidPath {
		t.Errorf("validatePathSegment(\".\") = %v, want %v", err, ErrInvalidPath)
	}

	// Test that double dot fails (caught by regex)
	err = validatePathSegment("..")
	if err != ErrInvalidPath {
		t.Errorf("validatePathSegment(\"..\") = %v, want %v", err, ErrInvalidPath)
	}
}

func TestNormalizePath_AdditionalCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"multiple leading slashes", "///path/to/file", "path/to/file"},
		{"multiple trailing slashes", "path/to/file///", "path/to/file"},
		{"only slashes", "///", ""},
		{"single slash", "/", ""},
		{"dot in middle", "path/./file", "path/file"},
		{"complex traversal", "a/b/../c/d/../e", "a/c/e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePath(tt.input)
			if got != tt.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidatePath_AdditionalCases(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{"empty segments from double slash", "path//to//file", nil},
		{"only empty segments", "///", nil},
		{"segment with number", "v1/api/users", nil},
		{"segment with underscore", "api_v1/users", nil},
		{"segment with hyphen", "api-v1/users", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if err != tt.wantErr {
				t.Errorf("validatePath(%q) = %v, want %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestSafePath_AdditionalCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty input", "", "", false},
		{"single segment", "file", "file", false},
		{"nested path", "a/b/c/d/e", "a/b/c/d/e", false},
		{"with underscores", "path_to/file_name", "path_to/file_name", false},
		{"with hyphens", "path-to/file-name", "path-to/file-name", false},
		{"with numbers", "v1/api2/route3", "v1/api2/route3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SafePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSafeFilePath_AdditionalCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"nested with extension", "path/to/file.txt", "path/to/file.txt", false},
		{"multiple extensions", "file.tar.gz", "file.tar.gz", false},
		{"hidden file", ".hidden", ".hidden", false},
		{"dot in directory", "dir.name/file.txt", "dir.name/file.txt", false},
		{"complex path", "a/b/c/d/e.txt", "a/b/c/d/e.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeFilePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeFilePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SafeFilePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSafeFilePathWithBase_AdditionalCases(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		userPath string
		want     string
		wantErr  bool
	}{
		{"nested user path", "/data", "a/b/c/file", "/data/a/b/c/file", false},
		{"base with trailing slash", "/data/", "file", "/data/file", false},
		{"deep nesting", "/data", "a/b/c/d/e/f", "/data/a/b/c/d/e/f", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeFilePathWithBase(tt.baseDir, tt.userPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeFilePathWithBase(%q, %q) error = %v, wantErr %v", tt.baseDir, tt.userPath, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SafeFilePathWithBase(%q, %q) = %q, want %q", tt.baseDir, tt.userPath, got, tt.want)
			}
		})
	}
}
