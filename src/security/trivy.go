package security

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	trivyDBLayerMediaType       = types.MediaType("application/vnd.aquasec.trivy.db.layer.v1.tar+gzip")
	trivyDBMaxBytes       int64 = 2 << 30
	trivyMetadataMaxBytes int64 = 1 << 20
	trivyArchiveMaxBytes  int64 = trivyDBMaxBytes + 16<<20
)

var fetchTrivyImage = remote.Image

var sqliteFileHeader = []byte("SQLite format 3\x00")

// HasTrivyDB reports whether a bounded, SQLite-formatted Trivy database exists.
func (m *CVEManager) HasTrivyDB() bool {
	path := filepath.Join(m.securityDataDir(), "trivy", "trivy.db")
	return validTrivyDB(path)
}

func validTrivyDB(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= int64(len(sqliteFileHeader)) || info.Size() > trivyDBMaxBytes {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer closeutil.Close(file)
	header := make([]byte, len(sqliteFileHeader))
	_, err = io.ReadFull(file, header)
	return err == nil && bytes.Equal(header, sqliteFileHeader)
}

// TrivyHealthStatus validates the installed database and its public metadata.
// A database past NextUpdate remains usable but reports degraded.
func (m *CVEManager) TrivyHealthStatus() string {
	dir := filepath.Join(m.securityDataDir(), "trivy")
	if !m.HasTrivyDB() {
		return "error"
	}
	updated, err := os.ReadFile(filepath.Join(dir, ".last_updated"))
	if err != nil {
		return "error"
	}
	updatedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(string(updated)))
	if err != nil || updatedAt.Location() != time.UTC || updatedAt.Nanosecond() != 0 || updatedAt.Format(time.RFC3339) != strings.TrimSpace(string(updated)) {
		return "error"
	}
	metadata, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil || len(metadata) == 0 || int64(len(metadata)) > trivyMetadataMaxBytes {
		return "error"
	}
	nextUpdate, err := trivyNextUpdate(metadata)
	if err != nil {
		return "error"
	}
	if time.Now().After(nextUpdate) {
		return "degraded"
	}
	return "ok"
}

func trivyNextUpdate(metadata []byte) (time.Time, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(metadata, &fields); err != nil {
		return time.Time{}, err
	}
	for key, raw := range fields {
		if strings.EqualFold(key, "nextUpdate") {
			var nextUpdate time.Time
			if err := json.Unmarshal(raw, &nextUpdate); err != nil {
				return time.Time{}, err
			}
			if nextUpdate.IsZero() {
				return time.Time{}, errors.New("trivy NextUpdate is empty")
			}
			return nextUpdate, nil
		}
	}
	return time.Time{}, errors.New("trivy metadata has no NextUpdate")
}

// UpdateTrivyDB fetches the OCI database layer, extracts only its two expected
// files into a bounded staging directory, then atomically swaps the directory.
func (m *CVEManager) UpdateTrivyDB(ctx context.Context, source string) error {
	if m.TrivyHealthStatus() == "ok" {
		return nil
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "ghcr.io/aquasecurity/trivy-db:2"
	}
	ref, err := name.ParseReference(source)
	if err != nil {
		return fmt.Errorf("parse trivy OCI reference: %w", err)
	}
	img, err := fetchTrivyImage(ref, remote.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("fetch trivy OCI artifact: %w", err)
	}
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("read trivy OCI layers: %w", err)
	}
	var dbLayer v1.Layer
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			return fmt.Errorf("read trivy layer media type: %w", err)
		}
		if mediaType == trivyDBLayerMediaType {
			if dbLayer != nil {
				return errors.New("trivy OCI artifact contains multiple database layers")
			}
			dbLayer = layer
		}
	}
	if dbLayer == nil {
		return errors.New("trivy OCI artifact has no database layer")
	}
	layerSize, err := dbLayer.Size()
	if err != nil {
		return fmt.Errorf("read trivy database size: %w", err)
	}
	if layerSize <= 0 || layerSize > trivyDBMaxBytes {
		return errors.New("trivy database layer size is invalid")
	}
	reader, err := dbLayer.Compressed()
	if err != nil {
		return fmt.Errorf("read compressed trivy database: %w", err)
	}
	defer closeutil.Close(reader)
	securityDir := m.securityDataDir()
	if err := os.MkdirAll(securityDir, 0750); err != nil {
		return fmt.Errorf("create security data directory: %w", err)
	}
	stage, err := os.MkdirTemp(securityDir, ".trivy-update-*")
	if err != nil {
		return fmt.Errorf("create Trivy staging directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(stage); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove Trivy staging directory", "error", err)
		}
	}()
	if err := extractTrivyDatabase(reader, stage); err != nil {
		return fmt.Errorf("extract Trivy database: %w", err)
	}
	if err := writeSecurityDataTimestamp(stage, time.Now().UTC()); err != nil {
		return err
	}
	return replaceTrivyDirectory(stage, filepath.Join(securityDir, "trivy"))
}

