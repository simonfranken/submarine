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

// Change reports that the git metadata of a repository changed.
type Change struct {
	Path      string // "" is the superproject
	Local     bool   // HEAD, index or local branches changed, not only remote refs
	Structure bool   // .gitmodules changed: submodules may have been added or removed
}
