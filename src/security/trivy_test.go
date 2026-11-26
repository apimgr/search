package security

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/fake"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

type fixtureTrivyLayer struct {
	data      []byte
	mediaType types.MediaType
}

func (l fixtureTrivyLayer) Digest() (v1.Hash, error) { return v1.Hash{}, nil }
func (l fixtureTrivyLayer) DiffID() (v1.Hash, error) { return v1.Hash{}, nil }
func (l fixtureTrivyLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.data)), nil
}
func (l fixtureTrivyLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.data)), nil
}
func (l fixtureTrivyLayer) Size() (int64, error)                { return int64(len(l.data)), nil }
func (l fixtureTrivyLayer) MediaType() (types.MediaType, error) { return l.mediaType, nil }

func fixtureTrivyArchive(t *testing.T, db []byte, nextUpdate time.Time) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(gz)
	metadata := []byte(`{"NextUpdate":"` + nextUpdate.UTC().Format(time.RFC3339) + `"}`)
	for name, body := range map[string][]byte{"trivy.db": db, "metadata.json": metadata} {
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestUpdateTrivyDBExtractsOCIArtifactLayer(t *testing.T) {
	databaseBytes := append(append([]byte(nil), sqliteFileHeader...), []byte("fixture database")...)
	payload := fixtureTrivyArchive(t, databaseBytes, time.Now().UTC().Add(24*time.Hour))
	image := &fake.FakeImage{}
	image.LayersReturns([]v1.Layer{
		fixtureTrivyLayer{data: []byte("irrelevant layer"), mediaType: types.OCILayer},
		fixtureTrivyLayer{data: payload, mediaType: trivyDBLayerMediaType},
	}, nil)
	original := fetchTrivyImage
	fetchCount := 0
	fetchTrivyImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
		fetchCount++
		if ref.Name() != "ghcr.io/aquasecurity/trivy-db:2" {
			t.Errorf("OCI reference = %q", ref.Name())
		}
		return image, nil
	}
	t.Cleanup(func() { fetchTrivyImage = original })

	manager := NewCVEManager(t.TempDir(), []CVESource{})
	if err := manager.UpdateTrivyDB(context.Background(), "ghcr.io/aquasecurity/trivy-db:2"); err != nil {
		t.Fatalf("update Trivy DB: %v", err)
	}
	dbPath := filepath.Join(manager.securityDir, "trivy", "trivy.db")
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read Trivy DB: %v", err)
	}
	if !bytes.Equal(got, databaseBytes) {
		t.Fatalf("Trivy DB = %q, want %q", got, databaseBytes)
	}
	if _, err := os.Stat(filepath.Join(manager.securityDir, "trivy", ".last_updated")); err != nil {
		t.Fatalf("Trivy timestamp missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.securityDir, "trivy", "metadata.json")); err != nil {
		t.Fatalf("Trivy metadata missing: %v", err)
	}
	if got := manager.TrivyHealthStatus(); got != "ok" {
		t.Fatalf("fresh extracted Trivy health = %q, want ok", got)
	}
	if err := manager.UpdateTrivyDB(context.Background(), "ghcr.io/aquasecurity/trivy-db:2"); err != nil {
		t.Fatalf("fresh Trivy update skip: %v", err)
	}
	if fetchCount != 1 {
		t.Fatalf("fresh database caused %d OCI fetches, want one initial fetch", fetchCount)
	}
}

func TestUpdateTrivyDBRejectsOCIImageWithoutDatabaseLayer(t *testing.T) {
	image := &fake.FakeImage{}
	image.LayersReturns([]v1.Layer{fixtureTrivyLayer{data: []byte("other"), mediaType: types.OCILayer}}, nil)
	original := fetchTrivyImage
	fetchTrivyImage = func(name.Reference, ...remote.Option) (v1.Image, error) { return image, nil }
	t.Cleanup(func() { fetchTrivyImage = original })

	manager := NewCVEManager(t.TempDir(), []CVESource{})
	dbPath := filepath.Join(manager.securityDir, "trivy", "trivy.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, append(append([]byte(nil), sqliteFileHeader...), []byte("old database")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.UpdateTrivyDB(context.Background(), "ghcr.io/aquasecurity/trivy-db:2"); err == nil {
		t.Fatal("artifact without the Trivy DB layer succeeded")
	}
	got, err := os.ReadFile(dbPath)
	if err != nil || !bytes.Contains(got, []byte("old database")) {
		t.Fatalf("failed update changed existing database: %q, %v", got, err)
	}
}

