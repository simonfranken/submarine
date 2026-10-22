package branches

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

func list() []submodule.Branch {
	return []submodule.Branch{
		{Ref: "refs/heads/main", Name: "main", Local: "main", Current: true, SHA: "aaaaaaaaaa"},
		{Ref: "refs/heads/dev", Name: "dev", Local: "dev", SHA: "bbbbbbbbbb"},
		{Ref: "refs/remotes/origin/main", Name: "origin/main", Local: "main", Remote: true, SHA: "aaaaaaaaaa"},
		{Ref: "refs/remotes/origin/feat/search", Name: "origin/feat/search", Local: "feat/search", Remote: true, SHA: "cccccccccc"},
	}
}

func typed(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func name(m *Model) string {
	if b := m.Selected(); b != nil {
		return b.Name
	}
	return ""
}

func TestSelectionMovesAndClamps(t *testing.T) {
	m := New(theme.Default(), "src/app")
	m.SetBranches(list())
	if name(m) != "main" {
		t.Fatalf("first = %q", name(m))
	}
	m.Up()
	if name(m) != "main" {
		t.Errorf("up at the top = %q", name(m))
	}
	m.Down()
	m.Down()
	if name(m) != "origin/main" {
		t.Errorf("down twice = %q", name(m))
	}
	m.PageDown()
	if name(m) != "origin/feat/search" {
		t.Errorf("page down = %q, want the last", name(m))
	}
	m.Down()
	if name(m) != "origin/feat/search" {
		t.Errorf("down at the end = %q", name(m))
	}
}

func TestSearchFiltersAndAllWordsMustMatch(t *testing.T) {
	m := New(theme.Default(), "src/app")
	m.SetBranches(list())
	typed(m, "ORIGIN")
	if len(m.shown) != 2 {
		t.Errorf("origin matches %d, want 2", len(m.shown))
	}
	typed(m, " sea")
	if len(m.shown) != 1 || name(m) != "origin/feat/search" {
		t.Errorf("shown = %d, selected %q", len(m.shown), name(m))
	}
	typed(m, "zzz")
	if m.Selected() != nil {
		t.Errorf("no match must select nothing, got %q", name(m))
	}
	if !strings.Contains(m.View(100, 30), "no branch matches") {
		t.Error("view does not say that nothing matches")
	}
}

func TestSelectionStaysOnTheSameBranchWhenItStillMatches(t *testing.T) {
	m := New(theme.Default(), "src/app")
	m.SetBranches(list())
	m.Down()
	m.Down() // origin/main
	typed(m, "main")
	if name(m) != "origin/main" {
		t.Errorf("selected %q, want origin/main kept", name(m))
	}
}

func TestViewStates(t *testing.T) {
	m := New(theme.Default(), "src/app")
	if v := m.View(100, 30); !strings.Contains(v, "reading branches") || !strings.Contains(v, "Switch src/app") {
		t.Errorf("loading view:\n%s", v)
	}
	m.SetErr(errors.New("not initialized; restore it first"))
	if v := m.View(100, 30); !strings.Contains(v, "not initialized") {
		t.Errorf("error view:\n%s", v)
	}
	m.SetBranches(nil)
	if v := m.View(100, 30); !strings.Contains(v, "no branches") {
		t.Errorf("empty view:\n%s", v)
	}
	m.SetBranches(list())
	v := m.View(100, 30)
	for _, want := range []string{"main", "origin/", "feat/search", "aaaaaaa", "enter switch"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
}

func TestListScrollsInASmallTerminal(t *testing.T) {
	var many []submodule.Branch
	for i := 0; i < 30; i++ {
		n := string(rune('a'+i%26)) + string(rune('a'+i/26))
		many = append(many, submodule.Branch{Ref: "refs/heads/" + n, Name: n, Local: n, SHA: "abcdef0123"})
	}
	m := New(theme.Default(), "x")
	m.SetBranches(many)
	m.View(80, 14) // fewer rows than branches
	for i := 0; i < 20; i++ {
		m.Down()
	}
	if m.cursor < m.offset || m.cursor >= m.offset+m.rows {
		t.Errorf("cursor %d outside the window [%d,%d)", m.cursor, m.offset, m.offset+m.rows)
	}
}
