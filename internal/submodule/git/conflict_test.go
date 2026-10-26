package git

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"submarine/internal/submodule"
)

// conflictFixture builds the superproject of make-fixture.sh --pin-conflict,
// in a merge of feat into main with a pin conflict on every library.
func conflictFixture(t *testing.T) (*Provider, *submodule.Repo, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := filepath.Join(t.TempDir(), "fx")
	if err := makeFixture(dir, "--pin-conflict"); err != nil {
		t.Fatal(err)
	}
	super := filepath.Join(dir, "super")
	p, err := New(super)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := p.Tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return p, repo, super
}

func TestPinConflictStatus(t *testing.T) {
	p, repo, super := conflictFixture(t)
	ctx := context.Background()
	if repo.Operation != "merging feat" {
		t.Errorf("Operation = %q, want merging feat", repo.Operation)
	}
	root, err := p.Status(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if root.Worktree != submodule.WorktreeClean {
		t.Errorf("superproject Worktree = %v: the pin conflicts are shown by their rows", root.Worktree)
	}

	sha := func(name, rev string) string {
		return out(t, filepath.Join(super, "..", "work", name), "rev-parse", rev)
	}
	for _, tt := range []struct {
		name       string
		fetched    bool
		candidates int
	}{
		{"alpha", true, 1},
		{"beta", true, 0},
		{"gamma", false, 0},
	} {
		st, err := p.Status(ctx, "libs/"+tt.name)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		c := st.PinConflict
		if c == nil {
			t.Fatalf("%s: no PinConflict", tt.name)
		}
		if got := kindsOf(st.Issues()); len(got) != 1 || got[0] != submodule.IssuePinConflict {
			t.Errorf("%s: issues %v, want only pin conflict", tt.name, got)
		}
		if st.PinStaged {
			t.Errorf("%s: PinStaged during a conflict", tt.name)
		}
		if c.Ours.Name != "main" || c.Theirs.Name != "feat" {
			t.Errorf("%s: sides %q/%q, want main/feat", tt.name, c.Ours.Name, c.Theirs.Name)
		}
		if want := sha(tt.name, "main"); tt.name != "alpha" && c.Ours.SHA != want {
			t.Errorf("%s: ours %s, want %s", tt.name, c.Ours.SHA, want)
		}
		if want := sha(tt.name, "feat/"+tt.name); c.Theirs.SHA != want {
			t.Errorf("%s: theirs %s, want %s", tt.name, c.Theirs.SHA, want)
		}
		if !strings.HasPrefix(c.Ours.SHA, st.Pinned.SHA) || c.Ours.Commit == nil || c.Base == nil {
			t.Errorf("%s: pin %v, ours %v, base %v", tt.name, st.Pinned, c.Ours.Commit, c.Base)
		}
		if (c.Theirs.Commit != nil) != tt.fetched {
			t.Errorf("%s: theirs commit %v, fetched %v", tt.name, c.Theirs.Commit, tt.fetched)
		}
		if tt.fetched && (c.Drift != submodule.Distance{Ahead: 1, Behind: 1} || c.MergeBase == nil) {
			t.Errorf("%s: drift %+v, merge base %v", tt.name, c.Drift, c.MergeBase)
		}
		// alpha's main has merged the feature: it is on the target
		if want := map[string]string{"alpha": "origin/main", "beta": "origin/feat/beta"}[tt.name]; c.Theirs.Pin.Ref != want {
			t.Errorf("%s: theirs ref %q", tt.name, c.Theirs.Pin.Ref)
		}
		if len(c.Candidates) != tt.candidates {
			t.Errorf("%s: candidates %+v, want %d", tt.name, c.Candidates, tt.candidates)
		}
	}

	// alpha: main merged the feature, then moved on by one commit
	st, _ := p.Status(ctx, "libs/alpha")
	cand := st.PinConflict.Candidates[0]
	if cand.Ref != "origin/main" || cand.Behind != 1 || cand.Commit.Subject != "Merge feat/alpha" {
		t.Errorf("alpha candidate %+v %+v", cand, cand.Commit)
	}

	// restore refuses: there is no pin until one is chosen
	if err := p.Restore(ctx, "libs/alpha", submodule.RestoreStaged); err == nil || !strings.Contains(err.Error(), "pin conflict") {
		t.Errorf("Restore during a pin conflict: %v", err)
	}
}

func kindsOf(is []submodule.Issue) []submodule.IssueKind {
	var out []submodule.IssueKind
	for _, i := range is {
		out = append(out, i.Kind)
	}
	return out
}

// A rebase swaps the sides: ours is the branch rebased onto.
func TestPinConflictRebaseNames(t *testing.T) {
	_, _, super := conflictFixture(t)
	ctx := context.Background()
	for _, args := range [][]string{{"merge", "--abort"}, {"switch", "-q", "feat"}} {
		out(t, super, args...)
	}
	exec.Command("git", "-C", super, "-c", "user.name=A", "-c", "user.email=a@b", "rebase", "main").Run() // stops at the conflict
	p, err := New(super)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := p.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Operation != "rebasing feat" {
		t.Errorf("Operation = %q, want rebasing feat", repo.Operation)
	}
	st, err := p.Status(ctx, "libs/beta")
	if err != nil {
		t.Fatal(err)
	}
	if c := st.PinConflict; c == nil || c.Ours.Name != "main" || c.Theirs.Name != "feat" {
		t.Errorf("PinConflict = %+v, want sides main/feat", c)
	}
}

func TestResolve(t *testing.T) {
	p, _, super := conflictFixture(t)
	ctx := context.Background()
	pick := func(name string) string {
		st, err := p.Status(ctx, "libs/"+name)
		if err != nil || st.PinConflict == nil {
			t.Fatalf("%s: %v, %+v", name, err, st)
		}
		if c := st.PinConflict.Candidates; len(c) > 0 {
			return c[0].SHA
		}
		return st.PinConflict.Theirs.SHA
	}
	for _, name := range []string{"alpha", "beta", "gamma"} { // gamma: theirs is fetched
		sha := pick(name)
		if loss, err := p.ResolveLoss(ctx, "libs/"+name, sha); err != nil || !loss.Empty() {
			t.Errorf("%s: ResolveLoss = %+v, %v", name, loss, err)
		}
		if err := p.Resolve(ctx, "libs/"+name, sha); err != nil {
			t.Fatalf("%s: Resolve: %v", name, err)
		}
		if got := out(t, super, "ls-files", "-s", "--", "libs/"+name); got != "160000 "+sha+" 0\tlibs/"+name {
			t.Errorf("%s: index %q, want stage 0 at %s", name, got, sha)
		}
		if got := out(t, filepath.Join(super, "libs", name), "rev-parse", "HEAD"); got != sha {
			t.Errorf("%s: checkout %s, want %s", name, got, sha)
		}
		st, err := p.Status(ctx, "libs/"+name)
		if err != nil || st.PinConflict != nil || !st.PinStaged {
			t.Errorf("%s after Resolve: %v, conflict %v, staged %v", name, err, st.PinConflict, st.PinStaged)
		}
	}
	if err := p.Resolve(ctx, "libs/alpha", out(t, super, "rev-parse", "HEAD")); err == nil {
		t.Error("Resolve of a resolved pin succeeded")
	}
	out(t, super, "-c", "user.name=A", "-c", "user.email=a@b", "commit", "-q", "--no-edit")
	assertExact(t, super)
}

// Resolving with no SHA removes the gitlink: what a side that removed the
// submodule chose.
func TestResolveRemove(t *testing.T) {
	p, _, super := conflictFixture(t)
	if err := p.Resolve(context.Background(), "libs/beta", ""); err != nil {
		t.Fatal(err)
	}
	if got := out(t, super, "ls-files", "-s", "--", "libs/beta"); got != "" {
		t.Errorf("index still has %q", got)
	}
}

// A side that removed the submodule with its .gitmodules entry leaves a
// conflicted gitlink without a row: the superproject shows it, and it
// counts as unmerged.
func TestPinConflictWithoutRow(t *testing.T) {
	_, _, super := conflictFixture(t)
	ctx := context.Background()
	git := func(args ...string) {
		t.Helper()
		out(t, super, append([]string{"-c", "user.name=A", "-c", "user.email=a@b"}, args...)...)
	}
	git("merge", "--abort")
	git("rm", "-q", "-f", "libs/beta")
	git("commit", "-q", "-m", "Remove beta")
	exec.Command("git", "-C", super, "-c", "user.name=A", "-c", "user.email=a@b", "merge", "feat").Run() // stops at the conflicts

	p, err := New(super)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := p.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	submodule.Walk(repo.Submodules, func(s *submodule.Submodule) {
		if s.Path == "libs/beta" {
			t.Error("libs/beta still has a row")
		}
	})
	root, err := p.Status(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if root.Unmerged != 3 {
		t.Errorf("Unmerged = %d, want 3 (alpha, gamma, beta)", root.Unmerged)
	}
	if root.Worktree != submodule.WorktreeConflict || root.Changes == nil || len(root.Changes.Files) != 1 || root.Changes.Files[0].Path != "libs/beta" {
		t.Errorf("superproject: worktree %v, changes %+v; want the conflict on libs/beta only", root.Worktree, root.Changes)
	}
}
