package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.10.0", "v0.9.0", false},
		{"v1.0.0", "v0.9.9", false},
		{"dev", "v0.1.0", true},
		{"v0.1.0-3-gabc", "v0.1.0", true},
		{"v0.1.0", "garbage", false},
	}
	for _, c := range cases {
		if got := Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func registry(t *testing.T, bin, sum string) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/submarine-latest/latest/VERSION", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("v0.2.0\n"))
	})
	mux.HandleFunc("/submarine/v0.2.0/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(sum + "  submarine_linux_arm64\n"))
	})
	mux.HandleFunc("/submarine/v0.2.0/submarine_linux_arm64", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(bin))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client(), GOOS: "linux", GOARCH: "arm64"}
}

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func TestLatest(t *testing.T) {
	c := registry(t, "x", sha("x"))
	v, err := c.Latest(context.Background())
	if err != nil || v != "v0.2.0" {
		t.Fatalf("Latest = %q, %v", v, err)
	}
}

func TestInstallReplaces(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "submarine")
	os.WriteFile(target, []byte("old"), 0o755)
	c := registry(t, "new-binary", sha("new-binary"))
	if err := c.InstallTo(context.Background(), "v0.2.0", target); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new-binary" {
		t.Fatalf("target = %q", got)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm()&0o111 == 0 {
		t.Error("target not executable")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "submarine")
	os.WriteFile(target, []byte("old"), 0o755)
	c := registry(t, "tampered", sha("original"))
	err := c.InstallTo(context.Background(), "v0.2.0", target)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Errorf("target modified: %q", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
}

func TestInstallMissingAsset(t *testing.T) {
	c := registry(t, "x", sha("x"))
	c.GOARCH = "riscv64"
	err := c.InstallTo(context.Background(), "v0.2.0", filepath.Join(t.TempDir(), "submarine"))
	if err == nil {
		t.Fatal("expected error")
	}
}
