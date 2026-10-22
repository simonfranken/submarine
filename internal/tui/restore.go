package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

type restorePhase int

const (
	restoreChoosing restorePhase = iota // staged or committed pin?
	restoreChecking                     // reading what would be lost
	restoreAsking                       // waiting for y / n
	restoreRunning
)

// restoreState is the restore in progress; only one runs at a time.
type restoreState struct {
	path, name string // name is how the footer calls it
	phase      restorePhase
	to         submodule.RestoreTarget
	loss       submodule.Loss
	branch     *submodule.Branch // set for a branch switch, which reuses this flow
}

// lossMsg is the result of the check before a restore.
type lossMsg struct {
	path string
	loss submodule.Loss
	err  error
}

type restoreDoneMsg struct {
	path   string
	branch string // the branch a switch ended on, "" if detached
	err    error
}

// startRestore checks what restoring the selection would discard. If nothing
// would be lost it restores at once, else it asks first. A submodule with a
// staged pin first asks which pin to restore to.
func (m Model) startRestore() (tea.Model, tea.Cmd) {
	s := m.tree.Selected()
	_, ok := m.provider.(submodule.Restorer)
	var flash string
	switch {
	case s == nil:
		return m, nil
	case !ok:
		flash = "this data source can't restore"
	case m.restore != nil:
		flash = "wait for the restore of " + m.restore.name + " to finish"
	}
	if flash != "" {
		cmd := m.footer.Flash(flash)
		m.layout()
		return m, cmd
	}

	m.restore = &restoreState{path: s.Path, name: restoreName(s)}
	if !s.IsRoot() && s.Status != nil && s.Status.PinStaged {
		m.restore.phase = restoreChoosing
		m.layout()
		return m, nil
	}
	return m.checkRestore()
}

// checkRestore reads what the restore would discard.
func (m Model) checkRestore() (tea.Model, tea.Cmd) {
	m.restore.phase = restoreChecking
	m.layout()
	ctx, path, to, b := m.ctx, m.restore.path, m.restore.to, m.restore.branch
	check := func() tea.Msg {
		var loss submodule.Loss
		var err error
		if b != nil {
			loss, err = m.provider.(submodule.Switcher).SwitchLoss(ctx, path, *b)
		} else {
			loss, err = m.provider.(submodule.Restorer).RestoreLoss(ctx, path, to)
		}
		return lossMsg{path, loss, err}
	}
	return m, tea.Batch(check, m.tick())
}

// choosePin handles a key while the restore asks for the pin: s the staged
// one, c the committed one (which unstages it), anything else cancels.
func (m Model) choosePin(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		return m, tea.Quit
	case msg.String() == "s":
		m.restore.to = submodule.RestoreStaged
		return m.checkRestore()
	case msg.String() == "c":
		m.restore.to = submodule.RestoreCommitted
		return m.checkRestore()
	}
	m.restore = nil
	m.layout()
	return m, nil
}

// lossChecked reacts to the check: restore, or ask.
func (m Model) lossChecked(msg lossMsg) (tea.Model, tea.Cmd) {
	if m.restore == nil || m.restore.path != msg.path || m.restore.phase != restoreChecking {
		return m, nil
	}
	if msg.err != nil {
		m.restore = nil
		m.notice = "can't check " + restoreName(m.byPath[msg.path]) + ": " + msg.err.Error()
		m.layout()
		return m, nil
	}
	m.restore.loss = msg.loss
	if msg.loss.Empty() {
		return m.runRestore()
	}
	m.restore.phase = restoreAsking
	m.layout()
	return m, nil
}

// answerRestore handles a key while the restore waits for its answer: y
// restores, anything else cancels.
func (m Model) answerRestore(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		return m, tea.Quit
	case msg.String() == "y":
		return m.runRestore()
	}
	m.restore = nil
	m.layout()
	return m, nil
}

func (m Model) runRestore() (tea.Model, tea.Cmd) {
	m.restore.phase = restoreRunning
	m.layout()
	ctx, path, to, b := m.ctx, m.restore.path, m.restore.to, m.restore.branch
	run := func() tea.Msg {
		if b != nil {
			branch, err := m.provider.(submodule.Switcher).Switch(ctx, path, *b)
			return restoreDoneMsg{path, branch, err}
		}
		return restoreDoneMsg{path, "", m.provider.(submodule.Restorer).Restore(ctx, path, to)}
	}
	return m, tea.Batch(run, m.tick())
}

