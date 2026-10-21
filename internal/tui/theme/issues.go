package theme

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"submarine/internal/submodule"
)

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
	default:
		return "changed"
	}
}

// LevelStyle returns base with the text color (and weight) for a level.
func (t Theme) LevelStyle(l submodule.Level, base lipgloss.Style) lipgloss.Style {
	switch l {
	case submodule.LevelDanger:
		return base.Foreground(t.Danger).Bold(true)
	case submodule.LevelWarn:
		return base.Foreground(t.Warn)
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

// Ago formats a time relative to now, e.g. "5m ago".
func Ago(t time.Time) string {
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
