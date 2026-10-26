// Package detail renders the information pane for the selected submodule.
package detail

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

const labelWidth = 11

// staleFetch is the age of the last fetch from which the pane suggests f.
const staleFetch = 24 * time.Hour

type Model struct {
	theme   theme.Theme
	width   int
	height  int
	spinner string
	help    help.Model
	actions []key.Binding
}

// paletteMinHeight is the pane height below which the palette is left out:
// the information is worth more than the hints.
const paletteMinHeight = 12

func New(t theme.Theme, actions []key.Binding) Model {
	return Model{theme: t, help: theme.NewHelp(t), actions: actions}
}

// PaletteVisible reports whether the palette of submodule actions fits.
func (m Model) PaletteVisible() bool { return m.height >= paletteMinHeight }

// SetActions replaces the bindings of the palette.
func (m *Model) SetActions(actions []key.Binding) { m.actions = actions }

func (m *Model) SetSize(w, h int)        { m.width, m.height = w, h }
func (m *Model) SetSpinner(frame string) { m.spinner = frame }

// View renders s; repo is the loaded superproject (nil while loading), which
// the superproject's own view needs for its path and submodule summary.
func (m Model) View(s *submodule.Submodule, repo *submodule.Repo) string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	var lines []string
	if s != nil {
		lines = m.lines(s, repo)
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	height := m.height
	var palette []string
	if m.PaletteVisible() {
		palette = m.palette()
		height -= len(palette) // the palette is at the bottom
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	body := lipgloss.NewStyle().Width(m.width).Height(height).Render(strings.Join(lines, "\n"))
	if len(palette) == 0 {
		return body
	}
	return body + "\n" + strings.Join(palette, "\n")
}

// palette lays the actions out on as many lines as the width needs, so no
// key is cut off.
func (m Model) palette() []string {
	m.help.Width = 0
	var out []string
	var cur []key.Binding
	for _, b := range m.actions {
		if len(cur) > 0 && lipgloss.Width(m.help.ShortHelpView(append(cur, b))) > m.width {
			out = append(out, m.help.ShortHelpView(cur))
			cur = nil
		}
		cur = append(cur, b)
	}
	if len(cur) > 0 {
		out = append(out, m.help.ShortHelpView(cur))
	}
	for i, l := range out {
		out[i] = ansi.Truncate(l, m.width, "…")
	}
	return out
}

func (m Model) lines(s *submodule.Submodule, repo *submodule.Repo) []string {
	t := m.theme
	muted, text := theme.Fg(t.Muted), theme.Fg(t.Text)
	label := func(l string) string { return muted.Width(labelWidth).Render(l) }

	root := s.IsRoot() && repo != nil
	full := s.Path
	if root {
		full = repo.Path
	}
	dir := path.Dir(full)
	if dir == "." {
		dir = ""
	} else {
		dir += "/"
	}
	out := []string{
		muted.Render(dir),
		text.Bold(true).Render(s.Name),
		"",
	}

	switch {
	case s.Err != nil:
		return append(out, t.LevelStyle(submodule.LevelDanger, lipgloss.NewStyle()).Render("error")+" "+muted.Render(s.Err.Error()))
	case s.Status == nil:
		return append(out, theme.Fg(t.Primary).Render(m.spinner)+muted.Render(" loading…"))
	}
	st := s.Status

	// the age of the last fetch is right-aligned in the name's line
	// with a hint to fetch if the last fetch is old, and room for it
	if st.Presence == submodule.PresenceOK {
		fetched := muted.Render("fetched " + theme.Ago(st.FetchedAt))
		if time.Since(st.FetchedAt) > staleFetch {
			hinted := fetched + muted.Render(" · ") + text.Bold(true).Render("f") + muted.Render(" fetch")
			if m.width-ansi.StringWidth(s.Name)-lipgloss.Width(hinted) >= 2 {
				fetched = hinted
			}
		}
		if gap := m.width - ansi.StringWidth(s.Name) - lipgloss.Width(fetched); gap >= 2 {
			out[1] += strings.Repeat(" ", gap) + fetched
		}
	}

	issues := st.Issues()
	for _, is := range issues {
		// a word longer than the label column pushes the text, not wraps
		word := theme.IssueWord(is.Kind)
		pad := strings.Repeat(" ", max(labelWidth-ansi.StringWidth(word), 1))
		out = append(out, t.LevelStyle(is.Level, lipgloss.NewStyle()).Render(word)+pad+text.Render(is.Detail))
	}
	if len(issues) == 0 && !st.PinStaged {
		out = append(out, label(theme.InSync)+muted.Render("nothing to do"))
	}
	if st.PinStaged {
		out = append(out, theme.Fg(t.Info).Width(labelWidth).Render(theme.Staged)+text.Render("pin change staged, not committed"))
	}
	if st.Checkout == submodule.CheckoutDetached && st.PinConflict == nil {
		text := "detached HEAD on pin"
		if st.Root {
			text = "detached HEAD"
		}
		out = append(out, label("")+muted.Render(text))
	}
	if st.Changes != nil {
		out = append(out, "")
		out = append(out, m.changeLines(st.Changes, label)...)
	}
	out = append(out, "")

	// ref and spine; during a pin conflict the sides instead of the ref,
	// and below the spine the commits that have both
	names := theme.RoleNames{Target: st.Pin.Target}
	switch c := st.PinConflict; {
	case c != nil:
		names.Ours, names.Theirs = c.Ours.Name, c.Theirs.Name
		out = append(out, m.sideLine(c.Ours, "ours", label), m.sideLine(c.Theirs, "theirs", label), "")
	case st.Pin.Ref != "":
		out = append(out, label("Ref")+text.Render(st.Pin.Ref)+muted.Render(" · via "+st.Pin.Source.String()), "")
	}
	out = append(out, m.spineLines(st.Spine(), st.Checkout == submodule.CheckoutDiffers && st.PinConflict == nil, names)...)
	if c := st.PinConflict; c != nil && c.Ours.Commit != nil && c.Theirs.Commit != nil {
		out = append(out, "")
		out = append(out, m.candidateLines(c.Candidates, label)...)
	}
	if s.FetchErr != nil {
		out = append(out, theme.Fg(t.Warn).Width(labelWidth).Render("fetch")+text.Render("failed: "+s.FetchErr.Error()))
	}

	// children & meta
	out = append(out, "")
	if root {
		n := 0
		submodule.Walk(repo.Submodules, func(*submodule.Submodule) { n++ })
		summary := fmt.Sprintf("%d", n)
		if parts := m.hiddenParts(submodule.HiddenIn(repo.Submodules)); parts != "" {
			summary += muted.Render(" · ") + parts
		}
		out = append(out, label("Modules")+summary)
	} else if n := len(s.Children); n > 0 {
		children := fmt.Sprintf("%d direct", n)
		if parts := m.hiddenParts(s.HiddenIssues()); parts != "" {
			children += muted.Render(" · ") + parts + muted.Render(" below")
		}
		out = append(out, label("Children")+children)
	}
	if s.URL != "" {
		out = append(out, label("URL")+muted.Render(s.URL))
	}
	return out
}

// sideLine describes one side of a pin conflict: its commit and the branch
// it is on, or why that is unknown.
func (m Model) sideLine(side submodule.ConflictSide, role string, label func(string) string) string {
	t := m.theme
	muted, text := theme.Fg(t.Muted), theme.Fg(t.Text)
	head := label(ansi.Truncate(side.Name, labelWidth-1, "…"))
	switch {
	case side.Deleted():
		return head + muted.Render(role+" · ") + text.Render("removed the submodule")
	case side.Commit == nil:
		return head + muted.Render(role+" · ") + theme.Fg(t.Warn).Render("not fetched") +
			muted.Render(" · ") + text.Bold(true).Render("f") + muted.Render(" fetch")
	}
	line := head + theme.Fg(t.Primary).Render(side.Commit.SHA) + muted.Render(" "+role)
	switch {
	case !side.Pin.Pushed:
		line += muted.Render(" · ") + t.LevelStyle(submodule.LevelDanger, lipgloss.NewStyle()).Render("unpushed")
	case side.Pin.Ref != "":
		line += muted.Render(" · ") + text.Render(side.Pin.Ref)
		if d := t.Delta(side.Pin, lipgloss.NewStyle()); d != "" {
			line += " " + d
		}
	}
	return line
}

// candidateLines list the commits that contain both sides of a pin
// conflict, or say that there is none yet.
func (m Model) candidateLines(cands []submodule.Candidate, label func(string) string) []string {
	t := m.theme
	muted, text := theme.Fg(t.Muted), theme.Fg(t.Text)
	if len(cands) == 0 {
		return []string{label("Has both") + muted.Render("no commit yet · merge in the submodule, or pick a side")}
	}
	var out []string
	for i, c := range cands {
		head := label("")
		if i == 0 {
			head = label("Has both")
		}
		var where string
		switch {
		case c.Checkout:
			where = muted.Render("your checkout")
		case c.Side != "":
			where = muted.Render(c.Side + " has both")
		default:
			where = text.Render(c.Ref)
			if d := t.Delta(submodule.PinInfo{Behind: c.Behind, Pushed: true}, lipgloss.NewStyle()); d != "" {
				where += " " + d
			}
		}
		out = append(out, head+theme.Fg(t.Primary).Render(c.Commit.SHA)+" "+where+muted.Render(" · ")+text.Render(c.Commit.Subject))
	}
	return out
}

// hiddenParts words issue counts like "1 error(s), 2 warning(s)"; empty if
// there are none.
func (m Model) hiddenParts(h submodule.Hidden) string {
	t := m.theme
	var parts []string
	if h.Danger > 0 {
		parts = append(parts, t.LevelStyle(submodule.LevelDanger, lipgloss.NewStyle()).Render(fmt.Sprintf("%d error(s)", h.Danger)))
	}
	if h.Warn > 0 {
		parts = append(parts, t.LevelStyle(submodule.LevelWarn, lipgloss.NewStyle()).Render(fmt.Sprintf("%d warning(s)", h.Warn)))
	}
	if h.Info > 0 {
		parts = append(parts, t.LevelStyle(submodule.LevelInfo, lipgloss.NewStyle()).Render(fmt.Sprintf("%d note(s)", h.Info)))
	}
	if h.Staged > 0 {
		parts = append(parts, theme.Fg(t.Info).Render(fmt.Sprintf("%d %s", h.Staged, theme.Staged)))
	}
	return strings.Join(parts, theme.Fg(t.Muted).Render(", "))
}

// spineLines draws the pin, the tip and the checkout as one line of history,
// newest on top: a node per commit with its roles in the label column, and
// between two nodes how many commits separate them. offPin colors the
// checkout's own node.
func (m Model) spineLines(nodes []submodule.SpineNode, offPin bool, names theme.RoleNames) []string {
	t := m.theme
	muted, text := theme.Fg(t.Muted), theme.Fg(t.Text)

	// the roles of a node, each word in its own style: the checkout stands
	// out from the rest, and a staged pin is blue
	labels := make([]string, len(nodes))
	labelWidths := make([]int, len(nodes))
	width := labelWidth
	for i, n := range nodes {
		var plain, styled []string
		for _, r := range n.Roles {
			word := theme.RoleWord(r, names)
			style := muted
			switch {
			case r == submodule.RoleStaged:
				style = theme.Fg(t.Info)
			case r == submodule.RoleYou && offPin && len(n.Roles) == 1:
				style = theme.Fg(t.Warn)
			case r == submodule.RoleYou:
				style = text.Bold(true)
			}
			plain = append(plain, word)
			styled = append(styled, style.Render(word))
		}
		labels[i] = strings.Join(styled, muted.Render(" · "))
		labelWidths[i] = ansi.StringWidth(strings.Join(plain, " · "))
		if n.InSync() {
			labels[i], labelWidths[i] = muted.Render(theme.InSync), len(theme.InSync)
		}
		width = max(width, labelWidths[i]+1)
	}

	// a staged pin below the committed one moves the pin back
	hasRole := func(n submodule.SpineNode, r submodule.Role) bool { return slices.Contains(n.Roles, r) }
	pinAt := slices.IndexFunc(nodes, func(n submodule.SpineNode) bool { return hasRole(n, submodule.RolePin) })
	stagedAt := slices.IndexFunc(nodes, func(n submodule.SpineNode) bool { return hasRole(n, submodule.RoleStaged) })

	// a fork has its own lane to the right of the main one; it starts at the
	// first node and joins the main lane at the base
	fork, baseAt := len(nodes) > 0 && nodes[0].Fork, -1
	if fork {
		baseAt = slices.IndexFunc(nodes, func(n submodule.SpineNode) bool { return hasRole(n, submodule.RoleBase) })
	}
	line := muted.Render(t.Glyphs.Gap)
	lanes := func(main, side string) string {
		if !fork {
			return main
		}
		return main + " " + side
	}

	var out []string
	for i, n := range nodes {
		sha := theme.Fg(t.Primary)
		if offPin && len(n.Roles) == 1 && n.Roles[0] == submodule.RoleYou {
			sha = theme.Fg(t.Warn)
		}
		node := sha.Render(t.Glyphs.Node)
		var cells string
		switch {
		case n.Fork:
			cells = lanes(" ", node)
		case i == baseAt:
			cells = node + muted.Render(t.Glyphs.Rule+t.Glyphs.Join)
		case fork && i < baseAt:
			cells = lanes(node, line)
		default:
			cells = lanes(node, " ")
		}
		out = append(out, labels[i]+strings.Repeat(" ", width-labelWidths[i])+cells+" "+sha.Render(n.Commit.SHA)+" "+text.Render(n.Commit.Subject))
		if i < len(nodes)-1 {
			// the main lane starts below the fork's head, the fork's lane ends at the base
			main, side := " ", " "
			if !fork || i > 0 {
				main = line
			}
			if fork && i < baseAt {
				side = line
			}
			gap := lanes(main, side) + muted.Render(m.linkText(n.Below))
			if !fork && i+1 == stagedAt && pinAt >= 0 && pinAt < stagedAt && n.Below.Linear() {
				gap += theme.Fg(t.Info).Render(" (staged pin moves back)")
			}
			out = append(out, strings.Repeat(" ", width)+gap)
		}
	}
	return out
}

// linkText words the history between two nodes, with a leading space; empty
// if it is unknown.
func (m Model) linkText(l submodule.Link) string {
	g := m.theme.Glyphs
	switch {
	case l.Diverged():
		return fmt.Sprintf(" diverged %s%d %s%d", g.Ahead, l.Below, g.Behind, l.Above)
	case l.Above == 1:
		return " 1 commit"
	case l.Above > 1:
		return fmt.Sprintf(" %d commits", l.Above)
	}
	return ""
}
