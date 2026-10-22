package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
func (p *Provider) RestoreLoss(ctx context.Context, path string) (submodule.Loss, error) {
	dir, target, err := p.restoreTarget(ctx, path)
	if err != nil {
		return submodule.Loss{}, err
	}
	if dir == "" { // not initialized: nothing on disk to lose
		return submodule.Loss{}, nil
	}
	return p.lossOf(ctx, dir, target), nil
}

// Restore makes path ("" is the superproject) and all submodules below it
// exactly what git records, see docs/actions.md. A missing submodule is
// cloned and missing commits are fetched; git never prompts.
func (p *Provider) Restore(ctx context.Context, path string) error {
	if path == "" {
		return p.restoreRepo(ctx, p.root, "HEAD")
	}
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown submodule %q", path)
	}
	pin, err := indexGitlink(ctx, in)
	if err != nil {
		return err
	}
	if !initialized(in.dir) {
		if err := p.initSubmodules(ctx, in.superDir, []string{in.relPath}); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return p.restoreRepo(ctx, in.dir, pin)
}

// restoreTarget returns the directory and the commit path is restored to:
// HEAD for the superproject, else the gitlink in its parent's index. The
// directory is "" if the submodule is not initialized.
func (p *Provider) restoreTarget(ctx context.Context, path string) (dir, target string, err error) {
	if path == "" {
		return p.root, "HEAD", nil
	}
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("unknown submodule %q", path)
	}
	pin, err := indexGitlink(ctx, in)
	if err != nil {
		return "", "", err
	}
	if !initialized(in.dir) {
		return "", pin, nil
	}
	return in.dir, pin, nil
}

func indexGitlink(ctx context.Context, in info) (string, error) {
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
// or "HEAD") and then every submodule that target records. Branches are never
// moved: a checkout that is not on target is detached first.
func (p *Provider) restoreRepo(ctx context.Context, dir, target string) error {
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
	if head, err := g("rev-parse", "--verify", "-q", "HEAD"); err != nil || head != sha {
		if _, err := g("checkout", "--force", "--detach", sha); err != nil {
			return err
		}
	}
	if _, err := g("reset", "--hard", "-q"); err != nil {
		return err
	}
	// -ff also removes untracked repositories; ignored files stay
	if _, err := g("clean", "-ffdq"); err != nil {
		return err
	}
	if _, err := g("submodule", "sync", "-q"); err != nil {
		return err
	}

	links, err := treeGitlinks(ctx, dir, sha)
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

	// the children are independent repositories
	var wg sync.WaitGroup
	errs := make([]error, len(links))
	for i, l := range links {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.restoreRepo(ctx, filepath.Join(dir, filepath.FromSlash(l.path)), l.sha); err != nil {
				errs[i] = err
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
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

// --- loss --------------------------------------------------------------------

// lossOf adds up what a restore of the repository at dir to target
// discards, including the submodules on disk below it. target is "" if it is
// not known; then commits are only judged by the refs that reach them.
func (p *Provider) lossOf(ctx context.Context, dir, target string) submodule.Loss {
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
		}

		if children, err = indexGitlinks(ctx, dir); err != nil {
			unknown()
		}
		if target != "" {
			if links, err := treeGitlinks(ctx, dir, target); err == nil {
				targets = map[string]string{}
				for _, l := range links {
					targets[l.path] = l.sha
				}
			}
		}
	}()

	if own.Files+own.Operations+own.Commits+own.Repositories+own.Unknown > 0 {
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
			results[i] = p.lossOf(ctx, sub, targets[c.path])
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
