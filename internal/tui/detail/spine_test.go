package detail

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

func TestSpineLines(t *testing.T) {
	m := New(theme.Default(), nil)
	st := submodule.Status{
		Pinned:     &submodule.Commit{SHA: "9ac01de", Subject: "Add node"},
		CheckedOut: &submodule.Commit{SHA: "48005ac", Subject: "Merge branch"},
		Tip:        &submodule.Commit{SHA: "3f1bf20", Subject: "Fix deps"},
		Pin:        submodule.PinInfo{Behind: 4},
		Drift:      submodule.Distance{Behind: 1},
	}
	var got []string
	for _, l := range m.spineLines(st.Spine(), true) {
		got = append(got, ansi.Strip(l))
	}
	g := theme.Default().Glyphs
	want := []string{
		"tip       " + g.Node + " 3f1bf20 Fix deps",
		"          " + g.Gap + " 4 commits",
		"pin       " + g.Node + " 9ac01de Add node",
		"          " + g.Gap + " 1 commit",
		"you       " + g.Node + " 48005ac Merge branch",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestSpineLinesStagedMovesBack(t *testing.T) {
	m := New(theme.Default(), nil)
	st := submodule.Status{
		Pinned:         &submodule.Commit{SHA: "87a1059", Subject: "refactor"},
		CheckedOut:     &submodule.Commit{SHA: "87a1059", Subject: "refactor"},
		PinStaged:      true,
		Committed:      &submodule.Commit{SHA: "8f23a6e", Subject: "Merge"},
		CommittedDrift: submodule.Distance{Ahead: 15},
	}
	var got []string
	for _, l := range m.spineLines(st.Spine(), false) {
		got = append(got, ansi.Strip(l))
	}
	g := theme.Default().Glyphs
	want := []string{
		"pin          " + g.Node + " 8f23a6e Merge",
		"             " + g.Gap + " 15 commits (staged pin moves back)",
		"staged · you " + g.Node + " 87a1059 refactor",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestSpineLinesInSync(t *testing.T) {
	m := New(theme.Default(), nil)
	c := &submodule.Commit{SHA: "3f1bf20", Subject: "Fix deps"}
	st := submodule.Status{Pinned: c, CheckedOut: c, Tip: c}
	lines := m.spineLines(st.Spine(), false)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	want := "in sync   " + theme.Default().Glyphs.Node + " 3f1bf20 Fix deps"
	if got := ansi.Strip(lines[0]); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
