package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"submarine/internal/submodule"
)

// restoreArgs are prepended to every git command of a restore: the user's
// submodule.recurse must not make checkout and reset recurse behind our back,
// the restore walks the submodules itself.
var restoreArgs = []string{"-c", "submodule.recurse=false"}

// maxWalk bounds the entries visited when looking for repositories inside an
// untracked directory.
const maxWalk = 20000

// gitlink is a submodule entry of a tree or the index.
type gitlink struct{ path, sha string }

// RestoreLoss reports what Restore(path) would discard. It walks the
// repositories on disk, not the loaded tree.
func (p *Provider) RestoreLoss(ctx context.Context, path string, to submodule.RestoreTarget) (submodule.Loss, error) {
	dir, target, err := p.restoreTarget(ctx, path, to)
	if err != nil {
		return submodule.Loss{}, err
	}
	var loss submodule.Loss
	if dir != "" { // not initialized: nothing on disk to lose
		loss = p.lossOf(ctx, dir, target, to.KeepsStaged())
	}
	if to == submodule.RestoreHard && path != "" && p.pinStaged(ctx, path, target) {
		// a hard restore unstages the submodule's own pin too
		loss.StagedPins++
		loss.Modules = max(loss.Modules, 1)
	}
	return loss, nil
}

// pinStaged reports whether the pin of path in its parent's index differs
// from committed, the pin a hard restore goes to.
func (p *Provider) pinStaged(ctx context.Context, path, committed string) bool {
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return false
	}
	staged, err := pinOf(ctx, in, submodule.RestoreStaged)
	return err == nil && staged != committed
}

// Restore makes path ("" is the superproject) and all submodules below it
// exactly what git records, see docs/actions.md. Staged pins are kept. With
// RestoreCommitted, a submodule's own staged pin is unstaged first; with
// RestoreHard, the staged pins below are dropped too. A missing submodule is
// cloned and missing commits are fetched; git never prompts.
func (p *Provider) Restore(ctx context.Context, path string, to submodule.RestoreTarget) error {
	if path == "" {
		return p.restoreRepo(ctx, p.root, "HEAD", nil, to.KeepsStaged())
	}
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown submodule %q", path)
	}
	pin, err := pinOf(ctx, in, to)
	if err != nil {
		return err
	}
	if to.Committed() {
		if _, err := p.exec(ctx, in.superDir, nil, "update-index", "--cacheinfo", "160000,"+pin+","+in.relPath); err != nil {
			return err
		}
	}
	if !initialized(in.dir) {
		if err := p.initSubmodules(ctx, in.superDir, []string{in.relPath}); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return p.restoreRepo(ctx, in.dir, pin, &in, to.KeepsStaged())
}

// restoreTarget returns the directory and the commit path is restored to:
// HEAD for the superproject, else the pin to. The directory is "" if the
// submodule is not initialized.
func (p *Provider) restoreTarget(ctx context.Context, path string, to submodule.RestoreTarget) (dir, target string, err error) {
	if path == "" {
		return p.root, "HEAD", nil
	}
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("unknown submodule %q", path)
	}
	pin, err := pinOf(ctx, in, to)
	if err != nil {
		return "", "", err
	}
	if !initialized(in.dir) {
		return "", pin, nil
	}
	return in.dir, pin, nil
}

// pinOf returns the gitlink of a submodule in its parent's index (staged)
// or HEAD (committed). A hard restore of a submodule that was added but not
// committed yet uses the index: there is no staged pin to drop.
func pinOf(ctx context.Context, in info, to submodule.RestoreTarget) (string, error) {
	if to.Committed() {
		out, _, err := try(ctx, in.superDir, "ls-tree", "HEAD", "--", in.relPath)
		if err != nil {
			return "", err
		}
		if pin := parseGitlink(out); pin != "" {
			return pin, nil
		}
		if to != submodule.RestoreHard {
			return "", errors.New("has no committed pin (added, but not committed yet)")
		}
	}
	out, err := run(ctx, in.superDir, "ls-files", "-s", "--", in.relPath)
	if err != nil {
		return "", err
	}
	pin := parseGitlink(out)
	if pin == "" {
		return "", errors.New("has no gitlink in the superproject index (stale .gitmodules entry?)")
	}
	return pin, nil
}

