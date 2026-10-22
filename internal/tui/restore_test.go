package tui

import (
	"context"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"submarine/internal/submodule"
	"submarine/internal/submodule/mock"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

func TestLossText(t *testing.T) {
	cases := []struct {
		in   submodule.Loss
		want string
	}{
		{submodule.Loss{Files: 1, Modules: 1}, "1 changed file"},
		{submodule.Loss{Files: 7, Modules: 3, Commits: 2, Repositories: 1}, "7 changed files in 3 repositories, 2 commits on no branch, 1 untracked repository"},
		{submodule.Loss{Operations: 1, Modules: 1, Unknown: 2}, "1 operation in progress, 2 repositories of unknown state"},
		{submodule.Loss{StagedPins: 2, Modules: 1}, "2 staged pins"},
	}
	for _, c := range cases {
		if got := lossText(c.in); got != c.want {
			t.Errorf("lossText(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// fakeRestorer is the mock data with a restore that records its calls.
type fakeRestorer struct {
	submodule.Provider
	loss     submodule.Loss
	restored []string
	to       []submodule.RestoreTarget
}

func (f *fakeRestorer) RestoreLoss(context.Context, string, submodule.RestoreTarget) (submodule.Loss, error) {
	return f.loss, nil
}

func (f *fakeRestorer) Restore(_ context.Context, path string, to submodule.RestoreTarget) error {
	f.restored = append(f.restored, path)
	f.to = append(f.to, to)
	return nil
}

// run executes a command, flattening batches, and returns the messages.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			if c != nil {
				out = append(out, run(c)...)
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

func press(m tea.Model, k string) (tea.Model, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
}

// newRestoreModel returns a model with the mock tree loaded.
func newRestoreModel(t *testing.T, loss submodule.Loss) (tea.Model, *fakeRestorer) {
	t.Helper()
	f := &fakeRestorer{Provider: mock.New(), loss: loss}
	var m tea.Model = New(context.Background(), f)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	repo, err := f.Tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(treeLoadedMsg{repo: repo})
	return m, f
}

// feed delivers the messages of cmd to m, once.
func feed(m tea.Model, cmd tea.Cmd) tea.Model {
	for _, msg := range run(cmd) {
		switch msg.(type) {
		case lossMsg, restoreDoneMsg, branchesMsg:
			m, _ = m.Update(msg)
		}
	}
	return m
}

func TestRestoreWithoutLossRunsAtOnce(t *testing.T) {
	m, f := newRestoreModel(t, submodule.Loss{})
	m, cmd := press(m, "x")
	var next tea.Cmd
	for _, msg := range run(cmd) {
		if l, ok := msg.(lossMsg); ok {
			m, next = m.Update(l)
		}
	}
	m = feed(m, next)
	if len(f.restored) != 1 || f.restored[0] != "" {
		t.Fatalf("restored %q, want the superproject once", f.restored)
	}
	if m.(Model).restore != nil {
		t.Error("restore still in progress")
	}
}

func TestRestoreAsksWhenSomethingIsLost(t *testing.T) {
	m, f := newRestoreModel(t, submodule.Loss{Files: 3, Modules: 1})
	m, cmd := press(m, "x")
	var next tea.Cmd
	for _, msg := range run(cmd) {
		if l, ok := msg.(lossMsg); ok {
			m, next = m.Update(l)
		}
	}
	if next != nil || m.(Model).restore == nil || m.(Model).restore.phase != restoreAsking {
		t.Fatalf("want a question, got %+v", m.(Model).restore)
	}

	// any other key cancels
	c, _ := press(m, "n")
	if c.(Model).restore != nil || len(f.restored) != 0 {
		t.Fatal("n must cancel without restoring")
	}

	// y restores
	m, cmd = press(m, "y")
	m = feed(m, cmd)
	if len(f.restored) != 1 {
		t.Fatalf("restored %q", f.restored)
	}
}

func TestRestoreWithoutRestorerSaysSo(t *testing.T) {
	var m tea.Model = New(context.Background(), mock.New())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	repo, _ := mock.New().Tree(context.Background())
	m, _ = m.Update(treeLoadedMsg{repo: repo})
	m, _ = press(m, "x")
	if m.(Model).restore != nil {
		t.Fatal("the mock can't restore")
	}
}

func TestRestoreOfAStagedPinAsksWhichPin(t *testing.T) {
	m, f := newRestoreModel(t, submodule.Loss{})
	mm := m.(Model)
	var staged *submodule.Submodule
	submodule.Walk(mm.repo.Submodules, func(s *submodule.Submodule) {
		if staged == nil && s.Children == nil {
			staged = s
		}
	})
	staged.Status = &submodule.Status{PinStaged: true, Pin: submodule.PinInfo{Pushed: true}}
	mm.tree.Select(staged.Path)
	if mm.tree.Selected() != staged {
		t.Fatalf("setup: selected %v", mm.tree.Selected())
	}
	m = mm

	m, _ = press(m, "x")
	if m.(Model).restore == nil || m.(Model).restore.phase != restoreChoosing {
		t.Fatalf("want the pin question, got %+v", m.(Model).restore)
	}
	m, cmd := press(m, "c")
	var next tea.Cmd
	for _, msg := range run(cmd) {
		if l, ok := msg.(lossMsg); ok {
			m, next = m.Update(l)
		}
	}
	feed(m, next)
	if len(f.to) != 1 || f.to[0] != submodule.RestoreCommitted || f.restored[0] != staged.Path {
		t.Fatalf("restored %q to %v, want %s to the committed pin", f.restored, f.to, staged.Path)
	}
}
