// Package submodule holds the domain model for a superproject and its
// (recursively nested) git submodules. It has no UI dependencies.
package submodule

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// Repo is the superproject.
type Repo struct {
	Path       string
	Branch     string
	Ahead      int
	Behind     int
	Root       *Submodule // the superproject as a selectable node; Path is "", no Children
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
	FetchErr error   // set if the last fetch failed; not an issue
}

// IsRoot reports whether s is the superproject itself.
func (s *Submodule) IsRoot() bool { return s != nil && s.Path == "" }

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
	RefFromGitmodules    RefSource = iota // the target, `branch =` in .gitmodules; the pin is on it
	RefFromDefaultBranch                  // the target, the remote's default branch; the pin is on it
	RefFromContains                       // a branch containing the pin, which is not on the target
	RefFromCurrentBranch                  // upstream of the checked-out branch, there being no better
	RefFromTag
	RefFromSHA
)

func (r RefSource) String() string {
	switch r {
	case RefFromGitmodules:
		return ".gitmodules branch"
	case RefFromDefaultBranch:
		return "remote default branch"
	case RefFromContains:
		return "branch containing pin"
	case RefFromCurrentBranch:
		return "checked out branch"
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
	// Target is the branch the pin is supposed to be on (`branch =` of
	// .gitmodules, else the remote's default branch); empty if there is none or
	// it does not exist locally. Ref is the target if the pin is on it, else
	// the branch the pin is on instead.
	Target string
	Ahead  int  // commits in pin not in ref
	Behind int  // commits in ref not in pin
	Pushed bool // pin is reachable from some remote ref
	// Branches are remote branches other than Ref that contain the pin, oldest
	// tip first and limited.
	Branches []string
}

// Commit is a short commit description.
type Commit struct {
	SHA     string
	Subject string
	Author  string
	When    time.Time
}

// ChangeKind classifies one changed file. The order is the display order.
type ChangeKind int

const (
	ChangeConflicted ChangeKind = iota
	ChangeModified
	ChangeAdded
	ChangeRemoved
	ChangeRenamed
	ChangeUntracked
	NumChangeKinds
)

// MaxChangeFiles limits Changes.Files; Changes.Total keeps the real count.
const MaxChangeFiles = 200

// FileChange is one changed file, relative to HEAD of the submodule
// (staged and unstaged changes are combined).
type FileChange struct {
	Path      string
	OrigPath  string // old path, only for renames
	Kind      ChangeKind
	Added     int       // added lines; 0 for untracked, binary and nested submodules
	Removed   int       // removed lines
	Binary    bool      // line counts are unknown
	Submodule bool      // a nested submodule pointer changed
	Repo      bool      // an untracked directory that is a git repository itself
	Modified  time.Time // modification time of the file; zero if it doesn't exist
}

// Changes describes what differs in the worktree of a submodule.
type Changes struct {
	Files   []FileChange // at most MaxChangeFiles, sorted by kind, then by changed lines
	Total   int          // all changed files; may exceed len(Files)
	Added   int          // added lines in all files
	Removed int          // removed lines in all files
}

// Counts returns the number of files per kind among files (all of
// c.Files if files is nil).
func (c *Changes) Counts(files []FileChange) [NumChangeKinds]int {
	if files == nil && c != nil {
		files = c.Files
	}
	var n [NumChangeKinds]int
	for _, f := range files {
		n[f.Kind]++
	}
	return n
}

// Sort orders Files by kind, then by changed lines (largest first), then by
// path.
func (c *Changes) Sort() {
	slices.SortStableFunc(c.Files, func(a, b FileChange) int {
		return cmp.Or(
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(b.Added+b.Removed, a.Added+a.Removed),
			strings.Compare(a.Path, b.Path),
		)
	})
}

// Distance counts the commits only one side of a comparison has, like
// `git rev-list --left-right --count a...b`.
type Distance struct{ Ahead, Behind int }

// Bases are the merge bases of the pin with the commits that have diverged
// from it, where the histories split. A field is nil if that commit has not
// diverged or the base is unknown.
type Bases struct{ Tip, Checkout, Committed, Target *Commit }

// Status is the full state of one submodule.
type Status struct {
	Root       bool // the superproject: the "pin" is its HEAD
	Presence   Presence
	Checkout   Checkout
	Worktree   Worktree
	PinStaged  bool // pin changed in the superproject index, not committed
	Pin        PinInfo
	Pinned     *Commit
	CheckedOut *Commit
	Tip        *Commit
	Drift      Distance // checkout compared with the pin: Ahead = commits only the checkout has
	// Committed is the pin in the superproject's HEAD while Pinned (the index) is
	// staged; nil if nothing is staged, the commit is not on disk, or the pin was
	// only added. CommittedDrift compares it with Pinned like Drift.
	Committed      *Commit
	CommittedDrift Distance
	// TargetTip is the tip of Pin.Target if the pin is not on it (the ref is
	// another branch), else nil. TargetDrift compares it with Pinned like
	// Drift: Ahead = commits only the target has.
	TargetTip   *Commit
	TargetDrift Distance
	Bases       Bases
	Changes     *Changes  // nil if the worktree is clean or the details are unknown
	FetchedAt   time.Time // last fetch of the submodule; zero if never
}
