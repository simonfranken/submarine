package detail

import (
	"fmt"
	"slices"
	"strings"
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
	for _, l := range m.spineLines(st.Spine(), true, theme.RoleNames{}) {
		got = append(got, ansi.Strip(l))
	}
	g := theme.Default().Glyphs
	want := []string{
		lab("tip") + g.Node + " 3f1bf20 Fix deps",
		gap + g.Gap + " 4 commits",
		lab("pin") + g.Node + " 9ac01de Add node",
		gap + g.Gap + " 1 commit",
		lab("you") + g.Node + " 48005ac Merge branch",
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
	for _, l := range m.spineLines(st.Spine(), false, theme.RoleNames{}) {
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
	lines := m.spineLines(st.Spine(), false, theme.RoleNames{})
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	want := lab("in sync") + theme.Default().Glyphs.Node + " 3f1bf20 Fix deps"
	if got := ansi.Strip(lines[0]); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSpineLinesFork(t *testing.T) {
	m := New(theme.Default(), nil)
	c := func(sha, subject string) *submodule.Commit { return &submodule.Commit{SHA: sha, Subject: subject} }
	st := submodule.Status{
		Pinned: c("c4d5e6f", "Experimental"), CheckedOut: c("c4d5e6f", "Experimental"), Tip: c("e1f2a3b", "Drop Go 1.21"),
		Pin:   submodule.PinInfo{Ahead: 1, Behind: 2},
		Bases: submodule.Bases{Tip: c("a1b2c3d", "Release 1.4")},
	}
	var got []string
	for _, l := range m.spineLines(st.Spine(), false, theme.RoleNames{}) {
		got = append(got, ansi.Strip(l))
	}
	g := theme.Default().Glyphs
	want := []string{
		lab("tip") + "  " + g.Node + " e1f2a3b Drop Go 1.21",
		gap + "  " + g.Gap + " 2 commits",
		lab("pin · you") + g.Node + " " + g.Gap + " c4d5e6f Experimental",
		gap + g.Gap + " " + g.Gap + " 1 commit",
		lab("base") + g.Node + g.Rule + g.Join + " a1b2c3d Release 1.4",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

// lab pads a label like the label column; gap is the empty label column.
func lab(s string) string { return fmt.Sprintf("%-*s", labelWidth, s) }

var gap = lab("")

func TestSpineLinesTargetName(t *testing.T) {
	m := New(theme.Default(), nil)
	c := func(sha string) *submodule.Commit { return &submodule.Commit{SHA: sha, Subject: "s"} }
	st := submodule.Status{
		Pinned: c("p"), CheckedOut: c("p"), Tip: c("t"), Pin: submodule.PinInfo{Behind: 1},
		TargetTip: c("m"), TargetDrift: submodule.Distance{Behind: 3},
	}
	for target, want := range map[string]string{
		"origin/marco-dev":                       "marco-dev",
		"origin/feature/a-very-long-branch-name": "feature/a-very-…",
		"":                                       "target",
	} {
		lines := m.spineLines(st.Spine(), false, theme.RoleNames{Target: target})
		last := ansi.Strip(lines[len(lines)-1])
		if !strings.HasPrefix(last, want) {
			t.Errorf("target %q: last line %q, want label %q", target, last, want)
		}
	}
}
