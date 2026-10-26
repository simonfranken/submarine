// Package branches is the branch picker: a search line above a list of
// branches. It holds the state and renders itself; the app decides what the
// keys do. It also offers fixed choices of commits, e.g. the pins that
// resolve a pin conflict, with a note per row.
package branches

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

const (
	maxWidth  = 72
	minWidth  = 36
	maxRows   = 14
	frameRows = 6 // border, title, search, rule, hint, border
)

// Model is the picker. The zero value is not usable, use New.
type Model struct {
	theme   theme.Theme
	title   string
	verb    string // what enter does, e.g. "Switch"
	input   textinput.Model
	all     []submodule.Branch
	notes   []string // per entry of all, shown after the name; nil for none
	shown   []int    // indexes into all that match the search
	cursor  int      // index into shown
	offset  int      // first visible index into shown
	rows    int      // visible list rows
	loading bool
	err     error
	spinner string
	w, h    int // size of the popup as last rendered, for hit tests
}

// New returns a picker in the loading state.
func New(t theme.Theme, title string) *Model {
	in := textinput.New()
	in.Prompt = "/ "
	in.PromptStyle = theme.Fg(t.Primary)
	in.Cursor.Style = theme.Fg(t.Primary)
	in.Placeholder = "filter branches"
	in.PlaceholderStyle = theme.Fg(t.Muted)
	in.Focus()
	return &Model{theme: t, title: title, verb: "Switch", input: in, loading: true, rows: maxRows}
}

// NewChoices returns a picker filled with items at once: the verb says what
// enter does ("Resolve"), notes describe each item (same length as items),
// and cursor is the item selected first.
func NewChoices(t theme.Theme, verb, title string, items []submodule.Branch, notes []string, cursor int) *Model {
	m := New(t, title)
	m.verb, m.notes = verb, notes
	m.input.Placeholder = "filter"
	m.SetBranches(items)
	m.cursor = max(min(cursor, len(m.shown)-1), 0)
	m.scroll()
	return m
}

// SetBranches fills the list and ends the loading state.
func (m *Model) SetBranches(bs []submodule.Branch) {
	m.all, m.loading, m.err = bs, false, nil
	m.refilter()
}

// SetErr shows err instead of the list.
func (m *Model) SetErr(err error) { m.loading, m.err = false, err }

// SetSpinner sets the spinner frame shown while loading.
func (m *Model) SetSpinner(s string) { m.spinner = s }

// Loading reports whether the list is still being read.
func (m *Model) Loading() bool { return m.loading }

// Selected returns the branch under the cursor, nil if none matches.
func (m *Model) Selected() *submodule.Branch {
	if m.cursor < 0 || m.cursor >= len(m.shown) {
		return nil
	}
	b := m.all[m.shown[m.cursor]]
	return &b
}

// Index returns the position of the selected item in the list it was
// filled with, -1 if none matches.
func (m *Model) Index() int {
	if m.cursor < 0 || m.cursor >= len(m.shown) {
		return -1
	}
	return m.shown[m.cursor]
}

// Title is what the picker switches, e.g. "src/app".
func (m *Model) Title() string { return m.title }

// Search is the text in the search line.
func (m *Model) Search() string { return m.input.Value() }

func (m *Model) Up()       { m.move(-1) }
func (m *Model) Down()     { m.move(1) }
func (m *Model) PageUp()   { m.move(-max(m.rows-1, 1)) }
func (m *Model) PageDown() { m.move(max(m.rows-1, 1)) }

func (m *Model) move(d int) {
	m.cursor = max(min(m.cursor+d, len(m.shown)-1), 0)
	m.scroll()
}

func (m *Model) scroll() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.rows {
		m.offset = m.cursor - m.rows + 1
	}
	m.offset = max(min(m.offset, len(m.shown)-m.rows), 0)
}

// Update edits the search line. The cursor stays on the same branch if it
// still matches.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.refilter()
	}
	return cmd
}

// refilter recomputes shown: every word of the search must be in the name.
func (m *Model) refilter() {
	var keep string
	if b := m.Selected(); b != nil {
		keep = b.Ref
	}
	words := strings.Fields(strings.ToLower(m.input.Value()))
	m.shown = m.shown[:0]
	m.cursor = 0
	for i, b := range m.all {
		name := strings.ToLower(b.Name)
		ok := true
		for _, w := range words {
			ok = ok && strings.Contains(name, w)
		}
		if !ok {
			continue
		}
		if b.Ref == keep {
			m.cursor = len(m.shown)
		}
		m.shown = append(m.shown, i)
	}
	m.offset = 0
	m.scroll()
}

