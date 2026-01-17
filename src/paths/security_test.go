package path

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
		{"simple path", "/path/to/file", "path/to/file"},     // Strips leading/trailing slashes
		{"no leading slash", "path/to/file", "path/to/file"},
		{"double slashes", "path//to//file", "path/to/file"}, // path.Clean handles //
		{"trailing slash", "/path/to/", "path/to"},           // Strips trailing
		{"dot segments", "/path/./to", "path/to"},            // Removes .
		{"dotdot becomes empty", "../path", ""},              // Contains .. after clean
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
		name    string
		segment string
		wantErr bool
	}{
		{"valid", "filename", false},
		{"valid with dash", "file-name", false},
		{"valid with underscore", "file_name", false},
		{"valid with extension", "file.txt", false},
		{"dot dot", "..", true},
		{"single dot", ".", true},
		{"empty", "", true},
		// Uppercase is invalid per regex
		{"uppercase", "FileName", true},
		// Too long
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathSegment(tt.segment)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePathSegment(%q) error = %v, wantErr %v", tt.segment, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid lowercase", "/path/to/file", false},
		{"valid with extension", "/path/to/file.txt", false},
		{"contains dotdot", "/path/../file", true},
		{"starts with dotdot", "../file", true},
		// Empty segments are skipped (from //)
		{"double slash", "/path//file", false},
		// Uppercase fails validation
		{"uppercase", "/Path/To/File", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestSafePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid path", "/path/to/file", false},
		{"path traversal", "/path/../etc/passwd", true},
		// Uppercase is invalid
		{"uppercase", "/Path/To", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSafeFilePath(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		userPath string
		wantErr  bool
	}{
		{"valid", "/base", "subdir/file.txt", false},
		{"traversal attempt", "/base", "../etc/passwd", true},
		// SafeFilePath first validates with SafePath which checks for uppercase
		{"uppercase in path", "/base", "Subdir/file.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeFilePath(tt.baseDir, tt.userPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeFilePath(%q, %q) error = %v, wantErr %v", tt.baseDir, tt.userPath, err, tt.wantErr)
			}
		})
	}
}

func TestNormalizePathExported(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "/path/to/file", "path/to/file"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePath(tt.input)
			if got != tt.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPathErrors(t *testing.T) {
	// Test that error variables are defined
	if ErrInvalidPath == nil {
		t.Error("ErrInvalidPath should not be nil")
	}
	if ErrPathTooLong == nil {
		t.Error("ErrPathTooLong should not be nil")
	}
	if ErrPathTraversal == nil {
		t.Error("ErrPathTraversal should not be nil")
	}
}