// --- restore -----------------------------------------------------------------

// restoreRepo restores the initialized repository at dir to target (a commit
// or "HEAD") and then every submodule its index records. Branches are never
// moved: a checkout that is not on target is detached first. If the checkout
// stays on target and keep is set, its staged pins are kept and are the
// submodules' targets; else the submodules follow target's tree.
// For a submodule (in is not nil) a detached HEAD is then put on a branch at
// target if there is one, see attachBranch; the submodules below always are.
func (p *Provider) restoreRepo(ctx context.Context, dir, target string, in *info, keep bool) error {
	env := p.networkEnv()
	g := func(args ...string) (string, error) {
		return p.exec(ctx, dir, env, append(append([]string{}, restoreArgs...), args...)...)
	}

	sha, err := g("rev-parse", "--verify", "-q", target+"^{commit}")
	if err != nil {
		if sha, err = p.fetchMissing(ctx, dir, target); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
	}
	if gitDir, _, err := gitDirs(dir); err == nil {
		p.clearOperations(ctx, dir, gitDir)
	}
	prev, _, _ := try(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD") // for the ref, see attachBranch
	var kept []gitlink
	if head, err := g("rev-parse", "--verify", "-q", "HEAD"); err != nil || head != sha {
		if _, err := g("checkout", "--force", "--detach", sha); err != nil {
			return err
		}
	} else if keep {
		if kept, err = stagedPins(ctx, dir); err != nil {
			return err
		}
	}
	if _, err := g("reset", "--hard", "-q"); err != nil {
		return err
	}
	for _, l := range kept {
		if _, err := g("update-index", "--cacheinfo", "160000,"+l.sha+","+l.path); err != nil {
			return err
		}
	}
	// -ff also removes untracked repositories; ignored files stay
	if _, err := g("clean", "-ffdq"); err != nil {
		return err
	}
	if _, err := g("submodule", "sync", "-q"); err != nil {
		return err
	}
	if in != nil {
		if err := p.attachBranch(ctx, *in, sha, prev); err != nil {
			return err
		}
	}

	links, err := indexGitlinks(ctx, dir)
	if err != nil {
		return err
	}
	var missing []string
	for _, l := range links {
		if !initialized(filepath.Join(dir, filepath.FromSlash(l.path))) {
			missing = append(missing, l.path)
		}
	}
	if len(missing) > 0 {
		if err := p.initSubmodules(ctx, dir, missing); err != nil {
			return err
		}
	}

	// the .gitmodules branch of each child, for its ref
	branches := map[string]string{}
	if out, ok, err := try(ctx, dir, "config", "-f", ".gitmodules", "--get-regexp", `^submodule\..*\.(path|branch)$`); err != nil {
		return err
	} else if ok {
		for _, m := range parseGitmodules(out) {
			branches[path.Clean(m.path)] = m.branch
		}
	}

	// the children are independent repositories
	var wg sync.WaitGroup
	errs := make([]error, len(links))
	for i, l := range links {
		wg.Add(1)
		go func() {
			defer wg.Done()
			child := info{superDir: dir, dir: filepath.Join(dir, filepath.FromSlash(l.path)), relPath: l.path, branch: branches[l.path]}
			if err := p.restoreRepo(ctx, child.dir, l.sha, &child, keep); err != nil {
				errs[i] = err
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// attachBranch puts the detached HEAD of the restored submodule in on a
// branch whose tip is sha, the first that git accepts of, in this order:
//
//  1. a local branch tracking the ref the UI shows (see resolve), or named
//     like it;
//  2. the ref itself, as a new local branch tracking it, if no local branch
//     has its name;
//  3. any other local branch, alphabetically;
//  4. any other remote branch whose name no local branch has, as a new
//     tracking branch; the ref's remote first, then alphabetically.
//
// A HEAD that is on a branch stays there. No branch is moved: a local branch
// elsewhere is no candidate, and its remote branch neither. If nothing fits,
// HEAD stays detached; only a context error is returned. prev is the branch
// checked out before the restore; the ref depends on it.
func (p *Provider) attachBranch(ctx context.Context, in info, sha, prev string) error {
	if _, onBranch, err := try(ctx, in.dir, "symbolic-ref", "-q", "HEAD"); err != nil || onBranch {
		return err
	}
	out, ok, err := try(ctx, in.dir, "for-each-ref", "--points-at="+sha,
		"--format=%(refname)%00%(upstream)%00%(symref)", "refs/heads", "refs/remotes")
	if err != nil || !ok {
		return err
	}
	var locals, remotes []string // alphabetical, as for-each-ref sorts them
	upstreams := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x00")
		if len(f) != 3 || f[2] != "" { // a symref is origin/HEAD
			continue
		}
		if name, ok := strings.CutPrefix(f[0], "refs/heads/"); ok {
			locals = append(locals, name)
			upstreams[name] = strings.TrimPrefix(f[1], "refs/remotes/")
		} else if name, ok := strings.CutPrefix(f[0], "refs/remotes/"); ok {
			remotes = append(remotes, name)
		}
	}
	if len(locals)+len(remotes) == 0 {
		return nil
	}

	// the ref, if it is a remote branch
	var ref string
	containing, err := remoteContaining(ctx, in.dir, sha)
	if err != nil {
		return err
	}
	if r, source, _, err := resolve(ctx, in, sha, prev, containing); err != nil {
		return err
	} else if source != submodule.RefFromTag && source != submodule.RefFromSHA {
		ref = r
	}

	localOf := func(remote string) string { _, l, _ := strings.Cut(remote, "/"); return l }
	hasLocal := func(name string) (bool, error) {
		_, ok, err := try(ctx, in.dir, "rev-parse", "--verify", "-q", "refs/heads/"+name)
		return ok, err
	}
	type candidate struct{ local, remote string } // remote: create local, tracking it
	var cands []candidate
	addRemote := func(r string) error {
		l := localOf(r)
		if l == "" {
			return nil
		}
		exists, err := hasLocal(l)
		if err == nil && !exists {
			cands = append(cands, candidate{l, r})
		}
		return err
	}
	if ref != "" {
		for _, l := range locals {
			if upstreams[l] == ref {
				cands = append(cands, candidate{local: l})
			}
		}
		if l := localOf(ref); slices.Contains(locals, l) {
			cands = append(cands, candidate{local: l})
		}
		if slices.Contains(remotes, ref) {
			if err := addRemote(ref); err != nil {
				return err
			}
		}
	}
	for _, l := range locals {
		cands = append(cands, candidate{local: l})
	}
	preferred, _, _ := strings.Cut(ref, "/")
	if preferred == "" {
		if preferred, err = remoteName(ctx, in.dir); err != nil {
			return err
		}
	}
	slices.SortStableFunc(remotes, func(a, b string) int {
		ap, bp := strings.HasPrefix(a, preferred+"/"), strings.HasPrefix(b, preferred+"/")
		switch {
		case ap && !bp:
			return -1
		case bp && !ap:
			return 1
		}
		return 0
	})
	for _, r := range remotes {
		if r != ref {
			if err := addRemote(r); err != nil {
				return err
			}
		}
	}

	for _, c := range cands {
		args := append(append([]string{}, restoreArgs...), "switch", "-q", c.local)
		if c.remote != "" {
			args = append(append([]string{}, restoreArgs...), "switch", "-q", "-c", c.local, "--track", c.remote)
		}
		if _, err := p.exec(ctx, in.dir, nil, args...); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
		// e.g. checked out in another worktree: try the next one
	}
	return nil
}

// networkEnv is the environment of commands that may clone or fetch.
func (p *Provider) networkEnv() []string {
	return append(append([]string{}, p.sshEnv...), p.extraEnv...)
}

// initSubmodules clones or checks out the given submodules of the repository
// at dir, whatever their active and update settings say (--checkout overrides
// update = none/rebase/merge, the explicit paths override inactive).
func (p *Provider) initSubmodules(ctx context.Context, dir string, rels []string) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout*time.Duration(len(rels)))
	defer cancel()
	args := append(append([]string{}, restoreArgs...), "submodule", "update", "--init", "--checkout", "--force", "-q", "--")
	_, err := p.exec(ctx, dir, p.networkEnv(), append(args, rels...)...)
	return err
}

// exec runs git like runEnv, but counts against the concurrency limit. Hold
// it only for the command, never while recursing.
func (p *Provider) exec(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return runEnv(ctx, dir, env, args...)
}

// fetchMissing fetches all remotes of dir to get target, and returns its SHA.
func (p *Provider) fetchMissing(ctx context.Context, dir, target string) (string, error) {
	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	// a failing fetch is reported with the missing commit below
	_, ferr := runEnv(fctx, dir, p.networkEnv(), "fetch", "--all", "--no-recurse-submodules", "--quiet")
	sha, ok, err := try(ctx, dir, "rev-parse", "--verify", "-q", target+"^{commit}")
	if err != nil {
		return "", err
	}
	if !ok {
		msg := fmt.Sprintf("commit %.7s is not available", target)
		if ferr != nil {
			msg += " (fetch failed: " + firstLine(ferr) + ")"
		} else {
			msg += " on any remote"
		}
		return "", errors.New(msg)
	}
	return sha, nil
}

func firstLine(err error) string {
	var ge *gitError
	msg := err.Error()
	if errors.As(err, &ge) && ge.Stderr != "" {
		msg = ge.Stderr
	}
	line, _, _ := strings.Cut(msg, "\n")
	return strings.TrimPrefix(line, "fatal: ")
}

// clearOperations ends a merge, rebase, cherry-pick or revert in progress
// without moving any ref (--quit, not --abort).
func (p *Provider) clearOperations(ctx context.Context, dir, gitDir string) {
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(gitDir, name)); return err == nil }
	if exists("rebase-merge") || exists("rebase-apply") {
		_, _, _ = try(ctx, dir, "rebase", "--quit")
	}
	if exists("CHERRY_PICK_HEAD") || exists("sequencer") {
		_, _, _ = try(ctx, dir, "cherry-pick", "--quit")
	}
	if exists("REVERT_HEAD") {
		_, _, _ = try(ctx, dir, "revert", "--quit")
	}
	// a merge is ended by the reset --hard that follows
}

