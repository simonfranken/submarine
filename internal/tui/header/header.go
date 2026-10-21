// Package header renders the superproject summary line.
package header

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

// View renders repo info, state counts and fetch age on one line.
func View(t theme.Theme, repo *submodule.Repo, width int) string {
	bar := lipgloss.NewStyle().Background(t.Bar)
	seg := func(c lipgloss.TerminalColor, s string) string { return bar.Foreground(c).Render(s) }

	if repo == nil {
		return bar.Width(width).Render(seg(t.Muted, " loading…"))
	}

	left := seg(t.Text, " "+repo.Path) + seg(t.Muted, "  ") + seg(t.Primary, repo.Branch)
	if d := t.Delta(submodule.PinInfo{Ahead: repo.Ahead, Behind: repo.Behind, Pushed: true}, bar); d != "" {
		left += seg(t.Text, " ") + d
	}

	// count submodules per issue kind (most severe first), plus staged pins
	var total, loading, staged int
	perKind := map[submodule.IssueKind]int{}
	level := map[submodule.IssueKind]submodule.Level{}
	submodule.Walk(repo.Submodules, func(s *submodule.Submodule) {
		total++
		if s.Status == nil {
			loading++
			return
		}
		if s.Status.PinStaged {
			staged++
		}
		for _, is := range s.Status.Issues() {
			perKind[is.Kind]++
			level[is.Kind] = is.Level
		}
	})

	sep := seg(t.Muted, " · ")
	mid := seg(t.Muted, fmt.Sprintf("%d submodules", total))
	for _, k := range []submodule.IssueKind{
		submodule.IssueUnpushed, submodule.IssueConflict, submodule.IssueMissing,
		submodule.IssueOffPin, submodule.IssueChanged,
	} {
		if n := perKind[k]; n > 0 {
			mid += sep + t.LevelStyle(level[k], bar).Render(fmt.Sprintf("%d %s", n, theme.IssueWord(k)))
		}
	}
	if staged > 0 {
		mid += sep + seg(t.Info, fmt.Sprintf("%d pin staged", staged))
	}
	if loading > 0 {
		mid += sep + seg(t.Muted, fmt.Sprintf("%d loading", loading))
	}

	right := seg(t.Muted, "fetched "+theme.Ago(repo.FetchedAt)+" ")

	gap := width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)
	if gap < 4 {
		return bar.Width(width).MaxWidth(width).Render(left + seg(t.Text, "   ") + mid)
	}
	sp := func(n int) string { return bar.Render(fmt.Sprintf("%*s", n, "")) }
	return left + sp(gap/2) + mid + sp(gap-gap/2) + right
}
