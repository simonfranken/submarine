// Package git provides a submodule.Provider backed by the git command line.
// It reads; it writes only in Restore, and uses the network only in Fetch and
// in Restore (for what is missing).
package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"submarine/internal/submodule"
)

// maxPinBranches limits Pin.Branches, the other remote branches containing the
// pin.
const maxPinBranches = 8

// maxContainsCandidates limits how many remote branches containing the pin
// are compared when the pin is not on its target.
const maxContainsCandidates = 32

// Provider reads a superproject and its submodules with git.
type Provider struct {
	root     string        // absolute superproject root
	sem      chan struct{} // bounds concurrent Status calls
	fetchSem chan struct{} // bounds concurrent fetches
	sshEnv   []string      // extra environment for fetches
	extraEnv []string      // more environment for fetches and clones; tests

	// WatchInterval is how often Watch checks for changes.
	WatchInterval time.Duration

	mu   sync.RWMutex
	mods map[string]info // by Submodule.Path; filled by Tree
}

// info is what Status needs about a submodule beyond its path.
type info struct {
	superDir string // absolute path of the parent repository
	dir      string // absolute path of the submodule
	relPath  string // relative to superDir, slash-separated
	branch   string // branch from .gitmodules, may be "" or "."
}

// New returns a provider for the repository containing dir.
func New(dir string) (*Provider, error) {
	root, err := run(context.Background(), dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%s is not a git repository", dir)
	}
	return &Provider{
		root:          root,
		sem:           make(chan struct{}, runtime.NumCPU()),
		fetchSem:      make(chan struct{}, maxFetches),
		sshEnv:        sshBatchEnv(root),
		WatchInterval: 2 * time.Second,
	}, nil
}

func (p *Provider) Tree(ctx context.Context) (*submodule.Repo, error) {
	repo := &submodule.Repo{Path: abbrevHome(p.root)}
	if b, ok, err := try(ctx, p.root, "symbolic-ref", "--short", "-q", "HEAD"); err != nil {
		return nil, err
	} else if ok {
		repo.Branch = b
	} else {
		repo.Branch = "(detached)"
	}
	if out, ok, err := try(ctx, p.root, "rev-list", "--left-right", "--count", "HEAD...@{upstream}"); err != nil {
		return nil, err
	} else if ok {
		repo.Ahead, repo.Behind, _ = parseLeftRight(out)
	}

	_, _, repo.Operation = operation(ctx, p.root)

	repo.Root = &submodule.Submodule{Name: filepath.Base(p.root), Path: ""}
	if remote, err := remoteName(ctx, p.root); err != nil {
		return nil, err
	} else if remote != "" {
		repo.Root.URL, _, _ = try(ctx, p.root, "config", "--get", "remote."+remote+".url")
	}

	mods := map[string]info{}
	subs, err := p.build(ctx, p.root, nil, mods)
	if err != nil {
		return nil, err
	}
	repo.Submodules = subs

	p.mu.Lock()
	p.mods = mods
	p.mu.Unlock()
	return repo, nil
}

// build reads the .gitmodules of the repository at dir and recurses into
// initialized submodules.
func (p *Provider) build(ctx context.Context, dir string, parent *submodule.Submodule, mods map[string]info) ([]*submodule.Submodule, error) {
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); err != nil {
		return nil, nil
	}
	out, _, err := try(ctx, dir, "config", "-f", ".gitmodules", "--get-regexp", `^submodule\..*\.(path|url|branch)$`)
	if err != nil {
		return nil, err
	}
	var subs []*submodule.Submodule
	for _, g := range parseGitmodules(out) {
		rel := path.Clean(g.path)
		full := rel
		if parent != nil {
			full = path.Join(parent.Path, rel)
		}
		s := &submodule.Submodule{
			Name:    path.Base(rel),
			Path:    full,
			RelPath: rel,
			URL:     g.url,
			Parent:  parent,
		}
		subDir := filepath.Join(dir, filepath.FromSlash(rel))
		mods[full] = info{superDir: dir, dir: subDir, relPath: rel, branch: g.branch}
		if initialized(subDir) {
			if s.Children, err = p.build(ctx, subDir, s, mods); err != nil {
				return nil, err
			}
		}
		subs = append(subs, s)
	}
	return subs, nil
}