// View renders the popup for a terminal of the given size.
func (m *Model) View(termWidth, termHeight int) string {
	width := max(min(maxWidth, termWidth-4), minWidth)
	width = min(width, termWidth)
	inner := max(width-4, 1) // border and padding
	// the list is as tall as the branches, so filtering doesn't resize it
	m.rows = max(min(max(len(m.all), 3), maxRows, termHeight-frameRows-2), 1)
	m.scroll()
	m.input.Width = max(inner-3, 1)

	muted := theme.Fg(m.theme.Muted)
	lines := []string{
		theme.Fg(m.theme.Primary).Bold(true).Render(ansi.Truncate(m.verb+" "+m.title, inner, "…")),
		m.input.View(),
		muted.Render(strings.Repeat(m.theme.Glyphs.Rule, inner)),
	}

	var body []string
	switch {
	case m.loading:
		body = []string{theme.Fg(m.theme.Primary).Render(m.spinner) + " " + muted.Render("reading branches…")}
	case m.err != nil:
		body = []string{theme.Fg(m.theme.Danger).Render(ansi.Truncate(m.err.Error(), inner, "…"))}
	case len(m.shown) == 0 && len(m.all) == 0:
		body = []string{muted.Render("no branches")}
	case len(m.shown) == 0:
		body = []string{muted.Render("no branch matches")}
	default:
		end := min(m.offset+m.rows, len(m.shown))
		for i := m.offset; i < end; i++ {
			body = append(body, m.row(m.shown[i], i == m.cursor, inner))
		}
	}
	for len(body) < m.rows {
		body = append(body, "")
	}
	lines = append(lines, body[:m.rows]...)
	lines = append(lines, muted.Render(ansi.Truncate("↑↓ select · enter "+strings.ToLower(m.verb)+" · esc cancel", inner, "…")))

	out := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Border).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
	m.w, m.h = lipgloss.Width(out), lipgloss.Height(out)
	return out
}

// Origin is the top-left cell of the popup, centered in a terminal of the
// given size. Valid after View.
func (m *Model) Origin(termWidth, termHeight int) (x, y int) {
	return max((termWidth-m.w)/2, 0), max((termHeight-m.h)/2, 0)
}

// listTop is the line of the popup (border, title, search, rule) where the
// list starts.
const listTop = 4

// Click handles a left click at cell (x, y) of a terminal of the given size.
// inside reports whether the click was within the popup. If it hit a branch,
// that branch is selected and hit is true.
func (m *Model) Click(x, y, termWidth, termHeight int) (inside, hit bool) {
	ox, oy := m.Origin(termWidth, termHeight)
	x, y = x-ox, y-oy
	if x < 0 || y < 0 || x >= m.w || y >= m.h {
		return false, false
	}
	row := y - listTop
	if m.loading || m.err != nil || x == 0 || x == m.w-1 || row < 0 || row >= m.rows || m.offset+row >= len(m.shown) {
		return true, false
	}
	m.cursor = m.offset + row
	return true, true
}

// row renders the i-th branch: the name and its note, right-aligned the
// short SHA and age.
func (m *Model) row(i int, selected bool, width int) string {
	b := m.all[i]
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.theme.Selected)
	}
	marker := " "
	if b.Current {
		marker = "*"
	}
	name, prefix := b.Name, ""
	if b.Remote {
		if remote, rest, ok := strings.Cut(b.Name, "/"); ok {
			prefix, name = remote+"/", rest
		}
	}
	meta := short(b.SHA)
	if !b.When.IsZero() {
		meta += "  " + theme.Ago(b.When)
	}
	right := base.Foreground(m.theme.Muted).Render(meta)
	room := max(width-lipgloss.Width(right)-3, 1)

	nameStyle := base.Foreground(m.theme.Text)
	if b.Current {
		nameStyle = nameStyle.Bold(true)
	}
	left := base.Foreground(m.theme.Primary).Render(marker) + base.Render(" ") +
		base.Foreground(m.theme.Muted).Render(prefix) + nameStyle.Render(name)
	if i < len(m.notes) && m.notes[i] != "" {
		left += base.Foreground(m.theme.Muted).Render("  " + m.notes[i])
	}
	left = ansi.Truncate(left, room, "…")
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + base.Render(strings.Repeat(" ", gap)) + right
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
