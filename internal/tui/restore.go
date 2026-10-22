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
	restoreChecking restorePhase = iota // reading what would be lost
	restoreAsking                       // waiting for y / n
	restoreRunning
)

// restoreState is the restore in progress; only one runs at a time.
type restoreState struct {
	path, name string // name is how the footer calls it
	phase      restorePhase
	loss       submodule.Loss
}

// lossMsg is the result of the check before a restore.
type lossMsg struct {
	path string
	loss submodule.Loss
	err  error
}

type restoreDoneMsg struct {
	path string
	err  error
}

// startRestore checks what restoring the selection would discard. If nothing
// would be lost it restores at once, else it asks first.
func (m Model) startRestore() (tea.Model, tea.Cmd) {
	s := m.tree.Selected()
	r, ok := m.provider.(submodule.Restorer)
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
	m.layout()
	ctx, path := m.ctx, s.Path
	check := func() tea.Msg {
		loss, err := r.RestoreLoss(ctx, path)
		return lossMsg{path, loss, err}
	}
	return m, tea.Batch(check, m.tick())
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
	r := m.provider.(submodule.Restorer)
	ctx, path := m.ctx, m.restore.path
	run := func() tea.Msg { return restoreDoneMsg{path, r.Restore(ctx, path)} }
	return m, tea.Batch(run, m.tick())
}

// restoreDone shows the result and reloads everything: the structure may have
// changed.
func (m Model) restoreDone(msg restoreDoneMsg) (tea.Model, tea.Cmd) {
	name := "the superproject"
	if m.restore != nil {
		name = m.restore.name
	}
	m.restore = nil
	var cmds []tea.Cmd
	if msg.err != nil {
		m.notice = "restore of " + name + " failed: " + msg.err.Error()
	} else {
		cmds = append(cmds, m.footer.Flash("restored "+name))
	}
	cmds = append(cmds, m.reloadTree(true))
	m.layout()
	return m, tea.Batch(cmds...)
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
	switch r.phase {
	case restoreChecking:
		left = theme.Fg(m.theme.Primary).Render(m.spinner.View()) + " " + theme.Fg(m.theme.Muted).Render("checking "+r.name+"…")
	case restoreRunning:
		left = theme.Fg(m.theme.Primary).Render(m.spinner.View()) + " " + theme.Fg(m.theme.Warn).Render("restoring "+r.name+"…")
	case restoreAsking:
		left = theme.Fg(m.theme.Warn).Render("Restore "+r.name+"?") + " " +
			theme.Fg(m.theme.Danger).Render("Discards "+lossText(r.loss)+".")
		right = theme.Fg(m.theme.Text).Bold(true).Render("y") + theme.Fg(m.theme.Muted).Render(" restore · ") +
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