func (p *Provider) Status(ctx context.Context, path string) (*submodule.Status, error) {
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if path == "" {
		return p.rootStatus(ctx)
	}
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown submodule %q", path)
	}
	return status(ctx, in)
}

func status(ctx context.Context, in info) (*submodule.Status, error) {
	// pin: index gitlink, compared with HEAD; ours during a conflict
	stages, err := indexStages(ctx, in)
	if err != nil {
		return nil, err
	}
	pin := stages.pin()
	if pin == "" {
		return nil, errors.New("listed in .gitmodules, but has no gitlink in the superproject index (stale entry?)")
	}
	out, _, err := try(ctx, in.superDir, "ls-tree", "HEAD", "--", in.relPath)
	if err != nil {
		return nil, err
	}
	committed := parseGitlink(out)
	st := &submodule.Status{PinStaged: !stages.conflicted() && committed != pin}
	if stages.conflicted() {
		st.PinConflict = conflictSides(ctx, in.superDir, stages)
	}

	// presence
	if _, err := os.Stat(in.dir); errors.Is(err, fs.ErrNotExist) {
		st.Presence = submodule.DirMissing
	} else if !initialized(in.dir) {
		st.Presence = submodule.NotInitialized
	}
	if st.Presence != submodule.PresenceOK {
		st.Pinned = &submodule.Commit{SHA: pin[:min(7, len(pin))]}
		return st, nil
	}

	if gitDir, _, err := gitDirs(in.dir); err == nil {
		if fi, err := os.Stat(filepath.Join(gitDir, "FETCH_HEAD")); err == nil {
			st.FetchedAt = fi.ModTime()
		}
	}

	// checkout and worktree
	head, err := run(ctx, in.dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	branch, onBranch, err := try(ctx, in.dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return nil, err
	}
	switch {
	case head != pin:
		st.Checkout = submodule.CheckoutDiffers
	case !onBranch:
		st.Checkout = submodule.CheckoutDetached
	}
	out, err = run(ctx, in.dir, "status", "--porcelain=v2", "-z", "--untracked-files=normal", "--ignore-submodules=dirty")
	if err != nil {
		return nil, err
	}
	entries := parseStatus(out)
	st.Worktree = worktreeOf(entries)
	if st.Changes, err = changes(ctx, in.dir, entries); err != nil {
		return nil, err
	}

	if st.PinConflict != nil {
		// the side the pin stands for may not be fetched: then only the
		// checkout is known
		if _, ok, err := try(ctx, in.dir, "cat-file", "-e", pin+"^{commit}"); err != nil {
			return nil, err
		} else if ok {
			err = pinState(ctx, in, st, pin, head, branch, nil)
		} else {
			st.Pinned = &submodule.Commit{SHA: pin[:min(7, len(pin))]}
			st.CheckedOut, err = commit(ctx, in.dir, head)
		}
		if err != nil {
			return nil, err
		}
		if err := conflictState(ctx, in, st, stages[1], head, branch); err != nil {
			return nil, err
		}
		return st, nil
	}
	if err := pinState(ctx, in, st, pin, head, branch, nil); err != nil {
		return nil, err
	}
	if st.PinStaged && committed != "" {
		if err := committedState(ctx, in, st, committed, pin); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// committedState fills the pin of the superproject's HEAD and how it compares
// with the staged pin. It stays empty if the commit is not on disk: the staged
// pin is what matters, so this is no reason to fail.
func committedState(ctx context.Context, in info, st *submodule.Status, committed, pin string) error {
	if _, ok, err := try(ctx, in.dir, "cat-file", "-e", committed+"^{commit}"); err != nil || !ok {
		return err
	}
	c, err := commit(ctx, in.dir, committed)
	if err != nil {
		return err
	}
	out, err := run(ctx, in.dir, "rev-list", "--left-right", "--count", committed+"..."+pin)
	if err != nil {
		return err
	}
	if st.CommittedDrift.Ahead, st.CommittedDrift.Behind, err = parseLeftRight(out); err != nil {
		return err
	}
	st.Committed = c
	if st.CommittedDrift.Ahead > 0 && st.CommittedDrift.Behind > 0 {
		if st.Bases.Committed, err = mergeBase(ctx, in.dir, committed, pin); err != nil {
			return err
		}
	}
	return nil
}

// mergeBase returns the commit where the histories of a and b split; nil if
// they have none in common.
func mergeBase(ctx context.Context, dir, a, b string) (*submodule.Commit, error) {
	base, ok, err := try(ctx, dir, "merge-base", a, b)
	if err != nil || !ok || base == "" {
		return nil, err
	}
	return commit(ctx, dir, base)
}

// rootStatus describes the superproject. Its "pin" is HEAD, compared with the
// upstream of the current branch; submodules are ignored, their rows show
// them.
func (p *Provider) rootStatus(ctx context.Context) (*submodule.Status, error) {
	in := info{superDir: p.root, dir: p.root}
	st := &submodule.Status{Root: true}
	if gitDir, _, err := gitDirs(in.dir); err == nil {
		if fi, err := os.Stat(filepath.Join(gitDir, "FETCH_HEAD")); err == nil {
			st.FetchedAt = fi.ModTime()
		}
	}

	head, err := run(ctx, in.dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	branch, onBranch, err := try(ctx, in.dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return nil, err
	}
	if !onBranch {
		st.Checkout = submodule.CheckoutDetached
	}
	out, err := run(ctx, in.dir, "status", "--porcelain=v2", "-z", "--untracked-files=normal", "--ignore-submodules=all")
	if err != nil {
		return nil, err
	}
	entries := parseStatus(out)
	// a leftover of --ignore-submodules=all in older git versions would
	// report the submodules the rows already show
	entries = slices.DeleteFunc(entries, func(e entry) bool { return e.submodule })
	st.Worktree = worktreeOf(entries)
	if st.Changes, err = changes(ctx, in.dir, entries); err != nil {
		return nil, err
	}

	// the root is compared with its upstream, so that the header and the
	// detail pane agree; only without one the target is looked for
	var chosen *candidate
	if branch != "" {
		if up, ok, err := try(ctx, in.dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", branch+"@{upstream}"); err != nil {
			return nil, err
		} else if ok {
			chosen = &candidate{up, submodule.RefFromCurrentBranch}
		}
	}
	if err := pinState(ctx, in, st, head, head, branch, chosen); err != nil {
		return nil, err
	}
	return st, nil
}

// pinState fills the fields of st that describe pin: whether it is pushed, the
// ref it resolves to (chosen, if not nil), the commits and the distances: of
// the checkout to the pin and of the pin to the ref. head is the checked-out
// commit.
func pinState(ctx context.Context, in info, st *submodule.Status, pin, head, branch string, chosen *candidate) error {
	// the pin must exist locally to say anything about it
	if _, ok, err := try(ctx, in.dir, "cat-file", "-e", pin+"^{commit}"); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("pinned commit %.7s not fetched", pin)
	}

	containing, err := remoteContaining(ctx, in.dir, pin)
	if err != nil {
		return err
	}
	st.Pin.Pushed = len(containing) > 0

	var ref string
	var source submodule.RefSource
	if chosen != nil {
		ref, source = chosen.ref, chosen.source
	} else if ref, source, st.Pin.Target, err = resolve(ctx, in, pin, branch, containing); err != nil {
		return err
	}
	st.Pin.Ref, st.Pin.Source = ref, source
	for _, b := range containing {
		if b != ref && len(st.Pin.Branches) < maxPinBranches {
			st.Pin.Branches = append(st.Pin.Branches, b)
		}
	}

	// commits
	if st.Pinned, err = commit(ctx, in.dir, pin); err != nil {
		return err
	}
	st.CheckedOut = st.Pinned
	var out string
	if head != pin {
		if st.CheckedOut, err = commit(ctx, in.dir, head); err != nil {
			return err
		}
		out, err = run(ctx, in.dir, "rev-list", "--left-right", "--count", head+"..."+pin)
		if err != nil {
			return err
		}
		if st.Drift.Ahead, st.Drift.Behind, err = parseLeftRight(out); err != nil {
			return err
		}
		if st.Drift.Ahead > 0 && st.Drift.Behind > 0 {
			if st.Bases.Checkout, err = mergeBase(ctx, in.dir, pin, head); err != nil {
				return err
			}
		}
	}
	if source == submodule.RefFromSHA {
		st.Pin.Ref = st.Pinned.SHA
		return nil
	}
	if st.Tip, err = commit(ctx, in.dir, ref); err != nil {
		return err
	}
	out, err = run(ctx, in.dir, "rev-list", "--left-right", "--count", pin+"..."+ref)
	if err != nil {
		return err
	}
	if st.Pin.Ahead, st.Pin.Behind, err = parseLeftRight(out); err != nil {
		return err
	}
	if st.Pin.Ahead > 0 && st.Pin.Behind > 0 {
		if st.Bases.Tip, err = mergeBase(ctx, in.dir, pin, ref); err != nil {
			return err
		}
	}

	// the pin is on another branch than its target: where the target is
	if target := st.Pin.Target; target != "" && target != ref {
		if st.TargetTip, err = commit(ctx, in.dir, target); err != nil {
			return err
		}
		out, err = run(ctx, in.dir, "rev-list", "--left-right", "--count", target+"..."+pin)
		if err != nil {
			return err
		}
		if st.TargetDrift.Ahead, st.TargetDrift.Behind, err = parseLeftRight(out); err != nil {
			return err
		}
		if st.TargetDrift.Ahead > 0 && st.TargetDrift.Behind > 0 {
			if st.Bases.Target, err = mergeBase(ctx, in.dir, pin, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// remoteContaining lists the remote branches containing pin, oldest tip
// first, without the origin/HEAD symref.
func remoteContaining(ctx context.Context, dir, pin string) ([]string, error) {
	out, err := run(ctx, dir, "for-each-ref", "--contains", pin, "--sort=committerdate", "--format=%(refname)", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var containing []string
	for _, r := range strings.Fields(out) {
		if !strings.HasSuffix(r, "/HEAD") {
			containing = append(containing, strings.TrimPrefix(r, "refs/remotes/"))
		}
	}
	return containing, nil
}

// resolve finds the ref the pin is compared with, following the order in
// docs/state-model.md: the target (see targetRef) if the pin is on it, else the
// branch containing the pin that is closest to it, else the target anyway,
// the upstream of the checked-out branch, a tag, the commit itself. It also
// returns the target ("" if there is none). containing lists the remote
// branches containing the pin, oldest tip first.
func resolve(ctx context.Context, in info, pin, branch string, containing []string) (string, submodule.RefSource, string, error) {
	target, source, hasTarget, err := targetRef(ctx, in)
	if err != nil {
		return "", 0, "", err
	}
	if !hasTarget {
		target = ""
	}
	if hasTarget && slices.Contains(containing, target) {
		return target, source, target, nil
	}
	if len(containing) > 0 {
		ref, err := closestContaining(ctx, in, pin, branch, containing)
		return ref, submodule.RefFromContains, target, err
	}
	if hasTarget {
		return target, source, target, nil
	}

	// the checked-out branch depends on the machine, so it is no target
	if up, ok, err := upstream(ctx, in.dir, branch); err != nil {
		return "", 0, "", err
	} else if ok {
		return up, submodule.RefFromCurrentBranch, "", nil
	}
	if tag, ok, err := try(ctx, in.dir, "describe", "--tags", "--abbrev=0", pin); err != nil {
		return "", 0, "", err
	} else if ok && tag != "" {
		return tag, submodule.RefFromTag, "", nil
	}
	return "", submodule.RefFromSHA, "", nil
}

// upstream returns the upstream of the local branch, if there is one.
func upstream(ctx context.Context, dir, branch string) (string, bool, error) {
	if branch == "" {
		return "", false, nil
	}
	return try(ctx, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", branch+"@{upstream}")
}

// closestContaining picks the branch the pin is closest to among the remote
// branches containing it: the fewest commits after the pin. A tie goes to the
// upstream of the checked-out branch, then to the first name in alphabetical
// order, so that the answer is the same on every machine. No branch,
// including the default branch, is preferred otherwise.
func closestContaining(ctx context.Context, in info, pin, branch string, containing []string) (string, error) {
	var best []string
	bestN := -1
	for _, c := range containing[:min(len(containing), maxContainsCandidates)] {
		out, err := run(ctx, in.dir, "rev-list", "--count", pin+".."+c)
		if err != nil {
			return "", err
		}
		var n int
		if _, err := fmt.Sscan(out, &n); err != nil {
			continue
		}
		switch {
		case bestN < 0 || n < bestN:
			best, bestN = []string{c}, n
		case n == bestN:
			best = append(best, c)
		}
	}
	if len(best) == 0 {
		return containing[0], nil
	}
	if up, ok, err := upstream(ctx, in.dir, branch); err != nil {
		return "", err
	} else if ok && slices.Contains(best, up) {
		return up, nil
	}
	slices.Sort(best)
	return best[0], nil
}

// targetRef returns the branch the pin should be on: the `branch =` of
// .gitmodules (`.` is the superproject's current branch), else the remote's
// default branch, else main or master. A declared branch that does not exist
// locally is not replaced by the default: the answer would be wrong.
func targetRef(ctx context.Context, in info) (string, submodule.RefSource, bool, error) {
	remote, err := remoteName(ctx, in.dir)
	if err != nil || remote == "" {
		return "", 0, false, err
	}
	exists := func(ref string) (bool, error) {
		_, ok, err := try(ctx, in.dir, "rev-parse", "--verify", "-q", "refs/remotes/"+ref)
		return ok, err
	}

	if b := in.branch; b != "" {
		if b == "." {
			if b, _, err = try(ctx, in.superDir, "symbolic-ref", "--short", "-q", "HEAD"); err != nil {
				return "", 0, false, err
			}
		}
		if b == "" {
			return "", 0, false, nil
		}
		ok, err := exists(remote + "/" + b)
		if err != nil || !ok {
			return "", 0, false, err
		}
		return remote + "/" + b, submodule.RefFromGitmodules, true, nil
	}

	if def, ok, err := try(ctx, in.dir, "symbolic-ref", "--short", "-q", "refs/remotes/"+remote+"/HEAD"); err != nil {
		return "", 0, false, err
	} else if ok {
		return def, submodule.RefFromDefaultBranch, true, nil
	}
	for _, name := range []string{"main", "master"} {
		if ok, err := exists(remote + "/" + name); err != nil {
			return "", 0, false, err
		} else if ok {
			return remote + "/" + name, submodule.RefFromDefaultBranch, true, nil
		}
	}
	return "", 0, false, nil
}

// candidate is a ref and the rule that proposed it.
type candidate struct {
	ref    string
	source submodule.RefSource
}

// remoteName returns "origin" if it exists, else the first remote, else "".
func remoteName(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "remote")
	if err != nil {
		return "", err
	}
	remotes := strings.Fields(out)
	for _, r := range remotes {
		if r == "origin" {
			return r, nil
		}
	}
	if len(remotes) > 0 {
		return remotes[0], nil
	}
	return "", nil
}

// changes describes the changed files of the worktree; nil if there are
// none. Line counts come from one `git diff HEAD`; if that fails (e.g. no
// commit yet) they stay zero.
func changes(ctx context.Context, dir string, entries []entry) (*submodule.Changes, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	var counts map[string]numstat
	if slices.ContainsFunc(entries, func(e entry) bool { return e.tracked && !e.submodule }) {
		out, ok, err := try(ctx, dir, "diff", "HEAD", "--numstat", "-z", "-M", "--no-ext-diff", "--ignore-submodules=dirty")
		if err != nil {
			return nil, err
		}
		if ok {
			counts = parseNumstat(out)
		}
	}

	c := &submodule.Changes{Total: len(entries)}
	for _, e := range entries {
		f := submodule.FileChange{Path: e.path, OrigPath: e.orig, Kind: e.kind, Submodule: e.submodule}
		if n, ok := counts[e.path]; ok && !e.submodule {
			f.Added, f.Removed, f.Binary = n.added, n.removed, n.binary
			c.Added += n.added
			c.Removed += n.removed
		}
		if e.kind == submodule.ChangeUntracked && strings.HasSuffix(e.path, "/") {
			f.Repo = initialized(filepath.Join(dir, filepath.FromSlash(e.path)))
		}
		if e.kind != submodule.ChangeRemoved {
			if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(e.path))); err == nil {
				f.Modified = fi.ModTime()
			}
		}
		c.Files = append(c.Files, f)
	}
	c.Sort()
	if len(c.Files) > submodule.MaxChangeFiles {
		c.Files = c.Files[:submodule.MaxChangeFiles]
	}
	return c, nil
}

func commit(ctx context.Context, dir, rev string) (*submodule.Commit, error) {
	out, err := run(ctx, dir, "log", "-1", commitFormat, rev, "--")
	if err != nil {
		return nil, err
	}
	cs := parseCommits(out)
	if len(cs) == 0 {
		return nil, fmt.Errorf("no commit %s", rev)
	}
	return &cs[0], nil
}

// initialized reports whether dir is a checked-out repository (.git file or
// directory present).
func initialized(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func abbrevHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return p
}
