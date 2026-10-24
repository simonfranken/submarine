package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"submarine/internal/submodule"
)

// newPinUpdateModel is the switch model with the first submodule selected
// and given st as its status.
func newPinUpdateModel(t *testing.T, loss submodule.Loss, st *submodule.Status) (tea.Model, *fakeSwitcher, string) {
	t.Helper()
	m, f := newSwitchModel(t, loss)
	repo, err := f.Tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := repo.Submodules[0].Path
	m, _ = m.Update(statusLoadedMsg{path: path, status: st})
	mm := m.(Model)
	mm.tree.Select(path)
	return mm, f, path
}

// behind is a pin two commits behind origin/feat, whose tip is bbbbbbb.
func behind() *submodule.Status {
	return &submodule.Status{
		Pin: submodule.PinInfo{Ref: "origin/feat", Behind: 2, Pushed: true, Source: submodule.RefFromGitmodules},
		Tip: &submodule.Commit{SHA: "bbbb"}, // abbreviated, like the git provider's
	}
}

func TestPinUpdateSwitchesToTheTipOfTheRef(t *testing.T) {
	m, f, path := newPinUpdateModel(t, submodule.Loss{}, behind())
	m, cmd := press(m, "u")
	m = settle(m, cmd)
	if len(f.switched) != 1 || f.switched[0] != path+" refs/remotes/origin/feat" {
		t.Fatalf("switched %q", f.switched)
	}
	if v := m.View(); !strings.Contains(v, "updated "+path+" to feat") {
		t.Errorf("no flash:\n%s", v)
	}
}

func TestPinUpdateAsksWhenSomethingIsLost(t *testing.T) {
	m, f, path := newPinUpdateModel(t, submodule.Loss{Files: 1, Modules: 1}, behind())
	m, cmd := press(m, "u")
	for _, msg := range run(cmd) {
		if l, ok := msg.(lossMsg); ok {
			m, _ = m.Update(l)
		}
	}
	if v := m.View(); !strings.Contains(v, "Update "+path+" to origin/feat?") || !strings.Contains(v, "y update") {
		t.Errorf("question not shown:\n%s", v)
	}
	m, cmd = press(m, "y")
	feed(m, cmd)
	if len(f.switched) != 1 {
		t.Fatalf("switched %q", f.switched)
	}
}

func TestPinUpdateAbortsWhenTheRefMoved(t *testing.T) {
	st := behind()
	st.Tip.SHA = "ccccccc" // the row showed another tip than the branch has now
	m, f, _ := newPinUpdateModel(t, submodule.Loss{}, st)
	m, cmd := press(m, "u")
	m = settle(m, cmd)
	if len(f.switched) != 0 {
		t.Fatalf("switched %q", f.switched)
	}
	if v := m.View(); !strings.Contains(v, "origin/feat moved") {
		t.Errorf("no notice:\n%s", v)
	}
}

func TestPinUpdateOnlyWhenThePinIsBehind(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*submodule.Status)
		want string
	}{
		{"up to date", func(s *submodule.Status) { s.Pin.Behind = 0 }, "is up to date with origin/feat"},
		{"ahead", func(s *submodule.Status) { s.Pin.Behind, s.Pin.Ahead = 0, 1 }, "is ahead of origin/feat"},
		{"diverged", func(s *submodule.Status) { s.Pin.Ahead = 1 }, "has diverged from origin/feat"},
		{"tag", func(s *submodule.Status) { s.Pin.Source = submodule.RefFromTag }, "is not on a branch"},
		{"missing", func(s *submodule.Status) { s.Presence = submodule.NotInitialized }, "is not initialized"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := behind()
			tc.edit(st)
			m, f, _ := newPinUpdateModel(t, submodule.Loss{}, st)
			m, _ = press(m, "u") // the command is only the flash's timer
			if len(f.switched) != 0 || m.(Model).restore != nil {
				t.Fatalf("updated anyway: %q", f.switched)
			}
			if v := m.View(); !strings.Contains(v, tc.want) {
				t.Errorf("flash lacks %q:\n%s", tc.want, v)
			}
		})
	}
}

func TestPinUpdateOfTheSuperprojectFastForwardsItsBranch(t *testing.T) {
	m, f := newSwitchModel(t, submodule.Loss{}) // the superproject is selected
	st := behind()
	st.Root = true
	m, _ = m.Update(statusLoadedMsg{path: "", status: st})
	m, cmd := press(m, "u")
	m = settle(m, cmd)
	if len(f.switched) != 1 || f.switched[0] != " refs/remotes/origin/feat" {
		t.Fatalf("switched %q", f.switched)
	}
	if v := m.View(); !strings.Contains(v, "updated the superproject to feat") {
		t.Errorf("no flash:\n%s", v)
	}
}

func TestPinUpdateOfTheSuperprojectWithoutUpstreamSaysSo(t *testing.T) {
	m, f := newSwitchModel(t, submodule.Loss{})
	st := behind()
	st.Root, st.Pin.Source, st.Tip = true, submodule.RefFromSHA, nil
	m, _ = m.Update(statusLoadedMsg{path: "", status: st})
	m, _ = press(m, "u")
	if len(f.switched) != 0 || !strings.Contains(m.View(), "the superproject has no upstream branch") {
		t.Errorf("switched %q, view:\n%s", f.switched, m.View())
	}
}

func TestPinUpdateRefusesALocalRef(t *testing.T) {
	st := behind()
	st.Pin.Ref, st.Tip.SHA = "main", "aaaa" // upstream is a local branch
	m, f, _ := newPinUpdateModel(t, submodule.Loss{}, st)
	m, cmd := press(m, "u")
	m = settle(m, cmd)
	if len(f.switched) != 0 || !strings.Contains(m.View(), "main is a local branch") {
		t.Errorf("switched %q, view:\n%s", f.switched, m.View())
	}
}
