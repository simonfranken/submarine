// Package detail renders the information pane for the selected submodule.
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

const labelWidth = 10

type Model struct {
	theme   theme.Theme
	width   int
	height  int
	spinner string
}

func New(t theme.Theme) Model { return Model{theme: t} }

func (m *Model) SetSize(w, h int)        { m.width, m.height = w, h }
func (m *Model) SetSpinner(frame string) { m.spinner = frame }

func (m Model) View(s *submodule.Submodule) string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	var lines []string
	if s != nil {
		lines = m.lines(s)
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(strings.Join(lines, "\n"))
}

func (m Model) lines(s *submodule.Submodule) []string {
	t := m.theme
	muted, text := theme.Fg(t.Muted), theme.Fg(t.Text)
	label := func(l string) string { return muted.Width(labelWidth).Render(l) }

	dir := path.Dir(s.Path)
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

	issues := st.Issues()
	for _, is := range issues {
		out = append(out, t.LevelStyle(is.Level, lipgloss.NewStyle()).Width(labelWidth).Render(theme.IssueWord(is.Kind))+text.Render(is.Detail))
	}
	if len(issues) == 0 {
		out = append(out, label("in sync")+muted.Render("nothing to do"))
	}
	if st.PinStaged {
		out = append(out, theme.Fg(t.Info).Width(labelWidth).Render("staged")+text.Render("pin change staged, not committed"))
	}
	if st.Checkout == submodule.CheckoutDetached {
		out = append(out, label("")+muted.Render("detached HEAD on pin"))
	}
	if st.Changes != nil {
		out = append(out, "")
		out = append(out, m.changeLines(st.Changes, label)...)
	}
	out = append(out, "")

	// ref
	if st.Pin.Ref != "" {
		delta := t.Delta(st.Pin, lipgloss.NewStyle())
		if delta != "" {
			delta = " " + delta
		}
		out = append(out,
			label("Ref")+text.Render(st.Pin.Ref)+delta,
			label("")+muted.Render("via "+st.Pin.Source.String()),
		)
	}

	// commits
	commitLine := func(name string, c *submodule.Commit, highlight bool) string {
		if c == nil {
			return label(name) + muted.Render("—")
		}
		shaStyle := theme.Fg(t.Primary)
		if highlight {
			shaStyle = theme.Fg(t.Warn)
		}
		return label(name) + shaStyle.Render(c.SHA) + " " + text.Render(c.Subject)
	}
	out = append(out,
		commitLine("Pinned", st.Pinned, false),
		commitLine("Checkout", st.CheckedOut, st.Checkout == submodule.CheckoutDiffers),
		commitLine("Tip", st.Tip, false),
	)

	// difference between pin and tip
	if len(st.Between) > 0 {
		out = append(out, "", text.Render(m.betweenTitle(st.Pin)))
		for _, c := range st.Between {
			out = append(out,
				"  "+theme.Fg(t.Primary).Render(c.SHA)+" "+text.Render(c.Subject),
				"  "+muted.Render(fmt.Sprintf("%7s %s · %s", "", c.Author, theme.Ago(c.When))),
			)
		}
	}

	// children & meta
	out = append(out, "")
	if n := len(s.Children); n > 0 {
		children := fmt.Sprintf("%d direct", n)
		if k, worst := s.HiddenIssues(); k > 0 {
			children += t.LevelStyle(worst, lipgloss.NewStyle()).Render(fmt.Sprintf(" · %d issue(s) below", k))
		}
		out = append(out, label("Children")+children)
	}
	out = append(out, label("URL")+muted.Render(s.URL))
	return out
}

func (m Model) betweenTitle(p submodule.PinInfo) string {
	plural := func(n int) string {
		if n == 1 {
			return "commit"
		}
		return "commits"
	}
	switch {
	case p.Ahead > 0 && p.Behind > 0:
		return fmt.Sprintf("Pin and %s have diverged (%s%d %s%d):",
			p.Ref, m.theme.Glyphs.Ahead, p.Ahead, m.theme.Glyphs.Behind, p.Behind)
	case p.Ahead > 0:
		return fmt.Sprintf("Pin is %d %s ahead of %s:", p.Ahead, plural(p.Ahead), p.Ref)
	default:
		return fmt.Sprintf("Pin is %d %s behind %s:", p.Behind, plural(p.Behind), p.Ref)
	}
}
