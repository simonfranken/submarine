// Package git provides a read-only submodule.Provider backed by the git
// command line. It never touches the network.
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

	"submarine/internal/submodule"
)

// maxContainsCandidates limits how many remote branches containing the pin
// are compared when resolving the pin by RefFromContains.
const maxContainsCandidates = 16

// maxBetween limits the commits listed between pin and tip.
const maxBetween = 50

// Provider reads a superproject and its submodules with git.
type Provider struct {
	root string        // absolute superproject root
	sem  chan struct{} // bounds concurrent Status calls

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
		root: root,
		sem:  make(chan struct{}, runtime.NumCPU()),
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
	if dir, ok, err := try(ctx, p.root, "rev-parse", "--path-format=absolute", "--git-common-dir"); err != nil {
		return nil, err
	} else if ok {
		if fi, err := os.Stat(filepath.Join(dir, "FETCH_HEAD")); err == nil {
			repo.FetchedAt = fi.ModTime()
		}
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
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown submodule %q", path)
	}
	return status(ctx, in)
}

func status(ctx context.Context, in info) (*submodule.Status, error) {
	// pin: index gitlink, compared with HEAD
	out, err := run(ctx, in.superDir, "ls-files", "-s", "--", in.relPath)
	if err != nil {
		return nil, err
	}
	pin := parseGitlink(out)
	if pin == "" {
		return nil, errors.New("no gitlink in the superproject index")
	}
	out, _, err = try(ctx, in.superDir, "ls-tree", "HEAD", "--", in.relPath)
	if err != nil {
		return nil, err
	}
	st := &submodule.Status{PinStaged: parseGitlink(out) != pin}

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

	// the pin must exist locally to say anything about it
	if _, ok, err := try(ctx, in.dir, "cat-file", "-e", pin+"^{commit}"); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("pinned commit %.7s not fetched", pin)
	}

	// remote branches containing the pin, oldest tip first
	out, err = run(ctx, in.dir, "for-each-ref", "--contains", pin, "--sort=committerdate", "--format=%(refname)", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var containing []string
	for _, r := range strings.Fields(out) {
		if !strings.HasSuffix(r, "/HEAD") {
			containing = append(containing, strings.TrimPrefix(r, "refs/remotes/"))
		}
	}
	st.Pin.Pushed = len(containing) > 0

	ref, source, err := resolve(ctx, in, pin, branch, containing)
	if err != nil {
		return nil, err
	}
	st.Pin.Ref, st.Pin.Source = ref, source

	// commits
	if st.Pinned, err = commit(ctx, in.dir, pin); err != nil {
		return nil, err
	}
	st.CheckedOut = st.Pinned
	if head != pin {
		if st.CheckedOut, err = commit(ctx, in.dir, head); err != nil {
			return nil, err
		}
	}
	if source == submodule.RefFromSHA {
		st.Pin.Ref = st.Pinned.SHA
		return st, nil
	}
	if st.Tip, err = commit(ctx, in.dir, ref); err != nil {
		return nil, err
	}
	out, err = run(ctx, in.dir, "rev-list", "--left-right", "--count", pin+"..."+ref)
	if err != nil {
		return nil, err
	}
	if st.Pin.Ahead, st.Pin.Behind, err = parseLeftRight(out); err != nil {
		return nil, err
	}
	if st.Pin.Ahead+st.Pin.Behind > 0 {
		out, err = run(ctx, in.dir, "log", commitFormat, fmt.Sprintf("--max-count=%d", maxBetween), pin+"..."+ref)
		if err != nil {
			return nil, err
		}
		st.Between = parseCommits(out)
	}
	return st, nil
}

// resolve finds the ref that describes the pin, following the order in
// docs/state-model.md. containing lists the remote branches containing the
// pin, oldest tip first.
func resolve(ctx context.Context, in info, pin, branch string, containing []string) (string, submodule.RefSource, error) {
	remote, err := remoteName(ctx, in.dir)
	if err != nil {
		return "", 0, err
	}

	// 1. branch from .gitmodules
	if b := in.branch; b != "" && remote != "" {
		if b == "." {
			b, _, err = try(ctx, in.superDir, "symbolic-ref", "--short", "-q", "HEAD")
			if err != nil {
				return "", 0, err
			}
		}
		if b != "" {
			if _, ok, err := try(ctx, in.dir, "rev-parse", "--verify", "-q", "refs/remotes/"+remote+"/"+b); err != nil {
				return "", 0, err
			} else if ok {
				return remote + "/" + b, submodule.RefFromGitmodules, nil
			}
		}
	}

	// 2. upstream of the checked-out branch
	if branch != "" {
		if up, ok, err := try(ctx, in.dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", branch+"@{upstream}"); err != nil {
			return "", 0, err
		} else if ok && up != "" {
			return up, submodule.RefFromCurrentBranch, nil
		}
	}

	// 3. remote default branch, if it contains the pin
	if remote != "" {
		if def, ok, err := try(ctx, in.dir, "symbolic-ref", "--short", "-q", "refs/remotes/"+remote+"/HEAD"); err != nil {
			return "", 0, err
		} else if ok {
			if _, ok, err := try(ctx, in.dir, "merge-base", "--is-ancestor", pin, def); err != nil {
				return "", 0, err
			} else if ok {
				return def, submodule.RefFromDefaultBranch, nil
			}
		}
	}

	// 4. remote branch containing the pin with the closest tip
	best, bestN := "", -1
	for _, r := range containing[:min(len(containing), maxContainsCandidates)] {
		out, err := run(ctx, in.dir, "rev-list", "--count", pin+".."+r)
		if err != nil {
			return "", 0, err
		}
		var n int
		if _, err := fmt.Sscan(out, &n); err == nil && (bestN < 0 || n < bestN) {
			best, bestN = r, n
		}
	}
	if best != "" {
		return best, submodule.RefFromContains, nil
	}

	// 5. nearest tag
	if tag, ok, err := try(ctx, in.dir, "describe", "--tags", "--abbrev=0", pin); err != nil {
		return "", 0, err
	} else if ok && tag != "" {
		return tag, submodule.RefFromTag, nil
	}

	// 6. the commit itself
	return "", submodule.RefFromSHA, nil
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
