// Package update implements self-update from GitHub release assets. It has no
// UI imports; the network is only used on explicit calls.
package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the releases root; see docs/architecture.md#distribution.
// Both paths below it are plain redirects to the assets, not API calls, so
// they are not rate limited.
const DefaultBaseURL = "https://github.com/simonfranken/submarine/releases"

// EnvBaseURL overrides the releases root (tests, manual verification).
const EnvBaseURL = "SUBMARINE_UPDATE_URL"

// maxBinary bounds a download so a bad server cannot fill the disk.
const maxBinary = 256 << 20

// Client talks to the releases.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	GOOS    string
	GOARCH  string
}

// New returns a client for the default (or $SUBMARINE_UPDATE_URL) releases.
func New() *Client {
	base := os.Getenv(EnvBaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimRight(base, "/"),
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
	}
}

// Latest returns the newest released version, e.g. "v0.2.0".
func (c *Client) Latest(ctx context.Context) (string, error) {
	b, err := c.get(ctx, c.BaseURL+"/latest/download/VERSION", 1<<10)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if _, ok := parse(v); !ok {
		return "", fmt.Errorf("releases returned an invalid version %q", v)
	}
	return v, nil
}

// Newer reports whether latest is a higher version than current. Builds that
// are not release versions (e.g. "dev") are never considered up to date.
func Newer(current, latest string) bool {
	l, ok := parse(latest)
	if !ok {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return true
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// Release reports whether v is a release version (vX.Y.Z).
func Release(v string) bool { _, ok := parse(v); return ok }

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if !strings.HasPrefix(v, "v") || len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Install downloads version for this platform, verifies its checksum and
// replaces the running executable (symlinks resolved) atomically.
func (c *Client) Install(ctx context.Context, version string) (string, error) {
	if !Release(version) {
		return "", fmt.Errorf("invalid version %q", version)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return exe, c.InstallTo(ctx, version, exe)
}

// InstallTo is Install with an explicit target path.
func (c *Client) InstallTo(ctx context.Context, version, target string) error {
	asset := fmt.Sprintf("submarine_%s_%s", c.GOOS, c.GOARCH)
	base := c.BaseURL + "/download/" + version + "/"

	sums, err := c.get(ctx, base+"checksums.txt", 1<<20)
	if err != nil {
		return err
	}
	want, err := checksumFor(sums, asset)
	if err != nil {
		return err
	}

	// Same directory as the target, so the final rename is atomic.
	tmp, err := os.CreateTemp(filepath.Dir(target), ".submarine-update-*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s (permission?): %w", target, err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	h := sha256.New()
	if err := c.stream(ctx, base+asset, io.MultiWriter(tmp, h)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", asset, got, want)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

func checksumFor(sums []byte, asset string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(string(sums)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum for %s in checksums.txt", asset)
}

func (c *Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	var buf strings.Builder
	if err := c.stream(ctx, url, &limitWriter{&buf, limit}); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

func (c *Client) stream(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, maxBinary+1))
	if err != nil {
		return err
	}
	if n > maxBinary {
		return errors.New("download too large")
	}
	return nil
}

type limitWriter struct {
	w io.Writer
	n int64
}

func (l *limitWriter) Write(p []byte) (int, error) {
	l.n -= int64(len(p))
	if l.n < 0 {
		return 0, errors.New("response too large")
	}
	return l.w.Write(p)
}
