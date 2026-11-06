package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadAndInstallRejectsInvalidChecksum(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cli/binaries/search-cli-linux-amd64" {
			t.Errorf("request path = %q", r.URL.Path)
		}
	if _, err := w.Write([]byte("not a binary")); err != nil {
		t.Errorf("write test binary response: %v", err)
	}
	}))
	defer server.Close()

	_, err := DownloadAndInstall(context.Background(), server.Client(), server.URL, "", "search-cli-linux-amd64", strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("DownloadAndInstall() error = %v, want checksum mismatch", err)
	}
}

func TestDownloadAndInstallRequiresHTTPS(t *testing.T) {
	_, err := DownloadAndInstall(context.Background(), http.DefaultClient, "http://updates.example.test", "", "search-cli-linux-amd64", strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "require HTTPS") {
		t.Fatalf("DownloadAndInstall() error = %v, want HTTPS requirement", err)
	}
}

func TestDownloadAndInstallRejectsPathLikeBinaryName(t *testing.T) {
	_, err := DownloadAndInstall(context.Background(), http.DefaultClient, "https://updates.example.test", "", "../search-cli", strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "invalid client binary name") {
		t.Fatalf("DownloadAndInstall() error = %v, want invalid binary name", err)
	}
}

func TestDownloadAndInstallRejectsCrossOriginRedirect(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://updates.example.test/binary", http.StatusFound)
	}))
	defer server.Close()

	_, err := DownloadAndInstall(context.Background(), server.Client(), server.URL, "", "search-cli-linux-amd64", strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "redirect changed origin") {
		t.Fatalf("DownloadAndInstall() error = %v, want redirect origin rejection", err)
	}
}

func TestWriteBoundedRejectsOversizedStream(t *testing.T) {
	var dst bytes.Buffer
	digest := sha256.New()
	written, err := writeBounded(&dst, strings.NewReader("123456"), digest, 5)
	if err == nil || written != 6 {
		t.Fatalf("writeBounded() = (%d, %v), want 6 bytes and size error", written, err)
	}
}

func TestWriteBoundedAcceptsStreamAtLimit(t *testing.T) {
	var dst bytes.Buffer
	digest := sha256.New()
	written, err := writeBounded(&dst, strings.NewReader("12345"), digest, 5)
	if err != nil || written != 5 || dst.String() != "12345" {
		t.Fatalf("writeBounded() = (%d, %v), body %q; want 5 bytes without error", written, err, dst.String())
	}
}
