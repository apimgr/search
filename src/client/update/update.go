// Package update installs verified client updates from the configured server.
package update

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// ErrRestartStarted indicates that a replacement process was started on
// platforms that cannot replace the currently running process image.
var ErrRestartStarted = errors.New("updated client process started")

const maxClientBinarySize = 512 << 20

// DownloadAndInstall fetches a platform binary, verifies its published SHA-256,
// and atomically replaces the running executable.
func DownloadAndInstall(ctx context.Context, client *http.Client, baseURL, token, binaryName, expectedSHA256 string) (installedPath string, resultErr error) {
	if client == nil {
		client = http.DefaultClient
	}
	if len(expectedSHA256) != sha256.Size*2 {
		return "", fmt.Errorf("server supplied an invalid SHA-256 checksum")
	}
	expected, err := hex.DecodeString(expectedSHA256)
	if err != nil {
		return "", fmt.Errorf("server supplied an invalid SHA-256 checksum: %w", err)
	}
	if filepath.Base(binaryName) != binaryName || strings.ContainsAny(binaryName, `/\\`) {
		return "", fmt.Errorf("invalid client binary name")
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil || parsedBase.Host == "" || parsedBase.User != nil || parsedBase.Scheme != "https" {
		return "", fmt.Errorf("client updates require HTTPS")
	}
	parsedBase.Path = strings.TrimRight(parsedBase.Path, "/") + "/cli/binaries/" + url.PathEscape(binaryName)
	parsedBase.RawQuery = ""
	parsedBase.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedBase.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create client update request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	clientCopy := *client
	if clientCopy.Timeout < 30*time.Minute {
		clientCopy.Timeout = 30 * time.Minute
	}
	previousRedirectPolicy := client.CheckRedirect
	clientCopy.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if next.URL.Scheme != parsedBase.Scheme || !strings.EqualFold(next.URL.Host, parsedBase.Host) {
			return fmt.Errorf("client update redirect changed origin")
		}
		if previousRedirectPolicy != nil {
			return previousRedirectPolicy(next, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many client update redirects")
		}
		return nil
	}
	resp, err := clientCopy.Do(req)
	if err != nil {
		return "", fmt.Errorf("download client update: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close client update response: %w", err))
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("client update download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxClientBinarySize {
		return "", fmt.Errorf("client update exceeds maximum size of %d bytes", maxClientBinarySize)
	}

	orgDir := filepath.Join(os.TempDir(), config.ProjectOrg)
	if err := os.MkdirAll(orgDir, 0700); err != nil {
		return "", fmt.Errorf("create update temp directory: %w", err)
	}
	tempDir, err := os.MkdirTemp(orgDir, config.ProjectName+"-")
	if err != nil {
		return "", fmt.Errorf("create update temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove client update temp directory: %w", err))
		}
	}()
	tempPath := filepath.Join(tempDir, "cli.update.tmp")
	f, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("create staged update: %w", err)
	}
	h := sha256.New()
	if _, err := writeBounded(f, resp.Body, h, maxClientBinarySize); err != nil {
		return "", errors.Join(fmt.Errorf("write staged update: %w", err), closeFile(f))
	}
	if err := f.Sync(); err != nil {
		return "", errors.Join(fmt.Errorf("sync staged update: %w", err), closeFile(f))
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close staged update: %w", err)
	}
	if subtle.ConstantTimeCompare(h.Sum(nil), expected) != 1 {
		return "", fmt.Errorf("client update SHA-256 mismatch")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tempPath, 0755); err != nil {
			return "", fmt.Errorf("set staged update permissions: %w", err)
		}
	}
	currentPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate installed client binary: %w", err)
	}
	if err := replaceBinary(currentPath, tempPath); err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("you do not have permission to update %s; ask your admin or move the binary to a writable path", currentPath)
		}
		return "", fmt.Errorf("replace client binary %s: %w", currentPath, err)
	}
	return currentPath, nil
}

func writeBounded(dst io.Writer, src io.Reader, digest hash.Hash, limit int64) (int64, error) {
	written, err := io.Copy(io.MultiWriter(dst, digest), io.LimitReader(src, limit+1))
	if err != nil {
		return written, err
	}
	if written > limit {
		return written, fmt.Errorf("client update exceeds maximum size of %d bytes", limit)
	}
	return written, nil
}

func closeFile(f *os.File) error {
	if err := f.Close(); err != nil {
		return fmt.Errorf("close staged update: %w", err)
	}
	return nil
}

func copyStagedBinary(stagedPath, destination string) (resultErr error) {
	info, err := os.Stat(stagedPath)
	if err != nil {
		return fmt.Errorf("stat staged client binary: %w", err)
	}
	target, err := os.CreateTemp(filepath.Dir(destination), ".search-cli-update-*")
	if err != nil {
		return fmt.Errorf("create atomic replacement file: %w", err)
	}
	targetPath := target.Name()
	defer func() {
		if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove atomic replacement file: %w", err))
		}
	}()
	source, err := os.Open(stagedPath)
	if err != nil {
		return fmt.Errorf("open staged client binary: %w", err)
	}
	_, copyErr := io.Copy(target, source)
	closeSourceErr := source.Close()
	if copyErr != nil || closeSourceErr != nil {
		return errors.Join(copyErr, closeSourceErr, target.Close())
	}
	if err := target.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync atomic replacement file: %w", err), target.Close())
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("close atomic replacement file: %w", err)
	}
	if err := os.Chmod(targetPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("set atomic replacement permissions: %w", err)
	}
	if err := os.Rename(targetPath, destination); err != nil {
		return fmt.Errorf("install atomic replacement file: %w", err)
	}
	return nil
}

// Restart re-executes the installed CLI with its original arguments.
func Restart(path string) error { return restart(path) }
