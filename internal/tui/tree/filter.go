package tree

import (
	"strings"

	"submarine/internal/submodule"
)

// SetFilter shows only the submodules whose path contains term (ignoring
// case), plus their ancestors, which are shown expanded. An empty term
// removes the filter. It combines with SetProblemsOnly.
//
// If the selected submodule is filtered out, the next visible one is
// selected (the previous one if there is none after it, none if nothing
// matches). Removing the filter keeps the selection visible by expanding its
// ancestors.
func (m *Model) SetFilter(term string) {
	term = strings.ToLower(term)
	if term == m.filter {
		return
	}
	old := m.cursor
	m.filter = term
	m.narrow(old)
}

// SetProblemsOnly shows only the submodules with issues, plus their
// ancestors, which are shown expanded. A submodule whose status is loading or
// failed to load counts as a problem, since its state is unknown. The
// selection behaves as with SetFilter.
func (m *Model) SetProblemsOnly(on bool) {
	if on == m.problemsOnly {
		return
	}
	old := m.cursor
	m.problemsOnly = on
	m.narrow(old)
}

// ProblemsOnly reports whether only submodules with issues are shown.
func (m Model) ProblemsOnly() bool { return m.problemsOnly }

// Refit keeps the selection visible after statuses changed, which can make a
// submodule enter or leave the problems-only view.
func (m *Model) Refit() {
	if m.problemsOnly {
		m.reselect(m.cursor)
		m.clampOffset()
	}
}

// narrow fixes the selection after the filter or the problems-only view
// changed.
func (m *Model) narrow(old *submodule.Submodule) {
	if m.narrowed() {
		m.reselect(old)
	} else {
		for p := parentOf(old); p != nil; p = p.Parent {
			m.expanded[p.Path] = true
		}
		if m.cursor == nil {
			m.cursor = m.firstNode()
		}
	}
	m.clampOffset()
}

// Filter returns the active filter term in lower case.
func (m Model) Filter() string { return m.filter }

// narrowed reports whether the tree is reduced to matches.
func (m Model) narrowed() bool { return m.filter != "" || m.problemsOnly }

func parentOf(s *submodule.Submodule) *submodule.Submodule {
	if s == nil {
		return nil
	}
	return s.Parent
}

// reselect makes sure the cursor is on a visible submodule. If old is not
// visible any more, the closest visible one after it in display order
// becomes the selection, else the closest one before it.
func (m *Model) reselect(old *submodule.Submodule) {
	if old != nil && old == m.root { // the superproject is never filtered out
		m.cursor = old
		return
	}
	visible := map[*submodule.Submodule]bool{}
	var first *submodule.Submodule
	for _, r := range m.rows() {
		if r.kind == rowNode {
			visible[r.node] = true
			if first == nil {
				first = r.node
			}
		}
	}
	if old != nil && visible[old] {
		m.cursor = old
		return
	}
	m.cursor = first
	if first == nil {
		m.cursor = m.root // nothing matches: the superproject remains
	}
	if old == nil || first == nil {
		return
	}
	var all []row
	m.appendAll(&all, m.roots, 0)
	i := 0
	for ; i < len(all); i++ {
		if all[i].node == old {
			break
		}
	}
	for j := i + 1; j < len(all); j++ {
		if visible[all[j].node] {
			m.cursor = all[j].node
			return
		}
	}
	for j := i - 1; j >= 0; j-- {
		if visible[all[j].node] {
			m.cursor = all[j].node
			return
		}
	}
}

// appendAll lists every submodule in display order, whatever is expanded or
// filtered.
func (m *Model) appendAll(out *[]row, mods []*submodule.Submodule, depth int) {
	for _, g := range groupByDir(mods) {
		for _, s := range g.mods {
			*out = append(*out, row{kind: rowNode, depth: depth, node: s})
			m.appendAll(out, s.Children, depth+1)
		}
	}
}

// isExpanded reports whether the children of s are shown. While narrowed,
// that is the case if a match is below s, whatever the user expanded.
func (m *Model) isExpanded(s *submodule.Submodule) bool {
	if !m.narrowed() {
		return m.expanded[s.Path]
	}
	for _, c := range s.Children {
		if m.shown(c) {
			return true
		}
	}
	return false
}

// filtered returns the submodules of mods that pass the filter.
func (m *Model) filtered(mods []*submodule.Submodule) []*submodule.Submodule {
	if !m.narrowed() {
		return mods
	}
	var out []*submodule.Submodule
	for _, s := range mods {
		if m.shown(s) {
			out = append(out, s)
		}
	}
	return out
}

// shown reports whether s matches the filter or has a descendant that does.
func (m *Model) shown(s *submodule.Submodule) bool {
	if strings.Contains(strings.ToLower(s.Path), m.filter) && (!m.problemsOnly || hasProblem(s)) {
		return true
	}
	for _, c := range s.Children {
		if m.shown(c) {
			return true
		}
	}
	return false
}

// hasProblem reports whether s has an issue or an unknown state (loading, or
// failed to load, like HiddenIn counts it).
func hasProblem(s *submodule.Submodule) bool {
	return s.Status == nil || len(s.Status.Issues()) > 0
}
