package submodule

import "context"

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
	RestoreLoss(ctx context.Context, path string) (Loss, error)
	Restore(ctx context.Context, path string) error
}

// Loss is what a restore discards. The zero Loss means nothing is lost.
type Loss struct {
	Files        int // changed, conflicted and untracked files and directories
	Modules      int // repositories with a loss, the repository itself included
	Operations   int // merges, rebases, cherry-picks, reverts in progress
	Commits      int // commits that no branch, tag or remote ref reaches
	Repositories int // untracked repositories with their own .git directory
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
		Unknown:      l.Unknown + o.Unknown,
	}
}

// Change reports that the git metadata of a repository changed.
type Change struct {
	Path      string // "" is the superproject
	Local     bool   // HEAD, index or local branches changed, not only remote refs
	Structure bool   // .gitmodules changed: submodules may have been added or removed
}
