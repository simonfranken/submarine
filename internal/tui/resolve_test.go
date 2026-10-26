package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"submarine/internal/submodule"
	"submarine/internal/submodule/mock"
)

// fakeResolver is the mock data, whose libs/parser has a pin conflict, with
// a resolve that records its calls.
type fakeResolver struct {
	submodule.Provider
	loss     submodule.Loss
	resolved []string // path + " " + sha
}

func (f *fakeResolver) ResolveLoss(context.Context, string, string) (submodule.Loss, error) {
	return f.loss, nil
}

func (f *fakeResolver) Resolve(_ context.Context, path, sha string) error {
	f.resolved = append(f.resolved, path+" "+sha)
	return nil
}

// Restorer, so that x reaches its own checks.
func (f *fakeResolver) RestoreLoss(context.Context, string, submodule.RestoreTarget) (submodule.Loss, error) {
	return submodule.Loss{}, nil
}

func (f *fakeResolver) Restore(context.Context, string, submodule.RestoreTarget) error { return nil }

// Switcher, so that u reaches its own checks.
func (f *fakeResolver) Branches(context.Context, string) ([]submodule.Branch, error) { return nil, nil }

func (f *fakeResolver) SwitchLoss(context.Context, string, submodule.Branch) (submodule.Loss, error) {
	return submodule.Loss{}, nil
}

func (f *fakeResolver) Switch(context.Context, string, submodule.Branch) (string, error) {
	return "", nil
}

// newResolveModel loads the mock data with every status and selects
// libs/parser.
func newResolveModel(t *testing.T, loss submodule.Loss) (tea.Model, *fakeResolver) {
	t.Helper()
	data := mock.New()
	data.MinDelay, data.MaxDelay = 0, 0
	f := &fakeResolver{Provider: data, loss: loss}
	var m tea.Model = New(context.Background(), f)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	repo, err := f.Tree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(treeLoadedMsg{repo: repo})
	var paths []string
	submodule.Walk(repo.Submodules, func(s *submodule.Submodule) { paths = append(paths, s.Path) })
	for _, p := range paths {
		st, err := data.Status(context.Background(), p)
		m, _ = m.Update(statusLoadedMsg{p, st, err})
	}
	m, _ = press(m, "G")
	m, _ = press(m, "k")
	if s := m.(Model).tree.Selected(); s == nil || s.Path != "libs/parser" {
		t.Fatalf("selected %v, want libs/parser", s)
	}
	return m, f
}

func TestResolvePickerOffersSidesAndPreselectsACommitWithBoth(t *testing.T) {
	m, f := newResolveModel(t, submodule.Loss{})
	m, _ = press(m, "m")
	mm := m.(Model)
	if mm.picker == nil {
		t.Fatal("m did not open the picker")
	}
	v := m.View()
	for _, want := range []string{"Resolve libs/parser", "ours · origin/main ↓3", "theirs · origin/feat/v2", "has both · 2 behind the tip", "enter resolve"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q", want)
		}
	}
	if b := mm.picker.Selected(); b == nil || b.Name != "origin/main" {
		t.Errorf("preselected %+v, want the commit on origin/main", b)
	}
	m, cmd := pressKey(m, tea.KeyEnter)
	m = settle(m, cmd)
	if len(f.resolved) != 1 || f.resolved[0] != "libs/parser d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3" {
		t.Fatalf("resolved %q", f.resolved)
	}
	if m.(Model).restore != nil {
		t.Error("resolve still in progress")
	}
}

func TestResolveAsksBeforeALoss(t *testing.T) {
	m, f := newResolveModel(t, submodule.Loss{Files: 2, Modules: 1})
	m, _ = press(m, "m")
	m, _ = pressKey(m, tea.KeyUp) // theirs
	m, cmd := pressKey(m, tea.KeyEnter)
	m = settle(m, cmd)
	if v := m.View(); !strings.Contains(v, "Resolve libs/parser to feat/v2 7a8b9c0?") || !strings.Contains(v, "Discards 2 changed files") {
		t.Fatalf("no question:\n%s", v)
	}
	m, cmd = press(m, "y")
	feed(m, cmd)
	if len(f.resolved) != 1 || !strings.HasSuffix(f.resolved[0], " 7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b") {
		t.Fatalf("resolved %q", f.resolved)
	}
}

func TestPinConflictBlocksRestoreAndUpdate(t *testing.T) {
	for _, k := range []string{"x", "u"} {
		m, _ := newResolveModel(t, submodule.Loss{})
		m, _ = press(m, k)
		if m.(Model).restore != nil {
			t.Errorf("%s started on a pin conflict", k)
		}
		if v := m.View(); !strings.Contains(v, "has a pin conflict; m chooses its pin") {
			t.Errorf("%s: no hint", k)
		}
	}
}

func TestResolveWithoutConflict(t *testing.T) {
	m, _ := newResolveModel(t, submodule.Loss{})
	m, _ = press(m, "j") // zlib
	m, _ = press(m, "m")
	if m.(Model).picker != nil || !strings.Contains(m.View(), "has no pin conflict") {
		t.Error("m on a submodule without a pin conflict")
	}
}