func extractTrivyDatabase(reader io.Reader, destination string) error {
	compressed := &io.LimitedReader{R: reader, N: trivyDBMaxBytes + 1}
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return err
	}
	defer closeutil.Close(gz)
	uncompressed := &io.LimitedReader{R: gz, N: trivyArchiveMaxBytes + 1}
	archive := tar.NewReader(uncompressed)
	seenDB, seenMetadata := false, false
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(filepath.FromSlash(header.Name))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return errors.New("trivy archive contains an unsafe path")
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if clean != "trivy.db" && clean != "metadata.json" {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 {
			return errors.New("invalid Trivy data file")
		}
		limit := trivyDBMaxBytes
		if clean == "metadata.json" {
			limit = trivyMetadataMaxBytes
		}
		if header.Size > limit {
			return errors.New("trivy data file exceeds its size limit")
		}
		if (clean == "trivy.db" && seenDB) || (clean == "metadata.json" && seenMetadata) {
			return errors.New("duplicate Trivy data file")
		}
		file, err := os.OpenFile(filepath.Join(destination, clean), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(file, io.LimitReader(archive, limit+1))
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
			return err
		}
		if written != header.Size {
			return errors.New("trivy data file size mismatch")
		}
		if clean == "trivy.db" {
			seenDB = true
		} else {
			seenMetadata = true
		}
	}
	if _, err := io.Copy(io.Discard, uncompressed); err != nil {
		return err
	}
	if compressed.N <= 0 || uncompressed.N <= 0 {
		return errors.New("trivy archive exceeds its size limit")
	}
	if !seenDB || !seenMetadata {
		return errors.New("trivy layer is missing trivy.db or metadata.json")
	}
	if !validTrivyDB(filepath.Join(destination, "trivy.db")) {
		return errors.New("trivy database file is invalid")
	}
	metadata, err := os.ReadFile(filepath.Join(destination, "metadata.json"))
	if err != nil {
		return err
	}
	if _, err := trivyNextUpdate(metadata); err != nil {
		return err
	}
	return nil
}

func replaceTrivyDirectory(stage, target string) error {
	parent := filepath.Dir(target)
	backup, err := os.MkdirTemp(parent, ".trivy-old-*")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	hadTarget := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		hadTarget = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		return err
	}
	if hadTarget {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove prior Trivy database: %w", err)
		}
	}
	return nil
}

func writeSecurityDataTimestamp(dir string, updated time.Time) error {
	tmp, err := os.CreateTemp(dir, ".timestamp-*.tmp")
	if err != nil {
		return fmt.Errorf("create security database timestamp: %w", err)
	}
	name := tmp.Name()
	defer func() {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove temporary security database timestamp", "error", err)
		}
	}()
	if _, err := fmt.Fprintln(tmp, updated.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("write security database timestamp: %w", errors.Join(err, tmp.Close()))
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync security database timestamp: %w", errors.Join(err, tmp.Close()))
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close security database timestamp: %w", err)
	}
	if err := os.Rename(name, filepath.Join(dir, ".last_updated")); err != nil {
		return fmt.Errorf("replace security database timestamp: %w", err)
	}
	return nil
}
