package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"submarine/internal/submodule"
	"submarine/internal/submodule/mock"
)

// fakeSwitcher is the mock data with a branch switch that records its calls.
type fakeSwitcher struct {
	submodule.Provider
	loss     submodule.Loss
	branches []submodule.Branch
	switched []string // path + " " + ref
	ends     string   // the branch Switch reports
}

func (f *fakeSwitcher) Branches(context.Context, string) ([]submodule.Branch, error) {
	return f.branches, nil
}

func (f *fakeSwitcher) SwitchLoss(context.Context, string, submodule.Branch) (submodule.Loss, error) {
	return f.loss, nil
}

func (f *fakeSwitcher) Switch(_ context.Context, path string, b submodule.Branch) (string, error) {
	f.switched = append(f.switched, path+" "+b.Ref)
	return f.ends, nil
}

func newSwitchModel(t *testing.T, loss submodule.Loss) (tea.Model, *fakeSwitcher) {
	t.Helper()
	f := &fakeSwitcher{
		Provider: mock.New(),
		loss:     loss,
		ends:     "feat",
		branches: []submodule.Branch{
			{Ref: "refs/heads/main", Name: "main", Local: "main", Current: true, SHA: "aaaaaaa"},
			{Ref: "refs/remotes/origin/feat", Name: "origin/feat", Local: "feat", Remote: true, SHA: "bbbbbbb"},
		},
	}
	var m tea.Model = New(context.Background(), f)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	repo, err := f.Tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(treeLoadedMsg{repo: repo})
	return m, f
}

// openPicker presses b and delivers the branch list.
func openPicker(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	m, cmd := press(m, "b")
	m = feed(m, cmd)
	if m.(Model).picker == nil {
		t.Fatal("the picker did not open")
	}
	return m
}

func pressKey(m tea.Model, t tea.KeyType) (tea.Model, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: t})
}

// settle delivers the loss check and, if it ran at once, the switch.
func settle(m tea.Model, cmd tea.Cmd) tea.Model {
	var next tea.Cmd
	for _, msg := range run(cmd) {
		if l, ok := msg.(lossMsg); ok {
			m, next = m.Update(l)
		}
	}
	return feed(m, next)
}

func TestSwitchOpensThePickerWithTheBranches(t *testing.T) {
	m, _ := newSwitchModel(t, submodule.Loss{})
	m = openPicker(t, m)
	v := m.View()
	for _, want := range []string{"Switch the superproject", "main", "origin/", "feat"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q", want)
		}
	}
}

func TestSwitchTypingFiltersAndEnterSwitchesWithoutLoss(t *testing.T) {
	m, f := newSwitchModel(t, submodule.Loss{})
	m = openPicker(t, m)
	m, _ = press(m, "fe") // j and k would be text, not navigation
	m, cmd := pressKey(m, tea.KeyEnter)
	if m.(Model).picker != nil {
		t.Error("enter must close the picker")
	}
	m = settle(m, cmd)
	if len(f.switched) != 1 || f.switched[0] != " refs/remotes/origin/feat" {
		t.Fatalf("switched %q", f.switched)
	}
	if m.(Model).restore != nil {
		t.Error("switch still in progress")
	}
}

func TestSwitchArrowsSelect(t *testing.T) {
	m, f := newSwitchModel(t, submodule.Loss{})
	m = openPicker(t, m)
	m, _ = pressKey(m, tea.KeyDown)
	m, cmd := pressKey(m, tea.KeyEnter)
	settle(m, cmd)
	if len(f.switched) != 1 || f.switched[0] != " refs/remotes/origin/feat" {
		t.Fatalf("switched %q, want the second branch", f.switched)
	}
}

func TestSwitchAsksWhenSomethingIsLost(t *testing.T) {
	m, f := newSwitchModel(t, submodule.Loss{Files: 2, Modules: 1})
	m = openPicker(t, m)
	m, cmd := pressKey(m, tea.KeyEnter)
	var next tea.Cmd
	for _, msg := range run(cmd) {
		if l, ok := msg.(lossMsg); ok {
			m, next = m.Update(l)
		}
	}
	r := m.(Model).restore
	if next != nil || r == nil || r.phase != restoreAsking {
		t.Fatalf("want a question, got %+v", r)
	}
	if v := m.View(); !strings.Contains(v, "Switch the superproject to main?") || !strings.Contains(v, "y switch") {
		t.Errorf("question not shown:\n%s", v)
	}
	if c, _ := press(m, "n"); c.(Model).restore != nil || len(f.switched) != 0 {
		t.Fatal("n must cancel")
	}
	m, cmd = press(m, "y")
	feed(m, cmd)
	if len(f.switched) != 1 {
		t.Fatalf("switched %q", f.switched)
	}
}

func TestSwitchEscClosesThePicker(t *testing.T) {
	m, f := newSwitchModel(t, submodule.Loss{})
	m = openPicker(t, m)
	m, _ = pressKey(m, tea.KeyEsc)
	if m.(Model).picker != nil || len(f.switched) != 0 {
		t.Fatal("esc must close without switching")
	}
}

func TestSwitchWithoutSwitcherSaysSo(t *testing.T) {
	var m tea.Model = New(context.Background(), mock.New())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	repo, _ := mock.New().Tree(context.Background())
	m, _ = m.Update(treeLoadedMsg{repo: repo})
	m, _ = press(m, "b")
	if m.(Model).picker != nil {
		t.Fatal("the mock can't switch")
	}
	if !strings.Contains(m.View(), "can't switch branches") {
		t.Error("no flash")
	}
}

func TestSwitchReportsADetachedHead(t *testing.T) {
	m, f := newSwitchModel(t, submodule.Loss{})
	f.ends = ""
	m = openPicker(t, m)
	m, _ = pressKey(m, tea.KeyDown)
	m, cmd := pressKey(m, tea.KeyEnter)
	m = settle(m, cmd)
	if v := m.View(); !strings.Contains(v, "detached") {
		t.Errorf("flash lacks the detached note:\n%s", v)
	}
}
