package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"submarine/internal/submodule"
)

// switchDir returns the directory of path ("" is the superproject), and the
// submodule's info if it is one.
func (p *Provider) switchDir(path string) (dir string, in info, sub bool, err error) {
	if path == "" {
		return p.root, info{}, false, nil
	}
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return "", info{}, false, fmt.Errorf("unknown submodule %q", path)
	}
	if !initialized(in.dir) {
		return "", info{}, false, errors.New("not initialized; restore it first")
	}
	return in.dir, in, true, nil
}

// Branches lists the local and remote-tracking branches of path, local ones
// first, each group by the date of its tip, newest first. It reads only what
// is on disk.
func (p *Provider) Branches(ctx context.Context, path string) ([]submodule.Branch, error) {
	dir, _, _, err := p.switchDir(path)
	if err != nil {
		return nil, err
	}
	out, err := run(ctx, dir, "for-each-ref",
		"--format=%(refname)%00%(objectname)%00%(committerdate:unix)%00%(HEAD)%00%(symref)",
		"refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var branches []submodule.Branch
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x00")
		if len(f) != 5 || f[4] != "" { // a symref is origin/HEAD
			continue
		}
		b := submodule.Branch{Ref: f[0], SHA: f[1], Current: f[3] == "*"}
		if secs, err := strconv.ParseInt(f[2], 10, 64); err == nil {
			b.When = time.Unix(secs, 0)
		}
		if name, ok := strings.CutPrefix(f[0], "refs/heads/"); ok {
			b.Name, b.Local = name, name
		} else if name, ok := strings.CutPrefix(f[0], "refs/remotes/"); ok {
			_, local, _ := strings.Cut(name, "/")
			b.Name, b.Local, b.Remote = name, local, true
		} else {
			continue
		}
		branches = append(branches, b)
	}
	slices.SortStableFunc(branches, func(a, b submodule.Branch) int {
		switch {
		case a.Remote != b.Remote:
			if a.Remote {
				return 1
			}
			return -1
		default:
			return b.When.Compare(a.When)
		}
	})
	return branches, nil
}

// SwitchLoss reports what switching path to b would discard: the same as a
// restore to b's tip.
func (p *Provider) SwitchLoss(ctx context.Context, path string, b submodule.Branch) (submodule.Loss, error) {
	dir, _, _, err := p.switchDir(path)
	if err != nil {
		return submodule.Loss{}, err
	}
	return p.lossOf(ctx, dir, b.SHA), nil
}

// Switch makes b's tip the staged pin of a submodule, restores the repository
// to it (see Restore) and puts HEAD on the branch. For a remote branch the
// local branch is the one checked out before if it tracks b, else the one
// of that name, created tracking b if there is none. A local branch that is
// only behind the tip is fast-forwarded; one with commits of its own, or
// checked out in another worktree, is never moved: HEAD stays detached and
// "" is returned.
func (p *Provider) Switch(ctx context.Context, path string, b submodule.Branch) (string, error) {
	dir, in, sub, err := p.switchDir(path)
	if err != nil {
		return "", err
	}
	if sha, ok, err := try(ctx, dir, "rev-parse", "--verify", "-q", b.Ref+"^{commit}"); err != nil {
		return "", err
	} else if !ok || sha != b.SHA {
		return "", fmt.Errorf("%s moved; open the list again", b.Name)
	}
	prev, _, err := try(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return "", err
	}
	if sub { // staged first, so a failed restore can be repeated with x
		if _, err := p.exec(ctx, in.superDir, nil, "update-index", "--cacheinfo", "160000,"+b.SHA+","+in.relPath); err != nil {
			return "", err
		}
	}
	if err := p.restoreRepo(ctx, dir, b.SHA, nil); err != nil { // HEAD goes on b below
		return "", err
	}

	g := func(args ...string) (string, error) {
		return p.exec(ctx, dir, nil, append(append([]string{}, restoreArgs...), args...)...)
	}
	if !b.Remote {
		_, err := g("switch", "-q", b.Local)
		return b.Local, err
	}
	local := b.Local
	if prev != "" && prev != local {
		if up, ok, err := upstream(ctx, dir, prev); err != nil {
			return "", err
		} else if ok && up == b.Name {
			local = prev // e.g. dev tracking origin/main: an update stays on dev
		}
	}
	switch tip, ok, err := try(ctx, dir, "rev-parse", "--verify", "-q", "refs/heads/"+local); {
	case err != nil:
		return "", err
	case !ok:
		_, err := g("switch", "-q", "-c", local, "--track", b.Name)
		return local, err
	case tip == b.SHA:
		_, err := g("switch", "-q", local)
		return local, err
	default:
		if ok, err := p.fastForward(ctx, dir, local, tip, b.SHA); err != nil || !ok {
			return "", err
		}
		_, err := g("switch", "-q", local)
		return local, err
	}
}

// fastForward moves the local branch from tip to sha if that loses nothing:
// tip is an ancestor of sha and no worktree has the branch checked out. It
// reports whether the branch was moved; a branch that moved meanwhile is
// left alone.
func (p *Provider) fastForward(ctx context.Context, dir, local, tip, sha string) (bool, error) {
	if _, ok, err := try(ctx, dir, "merge-base", "--is-ancestor", tip, sha); err != nil || !ok {
		return false, err
	}
	wts, err := run(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(wts, "\n") {
		if line == "branch refs/heads/"+local {
			return false, nil
		}
	}
	if _, ok, err := try(ctx, dir, "update-ref", "-m", "submarine: fast-forward", "refs/heads/"+local, sha, tip); err != nil || !ok {
		return false, err
	}
	return true, nil
}
