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
	statusWidth = 16 // "unpushed +1 ·2 ·3"
	deltaWidth  = 6
	maxRefWidth = 22
)

type Model struct {
	id       string // zone prefix, unique to this instance
	theme    theme.Theme
	root     *submodule.Submodule // the superproject: selectable, above the first row; may be nil
	roots    []*submodule.Submodule
	expanded map[string]bool // keyed by Submodule.Path; kept when ancestors collapse
	filter   string          // lower-case; empty if not filtering
	cursor   *submodule.Submodule
	offset   int // index of the first visible row
	width    int
	height   int
	spinner  string
}

func New(t theme.Theme) Model {
	return Model{id: zone.NewPrefix(), theme: t, expanded: map[string]bool{}}
}

// SetRoot sets the superproject, which can be selected like a row above the
// first one. Call it before SetRoots / ReplaceRoots.
func (m *Model) SetRoot(root *submodule.Submodule) { m.root = root }

// SetRoots replaces the tree. All nodes start collapsed. The superproject is
// selected if there is one, else the first submodule.
func (m *Model) SetRoots(roots []*submodule.Submodule) {
	m.roots = roots
	m.expanded = map[string]bool{}
	m.cursor = nil
	m.offset = 0
	if m.root != nil {
		m.cursor = m.root
		return
	}
	m.cursor = m.firstNode()
}

// ReplaceRoots swaps in a reloaded tree. Unlike SetRoots it keeps the
// expansion state and the cursor (by path); if the selected submodule is
// gone, the cursor moves to the first one.
func (m *Model) ReplaceRoots(roots []*submodule.Submodule) {
	var path string
	if m.cursor != nil {
		path = m.cursor.Path
	}
	wasRoot := m.cursor.IsRoot() // by path: SetRoot may have installed a new node
	m.roots = roots
	m.cursor = nil
	if wasRoot && m.root != nil {
		m.cursor = m.root
	}
	for _, r := range m.rows() {
		if m.cursor == nil && r.kind == rowNode && r.node.Path == path {
			m.cursor = r.node
			break
		}
	}
	if m.cursor == nil {
		m.reselect(nil)
	}
	m.clampOffset()
}

func (m *Model) SetSize(w, h int) {
	m.width, m.height = w, h
	m.clampOffset()
}

// SetSpinner sets the frame drawn for submodules that are still loading.
func (m *Model) SetSpinner(frame string) { m.spinner = frame }

// Selected returns the submodule under the cursor, or nil. It is the
// superproject (empty Path) while that is selected.
func (m Model) Selected() *submodule.Submodule { return m.cursor }

// RootSelected reports whether the superproject is selected.
func (m Model) RootSelected() bool { return m.onRoot() }

func (m Model) onRoot() bool { return m.root != nil && m.cursor == m.root }

// RootZone is the zone mark of the superproject's display; a click on it
// selects the superproject. The caller wraps the header in it.
func (m Model) RootZone() string { return m.id + "root" }

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
	if m.onRoot() {
		m.Down()
		return
	}
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
	case m.root != nil && !m.onRoot():
		m.cursor = m.root // the superproject is the parent of the top level
		m.offset = 0
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
// and stopping at the ends. The superproject counts as a row before the
// first one.
func (m *Model) moveBy(n int) {
	rows := m.rows()
	i := m.cursorRow(rows)
	if m.onRoot() {
		i = -1
	} else if i < 0 {
		return
	}
	step := 1
	if n < 0 {
		step, n = -1, -n
	}
	for j := i + step; n > 0 && j >= -1 && j < len(rows); j += step {
		switch {
		case j == -1:
			if m.root != nil {
				i = -1
				n--
			}
		case rows[j].kind == rowNode:
			i = j
			n--
		}
	}
	if i < 0 {
		m.cursor = m.root
	} else {
		m.cursor = rows[i].node
	}
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

func (m *Model) firstNode() *submodule.Submodule {
	for _, r := range m.rows() {
		if r.kind == rowNode {
			return r.node
		}
	}
	return nil
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
	if m.onRoot() {
		m.offset = 0
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
	if m.root != nil && zone.Get(m.RootZone()).InBounds(mm) {
		m.cursor = m.root
		m.offset = 0
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
	if len(rows) == 0 && m.filter != "" {
		lines = append(lines, theme.Fg(m.theme.Muted).Width(m.width).Render(" no submodule matches"))
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
	if len(s.Children) == 0 || m.isExpanded(s) {
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

	collapsed := len(s.Children) > 0 && !m.isExpanded(s)

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

	// status: the most severe issue as a word, "+n" for further issues;
	// a muted "in sync" without issues, so an empty cell never looks broken.
	// A staged pin is no issue: it replaces "in sync", or follows the issue
	// as a blue marker (also for staged pins below a collapsed row).
	staged := s.Status != nil && s.Status.PinStaged
	h := s.HiddenIssues()
	stagedMark := staged && len(issues) > 0
	status := t.StatusWord(s, base, m.spinner)
	if stagedMark || collapsed && h.Staged > 0 {
		status += pad(1) + seg(t.Info, t.Glyphs.Staged)
	}
	if collapsed && h.Total() > 0 {
		for _, c := range []struct {
			level submodule.Level
			n     int
		}{{submodule.LevelDanger, h.Danger}, {submodule.LevelWarn, h.Warn}} {
			if c.n == 0 {
				continue
			}
			if status != "" {
				status += pad(1)
			}
			status += t.LevelStyle(c.level, base).Render(fmt.Sprintf("%s%d", t.Glyphs.Problems, c.n))
		}
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
