// Package tree implements the collapsible submodule tree view.
package tree

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

const (
	statusWidth = 14 // "unpushed +1 ·2"
	deltaWidth  = 6
	maxRefWidth = 22
)

type Model struct {
	id       string // zone prefix, unique to this instance
	theme    theme.Theme
	roots    []*submodule.Submodule
	expanded map[string]bool // keyed by Submodule.Path; kept when ancestors collapse
	cursor   *submodule.Submodule
	offset   int // index of the first visible row
	width    int
	height   int
	spinner  string
}

func New(t theme.Theme) Model {
	return Model{id: zone.NewPrefix(), theme: t, expanded: map[string]bool{}}
}

// SetRoots replaces the tree. All nodes start collapsed.
func (m *Model) SetRoots(roots []*submodule.Submodule) {
	m.roots = roots
	m.expanded = map[string]bool{}
	m.cursor = nil
	m.offset = 0
	if len(roots) > 0 {
		m.cursor = m.rows()[m.firstNodeRow()].node
	}
}

func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.clampOffset()
}

// SetSpinner sets the frame drawn for submodules that are still loading.
func (m *Model) SetSpinner(frame string) { m.spinner = frame }

// Selected returns the submodule under the cursor, or nil.
func (m Model) Selected() *submodule.Submodule { return m.cursor }

// --- navigation --------------------------------------------------------------

// Up selects the previous submodule row.
func (m *Model) Up() { m.moveBy(-1) }

// Down selects the next submodule row.
func (m *Model) Down() { m.moveBy(1) }

// PageUp and PageDown move by one screen height.
func (m *Model) PageUp()   { m.moveBy(-max(m.height-1, 1)) }
func (m *Model) PageDown() { m.moveBy(max(m.height-1, 1)) }

// Home and End select the first and last visible submodule.
func (m *Model) Home() { m.moveBy(-len(m.rows())) }
func (m *Model) End()  { m.moveBy(len(m.rows())) }

// Toggle expands or collapses the selected submodule.
func (m *Model) Toggle() {
	if m.cursor != nil && len(m.cursor.Children) > 0 {
		m.setExpanded(m.cursor, !m.expanded[m.cursor.Path])
	}
}

// Right expands the selected submodule and selects its first child.
func (m *Model) Right() {
	if m.cursor == nil || len(m.cursor.Children) == 0 {
		return
	}
	m.setExpanded(m.cursor, true)
	m.Down() // children directly follow their parent
}

// Left collapses the selected submodule if it is expanded; otherwise it
// collapses the parent and selects it.
func (m *Model) Left() {
	switch {
	case m.cursor == nil:
	case m.expanded[m.cursor.Path] && len(m.cursor.Children) > 0:
		m.setExpanded(m.cursor, false)
	case m.cursor.Parent != nil:
		m.setExpanded(m.cursor.Parent, false)
	}
}

// CollapseAll collapses every submodule and selects the top-level ancestor
// of the current selection.
func (m *Model) CollapseAll() {
	m.expanded = map[string]bool{}
	for m.cursor != nil && m.cursor.Parent != nil {
		m.cursor = m.cursor.Parent
	}
	m.clampOffset()
}

// ExpandAll expands every submodule and follows the first child from the
// current selection down to a submodule without children.
func (m *Model) ExpandAll() {
	submodule.Walk(m.roots, func(s *submodule.Submodule) {
		if len(s.Children) > 0 {
			m.expanded[s.Path] = true
		}
	})
	for m.cursor != nil && len(m.cursor.Children) > 0 {
		m.Down() // children directly follow their parent
	}
	m.clampOffset()
}

// Select selects the submodule with the given path if it is visible.
func (m *Model) Select(path string) {
	for _, r := range m.rows() {
		if r.kind == rowNode && r.node.Path == path {
			m.cursor = r.node
			m.clampOffset()
			return
		}
	}
}

// setExpanded changes the expansion of s. If collapsing hides the cursor,
// s becomes the selection.
func (m *Model) setExpanded(s *submodule.Submodule, open bool) {
	m.expanded[s.Path] = open
	if !open && isAncestor(s, m.cursor) {
		m.cursor = s
	}
	m.clampOffset()
}

