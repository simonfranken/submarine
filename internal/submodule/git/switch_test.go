package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"submarine/internal/submodule"
)

func findBranch(t *testing.T, bs []submodule.Branch, name string) submodule.Branch {
	t.Helper()
	for _, b := range bs {
		if b.Name == name {
			return b
		}
	}
	t.Fatalf("no branch %q in %v", name, bs)
	return submodule.Branch{}
}

func TestBranchesListsLocalAndRemote(t *testing.T) {
	p, root := ownFixture(t)
	_ = root
	bs, err := p.Branches(context.Background(), "libs/feature")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	seenRemote := false
	for _, b := range bs {
		names = append(names, b.Name)
		if b.Remote {
			seenRemote = true
		} else if seenRemote {
			t.Errorf("local branch %s after a remote one: %v", b.Name, names)
		}
		if b.Name == "origin/HEAD" {
			t.Error("origin/HEAD must not be listed")
		}
	}
	rb := findBranch(t, bs, "origin/feat/search")
	if !rb.Remote || rb.Local != "feat/search" {
		t.Errorf("origin/feat/search = %+v", rb)
	}
	findBranch(t, bs, "origin/main")
}

func TestSwitchSubmoduleStagesTipAndTracksRemoteBranch(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	sub := filepath.Join(super, "libs", "feature")
	ctx := context.Background()

	bs, err := p.Branches(ctx, "libs/feature")
	if err != nil {
		t.Fatal(err)
	}
	b := findBranch(t, bs, "origin/feat/search")
	got, err := p.Switch(ctx, "libs/feature", b)
	if err != nil {
		t.Fatal(err)
	}
	if got != "feat/search" {
		t.Errorf("Switch returned %q, want feat/search", got)
	}
	if head := out(t, sub, "branch", "--show-current"); head != "feat/search" {
		t.Errorf("branch = %q", head)
	}
	if up := out(t, sub, "rev-parse", "--abbrev-ref", "feat/search@{upstream}"); up != "origin/feat/search" {
		t.Errorf("upstream = %q", up)
	}
	if head := out(t, sub, "rev-parse", "HEAD"); head != b.SHA {
		t.Errorf("HEAD = %s, want %s", head, b.SHA)
	}
	if staged := out(t, super, "ls-files", "-s", "--", "libs/feature"); !strings.Contains(staged, b.SHA) {
		t.Errorf("index = %q, want the tip %s staged", staged, b.SHA)
	}
	assertExact(t, sub)
}

func TestSwitchNeverMovesAnExistingLocalBranch(t *testing.T) {
	p, root := ownFixture(t)
	sub := filepath.Join(root, "super", "libs", "feature")
	ctx := context.Background()

	git(t, sub, "branch", "feat/search", "origin/main") // same name, elsewhere
	before := out(t, sub, "rev-parse", "feat/search")
	bs, err := p.Branches(ctx, "libs/feature")
	if err != nil {
		t.Fatal(err)
	}
	b := findBranch(t, bs, "origin/feat/search")
	got, err := p.Switch(ctx, "libs/feature", b)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("Switch returned %q, want detached", got)
	}
	if head := out(t, sub, "branch", "--show-current"); head != "" {
		t.Errorf("HEAD on %q, want detached", head)
	}
	if after := out(t, sub, "rev-parse", "feat/search"); after != before {
		t.Errorf("branch moved: %s -> %s", before, after)
	}
	if head := out(t, sub, "rev-parse", "HEAD"); head != b.SHA {
		t.Errorf("HEAD = %s, want %s", head, b.SHA)
	}
}

func TestSwitchAbortsWhenTheBranchMoved(t *testing.T) {
	p, root := ownFixture(t)
	sub := filepath.Join(root, "super", "libs", "feature")
	ctx := context.Background()

	bs, err := p.Branches(ctx, "libs/feature")
	if err != nil {
		t.Fatal(err)
	}
	b := findBranch(t, bs, "origin/main")
	b.SHA = out(t, sub, "rev-parse", "origin/feat/search") // what the list showed is stale
	if _, err := p.Switch(ctx, "libs/feature", b); err == nil || !strings.Contains(err.Error(), "moved") {
		t.Errorf("err = %v, want a moved error", err)
	}
}

func TestSwitchSuperprojectRestoresSubmodulesToItsPins(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	ctx := context.Background()

	git(t, super, "branch", "older", "HEAD~1")
	bs, err := p.Branches(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	b := findBranch(t, bs, "older")
	got, err := p.Switch(ctx, "", b)
	if err != nil {
		t.Fatal(err)
	}
	if got != "older" || out(t, super, "branch", "--show-current") != "older" {
		t.Errorf("Switch = %q, branch = %q", got, out(t, super, "branch", "--show-current"))
	}
	assertExact(t, super)
}

func TestSwitchLossCountsChangedFiles(t *testing.T) {
	p, root := ownFixture(t)
	sub := filepath.Join(root, "super", "libs", "feature")
	ctx := context.Background()

	bs, err := p.Branches(ctx, "libs/feature")
	if err != nil {
		t.Fatal(err)
	}
	b := findBranch(t, bs, "origin/feat/search")
	loss, err := p.SwitchLoss(ctx, "libs/feature", b)
	if err != nil {
		t.Fatal(err)
	}
	if !loss.Empty() {
		t.Errorf("clean checkout: loss = %+v", loss)
	}
	if err := os.WriteFile(filepath.Join(sub, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if loss, err = p.SwitchLoss(ctx, "libs/feature", b); err != nil || loss.Files != 1 {
		t.Errorf("loss = %+v, %v; want 1 file", loss, err)
	}
}

func TestBranchesOfAMissingSubmoduleSaysSo(t *testing.T) {
	p, _ := ownFixture(t)
	if _, err := p.Branches(context.Background(), "libs/missing"); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("err = %v", err)
	}
}
