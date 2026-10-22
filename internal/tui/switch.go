package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"submarine/internal/submodule"
	"submarine/internal/tui/branches"
)

// branchesMsg is the branch list for the picker of path.
type branchesMsg struct {
	path     string
	branches []submodule.Branch
	err      error
}

// startSwitch opens the branch picker for the selection and reads its
// branches.
func (m Model) startSwitch() (tea.Model, tea.Cmd) {
	s := m.tree.Selected()
	sw, ok := m.provider.(submodule.Switcher)
	var flash string
	switch {
	case s == nil:
		return m, nil
	case !ok:
		flash = "this data source can't switch branches"
	case m.restore != nil:
		flash = "wait for the restore of " + m.restore.name + " to finish"
	case !s.IsRoot() && s.Status != nil && s.Status.Presence != submodule.PresenceOK:
		flash = s.Path + " is not initialized; restore it first"
	}
	if flash != "" {
		cmd := m.footer.Flash(flash)
		m.layout()
		return m, cmd
	}

	m.picker, m.pickerPath = branches.New(m.theme, restoreName(s)), s.Path
	m.picker.SetSpinner(m.spinner.View())
	ctx, path := m.ctx, s.Path
	load := func() tea.Msg {
		bs, err := sw.Branches(ctx, path)
		return branchesMsg{path, bs, err}
	}
	return m, tea.Batch(load, m.tick())
}

// branchesLoaded fills the picker, unless it was closed or is for another
// repository meanwhile.
func (m Model) branchesLoaded(msg branchesMsg) (tea.Model, tea.Cmd) {
	if m.picker == nil || m.pickerPath != msg.path {
		return m, nil
	}
	if msg.err != nil {
		m.picker.SetErr(msg.err)
	} else {
		m.picker.SetBranches(msg.branches)
	}
	return m, nil
}

// pickBranch handles a key while the picker is open: arrows move, enter
// switches, esc closes, everything else edits the search.
func (m Model) pickBranch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.picker = nil
	case tea.KeyUp:
		m.picker.Up()
	case tea.KeyDown:
		m.picker.Down()
	case tea.KeyPgUp:
		m.picker.PageUp()
	case tea.KeyPgDown:
		m.picker.PageDown()
	case tea.KeyEnter:
		return m.confirmPick()
	default:
		return m, m.picker.Update(msg)
	}
	return m, nil
}

// confirmPick closes the picker and switches to the selected branch, with
// the same check and question as a restore.
func (m Model) confirmPick() (tea.Model, tea.Cmd) {
	b := m.picker.Selected()
	if b == nil {
		return m, nil
	}
	s := m.byPath[m.pickerPath]
	m.picker = nil
	if s == nil || m.restore != nil {
		return m, nil
	}
	m.restore = &restoreState{path: s.Path, name: restoreName(s), branch: b}
	return m.checkRestore()
}

// mousePicker handles the mouse while the picker is open: the wheel moves
// the selection, a click on a branch switches to it, a click outside the
// popup closes it.
func (m Model) mousePicker(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.picker.Up()
	case tea.MouseButtonWheelDown:
		m.picker.Down()
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		inside, hit := m.picker.Click(msg.X, msg.Y, m.width, m.height)
		switch {
		case hit:
			return m.confirmPick()
		case !inside:
			m.picker = nil
		}
	}
	return m, nil
}
