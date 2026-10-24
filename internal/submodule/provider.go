package submodule

import (
	"context"
	"time"
)

// Provider supplies repository data. Tree is expected to be fast (structure
// only); Status may be slow and is called once per submodule, concurrently.
type Provider interface {
	Tree(ctx context.Context) (*Repo, error)
	Status(ctx context.Context, path string) (*Status, error)
}

// Fetcher is implemented by providers that can download remote refs. Path
// "" is the superproject. Fetch must be safe to call concurrently and must
// never prompt the user.
type Fetcher interface {
	// Fetchable reports whether path is a repository that can be fetched
	// (e.g. not an uninitialized submodule).
	Fetchable(path string) bool
	Fetch(ctx context.Context, path string) error
}

// Watcher is implemented by providers that notice changes made outside the
// application, e.g. a fetch or commit in another shell. The channel is
// closed when ctx is done.
type Watcher interface {
	Watch(ctx context.Context) <-chan Change
}

// Locator is implemented by providers whose submodules live on the local
// disk. Dir returns the absolute directory of path ("" is the superproject),
// or "" if it is unknown.
type Locator interface {
	Dir(path string) string
}

// Restorer is implemented by providers that can make a repository (and all
// submodules below it) exactly what git records. Path "" is the
// superproject. Restore may clone or fetch objects that are missing, but must
// never prompt the user.
type Restorer interface {
	// RestoreLoss reports what Restore would discard, read from the disk at
	// this moment.
	RestoreLoss(ctx context.Context, path string, to RestoreTarget) (Loss, error)
	Restore(ctx context.Context, path string, to RestoreTarget) error
}

// Branch is a local or remote-tracking branch of a repository.
type Branch struct {
	Ref     string    // full ref, e.g. "refs/remotes/origin/feat"
	Name    string    // short name, e.g. "origin/feat"
	Local   string    // name of the local branch it stands for, e.g. "feat"
	Remote  bool      // a remote-tracking branch
	Current bool      // HEAD is on this branch
	SHA     string    // the tip when the list was read
	When    time.Time // committer date of the tip
}

// Switcher is implemented by providers that can switch a repository to a
// branch: the tip becomes the staged pin of a submodule, and the repository
// (with everything below it) is restored to it. Path "" is the superproject.
// Like Restore, Switch may clone or fetch what is missing below, but must
// never prompt the user. Branches uses no network.
type Switcher interface {
	Branches(ctx context.Context, path string) ([]Branch, error)
	// SwitchLoss reports what Switch would discard, read from the disk now.
	SwitchLoss(ctx context.Context, path string, b Branch) (Loss, error)
	// Switch returns the branch HEAD ends up on, "" if it is detached. It
	// fails if b.Ref no longer points at b.SHA.
	Switch(ctx context.Context, path string, b Branch) (string, error)
}

// RestoreTarget is the pin a submodule is restored to, and whether staged
// pins below it are kept. The superproject's target is always HEAD; only
// RestoreHard matters for it.
type RestoreTarget int

const (
	RestoreStaged    RestoreTarget = iota // the pin in the parent's index: a staged pin is kept and used
	RestoreCommitted                      // the pin in the parent's HEAD: a staged pin is unstaged
	RestoreHard                           // the pin in the parent's HEAD, and staged pins below are dropped too
)

// Committed reports whether a submodule goes to the pin in its parent's HEAD.
func (t RestoreTarget) Committed() bool { return t != RestoreStaged }

// KeepsStaged reports whether staged pins below the restored repository are
// kept.
func (t RestoreTarget) KeepsStaged() bool { return t != RestoreHard }

// Loss is what a restore discards. The zero Loss means nothing is lost.
type Loss struct {
	Files        int // changed, conflicted and untracked files and directories
	Modules      int // repositories with a loss, the repository itself included
	Operations   int // merges, rebases, cherry-picks, reverts in progress
	Commits      int // commits that no branch, tag or remote ref reaches
	Repositories int // untracked repositories with their own .git directory
	StagedPins   int // staged pins of repositories that move to another commit
	Unknown      int // repositories whose state could not be read
}

// Empty reports whether nothing would be lost.
func (l Loss) Empty() bool { return l == Loss{} }

// Add sums two losses.
func (l Loss) Add(o Loss) Loss {
	return Loss{
		Files:        l.Files + o.Files,
		Modules:      l.Modules + o.Modules,
		Operations:   l.Operations + o.Operations,
		Commits:      l.Commits + o.Commits,
		Repositories: l.Repositories + o.Repositories,
		StagedPins:   l.StagedPins + o.StagedPins,
		Unknown:      l.Unknown + o.Unknown,
	}
}

// Change reports that the git metadata of a repository changed.
type Change struct {
	Path      string // "" is the superproject
	Local     bool   // HEAD, index or local branches changed, not only remote refs
	Structure bool   // .gitmodules changed: submodules may have been added or removed
}
