package tree

import (
	"os"
	"testing"

	zone "github.com/lrstanley/bubblezone"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

// fixture:
//
//	a
//	src/
//	  b
//	    modules/
//	      c
//	        d
//	      e
//	  f
func fixture() []*submodule.Submodule {
	mk := func(path, rel string, parent *submodule.Submodule) *submodule.Submodule {
		s := &submodule.Submodule{Name: path, Path: path, RelPath: rel, Parent: parent}
		if parent != nil {
			parent.Children = append(parent.Children, s)
		}
		return s
	}
	a := mk("a", "a", nil)
	b := mk("b", "src/b", nil)
	c := mk("c", "modules/c", b)
	mk("d", "d", c)
	mk("e", "modules/e", b)
	f := mk("f", "src/f", nil)
	return []*submodule.Submodule{a, b, f}
}

func newTree(t *testing.T) *Model {
	t.Helper()
	m := New(theme.Default())
	m.SetRoots(fixture())
	m.SetSize(80, 20)
	return &m
}

func assertCursor(t *testing.T, m *Model, want string) {
	t.Helper()
	if got := m.Selected().Path; got != want {
		t.Fatalf("cursor = %q, want %q", got, want)
	}
}

func visible(m *Model) []string {
	var out []string
	for _, r := range m.rows() {
		if r.kind == rowDir {
			out = append(out, r.label)
		} else {
			out = append(out, r.node.Path)
		}
	}
	return out
}

func TestReplaceRootsKeepsState(t *testing.T) {
	m := newTree(t)
	m.Down()  // b
	m.Right() // expand b, cursor on c
	m.Right() // expand c, cursor on d
	assertCursor(t, m, "d")

	m.ReplaceRoots(fixture()) // same paths, new nodes
	assertCursor(t, m, "d")
	if got := len(visible(m)); got != 8 {
		t.Errorf("visible rows = %v, want b and c still expanded", visible(m))
	}

	m.ReplaceRoots(fixture()[:1]) // only "a" left
	assertCursor(t, m, "a")
}

func TestInitiallyCollapsedWithHeaders(t *testing.T) {
	m := newTree(t)
	got := visible(m)
	want := []string{"a", "src/", "b", "f"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
	assertCursor(t, m, "a")
}

func TestDownSkipsHeaders(t *testing.T) {
	m := newTree(t)
	m.Down()
	assertCursor(t, m, "b")
	m.Down()
	assertCursor(t, m, "f")
	m.Down() // stays at end
	assertCursor(t, m, "f")
	m.Up()
	m.Up()
	m.Up() // stays at start
	assertCursor(t, m, "a")
}

func TestRightExpandsAndSelectsFirstChild(t *testing.T) {
	m := newTree(t)
	m.Down()
	m.Right()
	assertCursor(t, m, "c")
	if !m.expanded["b"] {
		t.Fatal("b should be expanded")
	}
	m.Right()
	assertCursor(t, m, "d")
	m.Right() // leaf: no-op
	assertCursor(t, m, "d")
}

func TestRightOnLeafIsNoop(t *testing.T) {
	m := newTree(t)
	m.Right()
	assertCursor(t, m, "a")
}

func TestLeftCollapsesParentAndSelectsIt(t *testing.T) {
	m := newTree(t)
	m.Down()
	m.Right() // b expanded, cursor c
	m.Down()  // e
	assertCursor(t, m, "e")
	m.Left()
	assertCursor(t, m, "b")
	if m.expanded["b"] {
		t.Fatal("b should be collapsed")
	}
}

func TestLeftOnExpandedNodeCollapsesIt(t *testing.T) {
	m := newTree(t)
	m.Down()
	m.Toggle() // expand b, cursor stays
	assertCursor(t, m, "b")
	m.Left()
	assertCursor(t, m, "b")
	if m.expanded["b"] {
		t.Fatal("b should be collapsed")
	}
	m.Left() // top level and collapsed: no-op
	assertCursor(t, m, "b")
}

func TestExpansionStateKept(t *testing.T) {
	m := newTree(t)
	m.Down()
	m.Right()  // c
	m.Right()  // d, c expanded
	m.Left()   // collapse c, select c
	m.Right()  // d again
	m.Left()   // c
	m.Toggle() // re-expand c
	m.Left()   // c is expanded -> collapse c
	m.Toggle() // expand c
	m.Up()     // b
	m.Toggle() // collapse b
	m.Toggle() // expand b: c should still be expanded
	got := visible(m)
	want := []string{"a", "src/", "b", "modules/", "c", "d", "e", "f"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
}

func TestCollapsingAncestorMovesCursor(t *testing.T) {
	m := newTree(t)
	m.Down()
	m.Right()
	m.Right() // cursor d
	m.setExpanded(m.roots[1], false)
	assertCursor(t, m, "b")
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	m := newTree(t)
	m.SetSize(80, 2)
	m.Down()
	m.Right() // rows: a src/ b modules/ c e f ; cursor c (index 4)
	if i := m.cursorRow(m.rows()); i < m.offset || i >= m.offset+m.height {
		t.Fatalf("cursor row %d not in [%d,%d)", i, m.offset, m.offset+m.height)
	}
	m.Home()
	if m.offset != 0 {
		t.Fatalf("offset = %d after Home, want 0", m.offset)
	}
}

func TestCollapseAllSelectsTopLevelAncestor(t *testing.T) {
	m := newTree(t)
	m.Down()
	m.Right() // c
	m.Right() // d
	m.CollapseAll()
	assertCursor(t, m, "b")
	got := visible(m)
	want := []string{"a", "src/", "b", "f"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	m.Toggle() // re-expanding b must not restore c's expansion
	if m.expanded["c"] {
		t.Fatal("c should be collapsed after CollapseAll")
	}
}

func TestExpandAllDescendsFirstChildren(t *testing.T) {
	m := newTree(t)
	m.Down() // b
	m.ExpandAll()
	assertCursor(t, m, "d")
	got := visible(m)
	want := []string{"a", "src/", "b", "modules/", "c", "d", "e", "f"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestExpandAllOnLeafKeepsSelection(t *testing.T) {
	m := newTree(t) // cursor a, a leaf
	m.ExpandAll()
	assertCursor(t, m, "a")
	if !m.expanded["b"] || !m.expanded["c"] {
		t.Fatal("all nodes with children should be expanded")
	}
}

func TestFilterShowsMatchesWithAncestors(t *testing.T) {
	m := newTree(t)
	m.SetFilter("D")
	want := []string{"src/", "b", "modules/", "c", "d"}
	if got := visible(m); !equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	assertCursor(t, m, "b") // a was selected and is gone: next available (an ancestor of the match)
}

func TestFilterSelectionMovesToNextThenPrevious(t *testing.T) {
	m := newTree(t)
	m.Select("a")
	m.SetFilter("f")
	assertCursor(t, m, "f")
	m.SetFilter("")
	m.Select("f")
	m.SetFilter("a") // only a matches, it lies before f
	assertCursor(t, m, "a")
}

func TestFilterWithoutMatchSelectsNothing(t *testing.T) {
	m := newTree(t)
	m.SetFilter("zzz")
	if m.Selected() != nil || len(m.rows()) != 0 {
		t.Fatalf("selected %v, rows %v", m.Selected(), visible(m))
	}
	m.Down() // must not panic
	m.SetFilter("f")
	assertCursor(t, m, "f")
}

func TestClearingFilterKeepsSelectionVisible(t *testing.T) {
	m := newTree(t)
	m.SetFilter("d")
	m.Select("d")
	m.SetFilter("")
	assertCursor(t, m, "d")
	want := []string{"a", "src/", "b", "modules/", "c", "d", "e", "f"}
	if got := visible(m); !equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// newTreeWithRoot is newTree with a selectable superproject.
func newTreeWithRoot(t *testing.T) *Model {
	t.Helper()
	m := New(theme.Default())
	m.SetRoot(&submodule.Submodule{Name: "root"})
	m.SetRoots(fixture())
	m.SetSize(80, 20)
	return &m
}

func TestRootIsSelectedInitially(t *testing.T) {
	m := newTreeWithRoot(t)
	if !m.RootSelected() {
		t.Fatal("root not selected initially")
	}
	assertCursor(t, m, "")
}

func TestRootIsAboveFirstRow(t *testing.T) {
	m := newTreeWithRoot(t)
	m.Down()
	assertCursor(t, m, "a")
	if m.RootSelected() {
		t.Error("root still selected after Down")
	}
	m.Up()
	if !m.RootSelected() {
		t.Error("Up from the first submodule should select the root")
	}
	m.Up() // stays
	if !m.RootSelected() {
		t.Error("Up on the root should stay on the root")
	}

	m.End()
	assertCursor(t, m, "f")
	m.Home()
	if !m.RootSelected() {
		t.Error("Home should select the root")
	}
	m.PageDown()
	assertCursor(t, m, "f")
	m.PageUp()
	if !m.RootSelected() {
		t.Error("PageUp should reach the root")
	}
}

func TestRootNavigationKeys(t *testing.T) {
	m := newTreeWithRoot(t)
	m.Right() // like Down
	assertCursor(t, m, "a")
	m.Left() // top level, nothing to collapse: the parent is the root
	if !m.RootSelected() {
		t.Error("Left on a top-level submodule should select the root")
	}
	m.Left()
	m.Toggle()
	m.CollapseAll()
	m.ExpandAll()
	if !m.RootSelected() {
		t.Error("fold keys must not move the root selection")
	}

	m.Down()
	m.Down() // b
	m.Right()
	m.Right() // d
	m.CollapseAll()
	assertCursor(t, m, "b")
}

func TestReplaceRootsKeepsRootSelected(t *testing.T) {
	m := newTreeWithRoot(t)
	newRoot := &submodule.Submodule{Name: "root"}
	m.SetRoot(newRoot)
	m.ReplaceRoots(fixture())
	if !m.RootSelected() || m.Selected() != newRoot {
		t.Error("root selection should move to the reloaded root")
	}

	m.Down()
	m.SetRoot(&submodule.Submodule{Name: "root"})
	m.ReplaceRoots(fixture())
	assertCursor(t, m, "a")
}

func TestFilterKeepsRoot(t *testing.T) {
	m := newTreeWithRoot(t)
	m.SetFilter("f")
	if !m.RootSelected() {
		t.Error("the root stays selected while filtering")
	}
	m.Down()
	assertCursor(t, m, "f")

	m.SetFilter("zzz") // nothing matches: the root remains, not nil
	if !m.RootSelected() {
		t.Error("no match should fall back to the root")
	}
	m.SetFilter("")
	if !m.RootSelected() {
		t.Error("closing the filter should keep the root")
	}
}