func TestCVEManagerHasTrivyDBRequiresValidDatabaseFile(t *testing.T) {
	manager := NewCVEManager(t.TempDir(), []CVESource{})
	if manager.HasTrivyDB() {
		t.Fatal("missing Trivy database reported as installed")
	}

	dir := filepath.Join(manager.securityDir, "trivy")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "trivy.db")
	if err := os.WriteFile(dbPath, append(append([]byte(nil), sqliteFileHeader...), []byte("database")...), 0600); err != nil {
		t.Fatal(err)
	}
	if !manager.HasTrivyDB() {
		t.Fatal("valid Trivy database was not detected")
	}
	if err := os.Truncate(dbPath, 0); err != nil {
		t.Fatal(err)
	}
	if manager.HasTrivyDB() {
		t.Fatal("empty Trivy database reported as installed")
	}
	if err := os.Truncate(dbPath, trivyDBMaxBytes+1); err != nil {
		t.Fatal(err)
	}
	if manager.HasTrivyDB() {
		t.Fatal("oversized Trivy database reported as installed")
	}
}

func TestTrivyHealthStatusUsesNextUpdate(t *testing.T) {
	manager := NewCVEManager(t.TempDir(), []CVESource{})
	dir := filepath.Join(manager.securityDir, "trivy")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	database := append(append([]byte(nil), sqliteFileHeader...), []byte("database")...)
	if err := os.WriteFile(filepath.Join(dir, "trivy.db"), database, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".last_updated"), []byte(time.Now().UTC().Format(time.RFC3339)), 0600); err != nil {
		t.Fatal(err)
	}
	writeMetadata := func(nextUpdate time.Time) {
		t.Helper()
		body := []byte(`{"NextUpdate":"` + nextUpdate.UTC().Format(time.RFC3339) + `"}`)
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeMetadata(time.Now().UTC().Add(time.Hour))
	if got := manager.TrivyHealthStatus(); got != "ok" {
		t.Fatalf("fresh Trivy health = %q, want ok", got)
	}
	writeMetadata(time.Now().UTC().Add(-time.Second))
	if got := manager.TrivyHealthStatus(); got != "degraded" {
		t.Fatalf("stale Trivy health = %q, want degraded", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".last_updated"), []byte("not-a-timestamp"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := manager.TrivyHealthStatus(); got != "error" {
		t.Fatalf("invalid last_updated health = %q, want error", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(`{"Version":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := manager.TrivyHealthStatus(); got != "error" {
		t.Fatalf("invalid metadata health = %q, want error", got)
	}
}

func TestExtractTrivyDatabaseRejectsUnsafeAndOversizedArchives(t *testing.T) {
	goodDB := append(append([]byte(nil), sqliteFileHeader...), []byte("database")...)
	for _, name := range []string{"../trivy.db", "/trivy.db"} {
		t.Run(name, func(t *testing.T) {
			var buffer bytes.Buffer
			gz := gzip.NewWriter(&buffer)
			archive := tar.NewWriter(gz)
			if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(goodDB)), Typeflag: tar.TypeReg}); err != nil {
				t.Fatal(err)
			}
			if _, err := archive.Write(goodDB); err != nil {
				t.Fatal(err)
			}
			_ = archive.Close()
			_ = gz.Close()
			if err := extractTrivyDatabase(bytes.NewReader(buffer.Bytes()), t.TempDir()); err == nil {
				t.Fatal("unsafe/incomplete Trivy archive was accepted")
			}
		})
	}
	t.Run("oversized member", func(t *testing.T) {
		var buffer bytes.Buffer
		gz := gzip.NewWriter(&buffer)
		archive := tar.NewWriter(gz)
		if err := archive.WriteHeader(&tar.Header{Name: "trivy.db", Mode: 0600, Size: trivyDBMaxBytes + 1, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		if err := extractTrivyDatabase(bytes.NewReader(buffer.Bytes()), t.TempDir()); err == nil {
			t.Fatal("oversized Trivy member was accepted")
		}
	})
}
