package submodule

import "context"

// Provider supplies repository data. Tree is expected to be fast (structure
// only); Status may be slow and is called once per submodule, concurrently.
type Provider interface {
	Tree(ctx context.Context) (*Repo, error)
	Status(ctx context.Context, path string) (*Status, error)
}
