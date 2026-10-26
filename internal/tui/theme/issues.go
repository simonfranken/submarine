package theme

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
)

// InSync is the word for a submodule without issues.
const InSync = "in sync"

// Staged is the word for a pin change staged in the superproject. It is not
// an issue: it replaces InSync, or follows an issue word as a marker.
const Staged = "staged"

// IssueWord is the short label for an issue kind shown in the tree.
func IssueWord(k submodule.IssueKind) string {
	switch k {
	case submodule.IssueMissing:
		return "missing"
	case submodule.IssueUnpushed:
		return "unpushed"
	case submodule.IssueConflict:
		return "conflict"
	case submodule.IssueOffPin:
		return "off pin"
	case submodule.IssueOffTarget:
		return "off target"
	case submodule.IssuePinConflict:
		return "pin conflict"
	default:
		return "changed"
	}
}

// maxTargetWidth limits the branch name that labels the target on the spine.
const maxTargetWidth = 16

// ShortRef is a remote ref without its remote, e.g. "marco-dev" for
// "origin/marco-dev", cut to maxTargetWidth with an ellipsis.
func ShortRef(ref string) string {
	if _, name, ok := strings.Cut(ref, "/"); ok {
		ref = name
	}
	return ansi.Truncate(ref, maxTargetWidth, "…")
}

// RoleNames are the branch names some roles are labelled with: the target
// branch, and the sides of a pin conflict.
type RoleNames struct{ Target, Ours, Theirs string }

// RoleWord is the label of a commit's role on the detail pane's spine; the
// target and the sides of a pin conflict are labelled with the names of
// their branches.
func RoleWord(r submodule.Role, names RoleNames) string {
	// side names are local names already: "feat/x" keeps its prefix
	named := func(name, fallback string) string {
		if name == "" {
			return fallback
		}
		return ansi.Truncate(name, maxTargetWidth, "…")
	}
	switch r {
	case submodule.RolePin:
		return "pin"
	case submodule.RoleHead:
		return "HEAD"
	case submodule.RoleStaged:
		return Staged
	case submodule.RoleBase:
		return "base"
	case submodule.RoleTarget:
		if names.Target == "" {
			return "target"
		}
		return ShortRef(names.Target)
	case submodule.RoleOurs:
		return named(names.Ours, "ours")
	case submodule.RoleTheirs:
		return named(names.Theirs, "theirs")
	case submodule.RoleTip:
		return "tip"
	default:
		return "you"
	}
}

// StatusWord renders the state of s as one word: a spinner while loading,
// `error`, the most severe issue with "+n" for further ones, `staged`, or a
// muted "in sync". base is applied to every segment, e.g. to keep a row
// background.
func (t Theme) StatusWord(s *submodule.Submodule, base lipgloss.Style, spinner string) string {
	seg := func(c lipgloss.TerminalColor, text string) string { return base.Foreground(c).Render(text) }
	switch {
	case s.Err != nil:
		return t.LevelStyle(submodule.LevelDanger, base).Render("error")
	case s.Status == nil:
		return seg(t.Primary, spinner)
	}
	if issues := s.Status.Issues(); len(issues) > 0 {
		out := t.LevelStyle(issues[0].Level, base).Render(IssueWord(issues[0].Kind))
		if len(issues) > 1 {
			out += seg(t.Muted, fmt.Sprintf(" +%d", len(issues)-1))
		}
		return out
	}
	if s.Status.PinStaged {
		return seg(t.Info, Staged)
	}
	return seg(t.Muted, InSync)
}

// ChangeLetter is the one-letter label of a changed file.
func ChangeLetter(k submodule.ChangeKind) string {
	switch k {
	case submodule.ChangeConflicted:
		return "!"
	case submodule.ChangeModified:
		return "M"
	case submodule.ChangeAdded:
		return "A"
	case submodule.ChangeRemoved:
		return "D"
	case submodule.ChangeRenamed:
		return "R"
	default:
		return "?"
	}
}

// ChangeStyle returns base with the color of a change kind.
func (t Theme) ChangeStyle(k submodule.ChangeKind, base lipgloss.Style) lipgloss.Style {
	switch k {
	case submodule.ChangeConflicted:
		return base.Foreground(t.Danger).Bold(true)
	case submodule.ChangeModified:
		return base.Foreground(t.Warn)
	case submodule.ChangeAdded:
		return base.Foreground(t.Added)
	case submodule.ChangeRemoved:
		return base.Foreground(t.Danger)
	case submodule.ChangeRenamed:
		return base.Foreground(t.Primary)
	default:
		return base.Foreground(t.Muted)
	}
}

// LevelStyle returns base with the text color (and weight) for a level.
func (t Theme) LevelStyle(l submodule.Level, base lipgloss.Style) lipgloss.Style {
	switch l {
	case submodule.LevelDanger:
		return base.Foreground(t.Danger).Bold(true)
	case submodule.LevelWarn:
		return base.Foreground(t.Warn)
	case submodule.LevelInfo:
		return base.Foreground(t.Info)
	default:
		return base.Foreground(t.Muted)
	}
}

// Delta renders ahead/behind counts like "↑1↓2", muted unless the pin is
// unpushed. Empty if both are zero. base is applied to every segment, e.g.
// to keep a row background.
func (t Theme) Delta(p submodule.PinInfo, base lipgloss.Style) string {
	var s string
	if p.Ahead > 0 {
		c := t.Muted
		if !p.Pushed {
			c = t.Danger
		}
		s += base.Foreground(c).Render(fmt.Sprintf("%s%d", t.Glyphs.Ahead, p.Ahead))
	}
	if p.Behind > 0 {
		s += base.Foreground(t.Muted).Render(fmt.Sprintf("%s%d", t.Glyphs.Behind, p.Behind))
	}
	return s
}

// Ago formats a time relative to now, e.g. "5m ago"; the zero time is "never".
func Ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