// restoreDone shows the result and reloads everything: the structure may have
// changed.
func (m Model) restoreDone(msg restoreDoneMsg) (tea.Model, tea.Cmd) {
	r := m.restore
	m.restore = nil
	var cmds []tea.Cmd
	switch {
	case r != nil && r.branch != nil && msg.err != nil:
		m.notice = "switch of " + r.name + " to " + r.branch.Name + " failed: " + msg.err.Error()
	case r != nil && r.branch != nil:
		to := r.branch.Name
		if msg.branch != "" {
			to = msg.branch
		} else if r.branch.Remote {
			to += " (detached: local " + r.branch.Local + " is elsewhere)"
		}
		cmds = append(cmds, m.footer.Flash("switched "+r.name+" to "+to))
	default:
		name := "the superproject"
		if r != nil {
			name = r.name + r.toText()
		}
		if msg.err != nil {
			m.notice = "restore of " + name + " failed: " + msg.err.Error()
		} else {
			cmds = append(cmds, m.footer.Flash("restored "+name))
		}
	}
	cmds = append(cmds, m.reloadTree(true))
	m.layout()
	return m, tea.Batch(cmds...)
}

// toText is " to origin/feat" for a switch, " to the committed pin" if that was chosen; the staged pin is the
// usual target and needs no words.
func (r *restoreState) toText() string {
	if r.branch != nil {
		return " to " + r.branch.Name
	}
	if r.to == submodule.RestoreCommitted {
		return " to the committed pin"
	}
	return ""
}

// verb and verbIng word the operation: a switch is shown like a restore, but
// names its branch.
func (r *restoreState) verb() string {
	if r.branch != nil {
		return "Switch"
	}
	return "Restore"
}

func (r *restoreState) verbIng() string {
	if r.branch != nil {
		return "switching"
	}
	return "restoring"
}

func restoreName(s *submodule.Submodule) string {
	if s == nil || s.IsRoot() {
		return "the superproject"
	}
	return s.Path
}

// restoreView is the line above the footer while a restore is in progress.
func (m Model) restoreView() string {
	r := m.restore
	left, right := "", ""
	key := func(k string) string { return theme.Fg(m.theme.Text).Bold(true).Render(k) }
	muted := func(s string) string { return theme.Fg(m.theme.Muted).Render(s) }
	switch r.phase {
	case restoreChoosing:
		left = theme.Fg(m.theme.Warn).Render("Restore "+r.name+" to") + "  " +
			key("s") + muted(" staged pin · ") + key("c") + muted(" committed pin (unstages it)")
		right = muted("any other key cancels")
	case restoreChecking:
		left = theme.Fg(m.theme.Primary).Render(m.spinner.View()) + " " + theme.Fg(m.theme.Muted).Render("checking "+r.name+"…")
	case restoreRunning:
		left = theme.Fg(m.theme.Primary).Render(m.spinner.View()) + " " + theme.Fg(m.theme.Warn).Render(r.verbIng()+" "+r.name+r.toText()+"…")
	case restoreAsking:
		left = theme.Fg(m.theme.Warn).Render(r.verb()+" "+r.name+r.toText()+"?") + " " +
			theme.Fg(m.theme.Danger).Render("Discards "+lossText(r.loss)+".")
		right = theme.Fg(m.theme.Text).Bold(true).Render("y") + theme.Fg(m.theme.Muted).Render(" "+strings.ToLower(r.verb())+" · ") +
			theme.Fg(m.theme.Text).Bold(true).Render("n") + theme.Fg(m.theme.Muted).Render(" cancel")
	}
	return m.line(left, right)
}

// line lays out one line above the footer: left, right-aligned right. The
// left part is cut if both don't fit.
func (m Model) line(left, right string) string {
	w := m.width - 2
	if right != "" {
		left = ansi.Truncate(left, max(w-lipgloss.Width(right)-2, 1), "…")
	}
	gap := max(w-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(left + strings.Repeat(" ", gap) + right)
}

// noticeView is the line above the footer after a restore failed.
func (m Model) noticeView() string {
	return m.line(theme.Fg(m.theme.Danger).Render(m.notice), "")
}

// lossText words what a restore discards, e.g. "7 changed files in 3
// repositories, 2 commits on no branch, 1 untracked repository".
func lossText(l submodule.Loss) string {
	var parts []string
	if l.Files > 0 {
		s := plural(l.Files, "changed file", "changed files")
		if l.Modules > 1 {
			s += fmt.Sprintf(" in %d repositories", l.Modules)
		}
		parts = append(parts, s)
	}
	if l.Operations > 0 {
		parts = append(parts, plural(l.Operations, "operation in progress", "operations in progress"))
	}
	if l.Commits > 0 {
		parts = append(parts, plural(l.Commits, "commit on no branch", "commits on no branch"))
	}
	if l.Repositories > 0 {
		parts = append(parts, plural(l.Repositories, "untracked repository", "untracked repositories"))
	}
	if l.StagedPins > 0 {
		parts = append(parts, plural(l.StagedPins, "staged pin", "staged pins"))
	}
	if l.Unknown > 0 {
		parts = append(parts, plural(l.Unknown, "repository of unknown state", "repositories of unknown state"))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
