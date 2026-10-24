package theme

import "submarine/internal/submodule"

// LegendRow is one word or symbol of the UI, rendered as it appears, and
// what it means.
type LegendRow struct{ Word, Meaning string }

// LegendGroup is a titled column of the legend.
type LegendGroup struct {
	Title string
	Rows  []LegendRow
}

// issueMeaning explains an issue word in a few plain words.
func issueMeaning(k submodule.IssueKind) string {
	switch k {
	case submodule.IssueUnpushed:
		return "pin is on no remote"
	case submodule.IssueConflict:
		return "merge conflict"
	case submodule.IssueMissing:
		return "not checked out"
	case submodule.IssueOffPin:
		return "checkout is not the pin"
	case submodule.IssueChanged:
		return "uncommitted files"
	default:
		return "pin not on target branch"
	}
}

// Legend explains the words and symbols of the tree and the detail pane, in
// their colors: what the full help's second page shows.
func (t Theme) Legend() []LegendGroup {
	g := t.Glyphs
	muted := Fg(t.Muted)
	status := LegendGroup{Title: "Status"}
	for _, k := range submodule.Kinds {
		status.Rows = append(status.Rows, LegendRow{t.LevelStyle(k.Level(), Fg(t.Text)).Render(IssueWord(k)), issueMeaning(k)})
	}
	status.Rows = append(status.Rows,
		LegendRow{Fg(t.Info).Render(Staged), "new pin, not committed"},
		LegendRow{muted.Render(InSync), "nothing to do"},
	)
	symbols := LegendGroup{Title: "Symbols", Rows: []LegendRow{
		{Fg(t.Info).Render(g.Staged), "pin staged in parent"},
		{muted.Render(g.Ahead + "n " + g.Behind + "n"), "ahead / behind the ref"},
		{muted.Render("+n"), "n more issues"},
		{Fg(t.Warn).Render(g.Problems + "k"), "issues below, collapsed"},
		{muted.Render("[n]"), "children, collapsed"},
	}}
	commits := LegendGroup{Title: "Commits", Rows: []LegendRow{
		{muted.Render(RoleWord(submodule.RolePin, "")), "what the parent records"},
		{Fg(t.Text).Bold(true).Render(RoleWord(submodule.RoleYou, "")), "the checkout"},
		{muted.Render(RoleWord(submodule.RoleTip, "")), "newest on the ref"},
		{muted.Render("main"), "tip of target branch"},
		{muted.Render(RoleWord(submodule.RoleBase, "")), "where lines split"},
		{muted.Render(RoleWord(submodule.RoleHead, "")), "superproject checkout"},
	}}
	return []LegendGroup{status, symbols, commits}
}
