package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"submarine/internal/submodule"
)

// startPinUpdate moves the pin of the selected submodule to the tip of its
// ref, if the pin is only behind it: a switch to that branch, with the tip
// the row shows. Nothing is fetched. For the superproject the pin is HEAD
// and the ref its upstream: a fast-forward of its branch.
func (m Model) startPinUpdate() (tea.Model, tea.Cmd) {
	s := m.tree.Selected()
	if s == nil {
		return m, nil
	}
	_, ok := m.provider.(submodule.Switcher)
	var flash string
	switch {
	case !ok:
		flash = "this data source can't update pins"
	case m.restore != nil:
		flash = "wait for the restore of " + m.restore.name + " to finish"
	default:
		flash = pinUpdateBlocker(s)
	}
	if flash != "" {
		cmd := m.footer.Flash(flash)
		m.layout()
		return m, cmd
	}

	st := s.Status
	m.restore = &restoreState{path: s.Path, name: restoreName(s), pinUpdate: true, phase: restoreChecking}
	m.layout()
	ctx, path, ref, sha := m.ctx, s.Path, st.Pin.Ref, st.Tip.SHA
	sw := m.provider.(submodule.Switcher)
	check := func() tea.Msg {
		b, err := pinUpdateBranch(ctx, sw, path, ref, sha)
		if err != nil {
			return lossMsg{path: path, err: err}
		}
		loss, err := sw.SwitchLoss(ctx, path, *b)
		return lossMsg{path, loss, err, b}
	}
	return m, tea.Batch(check, m.tick())
}

// pinUpdateBlocker says why the pin of s can't be updated, "" if it can.
func pinUpdateBlocker(s *submodule.Submodule) string {
	st, name := s.Status, restoreName(s)
	pin := "the pin of " + name
	if s.IsRoot() {
		pin = name // its pin is HEAD
	}
	switch {
	case st == nil:
		return name + " is still loading"
	case st.Presence != submodule.PresenceOK:
		return name + " is not initialized; restore it first"
	case st.Pin.Ref == "" || st.Tip == nil || st.Pin.Source == submodule.RefFromTag || st.Pin.Source == submodule.RefFromSHA:
		if s.IsRoot() {
			return name + " has no upstream branch"
		}
		return pin + " is not on a branch"
	case st.Pin.Ahead > 0 && st.Pin.Behind > 0:
		return pin + " has diverged from " + st.Pin.Ref + "; b switches the branch"
	case st.Pin.Ahead > 0:
		return pin + " is ahead of " + st.Pin.Ref
	case st.Pin.Behind == 0:
		return name + " is up to date with " + st.Pin.Ref
	}
	return ""
}

// pinUpdateBranch finds the branch ref among the branches of path. Its tip
// must still be sha, the tip the row showed (abbreviated, see Commit).
func pinUpdateBranch(ctx context.Context, sw submodule.Switcher, path, ref, sha string) (*submodule.Branch, error) {
	bs, err := sw.Branches(ctx, path)
	if err != nil {
		return nil, err
	}
	for _, b := range bs {
		if b.Name != ref {
			continue
		}
		if !b.Remote {
			return nil, fmt.Errorf("%s is a local branch; b switches to it", ref)
		}
		if !strings.HasPrefix(b.SHA, sha) {
			return nil, fmt.Errorf("%s moved; check the new state", ref)
		}
		return &b, nil
	}
	return nil, fmt.Errorf("%s not found", ref)
}
