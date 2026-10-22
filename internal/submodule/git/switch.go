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
// local branch of that name is created, tracking it. A local branch that
// exists elsewhere is never moved: HEAD stays detached and "" is returned.
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
	if sub { // staged first, so a failed restore can be repeated with x
		if _, err := p.exec(ctx, in.superDir, nil, "update-index", "--cacheinfo", "160000,"+b.SHA+","+in.relPath); err != nil {
			return "", err
		}
	}
	if err := p.restoreRepo(ctx, dir, b.SHA); err != nil {
		return "", err
	}

	g := func(args ...string) (string, error) {
		return p.exec(ctx, dir, nil, append(append([]string{}, restoreArgs...), args...)...)
	}
	if !b.Remote {
		_, err := g("switch", "-q", b.Local)
		return b.Local, err
	}
	switch tip, ok, err := try(ctx, dir, "rev-parse", "--verify", "-q", "refs/heads/"+b.Local); {
	case err != nil:
		return "", err
	case !ok:
		_, err := g("switch", "-q", "-c", b.Local, "--track", b.Name)
		return b.Local, err
	case tip == b.SHA:
		_, err := g("switch", "-q", b.Local)
		return b.Local, err
	}
	return "", nil
}
