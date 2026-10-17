package security

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
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

func TestUpdateTrivyDBExtractsOCIArtifactLayer(t *testing.T) {
	payload := []byte("fixture compressed Trivy database")
	image := &fake.FakeImage{}
	image.LayersReturns([]v1.Layer{
		fixtureTrivyLayer{data: []byte("irrelevant layer"), mediaType: types.OCILayer},
		fixtureTrivyLayer{data: payload, mediaType: trivyDBLayerMediaType},
	}, nil)
	original := fetchTrivyImage
	fetchTrivyImage = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
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
	dbPath := filepath.Join(manager.securityDir, "trivy", "db.tar.gz")
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read Trivy DB: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("Trivy DB = %q, want %q", got, payload)
	}
	if _, err := os.Stat(filepath.Join(manager.securityDir, "trivy", ".last_updated")); err != nil {
		t.Fatalf("Trivy timestamp missing: %v", err)
	}
}

func TestUpdateTrivyDBRejectsOCIImageWithoutDatabaseLayer(t *testing.T) {
	image := &fake.FakeImage{}
	image.LayersReturns([]v1.Layer{fixtureTrivyLayer{data: []byte("other"), mediaType: types.OCILayer}}, nil)
	original := fetchTrivyImage
	fetchTrivyImage = func(name.Reference, ...remote.Option) (v1.Image, error) { return image, nil }
	t.Cleanup(func() { fetchTrivyImage = original })

	manager := NewCVEManager(t.TempDir(), []CVESource{})
	dbPath := filepath.Join(manager.securityDir, "trivy", "db.tar.gz")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("old database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.UpdateTrivyDB(context.Background(), "ghcr.io/aquasecurity/trivy-db:2"); err == nil {
		t.Fatal("artifact without the Trivy DB layer succeeded")
	}
	got, err := os.ReadFile(dbPath)
	if err != nil || string(got) != "old database" {
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
	dbPath := filepath.Join(dir, "db.tar.gz")
	if err := os.WriteFile(dbPath, []byte("database"), 0600); err != nil {
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
