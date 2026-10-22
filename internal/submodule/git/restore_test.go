package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"submarine/internal/submodule"
)

// out runs git and returns its trimmed stdout.
func out(t *testing.T, dir string, args ...string) string {
	t.Helper()
	b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(b))
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// assertExact checks that the superproject at dir and every submodule below
// it is initialized, on its pin and has no change at all.
func assertExact(t *testing.T, dir string) {
	t.Helper()
	if s := out(t, dir, "status", "--porcelain"); s != "" {
		t.Errorf("%s not clean:\n%s", dir, s)
	}
	raw, err := exec.Command("git", "-C", dir, "submodule", "status", "--recursive").Output() // the prefix is a leading space
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		if line[0] != ' ' {
			t.Errorf("submodule not exact: %s", line)
			continue
		}
		f := strings.Fields(line)
		if s := out(t, filepath.Join(dir, f[1]), "status", "--porcelain"); s != "" {
			t.Errorf("%s not clean:\n%s", f[1], s)
		}
	}
}

func TestRestoreSuperprojectMakesEverythingExact(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	ctx := context.Background()

	// ignored files stay
	if err := os.WriteFile(filepath.Join(super, ".git", "info", "exclude"), []byte("keep.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(super, "libs", "work", "keep.log")
	// libs/work is a submodule: its own exclude file lives in its git dir
	wkGit := out(t, filepath.Join(super, "libs", "work"), "rev-parse", "--absolute-git-dir")
	if err := os.WriteFile(filepath.Join(wkGit, "info", "exclude"), []byte("keep.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// a branch must not move
	conflict := filepath.Join(super, "libs", "conflict")
	mainTip := out(t, conflict, "rev-parse", "main")

	if err := p.Restore(ctx, ""); err != nil {
		t.Fatal(err)
	}
	assertExact(t, super)
	if got := out(t, conflict, "rev-parse", "main"); got != mainTip {
		t.Errorf("branch main moved: %s -> %s", mainTip, got)
	}
	if !exists(keep) {
		t.Error("ignored file was deleted")
	}
	if exists(filepath.Join(conflict, ".git", "MERGE_HEAD")) || exists(filepath.Join(out(t, conflict, "rev-parse", "--absolute-git-dir"), "MERGE_HEAD")) {
		t.Error("merge still in progress")
	}
	if !exists(filepath.Join(super, "libs", "missing", ".git")) {
		t.Error("libs/missing was not initialized")
	}
	if out(t, super, "diff", "--cached", "--name-only") != "" {
		t.Error("staged pin survived a superproject restore")
	}
}

func TestRestoreSubmoduleKeepsStagedPinAsTarget(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	staged := out(t, filepath.Join(super, "libs", "staged"), "rev-parse", "HEAD")
	// the fixture staged the checked-out commit; move the checkout away
	git(t, filepath.Join(super, "libs", "staged"), "checkout", "-q", "--detach", "HEAD~1")

	if err := p.Restore(context.Background(), "libs/staged"); err != nil {
		t.Fatal(err)
	}
	if got := out(t, filepath.Join(super, "libs", "staged"), "rev-parse", "HEAD"); got != staged {
		t.Errorf("HEAD = %s, want the staged pin %s", got, staged)
	}
	if !strings.Contains(out(t, super, "diff", "--cached", "--name-only"), "libs/staged") {
		t.Error("restoring a submodule must keep its staged pin")
	}
}

func TestRestoreLoss(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	ctx := context.Background()

	loss := func(path string) submodule.Loss {
		t.Helper()
		l, err := p.RestoreLoss(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}

	// off pin and missing lose nothing; neither does an unpushed pin
	for _, path := range []string{"src/core", "libs/missing", "src/net", "libs/staged"} {
		if l := loss(path); !l.Empty() {
			t.Errorf("%s: loss %+v, want none", path, l)
		}
	}

	// libs/work: 6 files and a directory, untracked and changed
	if l := loss("libs/work"); l.Files < 6 || l.Modules != 1 {
		t.Errorf("libs/work: %+v", l)
	}
	// a nested change counts for its ancestors
	if l := loss("libs/log"); l.Files != 1 || l.Modules != 1 {
		t.Errorf("libs/log: %+v, want the nested change", l)
	}
	// a merge in progress
	if l := loss("libs/conflict"); l.Operations != 1 || l.Files == 0 {
		t.Errorf("libs/conflict: %+v", l)
	}

	// commits: detached and unreachable are lost, on a branch they are not
	ui := filepath.Join(super, "src", "ui")
	git(t, ui, "checkout", "-q", "--detach")
	git(t, ui, "commit", "-q", "--allow-empty", "-m", "detached work")
	if l := loss("src/ui"); l.Commits != 1 {
		t.Errorf("detached commit: %+v", l)
	}
	git(t, ui, "switch", "-q", "-c", "wip")
	if l := loss("src/ui"); l.Commits != 0 {
		t.Errorf("commit on a branch: %+v", l)
	}

	// a clone nobody added as a submodule
	git(t, super, "clone", "-q", filepath.Join(root, "remotes", "core.git"), "stray")
	if l := loss(""); l.Repositories != 1 {
		t.Errorf("root: %+v, want one standalone repository", l)
	}
}

func TestRestoreRemovesLeftoversAndEndsRebase(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	ctx := context.Background()

	// a standalone clone is deleted
	git(t, super, "clone", "-q", filepath.Join(root, "remotes", "core.git"), "stray")

	// a submodule the branch no longer has leaves its directory behind
	git(t, super, "rm", "-q", "--cached", "libs/topic")
	git(t, super, "commit", "-q", "-m", "Drop topic")

	// a rebase in progress in vendor; the branches must survive
	v := filepath.Join(super, "vendor")
	base := out(t, v, "rev-parse", "HEAD")
	git(t, v, "switch", "-q", "-c", "x")
	os.WriteFile(filepath.Join(v, "README"), []byte("x\n"), 0o644)
	git(t, v, "commit", "-q", "-am", "x")
	git(t, v, "switch", "-q", "-c", "y", base)
	os.WriteFile(filepath.Join(v, "README"), []byte("y\n"), 0o644)
	git(t, v, "commit", "-q", "-am", "y")
	yTip := out(t, v, "rev-parse", "y")
	_ = exec.Command("git", "-C", v, "rebase", "x").Run() // conflicts
	if !exists(filepath.Join(out(t, v, "rev-parse", "--absolute-git-dir"), "rebase-merge")) {
		t.Fatal("setup: no rebase in progress")
	}

	if err := p.Restore(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"stray", "libs/topic"} {
		if exists(filepath.Join(super, gone)) {
			t.Errorf("%s still exists", gone)
		}
	}
	if exists(filepath.Join(out(t, v, "rev-parse", "--absolute-git-dir"), "rebase-merge")) {
		t.Error("rebase still in progress")
	}
	if got := out(t, v, "rev-parse", "y"); got != yTip {
		t.Errorf("branch y moved: %s -> %s", yTip, got)
	}
	if out(t, super, "status", "--porcelain") != "" {
		t.Errorf("superproject not clean:\n%s", out(t, super, "status", "--porcelain"))
	}
}

func TestRestoreClonesAMissingSubmodule(t *testing.T) {
	p, root := ownFixture(t)
	p.extraEnv = []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=protocol.file.allow", "GIT_CONFIG_VALUE_0=always"}
	super := filepath.Join(root, "super")

	// no object database left: it must be cloned again
	if err := os.RemoveAll(filepath.Join(super, ".git", "modules", "libs", "missing")); err != nil {
		t.Fatal(err)
	}
	if err := p.Restore(context.Background(), "libs/missing"); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(super, "libs", "missing", ".git")) {
		t.Error("not cloned")
	}
	if got, want := out(t, filepath.Join(super, "libs", "missing"), "rev-parse", "HEAD"), strings.Fields(out(t, super, "ls-files", "-s", "libs/missing"))[1]; got != want {
		t.Errorf("HEAD = %s, want pin %s", got, want)
	}
}

func TestRestoreReportsAnUnavailablePin(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	// pin a commit that exists on no remote and not in the submodule
	core := filepath.Join(super, "src", "core")
	bogus := "0123456789abcdef0123456789abcdef01234567"
	git(t, super, "update-index", "--cacheinfo", "160000,"+bogus+",src/core")
	_ = core
	err := p.Restore(context.Background(), "src/core")
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("err = %v, want a message about the unavailable commit", err)
	}
}

func TestRestoreIgnoresUpdateNoneAndInactive(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	git(t, super, "config", "submodule.libs/missing.update", "none")
	git(t, super, "config", "submodule.libs/missing.active", "false")
	git(t, super, "config", "submodule.active", "src")

	if err := p.Restore(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(super, "libs", "missing", ".git")) {
		t.Error("libs/missing not initialized")
	}
	assertExact(t, super)
}