// treeGitlinks lists the submodules recorded in commit's tree, at any depth
// of directories.
func treeGitlinks(ctx context.Context, dir, commit string) ([]gitlink, error) {
	out, err := run(ctx, dir, "ls-tree", "-r", "-z", commit)
	if err != nil {
		return nil, err
	}
	var links []gitlink
	for _, rec := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		if f := strings.Fields(meta); ok && len(f) == 3 && f[0] == "160000" {
			links = append(links, gitlink{path: path, sha: f[2]})
		}
	}
	return links, nil
}

// indexGitlinks lists the submodules in the index of the repository at dir.
func indexGitlinks(ctx context.Context, dir string) ([]gitlink, error) {
	out, err := run(ctx, dir, "ls-files", "-s", "-z")
	if err != nil {
		return nil, err
	}
	var links []gitlink
	for _, rec := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		if f := strings.Fields(meta); ok && len(f) == 3 && f[0] == "160000" && f[2] == "0" {
			links = append(links, gitlink{path: path, sha: f[1]})
		}
	}
	return links, nil
}

// stagedPins lists the gitlinks of the index of dir that differ from HEAD's,
// i.e. staged pins of submodules that HEAD has. A submodule that was added
// but not committed is not one: its .gitmodules entry is not kept either.
func stagedPins(ctx context.Context, dir string) ([]gitlink, error) {
	index, err := indexGitlinks(ctx, dir)
	if err != nil {
		return nil, err
	}
	tree, err := treeGitlinks(ctx, dir, "HEAD")
	if err != nil {
		return nil, err
	}
	committed := map[string]string{}
	for _, l := range tree {
		committed[l.path] = l.sha
	}
	var staged []gitlink
	for _, l := range index {
		if c, ok := committed[l.path]; ok && c != l.sha {
			staged = append(staged, l)
		}
	}
	return staged, nil
}

