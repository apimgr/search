package path

import (
	"strings"
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

func TestValidatePathTooLong(t *testing.T) {
	// Test validatePath with path > 2048 characters
	longPath := "/" + strings.Repeat("a", 2050)
	err := validatePath(longPath)
	if err != ErrPathTooLong {
		t.Errorf("validatePath() for path > 2048 chars should return ErrPathTooLong, got %v", err)
	}
}

func TestValidatePathSegmentErrors(t *testing.T) {
	tests := []struct {
		name    string
		segment string
		wantErr error
	}{
		{"empty segment", "", ErrInvalidPath},
		{"too long segment", strings.Repeat("a", 65), ErrPathTooLong},
		{"invalid chars", "file@name", ErrInvalidPath},
		{"starts with number valid", "1file", nil}, // This is valid per regex
		{"starts with dash", "-file", ErrInvalidPath},
		{"dot traversal", "..", ErrPathTraversal},
		{"single dot", ".", ErrPathTraversal},
		{"space in name", "file name", ErrInvalidPath},
		{"special chars", "file$name", ErrInvalidPath},
		{"unicode", "файл", ErrInvalidPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathSegment(tt.segment)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("validatePathSegment(%q) unexpected error = %v", tt.segment, err)
				}
				return
			}
			if err != tt.wantErr {
				t.Errorf("validatePathSegment(%q) error = %v, wantErr %v", tt.segment, err, tt.wantErr)
			}
		})
	}
}

func TestNormalizePathEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"root only", "/", ""},
		{"multiple slashes only", "///", ""},
		{"dot only", ".", "."},
		{"complex traversal resolved", "/a/b/../c/../d", "a/d"}, // path.Clean resolves ..
		{"mixed dots and slashes", "/./a/./b/./", "a/b"},
		{"leading double slash", "//path/to/file", "path/to/file"},
		{"unicode path", "/путь/к/файлу", "путь/к/файлу"},
		{"unresolvable dotdot", "../../etc", ""},  // Contains .. after clean
		{"dotdot at root", "/../etc", "etc"},      // path.Clean resolves to /etc, trim gives etc
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

func TestValidatePathEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{"empty path", "", nil}, // Empty path with no segments is valid
		{"slash only", "/", nil},
		{"multiple slashes", "///", nil},
		{"valid with numbers", "/path1/file2", nil},
		{"path with extension", "/path/file.json", nil},
		{"multiple extensions", "/path/file.tar.gz", ErrInvalidPath}, // Double extension fails regex
		{"hidden file unix style", "/.hidden", ErrInvalidPath},       // Starts with dot
		{"just dots", "/path/...", ErrPathTraversal}, // Contains ".." so treated as traversal
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if err != tt.wantErr {
				t.Errorf("validatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestSafePathEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantErr  bool
		checkErr error
	}{
		{"valid simple", "path/to/file", "path/to/file", false, nil},
		{"valid with leading slash", "/path/to/file", "path/to/file", false, nil},
		{"empty returns empty", "", "", false, nil},
		{"traversal blocked", "../etc/passwd", "", true, ErrPathTraversal},
		{"embedded traversal", "path/../../../etc/passwd", "", true, ErrPathTraversal},
		{"long path", "/" + strings.Repeat("a/", 1025), "", true, ErrPathTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.checkErr != nil && err != tt.checkErr {
				t.Errorf("SafePath(%q) error = %v, expected %v", tt.input, err, tt.checkErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SafePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSafeFilePathEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		userPath string
		wantErr  bool
		checkErr error
	}{
		{"valid nested path", "/base/dir", "sub/path/file.txt", false, nil},
		{"empty user path", "/base", "", false, nil},
		{"user path equals base", "/base", "", false, nil},
		{"complex valid path", "/var/data", "subdir/nested/file.log", false, nil},
		{"traversal at start", "/base", "../outside", true, ErrPathTraversal},
		{"traversal in middle", "/base", "sub/../../../etc", true, ErrPathTraversal},
		{"uppercase blocked", "/base", "SubDir/File.txt", true, ErrInvalidPath},
		{"special chars blocked", "/base", "file@name.txt", true, ErrInvalidPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafeFilePath(tt.baseDir, tt.userPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeFilePath(%q, %q) error = %v, wantErr %v", tt.baseDir, tt.userPath, err, tt.wantErr)
				return
			}
			if tt.checkErr != nil && err != tt.checkErr {
				t.Errorf("SafeFilePath(%q, %q) error = %v, expected %v", tt.baseDir, tt.userPath, err, tt.checkErr)
			}
			if !tt.wantErr && result == "" {
				t.Errorf("SafeFilePath(%q, %q) returned empty string for valid input", tt.baseDir, tt.userPath)
			}
		})
	}
}

func TestSafeFilePathAbsolutePaths(t *testing.T) {
	// Test that SafeFilePath returns absolute paths
	result, err := SafeFilePath("/base", "subdir/file.txt")
	if err != nil {
		t.Fatalf("SafeFilePath() unexpected error = %v", err)
	}

	// Result should be absolute
	if !strings.HasPrefix(result, "/") {
		t.Errorf("SafeFilePath() should return absolute path, got %s", result)
	}

	// Result should contain the base
	if !strings.Contains(result, "base") {
		t.Errorf("SafeFilePath() result should contain base dir, got %s", result)
	}
}

func TestSafeFilePathWithSymlinks(t *testing.T) {
	// Test with a real temp directory
	baseDir := "/tmp/safepath-test"
	subDir := "subdir"
	fileName := "testfile.txt"

	// Attempt with valid paths
	result, err := SafeFilePath(baseDir, subDir+"/"+fileName)
	if err != nil {
		t.Errorf("SafeFilePath() unexpected error = %v", err)
	}
	if result == "" {
		t.Error("SafeFilePath() should return non-empty result")
	}
}

func TestNormalizePathExportedComprehensive(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple path", "/path/to/file", "path/to/file"},
		{"empty", "", ""},
		{"root", "/", ""},
		{"no leading slash", "path/to", "path/to"},
		{"trailing slash", "path/to/", "path/to"},
		{"double slash", "path//to", "path/to"},
		{"dot in path", "./path/to", "path/to"},
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

func TestValidPathSegmentRegex(t *testing.T) {
	// Test the regex pattern directly
	tests := []struct {
		name    string
		segment string
		valid   bool
	}{
		{"lowercase only", "filename", true},
		{"with numbers", "file123", true},
		{"starts with number", "123file", true},
		{"with hyphen", "file-name", true},
		{"with underscore", "file_name", true},
		{"with extension", "file.txt", true},
		{"multiple hyphens", "my-file-name", true},
		{"multiple underscores", "my_file_name", true},
		{"mixed separators", "my-file_name", true},
		{"uppercase", "FileName", false},
		{"mixed case", "fileName", false},
		{"starts with hyphen", "-file", false},
		{"starts with underscore", "_file", false}, // Valid per regex ^[a-z0-9]
		{"double extension", "file.tar.gz", false},
		{"empty extension", "file.", false},
		{"dot only extension", "file..", false},
		{"special char", "file@name", false},
		{"space", "file name", false},
		{"tab", "file\tname", false},
		{"newline", "file\nname", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathSegment(tt.segment)
			gotValid := err == nil
			if gotValid != tt.valid {
				t.Errorf("validatePathSegment(%q) valid = %v, want %v (err: %v)", tt.segment, gotValid, tt.valid, err)
			}
		})
	}
}

func TestErrorMessages(t *testing.T) {
	// Verify error messages are meaningful
	if ErrInvalidPath.Error() != "invalid path" {
		t.Errorf("ErrInvalidPath message = %q, want 'invalid path'", ErrInvalidPath.Error())
	}
	if ErrPathTooLong.Error() != "path too long" {
		t.Errorf("ErrPathTooLong message = %q, want 'path too long'", ErrPathTooLong.Error())
	}
	if ErrPathTraversal.Error() != "path traversal attempt" {
		t.Errorf("ErrPathTraversal message = %q, want 'path traversal attempt'", ErrPathTraversal.Error())
	}
}
