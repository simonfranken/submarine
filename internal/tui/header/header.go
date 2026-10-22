// Package header renders the superproject summary line.
package header

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

// Fetch is the progress of the application's own background fetch.
type Fetch struct {
	Running bool
	Done    int       // repositories fetched (or failed)
	Total   int       // repositories to fetch
	Failed  int       // failed fetches of the last run
	At      time.Time // when the last run finished
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

	sep := seg(t.Muted, " · ")
	mid := seg(t.Muted, fmt.Sprintf("%d submodules", total))
	if errored > 0 {
		mid += sep + t.LevelStyle(submodule.LevelDanger, bar).Render(fmt.Sprintf("%d error", errored))
	}
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

	var right string
	switch {
	case fetch.Running:
		right = seg(t.Primary, spinner) + seg(t.Muted, fmt.Sprintf(" fetching %d/%d ", fetch.Done, fetch.Total))
	default:
		// our own fetch may have finished before the watcher saw FETCH_HEAD
		at := repo.FetchedAt
		if fetch.At.After(at) {
			at = fetch.At
		}
		right = seg(t.Muted, "fetched "+theme.Ago(at))
		if fetch.Failed > 0 {
			right += seg(t.Muted, " · ") + seg(t.Warn, fmt.Sprintf("%d failed", fetch.Failed))
		}
		right += seg(t.Muted, " ")
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)
	if gap < 4 {
		return bar.Width(width).MaxWidth(width).Render(left + seg(t.Text, "   ") + mid)
	}
	sp := func(n int) string { return bar.Render(fmt.Sprintf("%*s", n, "")) }
	return left + sp(gap/2) + mid + sp(gap-gap/2) + right
}