// --- loss --------------------------------------------------------------------

// lossOf adds up what a restore of the repository at dir to target
// discards, including the submodules on disk below it. target is "" if it is
// not known; then commits are only judged by the refs that reach them. keep
// is false for a hard restore, which drops all staged pins.
func (p *Provider) lossOf(ctx context.Context, dir, target string, keep bool) submodule.Loss {
	var own submodule.Loss
	var children []gitlink
	var targets map[string]string
	unknown := func() { own.Unknown++ }

	func() {
		select {
		case p.sem <- struct{}{}:
			defer func() { <-p.sem }()
		case <-ctx.Done():
			unknown()
			return
		}

		out, err := run(ctx, dir, "status", "--porcelain=v2", "-z", "--untracked-files=normal", "--ignore-submodules=all")
		if err != nil {
			unknown()
			return
		}
		for _, e := range parseStatus(out) {
			if e.submodule {
				continue // restored without loss
			}
			own.Files++
			if e.kind == submodule.ChangeUntracked && strings.HasSuffix(e.path, "/") {
				own.Repositories += standaloneRepos(ctx, filepath.Join(dir, filepath.FromSlash(e.path)))
			}
		}

		if gitDir, _, err := gitDirs(dir); err == nil {
			for _, name := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "sequencer"} {
				if _, err := os.Stat(filepath.Join(gitDir, name)); err == nil {
					own.Operations++
					break
				}
			}
		}

		// commits only this checkout reaches are lost if the checkout moves
		head, err := run(ctx, dir, "rev-parse", "--verify", "-q", "HEAD")
		if err != nil {
			unknown()
		} else {
			moves := target == ""
			if !moves {
				if sha, ok, _ := try(ctx, dir, "rev-parse", "--verify", "-q", target+"^{commit}"); ok {
					moves = sha != head
				} else {
					moves = true
				}
			}
			if moves {
				n, ok, _ := try(ctx, dir, "rev-list", "--count", "HEAD", "--not", "--branches", "--tags", "--remotes")
				if ok {
					fmt.Sscan(n, &own.Commits)
				} else {
					unknown()
				}
			}

			// staged pins are kept, unless the checkout moves away from
			// the commit they were staged on or the restore is hard; then
			// the submodules follow the target's tree
			staged, err := stagedPins(ctx, dir)
			if err != nil {
				unknown()
			}
			var links []gitlink
			switch {
			case !moves && keep:
				links, err = indexGitlinks(ctx, dir)
			case !moves:
				own.StagedPins = len(staged)
				links, err = treeGitlinks(ctx, dir, "HEAD")
			case target != "":
				own.StagedPins = len(staged)
				links, err = treeGitlinks(ctx, dir, target)
			default:
				own.StagedPins = len(staged)
			}
			if err == nil && links != nil {
				targets = map[string]string{}
				for _, l := range links {
					targets[l.path] = l.sha
				}
			}
		}

		if children, err = indexGitlinks(ctx, dir); err != nil {
			unknown()
		}
	}()

	if own.Files+own.Operations+own.Commits+own.Repositories+own.StagedPins+own.Unknown > 0 {
		own.Modules = 1
	}

	// the children on disk, concurrently
	results := make([]submodule.Loss, len(children))
	var wg sync.WaitGroup
	for i, c := range children {
		sub := filepath.Join(dir, filepath.FromSlash(c.path))
		if !initialized(sub) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = p.lossOf(ctx, sub, targets[c.path], keep)
		}()
	}
	wg.Wait()
	for _, r := range results {
		own = own.Add(r)
	}
	return own
}

// standaloneRepos counts the repositories with their own .git directory in
// the untracked directory dir (or dir itself). An absorbed submodule leaves
// only a .git file; its commits live on in .git/modules.
func standaloneRepos(ctx context.Context, dir string) int {
	n, seen := 0, 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if seen++; err != nil || seen > maxWalk || ctx.Err() != nil {
			return filepath.SkipAll
		}
		if d.IsDir() && d.Name() == ".git" {
			n++
			return filepath.SkipDir
		}
		return nil
	})
	return n
}
