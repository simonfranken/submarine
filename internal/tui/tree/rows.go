package tree

import (
	"path"

	"submarine/internal/submodule"
)

type rowKind int

const (
	rowDir  rowKind = iota // non-selectable directory header, e.g. "modules/"
	rowNode                // a submodule
)

type row struct {
	kind  rowKind
	depth int
	label string // directory label for rowDir
	node  *submodule.Submodule
}

// rows flattens the visible part of the tree.
func (m *Model) rows() []row {
	var out []row
	m.appendRows(&out, m.roots, 0)
	return out
}

// appendRows groups mods by the directory of their RelPath. Each group with a
// directory gets a header row; its submodules are indented one level deeper.
func (m *Model) appendRows(out *[]row, mods []*submodule.Submodule, depth int) {
	for _, g := range groupByDir(mods) {
		d := depth
		if g.dir != "." {
			*out = append(*out, row{kind: rowDir, depth: depth, label: g.dir + "/"})
			d++
		}
		for _, s := range g.mods {
			*out = append(*out, row{kind: rowNode, depth: d, node: s})
			if m.expanded[s.Path] && len(s.Children) > 0 {
				m.appendRows(out, s.Children, d+1)
			}
		}
	}
}

type dirGroup struct {
	dir  string
	mods []*submodule.Submodule
}

// groupByDir keeps the order in which directories first appear.
func groupByDir(mods []*submodule.Submodule) []dirGroup {
	var groups []dirGroup
	idx := map[string]int{}
	for _, s := range mods {
		dir := path.Dir(s.RelPath)
		i, ok := idx[dir]
		if !ok {
			i = len(groups)
			idx[dir] = i
			groups = append(groups, dirGroup{dir: dir})
		}
		groups[i].mods = append(groups[i].mods, s)
	}
	return groups
}
