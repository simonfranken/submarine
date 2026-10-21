package detail

import (
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

// previewBudget is the number of file lines shown in the change preview.
const previewBudget = 10

// pickPreview chooses which files of a sorted change list to show. Every
// kind that occurs gets at least one line; the remaining lines go to the
// files in sort order. Both results keep the sort order. A single hidden
// file would cost a "1 more" line, so it is shown instead.
func pickPreview(files []submodule.FileChange, budget int) (shown, hidden []submodule.FileChange) {
	if len(files) <= budget+1 {
		return files, nil
	}
	pick := make([]bool, len(files))
	var seen [submodule.NumChangeKinds]bool
	left := budget
	for i, f := range files {
		if !seen[f.Kind] && left > 0 {
			seen[f.Kind], pick[i] = true, true
			left--
		}
	}
	for i := range files {
		if left > 0 && !pick[i] {
			pick[i] = true
			left--
		}
	}
	for i, f := range files {
		if pick[i] {
			shown = append(shown, f)
		} else {
			hidden = append(hidden, f)
		}
	}
	return shown, hidden
}

// changeLines renders the change preview: a summary line and a flat list of
// files, a colored letter in front of each.
func (m Model) changeLines(c *submodule.Changes, label func(string) string) []string {
	t := m.theme
	muted, text := theme.Fg(t.Muted), theme.Fg(t.Text)

	plural := "files"
	if c.Total == 1 {
		plural = "file"
	}
	// summary over all files: count, kinds, lines
	parts := []string{text.Render(fmt.Sprintf("%d %s", c.Total, plural))}
	if k := m.kindCounts(c.Counts(nil)); k != "" {
		parts = append(parts, k)
	}
	sep := muted.Render(" · ")
	if c.Added+c.Removed > 0 {
		// line totals are the least important part: drop them if they don't fit
		if withLines := append(parts, m.lineCounts(c.Added, c.Removed)); labelWidth+lipgloss.Width(strings.Join(withLines, sep)) <= m.width {
			parts = withLines
		}
	}
	out := []string{label("Changes") + strings.Join(parts, sep)}

	shown, hidden := pickPreview(c.Files, previewBudget)
	files := m.fileLines(shown)

	// a rule under the summary, as wide as the summary or the file table
	ruleW := lipgloss.Width(out[0]) - labelWidth
	for _, l := range files {
		ruleW = max(ruleW, lipgloss.Width(l)-labelWidth)
	}
	ruleW = min(ruleW, m.width-labelWidth)
	out = append(out, label("")+muted.Render(strings.Repeat(m.theme.Glyphs.Rule, ruleW)))
	out = append(out, files...)
	if more := c.Total - len(shown); more > 0 {
		line := label("") + muted.Render(fmt.Sprintf("…  %d more", more))
		if k := m.kindCounts(c.Counts(hidden)); k != "" {
			line += muted.Render(" (") + k + muted.Render(")")
		}
		out = append(out, line)
	}
	return out
}

// kindCounts renders "2M 1A 1?", each in the color of its kind.
func (m Model) kindCounts(n [submodule.NumChangeKinds]int) string {
	var parts []string
	for k := submodule.ChangeKind(0); k < submodule.NumChangeKinds; k++ {
		if n[k] > 0 {
			parts = append(parts, m.theme.ChangeStyle(k, lipgloss.NewStyle()).Render(fmt.Sprintf("%d%s", n[k], theme.ChangeLetter(k))))
		}
	}
	return strings.Join(parts, " ")
}

// lineCounts renders "+20 −4"; zero parts are omitted.
func (m Model) lineCounts(added, removed int) string {
	t := m.theme
	var parts []string
	if added > 0 {
		parts = append(parts, theme.Fg(t.Added).Render(fmt.Sprintf("+%d", added)))
	}
	if removed > 0 {
		parts = append(parts, theme.Fg(t.Danger).Render(fmt.Sprintf("%s%d", t.Glyphs.Minus, removed)))
	}
	return strings.Join(parts, " ")
}

// maxPathWidth caps the path column, so the counts stay close to the names
// in wide panes; longer paths have their directory cut.
const maxPathWidth = 48

// minPathWidth is the path width below which the age column is dropped.
const minPathWidth = 16

// fileRow is one file of the preview, split into its columns.
type fileRow struct {
	kind              submodule.ChangeKind
	name, prefix, dir string // prefix ("← ") is kept when dir is cut
	counts            string // rendered
	age               string // e.g. "19m ago"; empty for removed files
}

func (r fileRow) pathWidth() int {
	w := lipgloss.Width(r.name)
	if r.dir != "" {
		w += 2 + lipgloss.Width(r.prefix) + lipgloss.Width(r.dir)
	}
	return w
}

// fileLines renders the files as a table in the value column:
//
//	M  name  dir/   +20 −4  19m ago
//
// The path column is as wide as the widest path (up to maxPathWidth and the
// pane width); counts are right-aligned next to it, followed by the age. If
// space is short the age column goes first, then the directory is cut from
// the left, the name last.
func (m Model) fileLines(files []submodule.FileChange) []string {
	t := m.theme
	muted, text := theme.Fg(t.Muted), theme.Fg(t.Text)

	rows := make([]fileRow, len(files))
	pathW, countsW, ageW := 0, 0, 0
	for i, f := range files {
		r := fileRow{kind: f.Kind}
		r.name, r.dir = splitPath(f.Path)
		if f.OrigPath != "" {
			r.prefix, r.dir = t.Glyphs.From+" ", f.OrigPath
		}
		switch {
		case f.Submodule:
			r.counts = muted.Render("submodule")
		case f.Repo:
			r.counts = muted.Render("repository")
		case f.Binary:
			r.counts = muted.Render("bin")
		default:
			r.counts = m.lineCounts(f.Added, f.Removed)
		}
		if !f.Modified.IsZero() {
			r.age = theme.Ago(f.Modified)
		}
		rows[i] = r
		pathW = max(pathW, r.pathWidth())
		countsW = max(countsW, lipgloss.Width(r.counts))
		ageW = max(ageW, lipgloss.Width(r.age))
	}

	avail := m.width - labelWidth - 3 // letter and its gap
	if countsW > 0 {
		avail -= countsW + 2
	}
	if ageW > 0 && avail-ageW-2 < min(pathW, minPathWidth) {
		ageW = 0 // too narrow: drop the age column
	}
	if ageW > 0 {
		avail -= ageW + 2
	}
	pathW = max(min(pathW, maxPathWidth, avail), 1)

	out := make([]string, len(rows))
	for i, r := range rows {
		name := ansi.Truncate(r.name, pathW, "…")
		dir := ""
		if room := pathW - lipgloss.Width(name) - 2 - lipgloss.Width(r.prefix); room >= 2 && r.dir != "" {
			dir = r.dir
			if w := lipgloss.Width(dir); w > room {
				dir = ansi.TruncateLeft(dir, w-(room-1), "…")
			}
			dir = r.prefix + dir
		}

		path := text.Bold(true).Render(name)
		if dir != "" {
			path += "  " + muted.Render(dir)
		}
		line := strings.Repeat(" ", labelWidth) +
			t.ChangeStyle(r.kind, lipgloss.NewStyle()).Render(theme.ChangeLetter(r.kind)) + "  " + path
		line += strings.Repeat(" ", pathW-lipgloss.Width(path))
		if countsW > 0 {
			line += strings.Repeat(" ", 2+countsW-lipgloss.Width(r.counts)) + r.counts
		}
		if ageW > 0 && r.age != "" {
			line += "  " + muted.Render(r.age)
		}
		out[i] = strings.TrimRight(line, " ")
	}
	return out
}

// splitPath returns the base name and the directory with a trailing slash.
// A path ending in a slash (an untracked directory) keeps it on the name.
func splitPath(p string) (name, dir string) {
	suffix := ""
	if strings.HasSuffix(p, "/") {
		p, suffix = strings.TrimSuffix(p, "/"), "/"
	}
	name = path.Base(p) + suffix
	if d := path.Dir(p); d != "." {
		dir = d + "/"
	}
	return name, dir
}
