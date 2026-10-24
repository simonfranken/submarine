// Package footer renders key hints and short-lived status messages.
package footer

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/tui/theme"
)

const flashDuration = 2 * time.Second

type clearFlashMsg struct{ id int }

// Keys provides the key groups. The short bar shows Navigation, Tools (and
// Submodule, if requested) on the left, Global and then Highlight (on a
// highlighted background) on the right. The full help has two pages: the
// HelpGroups (the last one is always kept), then the theme's legend with
// LegendKeys next to it.
type Keys interface {
	Navigation() []key.Binding
	Tools() []key.Binding
	Submodule() []key.Binding
	Global() []key.Binding
	Highlight() []key.Binding
	HelpGroups() []Group
	LegendKeys() Group
}

// Group is a titled column of the full help.
type Group struct {
	Title string
	Keys  []key.Binding
}

// helpPage is what the footer shows besides a flash.
type helpPage int

const (
	pageBar    helpPage = iota // one line of key hints
	pageKeys                   // every key, in titled columns
	pageLegend                 // what the words and symbols mean
)

// columnGap separates the columns of the full help.
const columnGap = "    "

type Model struct {
	theme   theme.Theme
	help    help.Model
	keys    Keys
	width   int
	flash   string
	flashID int
	page    helpPage
	actions bool // show the submodule bindings in the short bar
}

func New(t theme.Theme, keys Keys) Model {
	return Model{theme: t, help: theme.NewHelp(t), keys: keys}
}

// SetKeys replaces the key groups, e.g. if bindings changed.
func (m *Model) SetKeys(k Keys) { m.keys = k }

// SetActions shows the submodule bindings in the short bar. Set it if the
// detail pane doesn't show them.
func (m *Model) SetActions(on bool) { m.actions = on }

func (m *Model) SetWidth(w int) {
	m.width = w
	m.help.Width = w - 2
}

// CycleHelp shows the next help page: the short bar, every key, the legend,
// then the short bar again.
func (m *Model) CycleHelp() { m.page = (m.page + 1) % 3 }

// HelpOpen reports whether a full help page is shown.
func (m Model) HelpOpen() bool { return m.page != pageBar }

// CloseHelp goes back to the short bar from any help page.
func (m *Model) CloseHelp() { m.page = pageBar }

// Legend reports whether the legend is shown.
func (m Model) Legend() bool { return m.page == pageLegend }

// Flash shows msg for a short time instead of the key hints.
func (m *Model) Flash(msg string) tea.Cmd {
	m.flashID++
	m.flash = msg
	id := m.flashID
	return tea.Tick(flashDuration, func(time.Time) tea.Msg { return clearFlashMsg{id} })
}

func (m *Model) Update(msg tea.Msg) {
	if c, ok := msg.(clearFlashMsg); ok && c.id == m.flashID {
		m.flash = ""
	}
}

func (m Model) View() string {
	style := lipgloss.NewStyle().Width(m.width).Padding(0, 1)
	if m.flash != "" {
		return style.Foreground(m.theme.Warn).Render(m.flash)
	}
	switch m.page {
	case pageKeys:
		var cols []column
		for _, g := range m.keys.HelpGroups() {
			cols = append(cols, m.keyColumn(g))
		}
		return style.Render(m.columns(cols[:len(cols)-1], cols[len(cols)-1], m.width-2))
	case pageLegend:
		var cols []column
		for _, g := range m.theme.Legend() {
			c := column{title: g.Title}
			for _, r := range g.Rows {
				c.rows = append(c.rows, [2]string{r.Word, m.help.Styles.FullDesc.Render(r.Meaning)})
			}
			cols = append(cols, c)
		}
		return style.Render(m.columns(cols, m.keyColumn(m.keys.LegendKeys()), m.width-2))
	}
	return style.Render(m.shortBar(m.width - 2))
}

// column is a titled column of the full help: rows of a rendered key (or
// word) and its description.
type column struct {
	title string
	rows  [][2]string
}

func (m Model) keyColumn(g Group) column {
	c := column{title: g.Title}
	for _, b := range g.Keys {
		if b.Enabled() {
			c.rows = append(c.rows, [2]string{m.help.Styles.FullKey.Render(b.Help().Key), m.help.Styles.FullDesc.Render(b.Help().Desc)})
		}
	}
	return c
}

// columns lays cols out side by side, each under its title, and last at the
// end. Like the global keys in the short bar, last is always kept: the
// columns before it that don't fit into w are left out from the right,
// marked with an ellipsis.
func (m Model) columns(cols []column, last column, w int) string {
	tail := m.block(last)
	avail := w - lipgloss.Width(tail) - len(columnGap)
	var out []string
	width := 0
	for i, c := range cols {
		b := m.block(c)
		bw := lipgloss.Width(b) + len(columnGap)
		if i == 0 {
			bw -= len(columnGap)
		}
		if width+bw > avail {
			for len(out) > 0 && width+2 > avail { // room for the ellipsis
				width -= lipgloss.Width(out[len(out)-1]) + len(columnGap)
				out = out[:len(out)-1]
			}
			if width+2 <= avail {
				out = append(out, " "+m.help.Styles.Ellipsis.Render("…"))
			}
			break
		}
		out = append(out, b)
		width += bw
	}
	var parts []string
	for i, b := range out {
		if i > 0 && !strings.HasPrefix(b, " ") {
			parts = append(parts, columnGap)
		}
		parts = append(parts, b)
	}
	if len(parts) > 0 {
		parts = append(parts, columnGap)
	}
	parts = append(parts, tail)
	return lipgloss.NewStyle().MaxWidth(w).Render(lipgloss.JoinHorizontal(lipgloss.Top, parts...))
}

// block renders a column: its title, then its rows with the descriptions
// aligned.
func (m Model) block(c column) string {
	left := 0
	for _, r := range c.rows {
		left = max(left, lipgloss.Width(r[0]))
	}
	lines := []string{theme.Fg(m.theme.Muted).Bold(true).Render(c.title)}
	for _, r := range c.rows {
		lines = append(lines, r[0]+strings.Repeat(" ", left-lipgloss.Width(r[0])+1)+r[1])
	}
	return strings.Join(lines, "\n")
}

// shortBar puts the global keys on the right and as many of the others on the
// left as fit; the submodule keys are dropped first, then the tools.
func (m Model) shortBar(w int) string {
	h := m.help
	h.Width = 0
	right := h.ShortHelpView(m.keys.Global())
	if hl := m.keys.Highlight(); len(hl) > 0 {
		var parts []string
		for _, b := range hl {
			parts = append(parts, b.Help().Key+" "+b.Help().Desc)
		}
		right += "  " + m.theme.Highlight().Render(" "+strings.Join(parts, "  ")+" ")
	}
	left := append(slices.Clip(m.keys.Navigation()), m.keys.Tools()...)
	if m.actions {
		left = append(left, m.keys.Submodule()...)
	}
	for ; ; left = left[:len(left)-1] {
		l := h.ShortHelpView(left)
		if gap := w - lipgloss.Width(l) - lipgloss.Width(right); gap >= 2 || len(left) == 0 {
			if gap < 0 { // not even the global keys fit
				return ansi.Truncate(right, w, "…")
			}
			return l + strings.Repeat(" ", gap) + right
		}
	}
}
