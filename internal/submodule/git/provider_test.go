package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"submarine/internal/submodule"
)

// fixtureDir is the superproject built by scripts/make-fixture.sh, or ""
// if git or bash is unavailable.
var fixtureDir string

func TestMain(m *testing.M) {
	code := func() int {
		tmp, err := os.MkdirTemp("", "submarine-fixture-")
		if err != nil {
			panic(err)
		}
		defer os.RemoveAll(tmp)
		if _, err := exec.LookPath("git"); err == nil {
			if err := makeFixture(filepath.Join(tmp, "fx")); err != nil {
				panic(err)
			}
			fixtureDir = filepath.Join(tmp, "fx", "super")
		}
		return m.Run()
	}()
	os.Exit(code)
}

// makeFixture runs scripts/make-fixture.sh; the superproject is dir/super.
func makeFixture(dir string) error {
	_, file, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(file), "..", "..", "..", "scripts", "make-fixture.sh")
	if out, err := exec.Command("bash", script, dir).CombinedOutput(); err != nil {
		return fmt.Errorf("make-fixture.sh: %v\n%s", err, out)
	}
	return nil
}

func loadFixture(t *testing.T) (*Provider, *submodule.Repo) {
	t.Helper()
	if fixtureDir == "" {
		t.Skip("git not available")
	}
	p, err := New(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := p.Tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return p, repo
}

func TestTree(t *testing.T) {
	_, repo := loadFixture(t)
	if repo.Branch != "main" {
		t.Errorf("Branch = %q, want main", repo.Branch)
	}
	if !repo.FetchedAt.IsZero() {
		t.Errorf("FetchedAt = %v, want zero (superproject never fetched)", repo.FetchedAt)
	}
	var paths []string
	submodule.Walk(repo.Submodules, func(s *submodule.Submodule) { paths = append(paths, s.Path) })
	want := []string{
		"src/core", "src/ui", "src/net", "libs/store", "libs/log", "libs/log/modules/nested",
		"libs/feature", "libs/conflict", "libs/staged", "libs/missing", "vendor", "libs/work",
	}
	if !slices.Equal(paths, want) {
		t.Errorf("paths = %v\nwant    %v", paths, want)
	}
	log := repo.Submodules[4]
	nested := log.Children[0]
	if nested.Parent != log || nested.Name != "nested" || nested.RelPath != "modules/nested" {
		t.Errorf("nested = %+v", nested)
	}
}

func TestStatus(t *testing.T) {
	p, _ := loadFixture(t)
	tests := []struct {
		path     string
		issues   []submodule.IssueKind
		ref      string
		source   submodule.RefSource
		ahead    int
		behind   int
		staged   bool
		checkout submodule.Checkout
	}{
		{path: "src/core", issues: []submodule.IssueKind{submodule.IssueOffPin}, ref: "origin/main", source: submodule.RefFromDefaultBranch, checkout: submodule.CheckoutDiffers},
		{path: "src/ui", ref: "origin/main", source: submodule.RefFromCurrentBranch, behind: 2},
		{path: "src/net", issues: []submodule.IssueKind{submodule.IssueUnpushed}, ref: "origin/main", source: submodule.RefFromCurrentBranch, ahead: 1},
		{path: "libs/store", ref: "origin/develop", source: submodule.RefFromGitmodules},
		{path: "libs/log", ref: "origin/main", source: submodule.RefFromCurrentBranch},
		{path: "libs/log/modules/nested", issues: []submodule.IssueKind{submodule.IssueChanged}, ref: "origin/main", source: submodule.RefFromDefaultBranch, checkout: submodule.CheckoutDetached},
		{path: "libs/feature", ref: "origin/feat/search", source: submodule.RefFromContains, behind: 1, checkout: submodule.CheckoutDetached},
		{path: "libs/conflict", issues: []submodule.IssueKind{submodule.IssueConflict}, ref: "origin/main", source: submodule.RefFromCurrentBranch, staged: true},
		{path: "libs/staged", ref: "origin/main", source: submodule.RefFromCurrentBranch, staged: true},
		{path: "libs/missing", issues: []submodule.IssueKind{submodule.IssueMissing}},
		{path: "vendor", issues: []submodule.IssueKind{submodule.IssueChanged}, ref: "origin/main", source: submodule.RefFromCurrentBranch, behind: 2},
		{path: "libs/work", issues: []submodule.IssueKind{submodule.IssueChanged}, ref: "origin/main", source: submodule.RefFromCurrentBranch},
	}
	for _, tt := range tests {
		st, err := p.Status(context.Background(), tt.path)
		if err != nil {
			t.Errorf("%s: %v", tt.path, err)
			continue
		}
		var kinds []submodule.IssueKind
		for _, is := range st.Issues() {
			kinds = append(kinds, is.Kind)
		}
		if !slices.Equal(kinds, tt.issues) {
			t.Errorf("%s: issues %v, want %v", tt.path, kinds, tt.issues)
		}
		if st.Presence != submodule.PresenceOK {
			continue
		}
		if st.Pin.Ref != tt.ref || st.Pin.Source != tt.source {
			t.Errorf("%s: ref %q (%v), want %q (%v)", tt.path, st.Pin.Ref, st.Pin.Source, tt.ref, tt.source)
		}
		if st.Pin.Ahead != tt.ahead || st.Pin.Behind != tt.behind {
			t.Errorf("%s: ↑%d↓%d, want ↑%d↓%d", tt.path, st.Pin.Ahead, st.Pin.Behind, tt.ahead, tt.behind)
		}
		if len(st.Between) != tt.ahead+tt.behind {
			t.Errorf("%s: %d commits between, want %d", tt.path, len(st.Between), tt.ahead+tt.behind)
		}
		if st.PinStaged != tt.staged {
			t.Errorf("%s: PinStaged %v, want %v", tt.path, st.PinStaged, tt.staged)
		}
		if st.Checkout != tt.checkout {
			t.Errorf("%s: Checkout %v, want %v", tt.path, st.Checkout, tt.checkout)
		}
		if st.Pinned == nil || st.CheckedOut == nil || st.Tip == nil || st.Pinned.Author != "Ada Lovelace" {
			t.Errorf("%s: commits %+v %+v %+v", tt.path, st.Pinned, st.CheckedOut, st.Tip)
		}
	}
}

func TestChanges(t *testing.T) {
	p, _ := loadFixture(t)
	ctx := context.Background()

	st, err := p.Status(ctx, "libs/work")
	if err != nil {
		t.Fatal(err)
	}
	c := st.Changes
	if c == nil {
		t.Fatal("libs/work: Changes = nil")
	}
	want := []submodule.FileChange{
		{Path: "main.go", Kind: submodule.ChangeModified, Added: 6, Removed: 2},
		{Path: "util.go", Kind: submodule.ChangeModified, Added: 1},
		{Path: "logo.png", Kind: submodule.ChangeModified, Binary: true},
		{Path: "notes_new.go", Kind: submodule.ChangeAdded, Added: 1},
		{Path: "old.go", Kind: submodule.ChangeRemoved, Removed: 8},
		{Path: "lib2.go", OrigPath: "lib.go", Kind: submodule.ChangeRenamed},
		{Path: "scratch.txt", Kind: submodule.ChangeUntracked},
		{Path: "tmp/", Kind: submodule.ChangeUntracked},
	}
	for i, f := range c.Files {
		if f.Modified.IsZero() != (f.Kind == submodule.ChangeRemoved) {
			t.Errorf("libs/work: %s: Modified %v", f.Path, f.Modified)
		}
		c.Files[i].Modified = time.Time{}
	}
	if !slices.Equal(c.Files, want) {
		t.Errorf("libs/work: Files =\n%+v\nwant\n%+v", c.Files, want)
	}
	if c.Total != 8 || c.Added != 8 || c.Removed != 10 {
		t.Errorf("libs/work: Total %d, +%d -%d; want 8, +8 -10", c.Total, c.Added, c.Removed)
	}
	if n := c.Counts(nil); n[submodule.ChangeModified] != 3 || n[submodule.ChangeUntracked] != 2 {
		t.Errorf("libs/work: Counts = %v", n)
	}

	for _, path := range []string{"libs/log", "src/ui"} {
		st, err := p.Status(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Changes != nil {
			t.Errorf("%s: clean submodule has Changes %+v", path, st.Changes)
		}
	}

	st, err = p.Status(ctx, "libs/conflict")
	if err != nil {
		t.Fatal(err)
	}
	if st.Changes == nil || len(st.Changes.Files) != 1 || st.Changes.Files[0].Kind != submodule.ChangeConflicted {
		t.Errorf("libs/conflict: Changes = %+v", st.Changes)
	}
}

func TestChangesCap(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	n := submodule.MaxChangeFiles + 5
	for i := range n {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := run(context.Background(), dir, "status", "--porcelain=v2", "-z", "--untracked-files=normal")
	if err != nil {
		t.Fatal(err)
	}
	c, err := changes(context.Background(), dir, parseStatus(out))
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != n || len(c.Files) != submodule.MaxChangeFiles || c.Files[0].Path != "f000.txt" {
		t.Errorf("Total %d, len %d, first %q; want %d, %d, f000.txt", c.Total, len(c.Files), c.Files[0].Path, n, submodule.MaxChangeFiles)
	}
}

func TestStatusMissingPresence(t *testing.T) {
	p, _ := loadFixture(t)
	st, err := p.Status(context.Background(), "libs/missing")
	if err != nil {
		t.Fatal(err)
	}
	if st.Presence != submodule.NotInitialized || st.Pinned == nil || len(st.Pinned.SHA) != 7 {
		t.Errorf("libs/missing: %+v", st)
	}
}

func TestStatusErrors(t *testing.T) {
	p, _ := loadFixture(t)
	if _, err := p.Status(context.Background(), "nope"); err == nil {
		t.Error("unknown path: want error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Status(ctx, "src/core"); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: err = %v, want context.Canceled", err)
	}
}

func TestNewNotARepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := New(t.TempDir()); err == nil {
		t.Error("New(non-repo): want error")
	}
}
