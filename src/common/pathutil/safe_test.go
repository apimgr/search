package pathutil

import (
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