func isAncestor(a, s *submodule.Submodule) bool {
	for s != nil {
		s = s.Parent
		if s == a {
			return true
		}
	}
	return false
}

// moveBy moves the cursor by n submodule rows, skipping directory headers
// and stopping at the ends.
func (m *Model) moveBy(n int) {
	rows := m.rows()
	i := m.cursorRow(rows)
	if i < 0 {
		return
	}
	step := 1
	if n < 0 {
		step, n = -1, -n
	}
	for j := i + step; n > 0 && j >= 0 && j < len(rows); j += step {
		if rows[j].kind == rowNode {
			i = j
			n--
		}
	}
	m.cursor = rows[i].node
	m.clampOffset()
}

func (m Model) cursorRow(rows []row) int {
	for i, r := range rows {
		if r.kind == rowNode && r.node == m.cursor {
			return i
		}
	}
	return -1
}

func (m Model) firstNodeRow() int {
	for i, r := range m.rows() {
		if r.kind == rowNode {
			return i
		}
	}
	return 0
}

// clampOffset scrolls so the cursor is visible. When the cursor is the first
// submodule, headers above it are shown too.
func (m *Model) clampOffset() {
	if m.height <= 0 {
		return
	}
	rows := m.rows()
	i := m.cursorRow(rows)
	if i < 0 {
		return
	}
	if i == m.firstNodeRow() {
		i = 0
	}
	if i < m.offset {
		m.offset = i
	}
	if i >= m.offset+m.height {
		m.offset = i - m.height + 1
	}
	m.offset = max(0, min(m.offset, len(rows)-m.height))
}

// --- mouse -------------------------------------------------------------------

// Update handles mouse input. Keyboard input is mapped by the parent model.
func (m *Model) Update(msg tea.Msg) {
	mm, ok := msg.(tea.MouseMsg)
	if !ok {
		return
	}
	switch mm.Button {
	case tea.MouseButtonWheelUp:
		m.Up()
		return
	case tea.MouseButtonWheelDown:
		m.Down()
		return
	}
	if mm.Action != tea.MouseActionPress || mm.Button != tea.MouseButtonLeft {
		return
	}
	for _, r := range m.visibleRows() {
		if r.kind != rowNode {
			continue
		}
		if zone.Get(m.id + "toggle:" + r.node.Path).InBounds(mm) {
			m.cursor = r.node
			m.Toggle()
			return
		}
		if zone.Get(m.id + "row:" + r.node.Path).InBounds(mm) {
			m.cursor = r.node
			m.clampOffset()
			return
		}
	}
}

func (m Model) visibleRows() []row {
	rows := m.rows()
	end := min(len(rows), m.offset+m.height)
	if m.offset >= end {
		return nil
	}
	return rows[m.offset:end]
}

// --- view --------------------------------------------------------------------

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	rows := m.visibleRows()
	refW := m.refWidth()
	nameColW := m.nameColumnWidth(rows, refW)
	lines := make([]string, 0, m.height)
	for _, r := range rows {
		if r.kind == rowDir {
			lines = append(lines, m.dirLine(r))
		} else {
			lines = append(lines, m.nodeLine(r, nameColW, refW))
		}
	}
	for len(lines) < m.height {
		lines = append(lines, strings.Repeat(" ", m.width))
	}
	return strings.Join(lines, "\n")
}

// nameColumnWidth is the width of indent + toggle + name for the widest
// visible row, so the status column sits close to the names. It shrinks to
// fit the available width.
func (m Model) nameColumnWidth(rows []row, refW int) int {
	w := 0
	for _, r := range rows {
		if r.kind == rowNode {
			w = max(w, 2*r.depth+3+lipgloss.Width(r.node.Name)+len(m.childCount(r.node)))
		}
	}
	return min(w+2, m.width-statusWidth-refW-deltaWidth-3)
}

func (m Model) childCount(s *submodule.Submodule) string {
	if len(s.Children) == 0 || m.expanded[s.Path] {
		return ""
	}
	return fmt.Sprintf(" [%d]", len(s.Children))
}

