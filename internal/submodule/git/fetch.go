package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// fetchTimeout bounds a single fetch, e.g. against an unreachable host.
const fetchTimeout = 2 * time.Minute

// maxFetches bounds concurrent fetches; they wait on the network, not the
// CPU.
const maxFetches = 8

// sshBatchEnv makes ssh fail instead of asking for a password or host key,
// which would corrupt the terminal UI. It is nil if the user configured
// their own ssh command.
func sshBatchEnv(root string) []string {
	if os.Getenv("GIT_SSH_COMMAND") != "" {
		return nil
	}
	if out, ok, _ := try(context.Background(), root, "config", "core.sshCommand"); ok && out != "" {
		return nil
	}
	return []string{"GIT_SSH_COMMAND=ssh -o BatchMode=yes"}
}

// Fetchable reports whether path ("" is the superproject) is a checked-out
// repository.
func (p *Provider) Fetchable(path string) bool {
	_, ok := p.repoDir(path)
	return ok
}

// Dir returns the absolute directory of path ("" is the superproject),
// whether or not it is initialized.
func (p *Provider) Dir(path string) string {
	if path == "" {
		return p.root
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.mods[path].dir
}

// repoDir returns the worktree of path if it is initialized.
func (p *Provider) repoDir(path string) (string, bool) {
	if path == "" {
		return p.root, true
	}
	p.mu.RLock()
	in, ok := p.mods[path]
	p.mu.RUnlock()
	if !ok || !initialized(in.dir) {
		return "", false
	}
	return in.dir, true
}

// Fetch fetches all remotes of one repository. Submodules are fetched
// separately, not recursively. The error is git's first line of output.
func (p *Provider) Fetch(ctx context.Context, path string) error {
	dir, ok := p.repoDir(path)
	if !ok {
		return fmt.Errorf("%s is not initialized", path)
	}
	select {
	case p.fetchSem <- struct{}{}:
		defer func() { <-p.fetchSem }()
	case <-ctx.Done():
		return ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	_, err := runEnv(ctx, dir, p.sshEnv, "fetch", "--all", "--prune", "--no-recurse-submodules", "--quiet")
	var ge *gitError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("timed out after %s", fetchTimeout)
	case errors.As(err, &ge) && ge.Stderr != "":
		line, _, _ := strings.Cut(ge.Stderr, "\n")
		return errors.New(strings.TrimPrefix(line, "fatal: "))
	}
	return err
}
