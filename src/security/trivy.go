package security

import (
	"context"
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
)

var fetchTrivyImage = remote.Image

// HasTrivyDB reports whether a non-empty Trivy database is installed.
func (m *CVEManager) HasTrivyDB() bool {
	info, err := os.Stat(filepath.Join(m.securityDataDir(), "trivy", "db.tar.gz"))
	return err == nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= trivyDBMaxBytes
}

// UpdateTrivyDB fetches the trivy database layer from its OCI artifact and
// atomically installs the compressed database in the configured security dir.
func (m *CVEManager) UpdateTrivyDB(ctx context.Context, source string) error {
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
				return fmt.Errorf("trivy OCI artifact contains multiple database layers")
			}
			dbLayer = layer
		}
	}
	if dbLayer == nil {
		return fmt.Errorf("trivy OCI artifact has no database layer")
	}
	layerSize, err := dbLayer.Size()
	if err != nil {
		return fmt.Errorf("read trivy database size: %w", err)
	}
	if layerSize <= 0 || layerSize > trivyDBMaxBytes {
		return fmt.Errorf("trivy database size is invalid: %d bytes", layerSize)
	}
	reader, err := dbLayer.Compressed()
	if err != nil {
		return fmt.Errorf("read compressed trivy database: %w", err)
	}
	defer closeutil.Close(reader)
	dir := filepath.Join(m.securityDataDir(), "trivy")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create trivy database directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".db-*.tmp")
	if err != nil {
		return fmt.Errorf("create trivy database temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err := os.Remove(tmpName); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove temporary Trivy database", "error", err)
		}
	}()
	written, err := io.Copy(tmp, io.LimitReader(reader, trivyDBMaxBytes+1))
	if err != nil {
		return fmt.Errorf("write trivy database: %w", errors.Join(err, tmp.Close()))
	}
	if written == 0 || written > trivyDBMaxBytes {
		return errors.Join(fmt.Errorf("trivy database size is invalid: %d bytes", written), tmp.Close())
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync trivy database: %w", errors.Join(err, tmp.Close()))
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close trivy database: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, "db.tar.gz")); err != nil {
		return fmt.Errorf("replace trivy database: %w", err)
	}
	if err := writeSecurityDataTimestamp(dir, time.Now().UTC()); err != nil {
		return err
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
