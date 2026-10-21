// Package submodule holds the domain model for a superproject and its
// (recursively nested) git submodules. It has no UI dependencies.
package submodule

import "time"

// Repo is the superproject.
type Repo struct {
	Path       string
	Branch     string
	Ahead      int
	Behind     int
	FetchedAt  time.Time
	Submodules []*Submodule
}

// Submodule is one node in the submodule tree.
type Submodule struct {
	Name     string
	Path     string // relative to the superproject root; unique ID
	RelPath  string // relative to the parent repository, e.g. "modules/foo"
	URL      string
	Parent   *Submodule
	Children []*Submodule
	Status   *Status // nil while loading
	Err      error   // set if loading the status failed
}

// Presence describes whether the submodule exists on disk.
type Presence int

const (
	PresenceOK Presence = iota
	NotInitialized
	DirMissing
)

// Checkout compares the submodule's HEAD with the pinned commit.
type Checkout int

const (
	CheckoutMatch Checkout = iota
	CheckoutDiffers
	CheckoutDetached // on the pinned commit, but detached (informational)
)

// Worktree describes uncommitted state inside the submodule.
type Worktree int

const (
	WorktreeClean Worktree = iota
	WorktreeModified
	WorktreeUntracked
	WorktreeConflict
)

// RefSource tells which rule resolved the pin to a ref.
type RefSource int

const (
	RefFromGitmodules RefSource = iota
	RefFromCurrentBranch
	RefFromDefaultBranch
	RefFromContains
	RefFromTag
	RefFromSHA
)

func (r RefSource) String() string {
	switch r {
	case RefFromGitmodules:
		return ".gitmodules branch"
	case RefFromCurrentBranch:
		return "checked out branch"
	case RefFromDefaultBranch:
		return "remote default branch"
	case RefFromContains:
		return "branch containing pin"
	case RefFromTag:
		return "tag"
	default:
		return "commit sha"
	}
}

// PinInfo relates the pinned commit to a ref.
type PinInfo struct {
	Ref    string // e.g. "origin/main"; empty if unresolved
	Source RefSource
	Ahead  int  // commits in pin not in ref
	Behind int  // commits in ref not in pin
	Pushed bool // pin is reachable from some remote ref
}

// Commit is a short commit description.
type Commit struct {
	SHA     string
	Subject string
	Author  string
	When    time.Time
}

// Status is the full state of one submodule.
type Status struct {
	Presence   Presence
	Checkout   Checkout
	Worktree   Worktree
	PinStaged  bool // pin changed in the superproject index, not committed
	Pin        PinInfo
	Pinned     *Commit
	CheckedOut *Commit
	Tip        *Commit
	Between    []Commit // commits that make up the pin <-> tip difference
}
