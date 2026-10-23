package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"submarine/internal/submodule"
)

// rootFixture is ownFixture with a bare origin for the superproject, main
// pushed and tracking it. It returns the provider, the fixture root and the
// path of the bare repository.
func rootFixture(t *testing.T) (*Provider, string, string) {
	t.Helper()
	p, root := ownFixture(t)
	bare := filepath.Join(root, "remotes", "super.git")
	git(t, root, "init", "-q", "--bare", bare)
	git(t, p.root, "remote", "add", "origin", bare)
	git(t, p.root, "push", "-q", "-u", "origin", "main")
	return p, root, bare
}

func rootStatus(t *testing.T, p *Provider) *submodule.Status {
	t.Helper()
	st, err := p.Status(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Root {
		t.Error("Status.Root = false for the superproject")
	}
	return st
}

func issueKinds(st *submodule.Status) []submodule.IssueKind {
	var kinds []submodule.IssueKind
	for _, is := range st.Issues() {
		kinds = append(kinds, is.Kind)
	}
	return kinds
}

func TestTreeRoot(t *testing.T) {
	p, root, bare := rootFixture(t)
	repo, err := p.Tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r := repo.Root; r == nil || !r.IsRoot() || r.Name != "super" || r.URL != bare || len(r.Children) != 0 {
		t.Errorf("Root = %+v (fixture %s)", r, root)
	}
}

// The fixture has dirty, conflicting and staged submodules; none of them
// makes the superproject itself changed, their rows show them.
func TestRootStatusIgnoresSubmodules(t *testing.T) {
	p, _, _ := rootFixture(t)
	st := rootStatus(t, p)
	if kinds := issueKinds(st); len(kinds) != 0 {
		t.Errorf("issues = %v, want none", kinds)
	}
	if st.Changes != nil || st.PinStaged || st.Presence != submodule.PresenceOK {
		t.Errorf("Changes = %+v, PinStaged = %v", st.Changes, st.PinStaged)
	}
	if st.Pin.Ref != "origin/main" || st.Pin.Source != submodule.RefFromCurrentBranch || !st.Pin.Pushed {
		t.Errorf("Pin = %+v", st.Pin)
	}
	if st.Pinned == nil || st.Pinned != st.CheckedOut && *st.Pinned != *st.CheckedOut {
		t.Errorf("Pinned = %+v, CheckedOut = %+v, want HEAD for both", st.Pinned, st.CheckedOut)
	}
}

func TestRootStatusUnpushed(t *testing.T) {
	p, _, _ := rootFixture(t)
	git(t, p.root, "commit", "--allow-empty", "-m", "Local work")
	st := rootStatus(t, p)
	if kinds := issueKinds(st); !slices.Equal(kinds, []submodule.IssueKind{submodule.IssueUnpushed}) {
		t.Errorf("issues = %v, want unpushed", kinds)
	}
	if st.Pin.Ahead != 1 || st.Pin.Behind != 0 {
		t.Errorf("ahead/behind = %d/%d, want 1/0", st.Pin.Ahead, st.Pin.Behind)
	}
	if d := st.Issues()[0].Detail; d != "HEAD not pushed to any remote" {
		t.Errorf("detail = %q", d)
	}
}

// A submodule the current branch no longer has stays on disk after a branch
// switch (its .git is a file into .git/modules). It is an untracked
// repository of the superproject.
func TestRootStatusLeftoverSubmodule(t *testing.T) {
	p, _, _ := rootFixture(t)
	git(t, p.root, "switch", "-q", "-c", "nosub")
	git(t, p.root, "rm", "-q", "--cached", "src/ui")
	git(t, p.root, "commit", "-q", "-m", "Drop src/ui")
	git(t, p.root, "switch", "-q", "main")
	git(t, p.root, "switch", "-q", "nosub")
	if _, err := os.Stat(filepath.Join(p.root, "src", "ui", ".git")); err != nil {
		t.Fatalf("src/ui should be left behind: %v", err)
	}

	st := rootStatus(t, p)
	if kinds := issueKinds(st); !slices.Equal(kinds, []submodule.IssueKind{submodule.IssueUnpushed, submodule.IssueChanged}) {
		t.Fatalf("issues = %v, want unpushed (new branch) and changed", kinds)
	}
	if d := st.Issues()[1].Detail; d != "untracked repository" {
		t.Errorf("detail = %q, want untracked repository", d)
	}
	if st.Changes == nil || len(st.Changes.Files) != 1 {
		t.Fatalf("Changes = %+v", st.Changes)
	}
	if f := st.Changes.Files[0]; f.Path != "src/ui/" || f.Kind != submodule.ChangeUntracked || !f.Repo {
		t.Errorf("file = %+v", f)
	}
}

func TestRootStatusModified(t *testing.T) {
	p, _, _ := rootFixture(t)
	if err := os.WriteFile(filepath.Join(p.root, "README"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := rootStatus(t, p)
	if kinds := issueKinds(st); !slices.Equal(kinds, []submodule.IssueKind{submodule.IssueChanged}) {
		t.Errorf("issues = %v, want changed", kinds)
	}
	if st.Worktree != submodule.WorktreeModified {
		t.Errorf("Worktree = %v", st.Worktree)
	}
}

// The ref of the superproject is the upstream of its branch, even if another
// remote branch is closer, so that it agrees with the header's ahead/behind.
func TestRootStatusRefIsUpstream(t *testing.T) {
	p, root, bare := rootFixture(t)
	other := filepath.Join(root, "other")
	git(t, root, "clone", "-q", bare, other)
	git(t, other, "switch", "-q", "-c", "feat")
	git(t, other, "commit", "--allow-empty", "-m", "Feature")
	git(t, other, "push", "-q", "origin", "feat")
	git(t, other, "switch", "-q", "main")
	for range 3 {
		git(t, other, "commit", "--allow-empty", "-m", "Upstream")
	}
	git(t, other, "push", "-q", "origin", "main")
	git(t, p.root, "fetch", "-q")

	st := rootStatus(t, p)
	if st.Pin.Ref != "origin/main" || st.Pin.Source != submodule.RefFromCurrentBranch {
		t.Errorf("ref = %q (%v), want origin/main via the current branch", st.Pin.Ref, st.Pin.Source)
	}
	if st.Pin.Ahead != 0 || st.Pin.Behind != 3 {
		t.Errorf("ahead/behind = %d/%d, want 0/3", st.Pin.Ahead, st.Pin.Behind)
	}
}

func TestRootStatusDetached(t *testing.T) {
	p, _, _ := rootFixture(t)
	git(t, p.root, "switch", "-q", "--detach")
	st := rootStatus(t, p)
	if st.Checkout != submodule.CheckoutDetached {
		t.Errorf("Checkout = %v, want detached", st.Checkout)
	}
	if !st.Pin.Pushed {
		t.Error("detached HEAD on a pushed commit should be pushed")
	}
}
