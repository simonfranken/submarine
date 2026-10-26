package git

import (
	"context"
	"errors"
	"fmt"

	"submarine/internal/submodule"
)

// conflicted returns the info of path, if its pin has a merge conflict.
func (p *Provider) conflicted(ctx context.Context, path string) (info, error) {
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok {
		return info{}, fmt.Errorf("unknown submodule %q", path)
	}
	st, err := indexStages(ctx, in)
	if err != nil {
		return info{}, err
	}
	if !st.conflicted() {
		return info{}, errors.New("has no pin conflict (resolved meanwhile?)")
	}
	return in, nil
}

// ResolveLoss reports what Resolve(path, sha) would discard: the same as a
// restore of the submodule to sha. Removing it discards nothing on disk.
func (p *Provider) ResolveLoss(ctx context.Context, path, sha string) (submodule.Loss, error) {
	in, err := p.conflicted(ctx, path)
	if err != nil || sha == "" || !initialized(in.dir) {
		return submodule.Loss{}, err
	}
	return p.lossOf(ctx, in.dir, sha, true), nil
}

// Resolve ends the pin conflict of path: sha replaces the conflicting
// stages in the parent's index, and the submodule is restored to it (see
// Restore; a missing commit is fetched, HEAD goes on a branch at it). With
// sha "" the gitlink is removed from the index; the directory and
// .gitmodules are left to the user.
func (p *Provider) Resolve(ctx context.Context, path, sha string) error {
	in, err := p.conflicted(ctx, path)
	if err != nil {
		return err
	}
	if sha == "" {
		_, err := p.exec(ctx, in.superDir, nil, "rm", "--cached", "-q", "--", in.relPath)
		return err
	}
	// staged first, like a switch: a failed restore can be repeated with x
	if _, err := p.exec(ctx, in.superDir, nil, "update-index", "--cacheinfo", "160000,"+sha+","+in.relPath); err != nil {
		return err
	}
	if !initialized(in.dir) {
		if err := p.initSubmodules(ctx, in.superDir, []string{in.relPath}); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return p.restoreRepo(ctx, in.dir, sha, &in, true)
}