func (m Model) refWidth() int {
	w := 0
	submodule.Walk(m.roots, func(s *submodule.Submodule) {
		w = max(w, lipgloss.Width(refLabel(s)))
	})
	return min(w, maxRefWidth)
}

func refLabel(s *submodule.Submodule) string {
	if s.Status == nil {
		return ""
	}
	if s.Status.Pin.Ref != "" {
		return s.Status.Pin.Ref
	}
	if s.Status.Pinned != nil {
		return s.Status.Pinned.SHA
	}
	return ""
}

func (m Model) dirLine(r row) string {
	text := strings.Repeat("  ", r.depth) + " " + r.label
	return theme.Fg(m.theme.Muted).Width(m.width).Render(ansi.Truncate(text, m.width, "…"))
}

// nodeLine renders: indent ▸ name [n] │ status ·k │ ref │ delta
func (m Model) nodeLine(r row, nameColW, refW int) string {
	t, s := m.theme, r.node
	base := lipgloss.NewStyle()
	if s == m.cursor {
		base = base.Background(t.Selected).Bold(true)
	}
	seg := func(c lipgloss.TerminalColor, text string) string {
		return base.Foreground(c).Render(text)
	}
	pad := func(n int) string { return base.Render(strings.Repeat(" ", max(n, 0))) }

	collapsed := len(s.Children) > 0 && !m.expanded[s.Path]

	// toggle marker
	marker := "  "
	if len(s.Children) > 0 {
		marker = t.Glyphs.Expanded + " "
		if collapsed {
			marker = t.Glyphs.Collapsed + " "
		}
	}
	indent := pad(len("  ") * r.depth)
	toggle := zone.Mark(m.id+"toggle:"+s.Path, seg(t.Muted, " "+marker))

	// Rows without issues are drawn muted so problems stand out.
	var issues []submodule.Issue
	if s.Status != nil {
		issues = s.Status.Issues()
	}
	quiet := s.Err == nil && s.Status != nil && len(issues) == 0
	textColor := t.Text
	if quiet && s != m.cursor {
		textColor = t.Muted
	}

	// name + child count
	nameW := nameColW - 2*r.depth - 3
	count := m.childCount(s)
	name := ansi.Truncate(s.Name, max(nameW-len(count), 1), "…")
	nameCell := seg(textColor, name) + seg(t.Muted, count) + pad(nameW-lipgloss.Width(name+count))

	// status: the most severe issue as a word, "+n" for further issues
	var status string
	switch {
	case s.Err != nil:
		status = t.LevelStyle(submodule.LevelDanger, base).Render("error")
	case s.Status == nil:
		status = seg(t.Primary, m.spinner)
	case len(issues) > 0:
		status = t.LevelStyle(issues[0].Level, base).Render(theme.IssueWord(issues[0].Kind))
		if len(issues) > 1 {
			status += seg(t.Muted, fmt.Sprintf(" +%d", len(issues)-1))
		}
	}
	if k, worst := s.HiddenIssues(); collapsed && k > 0 {
		if status != "" {
			status += pad(1)
		}
		status += t.LevelStyle(worst, base).Render(fmt.Sprintf("%s%d", t.Glyphs.Problems, k))
	}
	status = ansi.Truncate(status, statusWidth, "")
	status += pad(statusWidth - lipgloss.Width(status))

	// ref + delta
	refColor := textColor
	if s.Status != nil && s.Status.Pin.Ref == "" {
		refColor = t.Muted
	}
	ref := ansi.Truncate(refLabel(s), refW, "…")
	refCell := seg(refColor, ref) + pad(refW-lipgloss.Width(ref))
	delta := ""
	if s.Status != nil {
		delta = t.Delta(s.Status.Pin, base)
	}
	deltaCell := delta + pad(deltaWidth-lipgloss.Width(delta))

	content := nameCell + pad(1) + status + pad(1) + refCell + pad(1) + deltaCell
	line := indent + toggle + zone.Mark(m.id+"row:"+s.Path, content)
	if w := lipgloss.Width(line); w < m.width {
		line += pad(m.width - w)
	}
	return ansi.Truncate(line, m.width, "")
}
