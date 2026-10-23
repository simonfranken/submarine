// Package header renders the superproject summary line.
package header

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

// Fetch is the progress of the application's own background fetch.
type Fetch struct {
	Running bool
	Done    int // repositories fetched (or failed)
	Total   int // repositories to fetch
	Failed  int // failed fetches of the last run
}

// View renders repo info, state counts and fetch state on one line. spinner
// is the current spinner frame. The header stands for the superproject: with
// selected it is drawn like a selected tree row.
func View(t theme.Theme, repo *submodule.Repo, fetch Fetch, spinner string, width int, selected bool) string {
	bar := lipgloss.NewStyle().Background(t.Bar)
	if selected {
		bar = lipgloss.NewStyle().Background(t.Selected).Bold(true)
	}
	seg := func(c lipgloss.TerminalColor, s string) string { return bar.Foreground(c).Render(s) }

	if repo == nil {
		return bar.Width(width).Render(seg(t.Muted, " loading…"))
	}

	left := seg(t.Text, " "+repo.Path) + seg(t.Muted, "  ") + seg(t.Primary, repo.Branch)
	if d := t.Delta(submodule.PinInfo{Ahead: repo.Ahead, Behind: repo.Behind, Pushed: true}, bar); d != "" {
		left += seg(t.Text, " ") + d
	}
	if repo.Root != nil {
		left += seg(t.Text, "  ") + t.StatusWord(repo.Root, bar, spinner)
	}

	// count submodules per issue kind (most severe first), plus staged pins
	var total, loading, errored, staged int
	perKind := map[submodule.IssueKind]int{}
	level := map[submodule.IssueKind]submodule.Level{}
	submodule.Walk(repo.Submodules, func(s *submodule.Submodule) {
		total++
		if s.Status == nil {
			if s.Err == nil {
				loading++
			} else {
				errored++
			}
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

	// most important first: errors, issues by severity, staged pins, then the
	// total
	sep := seg(t.Muted, " · ")
	var parts []string
	if errored > 0 {
		parts = append(parts, t.LevelStyle(submodule.LevelDanger, bar).Render(fmt.Sprintf("%d error", errored)))
	}
	for _, k := range []submodule.IssueKind{
		submodule.IssueUnpushed, submodule.IssueConflict, submodule.IssueMissing,
		submodule.IssueOffPin, submodule.IssueChanged, submodule.IssueOffTarget,
	} {
		if n := perKind[k]; n > 0 {
			parts = append(parts, t.LevelStyle(level[k], bar).Render(fmt.Sprintf("%d %s", n, theme.IssueWord(k))))
		}
	}
	if staged > 0 {
		parts = append(parts, seg(t.Info, fmt.Sprintf("%d pin staged", staged)))
	}
	if loading > 0 {
		parts = append(parts, seg(t.Muted, fmt.Sprintf("%d loading", loading)))
	}
	parts = append(parts, seg(t.Muted, fmt.Sprintf("%d submodules", total)))
	mid := strings.Join(parts, sep)

	// the summary and the fetch state form one group at the right edge
	right := mid
	switch {
	case fetch.Running:
		right += sep + seg(t.Primary, spinner) + seg(t.Muted, fmt.Sprintf(" fetching %d/%d", fetch.Done, fetch.Total))
	case fetch.Failed > 0:
		right += sep + seg(t.Warn, fmt.Sprintf("%d failed", fetch.Failed))
	}
	right += seg(t.Muted, " ")

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 4 {
		// too narrow: the least important part, on the right, is cut
		return bar.Width(width).Render(ansi.Truncate(left+seg(t.Text, "   ")+mid, width, "…"))
	}
	return left + bar.Render(fmt.Sprintf("%*s", gap, "")) + right
}
