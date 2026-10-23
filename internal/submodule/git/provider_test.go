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
	var paths []string
	submodule.Walk(repo.Submodules, func(s *submodule.Submodule) { paths = append(paths, s.Path) })
	want := []string{
		"src/core", "src/ui", "src/net", "libs/store", "libs/log", "libs/log/modules/nested",
		"libs/feature", "libs/conflict", "libs/staged", "libs/missing", "vendor", "libs/work", "libs/topic",
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
		target   string // the target if the pin is not on it; else the ref is the target
		source   submodule.RefSource
		ahead    int
		behind   int
		staged   bool
		checkout submodule.Checkout
		drift    submodule.Distance // checkout compared with the pin
		branches []string           // other remote branches containing the pin
		toTarget submodule.Distance // target tip compared with the pin; zero if the pin is on it
	}{
		{path: "src/core", issues: []submodule.IssueKind{submodule.IssueOffPin}, ref: "origin/main", source: submodule.RefFromDefaultBranch, checkout: submodule.CheckoutDiffers, drift: submodule.Distance{Behind: 1}},
		{path: "src/ui", ref: "origin/main", source: submodule.RefFromDefaultBranch, behind: 2},
		{path: "src/net", issues: []submodule.IssueKind{submodule.IssueUnpushed}, ref: "origin/main", source: submodule.RefFromDefaultBranch, ahead: 1},
		{path: "libs/store", ref: "origin/develop", source: submodule.RefFromGitmodules},
		{path: "libs/log", ref: "origin/main", source: submodule.RefFromDefaultBranch, branches: []string{"origin/archive"}},
		{path: "libs/log/modules/nested", issues: []submodule.IssueKind{submodule.IssueChanged}, ref: "origin/main", source: submodule.RefFromDefaultBranch, checkout: submodule.CheckoutDetached},
		{path: "libs/feature", issues: []submodule.IssueKind{submodule.IssueOffTarget}, ref: "origin/feat/search", source: submodule.RefFromContains, target: "origin/main", behind: 1, checkout: submodule.CheckoutDetached, toTarget: submodule.Distance{Behind: 1}},
		{path: "libs/topic", issues: []submodule.IssueKind{submodule.IssueOffTarget}, ref: "origin/feat/search", source: submodule.RefFromContains, target: "origin/main", checkout: submodule.CheckoutDetached, toTarget: submodule.Distance{Behind: 2}},
		{path: "libs/conflict", issues: []submodule.IssueKind{submodule.IssueConflict}, ref: "origin/main", source: submodule.RefFromDefaultBranch, staged: true},
		{path: "libs/staged", ref: "origin/main", source: submodule.RefFromDefaultBranch, staged: true},
		{path: "libs/missing", issues: []submodule.IssueKind{submodule.IssueMissing}},
		{path: "vendor", issues: []submodule.IssueKind{submodule.IssueChanged}, ref: "origin/main", source: submodule.RefFromDefaultBranch, behind: 2},
		{path: "libs/work", issues: []submodule.IssueKind{submodule.IssueChanged}, ref: "origin/main", source: submodule.RefFromDefaultBranch},
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
		want := tt.target
		if want == "" && (tt.source == submodule.RefFromGitmodules || tt.source == submodule.RefFromDefaultBranch) {
			want = tt.ref
		}
		if st.Pin.Target != want {
			t.Errorf("%s: target %q, want %q", tt.path, st.Pin.Target, want)
		}
		if st.Pin.Ahead != tt.ahead || st.Pin.Behind != tt.behind {
			t.Errorf("%s: ↑%d↓%d, want ↑%d↓%d", tt.path, st.Pin.Ahead, st.Pin.Behind, tt.ahead, tt.behind)
		}
		if st.PinStaged != tt.staged {
			t.Errorf("%s: PinStaged %v, want %v", tt.path, st.PinStaged, tt.staged)
		}
		if tt.staged != (st.Committed != nil) {
			t.Errorf("%s: Committed = %+v with staged = %v", tt.path, st.Committed, tt.staged)
		}
		if tt.path == "libs/staged" && (st.CommittedDrift.Ahead != 0 || st.CommittedDrift.Behind == 0) {
			t.Errorf("%s: CommittedDrift %+v, want the committed pin behind the staged one", tt.path, st.CommittedDrift)
		}
		if !slices.Equal(st.Pin.Branches, tt.branches) {
			t.Errorf("%s: other branches %v, want %v", tt.path, st.Pin.Branches, tt.branches)
		}
		if st.TargetDrift != tt.toTarget || (st.TargetTip != nil) != (tt.toTarget != submodule.Distance{}) {
			t.Errorf("%s: TargetDrift %+v with tip %+v, want %+v", tt.path, st.TargetDrift, st.TargetTip, tt.toTarget)
		}
		if st.Drift != tt.drift {
			t.Errorf("%s: drift %+v, want %+v", tt.path, st.Drift, tt.drift)
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

// A staged pin older than the committed one: the committed pin is ahead.
func TestCommittedPinAheadOfStaged(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	git(t, filepath.Join(super, "src", "ui"), "checkout", "-q", "--detach", "HEAD~1")
	git(t, super, "add", "src/ui")

	st, err := p.Status(context.Background(), "src/ui")
	if err != nil {
		t.Fatal(err)
	}
	if !st.PinStaged || st.Committed == nil {
		t.Fatalf("PinStaged = %v, Committed = %v", st.PinStaged, st.Committed)
	}
	if st.CommittedDrift != (submodule.Distance{Ahead: 1}) {
		t.Errorf("CommittedDrift = %+v, want the committed pin 1 ahead", st.CommittedDrift)
	}
	spine := st.Spine()
	if n := len(spine); n < 2 || spine[n-1].Commit.SHA != st.Pinned.SHA || spine[n-2].Commit.SHA != st.Committed.SHA {
		t.Errorf("spine = %+v, want the committed pin directly above the staged one", spine)
	}
}

// A pin and its ref that each have a commit the other lacks, and a checkout
// that did the same to the pin: both get the merge base.
func TestBasesOfDivergedCommits(t *testing.T) {
	p, root := ownFixture(t)
	ctx := context.Background()
	super := filepath.Join(root, "super")

	// src/net: the pin is unpushed; upstream moved on
	pushUpstream(t, root, "net")
	if err := p.Fetch(ctx, "src/net"); err != nil {
		t.Fatal(err)
	}
	st, err := p.Status(ctx, "src/net")
	if err != nil {
		t.Fatal(err)
	}
	if st.Pin.Ahead != 1 || st.Pin.Behind != 1 {
		t.Fatalf("src/net: ↑%d↓%d, want ↑1↓1", st.Pin.Ahead, st.Pin.Behind)
	}
	if b := st.Bases.Tip; b == nil || b.SHA == st.Pinned.SHA || b.SHA == st.Tip.SHA {
		t.Errorf("src/net: Bases.Tip = %+v, want a commit before the pin and the tip", b)
	}
	if sp := st.Spine(); len(sp) != 3 || !sp[0].Fork || sp[0].Commit.SHA != st.Tip.SHA {
		t.Errorf("src/net: spine = %+v, want a fork headed by the tip", sp)
	}

	// src/core: a commit on top of a checkout that is behind the pin
	git(t, filepath.Join(super, "src", "core"), "commit", "--allow-empty", "-m", "Local")
	st, err = p.Status(ctx, "src/core")
	if err != nil {
		t.Fatal(err)
	}
	if st.Drift != (submodule.Distance{Ahead: 1, Behind: 1}) {
		t.Fatalf("src/core: drift %+v, want 1/1", st.Drift)
	}
	if b := st.Bases.Checkout; b == nil || b.SHA == st.Pinned.SHA || b.SHA == st.CheckedOut.SHA {
		t.Errorf("src/core: Bases.Checkout = %+v", b)
	}

	// libs/feature: its pin is on a feature branch, and main moved on
	pushUpstream(t, root, "feature")
	if err := p.Fetch(ctx, "libs/feature"); err != nil {
		t.Fatal(err)
	}
	st, err = p.Status(ctx, "libs/feature")
	if err != nil {
		t.Fatal(err)
	}
	if st.TargetDrift != (submodule.Distance{Ahead: 1, Behind: 1}) || st.Bases.Target == nil {
		t.Errorf("libs/feature: TargetDrift %+v, Bases.Target %+v, want a diverged target with a base", st.TargetDrift, st.Bases.Target)
	}
	if sp := st.Spine(); len(sp) == 0 || !sp[0].Fork || sp[0].Roles[0] != submodule.RoleTarget {
		t.Errorf("libs/feature: spine = %+v, want a fork headed by the target", sp)
	}

	// in sync and linear: no bases
	if st, err = p.Status(ctx, "src/ui"); err != nil {
		t.Fatal(err)
	} else if st.Bases != (submodule.Bases{}) {
		t.Errorf("src/ui: Bases = %+v, want none", st.Bases)
	}
}

// A `branch =` that does not exist locally is not replaced by the default
// branch: judging the pin against the wrong branch would be worse than not
// judging it.
func TestDeclaredBranchMissing(t *testing.T) {
	p, root := ownFixture(t)
	git(t, filepath.Join(root, "super"), "config", "-f", ".gitmodules", "submodule.libs/store.branch", "gone")
	if _, err := p.Tree(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, err := p.Status(context.Background(), "libs/store")
	if err != nil {
		t.Fatal(err)
	}
	if st.Pin.Target != "" {
		t.Errorf("target %q, want none", st.Pin.Target)
	}
	for _, is := range st.Issues() {
		if is.Kind == submodule.IssueOffTarget {
			t.Errorf("issue %+v without a target", is)
		}
	}
}

// Two branches contain the pin equally close: the branch the submodule is on
// wins, else the first name; the default branch is no favorite.
func TestContainingBranchTie(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	// the target is a branch that does not exist, so none applies
	git(t, super, "config", "-f", ".gitmodules", "submodule.libs/log.branch", "gone")
	if _, err := p.Tree(context.Background()); err != nil {
		t.Fatal(err)
	}
	ref := func() (string, submodule.RefSource, []string) {
		st, err := p.Status(context.Background(), "libs/log")
		if err != nil {
			t.Fatal(err)
		}
		return st.Pin.Ref, st.Pin.Source, st.Pin.Branches
	}

	// on branch main (upstream origin/main) in the fixture
	if r, src, _ := ref(); r != "origin/main" || src != submodule.RefFromContains {
		t.Errorf("on a branch: ref %q (%v), want origin/main", r, src)
	}

	git(t, filepath.Join(super, "libs", "log"), "checkout", "-q", "--detach")
	r, src, others := ref()
	if r != "origin/archive" || src != submodule.RefFromContains {
		t.Errorf("detached: ref %q (%v), want origin/archive", r, src)
	}
	if !slices.Equal(others, []string{"origin/main"}) {
		t.Errorf("detached: other branches %v, want [origin/main]", others)
	}
}
