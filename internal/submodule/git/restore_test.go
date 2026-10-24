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
// it is initialized, on its pin (staged ones included) and has no change
// besides staged pins.
func assertExact(t *testing.T, dir string) {
	t.Helper()
	if s := out(t, dir, "diff", "--name-only", "--ignore-submodules=dirty"); s != "" { // worktree vs index
		t.Errorf("%s not clean:\n%s", dir, s)
	}
	if s := out(t, dir, "ls-files", "--others", "--exclude-standard"); s != "" {
		t.Errorf("%s has untracked files:\n%s", dir, s)
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
		sub := filepath.Join(dir, f[1])
		if s := out(t, sub, "diff", "--name-only", "--ignore-submodules=dirty"); s != "" {
			t.Errorf("%s not clean:\n%s", f[1], s)
		}
		if s := out(t, sub, "ls-files", "--others", "--exclude-standard"); s != "" {
			t.Errorf("%s has untracked files:\n%s", f[1], s)
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

	if err := p.Restore(ctx, "", submodule.RestoreStaged); err != nil {
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
	// the fixture stages libs/staged and libs/conflict
	if got := out(t, super, "diff", "--cached", "--name-only"); got != "libs/conflict\nlibs/staged" {
		t.Errorf("staged = %q, want the staged pins kept", got)
	}
}

func TestRestoreSubmoduleKeepsStagedPinAsTarget(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	staged := out(t, filepath.Join(super, "libs", "staged"), "rev-parse", "HEAD")
	// the fixture staged the checked-out commit; move the checkout away
	git(t, filepath.Join(super, "libs", "staged"), "checkout", "-q", "--detach", "HEAD~1")

	if err := p.Restore(context.Background(), "libs/staged", submodule.RestoreStaged); err != nil {
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
		l, err := p.RestoreLoss(ctx, path, submodule.RestoreStaged)
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

	if err := p.Restore(ctx, "", submodule.RestoreStaged); err != nil {
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
	assertExact(t, super)
}

func TestRestoreClonesAMissingSubmodule(t *testing.T) {
	p, root := ownFixture(t)
	p.extraEnv = []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=protocol.file.allow", "GIT_CONFIG_VALUE_0=always"}
	super := filepath.Join(root, "super")

	// no object database left: it must be cloned again
	if err := os.RemoveAll(filepath.Join(super, ".git", "modules", "libs", "missing")); err != nil {
		t.Fatal(err)
	}
	if err := p.Restore(context.Background(), "libs/missing", submodule.RestoreStaged); err != nil {
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
	err := p.Restore(context.Background(), "src/core", submodule.RestoreStaged)
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

	if err := p.Restore(context.Background(), "", submodule.RestoreStaged); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(super, "libs", "missing", ".git")) {
		t.Error("libs/missing not initialized")
	}
	assertExact(t, super)
}

func TestRestoreKeepsNestedStagedPinAndResetsToIt(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	log := filepath.Join(super, "libs", "log")
	nested := filepath.Join(log, "modules", "nested")

	// stage a new pin of nested in libs/log, then change nested again
	git(t, nested, "commit", "-q", "-am", "Nested work")
	staged := out(t, nested, "rev-parse", "HEAD")
	git(t, log, "add", "modules/nested")
	os.WriteFile(filepath.Join(nested, "README"), []byte("more\n"), 0o644)

	if l, err := p.RestoreLoss(context.Background(), "", submodule.RestoreStaged); err != nil || l.StagedPins != 0 || l.Files == 0 {
		t.Fatalf("loss %+v, %v: want the change, not the staged pin", l, err)
	}
	if err := p.Restore(context.Background(), "", submodule.RestoreStaged); err != nil {
		t.Fatal(err)
	}
	if got := out(t, log, "diff", "--cached", "--name-only"); got != "modules/nested" {
		t.Errorf("libs/log staged = %q, want the nested pin", got)
	}
	if got := out(t, nested, "rev-parse", "HEAD"); got != staged {
		t.Errorf("nested HEAD = %s, want the staged pin %s", got, staged)
	}
	assertExact(t, super)
}

func TestRestoreCommittedUnstagesThePin(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	committed := strings.Fields(out(t, super, "ls-tree", "HEAD", "libs/staged"))[2]

	if err := p.Restore(context.Background(), "libs/staged", submodule.RestoreCommitted); err != nil {
		t.Fatal(err)
	}
	if got := out(t, super, "diff", "--cached", "--name-only"); strings.Contains(got, "libs/staged") {
		t.Errorf("still staged: %q", got)
	}
	if got := out(t, filepath.Join(super, "libs", "staged"), "rev-parse", "HEAD"); got != committed {
		t.Errorf("HEAD = %s, want the committed pin %s", got, committed)
	}
}

func TestRestoreHardDropsStagedPinsAtEveryDepth(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	log := filepath.Join(super, "libs", "log")
	nested := filepath.Join(log, "modules", "nested")
	committed := out(t, nested, "rev-parse", "HEAD")
	base, err := p.RestoreLoss(context.Background(), "", submodule.RestoreHard)
	if err != nil {
		t.Fatal(err)
	}

	// stage a new pin of nested in libs/log, on a commit only nested's
	// detached HEAD reaches
	git(t, nested, "checkout", "-q", "--detach")
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "Nested work")
	git(t, log, "add", "modules/nested")

	l, err := p.RestoreLoss(context.Background(), "", submodule.RestoreHard)
	if err != nil {
		t.Fatal(err)
	}
	if base.StagedPins == 0 || l.StagedPins != base.StagedPins+1 || l.Commits != base.Commits+1 {
		t.Errorf("loss %+v, want the fixture's %d staged pins, the nested one and its commit", l, base.StagedPins)
	}
	if err := p.Restore(context.Background(), "", submodule.RestoreHard); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{super, log} {
		if got := out(t, dir, "diff", "--cached", "--name-only"); got != "" {
			t.Errorf("%s still staged: %q", dir, got)
		}
	}
	if got := out(t, nested, "rev-parse", "HEAD"); got != committed {
		t.Errorf("nested HEAD = %s, want the committed pin %s", got, committed)
	}
	assertExact(t, super)
}

func TestRestoreHardOfASubmoduleUnstagesItsPin(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	committed := strings.Fields(out(t, super, "ls-tree", "HEAD", "libs/staged"))[2]

	if err := p.Restore(context.Background(), "libs/staged", submodule.RestoreHard); err != nil {
		t.Fatal(err)
	}
	if got := out(t, super, "diff", "--cached", "--name-only"); strings.Contains(got, "libs/staged") {
		t.Errorf("still staged: %q", got)
	}
	if got := out(t, filepath.Join(super, "libs", "staged"), "rev-parse", "HEAD"); got != committed {
		t.Errorf("HEAD = %s, want the committed pin %s", got, committed)
	}
}

func TestRestoreLossOfAHardRestoreCountsTheOwnStagedPin(t *testing.T) {
	p, _ := ownFixture(t)
	committed, err := p.RestoreLoss(context.Background(), "libs/staged", submodule.RestoreCommitted)
	if err != nil {
		t.Fatal(err)
	}
	hard, err := p.RestoreLoss(context.Background(), "libs/staged", submodule.RestoreHard)
	if err != nil {
		t.Fatal(err)
	}
	if hard.StagedPins != committed.StagedPins+1 || hard.Modules == 0 {
		t.Errorf("hard loss %+v, want the own staged pin on top of %+v", hard, committed)
	}
	// without a staged pin, a hard restore unstages nothing of its own
	clean, err := p.RestoreLoss(context.Background(), "src/ui", submodule.RestoreHard)
	if err != nil {
		t.Fatal(err)
	}
	if clean.StagedPins != 0 {
		t.Errorf("loss of src/ui %+v, want no staged pin", clean)
	}
}

func TestRestoreLossCountsStagedPinsOfAMovingRepository(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	log := filepath.Join(super, "libs", "log")
	nested := filepath.Join(log, "modules", "nested")

	// libs/log moves on (off pin, on its branch) and stages a nested pin
	git(t, nested, "commit", "-q", "-am", "Nested work")
	git(t, log, "add", "modules/nested")
	git(t, log, "commit", "-q", "--allow-empty", "-m", "Log work")
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "More nested work")
	git(t, log, "add", "modules/nested")

	l, err := p.RestoreLoss(context.Background(), "libs/log", submodule.RestoreStaged)
	if err != nil {
		t.Fatal(err)
	}
	// the nested commits are on a detached HEAD that moves back: lost too
	if l.StagedPins != 1 || l.Commits != 2 {
		t.Errorf("loss %+v, want 1 staged pin and the 2 detached nested commits", l)
	}
}

// restoreSub restores the submodule path and returns the branch its HEAD is
// on ("" if detached).
func restoreSub(t *testing.T, p *Provider, super, path string) string {
	t.Helper()
	if err := p.Restore(context.Background(), path, submodule.RestoreStaged); err != nil {
		t.Fatal(err)
	}
	return out(t, filepath.Join(super, filepath.FromSlash(path)), "branch", "--show-current")
}

func TestRestoreAttachesTheLocalBranchTrackingTheRef(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	// src/ui: in sync, the pin is behind origin/main, local main is at the pin
	git(t, filepath.Join(super, "src", "ui"), "checkout", "-q", "--detach")
	git(t, filepath.Join(super, "src", "ui"), "branch", "aaa") // alphabetically first, but no ref

	if got := restoreSub(t, p, super, "src/ui"); got != "main" {
		t.Errorf("branch = %q, want main", got)
	}
}

func TestRestoreCreatesTheRefBranchBeforeOtherBranches(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	log := filepath.Join(super, "libs", "log")
	// libs/log: the pin is the tip of origin/main and origin/archive
	git(t, log, "checkout", "-q", "--detach")
	git(t, log, "branch", "-D", "main")

	if got := restoreSub(t, p, super, "libs/log"); got != "main" {
		t.Errorf("branch = %q, want main", got)
	}
	if up := out(t, log, "rev-parse", "--abbrev-ref", "main@{upstream}"); up != "origin/main" {
		t.Errorf("upstream = %q, want origin/main", up)
	}
	if out(t, log, "branch", "--list", "archive") != "" {
		t.Error("archive must not be created")
	}
}

func TestRestoreCreatesARemoteFeatureBranch(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	topic := filepath.Join(super, "libs", "topic")
	mainTip := out(t, topic, "rev-parse", "main")
	// libs/topic: detached at the tip of origin/feat/search, local main elsewhere

	if got := restoreSub(t, p, super, "libs/topic"); got != "feat/search" {
		t.Errorf("branch = %q, want feat/search", got)
	}
	if got := out(t, topic, "rev-parse", "main"); got != mainTip {
		t.Errorf("main moved: %s -> %s", mainTip, got)
	}
}

func TestRestorePrefersLocalBranchesAlphabetically(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	feature := filepath.Join(super, "libs", "feature")
	// libs/feature: the pin is on no branch tip
	git(t, feature, "branch", "zz")
	git(t, feature, "branch", "aa")

	if got := restoreSub(t, p, super, "libs/feature"); got != "aa" {
		t.Errorf("branch = %q, want aa", got)
	}
}

func TestRestoreNeverMovesALocalBranchToAttach(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	topic := filepath.Join(super, "libs", "topic")
	git(t, topic, "branch", "feat/search", "origin/main") // same name, elsewhere
	before := out(t, topic, "rev-parse", "feat/search")

	if got := restoreSub(t, p, super, "libs/topic"); got != "" {
		t.Errorf("branch = %q, want detached", got)
	}
	if after := out(t, topic, "rev-parse", "feat/search"); after != before {
		t.Errorf("feat/search moved: %s -> %s", before, after)
	}
}

func TestRestoreKeepsTheCheckedOutBranch(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	log := filepath.Join(super, "libs", "log")
	git(t, log, "switch", "-q", "-c", "zz") // at the pin, like main

	if got := restoreSub(t, p, super, "libs/log"); got != "zz" {
		t.Errorf("branch = %q, want zz kept", got)
	}
}

func TestRestoreSkipsABranchOfAnotherWorktree(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	log := filepath.Join(super, "libs", "log")
	git(t, log, "checkout", "-q", "--detach")
	git(t, log, "worktree", "add", "-q", filepath.Join(root, "wt"), "main")

	if got := restoreSub(t, p, super, "libs/log"); got == "main" {
		t.Error("main is checked out in another worktree")
	}
}

func TestRestoreSuperprojectAttachesNestedSubmodules(t *testing.T) {
	p, root := ownFixture(t)
	super := filepath.Join(root, "super")
	nested := filepath.Join(super, "libs", "log", "modules", "nested")
	git(t, nested, "checkout", "-q", "--detach")

	if err := p.Restore(context.Background(), "", submodule.RestoreStaged); err != nil {
		t.Fatal(err)
	}
	if got := out(t, nested, "branch", "--show-current"); got != "main" {
		t.Errorf("nested branch = %q, want main", got)
	}
	if got := out(t, filepath.Join(super, "libs", "topic"), "branch", "--show-current"); got != "feat/search" {
		t.Errorf("libs/topic branch = %q, want feat/search", got)
	}
	if got := out(t, super, "branch", "--show-current"); got != "main" {
		t.Errorf("superproject branch = %q, want main", got)
	}
}
