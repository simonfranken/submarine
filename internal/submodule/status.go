package submodule

// Level is how much attention an issue deserves.
type Level int

const (
	LevelQuiet  Level = iota // nothing to do
	LevelWarn                // look at it
	LevelDanger              // act now
)

// IssueKind is the summarized category of a problem.
type IssueKind int

const (
	IssueMissing  IssueKind = iota // not initialized or directory missing
	IssueUnpushed                  // pin not reachable from any remote ref
	IssueConflict                  // merge conflict in the worktree
	IssueOffPin                    // checkout differs from pin
	IssueChanged                   // uncommitted changes or untracked files
)

// Issue is one problem of a submodule. Detail distinguishes the states that
// are summarized into one kind.
type Issue struct {
	Kind   IssueKind
	Level  Level
	Detail string
}

// Issues summarizes the status, most severe first. Being behind or ahead of
// the remote, a staged pin and a detached HEAD are not issues.
func (s *Status) Issues() []Issue {
	if s == nil {
		return nil
	}
	switch s.Presence {
	case NotInitialized:
		return []Issue{{IssueMissing, LevelWarn, "not initialized"}}
	case DirMissing:
		return []Issue{{IssueMissing, LevelWarn, "directory missing"}}
	}

	var out []Issue
	if !s.Pin.Pushed {
		out = append(out, Issue{IssueUnpushed, LevelDanger, "pin not pushed to any remote"})
	}
	if s.Worktree == WorktreeConflict {
		out = append(out, Issue{IssueConflict, LevelDanger, "merge conflict"})
	}
	if s.Checkout == CheckoutDiffers {
		out = append(out, Issue{IssueOffPin, LevelWarn, "checkout differs from pin"})
	}
	switch s.Worktree {
	case WorktreeModified:
		out = append(out, Issue{IssueChanged, LevelWarn, "uncommitted changes"})
	case WorktreeUntracked:
		out = append(out, Issue{IssueChanged, LevelWarn, s.untrackedText()})
	}
	return out
}

// HiddenIssues counts the issues of all loaded descendants and returns the
// most severe level among them. A descendant whose load failed counts as one
// danger, since its state is unknown.
func (m *Submodule) HiddenIssues() (count int, worst Level) {
	Walk(m.Children, func(c *Submodule) {
		if c.Status == nil && c.Err != nil {
			count++
			worst = max(worst, LevelDanger)
		}
		for _, is := range c.Status.Issues() {
			count++
			worst = max(worst, is.Level)
		}
	})
	return count, worst
}

// Walk calls fn for every submodule in depth-first order.
func Walk(mods []*Submodule, fn func(*Submodule)) {
	for _, m := range mods {
		fn(m)
		Walk(m.Children, fn)
	}
}

// untrackedText words the untracked-files issue: "untracked repository" if
// everything untracked is a git repository of its own, e.g. a clone that was
// never added as a submodule.
func (s *Status) untrackedText() string {
	if s.Changes == nil || s.Changes.Total != len(s.Changes.Files) {
		return "untracked files"
	}
	for _, f := range s.Changes.Files {
		if f.Kind != ChangeUntracked || !f.Repo {
			return "untracked files"
		}
	}
	return "untracked repository"
}
