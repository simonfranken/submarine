package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

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
			_, file, _, _ := runtime.Caller(0)
			script := filepath.Join(filepath.Dir(file), "..", "..", "..", "scripts", "make-fixture.sh")
			out, err := exec.Command("bash", script, filepath.Join(tmp, "fx")).CombinedOutput()
			if err != nil {
				panic("make-fixture.sh: " + err.Error() + "\n" + string(out))
			}
			fixtureDir = filepath.Join(tmp, "fx", "super")
		}
		return m.Run()
	}()
	os.Exit(code)
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
		"libs/feature", "libs/conflict", "libs/staged", "libs/missing", "vendor",
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
