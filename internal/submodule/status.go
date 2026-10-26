package submodule

import "fmt"

// Level is how much attention an issue deserves.
type Level int

const (
	LevelQuiet  Level = iota // nothing to do
	LevelInfo                // good to know
	LevelWarn                // look at it
	LevelDanger              // act now
)

// IssueKind is the summarized category of a problem.
type IssueKind int

const (
	IssueMissing     IssueKind = iota // not initialized or directory missing
	IssueUnpushed                     // pin not reachable from any remote ref
	IssueConflict                     // merge conflict in the worktree
	IssueOffPin                       // checkout differs from pin
	IssueChanged                      // uncommitted changes or untracked files
	IssueOffTarget                    // pin pushed, but not on the target branch
	IssuePinConflict                  // merge conflict on the pin in the parent
)

// Kinds lists every issue kind, most severe first.
var Kinds = []IssueKind{IssuePinConflict, IssueUnpushed, IssueConflict, IssueMissing, IssueOffPin, IssueChanged, IssueOffTarget}

// Level is how much attention an issue of kind k deserves.
func (k IssueKind) Level() Level {
	switch k {
	case IssuePinConflict, IssueUnpushed, IssueConflict:
		return LevelDanger
	case IssueOffTarget:
		return LevelInfo
	default:
		return LevelWarn
	}
}

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
	if s.PinConflict != nil {
		return s.conflictIssues()
	}
	switch s.Presence {
	case NotInitialized:
		return []Issue{{IssueMissing, IssueMissing.Level(), "not initialized"}}
	case DirMissing:
		return []Issue{{IssueMissing, IssueMissing.Level(), "directory missing"}}
	}

	var out []Issue
	if !s.Pin.Pushed {
		detail := "pin not pushed to any remote"
		if s.Root {
			detail = "HEAD not pushed to any remote"
		}
		out = append(out, Issue{IssueUnpushed, IssueUnpushed.Level(), detail})
	}
	if s.Worktree == WorktreeConflict {
		out = append(out, Issue{IssueConflict, IssueConflict.Level(), "merge conflict"})
	}
	if s.Checkout == CheckoutDiffers {
		out = append(out, Issue{IssueOffPin, IssueOffPin.Level(), "checkout differs from pin"})
	}
	switch s.Worktree {
	case WorktreeModified:
		out = append(out, Issue{IssueChanged, IssueChanged.Level(), "uncommitted changes"})
	case WorktreeUntracked:
		out = append(out, Issue{IssueChanged, IssueChanged.Level(), s.untrackedText()})
	}
	// a pin that is pushed, but not on the target (usually main); an
	// unpushed pin is not on it either, but has the worse issue
	if !s.Root && s.Pin.Pushed && s.Pin.Target != "" && s.Pin.Ref != s.Pin.Target {
		out = append(out, Issue{IssueOffTarget, IssueOffTarget.Level(), s.offTargetText()})
	}
	return out
}

// conflictIssues are the issues during a pin conflict: the conflict, then
// what concerns the checkout alone. Everything that compares with the pin
// waits until the pin is chosen.
func (s *Status) conflictIssues() []Issue {
	c := s.PinConflict
	detail := "both sides moved the pin"
	if c.Ours.Name != "" && c.Theirs.Name != "" {
		detail = c.Ours.Name + " and " + c.Theirs.Name + " both moved the pin"
	}
	switch {
	case c.Ours.Deleted():
		detail = c.Ours.Name + " removed the submodule, " + c.Theirs.Name + " moved its pin"
	case c.Theirs.Deleted():
		detail = c.Theirs.Name + " removed the submodule, " + c.Ours.Name + " moved its pin"
	}
	out := []Issue{{IssuePinConflict, IssuePinConflict.Level(), detail}}
	switch s.Presence {
	case NotInitialized:
		return append(out, Issue{IssueMissing, IssueMissing.Level(), "not initialized"})
	case DirMissing:
		return append(out, Issue{IssueMissing, IssueMissing.Level(), "directory missing"})
	}
	switch s.Worktree {
	case WorktreeConflict:
		out = append(out, Issue{IssueConflict, IssueConflict.Level(), "merge conflict"})
	case WorktreeModified:
		out = append(out, Issue{IssueChanged, IssueChanged.Level(), "uncommitted changes"})
	case WorktreeUntracked:
		out = append(out, Issue{IssueChanged, IssueChanged.Level(), s.untrackedText()})
	}
	return out
}

// offTargetText says which branch the pin is not on and where it is instead.
func (s *Status) offTargetText() string {
	text := fmt.Sprintf("pin not on %s, on %s", s.Pin.Target, s.Pin.Ref)
	if n := len(s.Pin.Branches); n > 0 {
		text += fmt.Sprintf(" +%d", n)
	}
	return text
}

// Hidden counts issues below a submodule per level, plus staged pins (which
// are not issues).
type Hidden struct{ Info, Warn, Danger, Staged int }

// Total is the number of issues of all levels; staged pins are not counted.
func (h Hidden) Total() int { return h.Info + h.Warn + h.Danger }

// HiddenIssues counts the issues and staged pins of all loaded descendants. A
// descendant whose load failed counts as one danger, since its state is
// unknown.
func (m *Submodule) HiddenIssues() Hidden { return HiddenIn(m.Children) }

// HiddenIn counts the issues and staged pins of mods and all their loaded
// descendants, like HiddenIssues.
func HiddenIn(mods []*Submodule) Hidden {
	var h Hidden
	Walk(mods, func(c *Submodule) {
		if c.Status == nil && c.Err != nil {
			h.Danger++
		}
		if c.Status != nil && c.Status.PinStaged {
			h.Staged++
		}
		for _, is := range c.Status.Issues() {
			switch is.Level {
			case LevelDanger:
				h.Danger++
			case LevelWarn:
				h.Warn++
			case LevelInfo:
				h.Info++
			}
		}
	})
	return h
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
