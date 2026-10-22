package git

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"submarine/internal/submodule"
)

// ownFixture builds a fixture the test may modify. It returns the provider
// (after Tree) and the fixture root (superproject in root/super, work
// clones of the remotes in root/work).
func ownFixture(t *testing.T) (*Provider, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := filepath.Join(t.TempDir(), "fx")
	if err := makeFixture(root); err != nil {
		t.Fatal(err)
	}
	p, err := New(filepath.Join(root, "super"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Tree(context.Background()); err != nil {
		t.Fatal(err)
	}
	return p, root
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	args = append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// pushUpstream adds a commit to a fixture remote, as if someone else pushed.
func pushUpstream(t *testing.T, root, remote string) {
	t.Helper()
	work := filepath.Join(root, "work", remote)
	git(t, work, "fetch", "-q")
	git(t, work, "reset", "-q", "--hard", "origin/main")
	git(t, work, "commit", "--allow-empty", "-m", "Upstream change")
	git(t, work, "push", "-q", "origin", "main")
}

func TestFetch(t *testing.T) {
	p, root := ownFixture(t)
	ctx := context.Background()

	before, err := p.Status(ctx, "src/core")
	if err != nil {
		t.Fatal(err)
	}
	pushUpstream(t, root, "core")
	if err := p.Fetch(ctx, "src/core"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	after, err := p.Status(ctx, "src/core")
	if err != nil {
		t.Fatal(err)
	}
	if after.Pin.Behind <= before.Pin.Behind {
		t.Errorf("after fetch: ↓%d, want more than ↓%d", after.Pin.Behind, before.Pin.Behind)
	}
	if after.FetchedAt.IsZero() {
		t.Error("after fetch: FetchedAt is zero")
	}

	if err := p.Fetch(ctx, ""); err != nil {
		t.Errorf("Fetch(superproject without remotes): %v", err)
	}
	if p.Fetchable("libs/missing") {
		t.Error("Fetchable(libs/missing) = true for an uninitialized submodule")
	}
	if err := p.Fetch(ctx, "libs/missing"); err == nil {
		t.Error("Fetch(libs/missing): want error")
	}
}

func TestFetchError(t *testing.T) {
	p, root := ownFixture(t)
	git(t, filepath.Join(root, "super", "src", "ui"), "remote", "set-url", "origin", filepath.Join(root, "nowhere.git"))

	err := p.Fetch(context.Background(), "src/ui")
	if err == nil {
		t.Fatal("Fetch with a bad remote: want error")
	}
	if strings.Contains(err.Error(), "\n") || strings.HasPrefix(err.Error(), "git ") {
		t.Errorf("error should be git's first line only, got %q", err)
	}
}

// next waits for a change or fails after timeout.
func next(t *testing.T, ch <-chan submodule.Change) submodule.Change {
	t.Helper()
	select {
	case c := <-ch:
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("no change reported")
		return submodule.Change{}
	}
}

func TestWatch(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	p.WatchInterval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := p.Watch(ctx)

	// our own reads must not look like changes
	for _, path := range []string{"src/core", "libs/work", "libs/conflict"} {
		if _, err := p.Status(ctx, path); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case c := <-ch:
		t.Fatalf("change while idle: %+v", c)
	case <-time.After(200 * time.Millisecond):
	}

	// a fetch in another shell
	pushUpstream(t, root, "net")
	git(t, filepath.Join(super, "src", "net"), "fetch", "-q")
	if c := next(t, ch); c != (submodule.Change{Path: "src/net"}) {
		t.Errorf("after fetch: %+v, want remote change of src/net", c)
	}

	// a commit in another shell
	git(t, filepath.Join(super, "libs", "store"), "commit", "-q", "--allow-empty", "-m", "Local")
	c := next(t, ch)
	if c == (submodule.Change{Path: "src/net"}) {
		// git fetch writes FETCH_HEAD and the refs at different moments, so
		// with a 20ms poll it can be reported in two rounds
		c = next(t, ch)
	}
	if c != (submodule.Change{Path: "libs/store", Local: true}) {
		t.Errorf("after commit: %+v, want local change of libs/store", c)
	}

	cancel()
	for range ch { // closed after cancel
	}
}

func TestGitDirs(t *testing.T) {
	p, _ := loadFixture(t)
	gd, cd, err := gitDirs(p.root)
	if err != nil || gd != filepath.Join(p.root, ".git") || cd != gd {
		t.Errorf("superproject: %q, %q, %v", gd, cd, err)
	}
	gd, cd, err = gitDirs(filepath.Join(p.root, "src", "core"))
	if err != nil || gd != filepath.Join(p.root, ".git", "modules", "src", "core") || cd != gd {
		t.Errorf("submodule: %q, %q, %v", gd, cd, err)
	}
}
