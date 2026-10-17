package update

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// copyFile copies src over dst, creating dst with executable permissions.
//
// It is the fallback used when a rename cannot cross the filesystem boundary
// between the download/backup directory and the install directory. The copy is
// staged through a temp file in dst's own directory and renamed into place, so
// a failure part-way through never leaves a truncated binary at dst.
//
// dst and src are (dst, src) ordered to match os.Rename's convention, which
// every caller in this package already uses.
func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			slog.Warn("failed to close update source file", "error", err)
		}
	}()

	// Stage next to dst so the final rename stays within one filesystem.
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// No-op on the success path, where the rename has already consumed it.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, in); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Sync(); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// The destination is a binary or a staged backup that will be installed,
	// so it is always created executable.
	if err := os.Chmod(tmpName, 0755); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}
