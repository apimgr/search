package path

import (
	"errors"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Path security errors per AI.md PART 5
var (
	ErrInvalidPath   = errors.New("invalid path")
	ErrPathTooLong   = errors.New("path too long")
	ErrPathTraversal = errors.New("path traversal attempt")
)

// validPathSegment allows lowercase alphanumeric, hyphens, and underscores
// No uppercase, no special chars, no dots except in filenames with extensions
var validPathSegment = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(\.[a-z0-9]+)?$`)

// normalizePath handles multiple slashes, leading/trailing slashes, etc.
// Per AI.md PART 5 PATH SECURITY (NON-NEGOTIABLE)
func normalizePath(input string) string {
	// Handle empty
	if input == "" {
		return ""
	}

	// Use path.Clean to handle .., ., and //
	cleaned := path.Clean(input)

	// Strip leading/trailing slashes
	cleaned = strings.Trim(cleaned, "/")

	// Reject if still contains .. after cleaning (shouldn't happen, but be safe)
	if strings.Contains(cleaned, "..") {
		return ""
	}

	return cleaned
}

// validatePathSegment checks a single path segment (e.g., "admin" in "/admin/dashboard")
// Per AI.md PART 5 PATH SECURITY (NON-NEGOTIABLE)
func validatePathSegment(segment string) error {
	if segment == "" {
		return ErrInvalidPath
	}
	// Check for traversal before other validation
	if segment == "." || segment == ".." {
		return ErrPathTraversal
	}
	if len(segment) > 64 {
		return ErrPathTooLong
	}
	if !validPathSegment.MatchString(segment) {
		return ErrInvalidPath
	}
	return nil
}

// validatePath checks an entire path
// Per AI.md PART 5 PATH SECURITY (NON-NEGOTIABLE)
func validatePath(p string) error {
	if len(p) > 2048 {
		return ErrPathTooLong
	}

	// Check for traversal attempts before normalization
	if strings.Contains(p, "..") {
		return ErrPathTraversal
	}

	// Check each segment
	segments := strings.Split(strings.Trim(p, "/"), "/")
	for _, seg := range segments {
		if seg == "" {
			continue // Skip empty (from //)
		}
		if err := validatePathSegment(seg); err != nil {
			return err
		}
	}

	return nil
}

// SafePath normalizes and validates - returns error if invalid
// Per AI.md PART 5 PATH SECURITY (NON-NEGOTIABLE)
func SafePath(input string) (string, error) {
	if err := validatePath(input); err != nil {
		return "", err
	}
	return normalizePath(input), nil
}

// SafeFilePath ensures path stays within base directory
// Per AI.md PART 5 FILE PATH SECURITY (NON-NEGOTIABLE)
func SafeFilePath(baseDir, userPath string) (string, error) {
	// Normalize user input
	safe, err := SafePath(userPath)
	if err != nil {
		return "", err
	}

	// Construct full path
	fullPath := filepath.Join(baseDir, safe)

	// Resolve to absolute
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}

	// Verify path is still within base
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", ErrPathTraversal
	}

	return absPath, nil
}

// NormalizePath is a simple path normalizer (no validation)
// Use SafePath() for user input that needs validation
func NormalizePath(input string) string {
	return normalizePath(input)
}
