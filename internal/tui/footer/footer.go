// Package footer renders key hints and short-lived status messages.
package footer

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/tui/theme"
)

const flashDuration = 2 * time.Second

type clearFlashMsg struct{ id int }

// Keys provides the key groups. The short bar shows Navigation (and
// Submodule, if requested) on the left, Global and then Highlight (on a
// highlighted background) on the right; full help
// shows FullHelp.
type Keys interface {
	help.KeyMap
	Navigation() []key.Binding
	Submodule() []key.Binding
	Global() []key.Binding
	Highlight() []key.Binding
}

type Model struct {
	theme   theme.Theme
	help    help.Model
	keys    Keys
	width   int
	flash   string
	flashID int
	actions bool // show the submodule bindings in the short bar
}

func New(t theme.Theme, keys Keys) Model {
	return Model{theme: t, help: theme.NewHelp(t), keys: keys}
}

// SetKeys replaces the key groups, e.g. if bindings changed.
func (m *Model) SetKeys(k Keys) { m.keys = k }

// SetActions shows the submodule bindings in the short bar. Set it if the
// detail pane doesn't show them.
func (m *Model) SetActions(on bool) { m.actions = on }

func (m *Model) SetWidth(w int) {
	m.width = w
	m.help.Width = w - 2
}

// ToggleHelp switches between short and full key help.
func (m *Model) ToggleHelp() { m.help.ShowAll = !m.help.ShowAll }

// Flash shows msg for a short time instead of the key hints.
func (m *Model) Flash(msg string) tea.Cmd {
	m.flashID++
	m.flash = msg
	id := m.flashID
	return tea.Tick(flashDuration, func(time.Time) tea.Msg { return clearFlashMsg{id} })
}

func (m *Model) Update(msg tea.Msg) {
	if c, ok := msg.(clearFlashMsg); ok && c.id == m.flashID {
		m.flash = ""
	}
}

func (m Model) View() string {
	style := lipgloss.NewStyle().Width(m.width).Padding(0, 1)
	if m.flash != "" {
		return style.Foreground(m.theme.Warn).Render(m.flash)
	}
	if m.help.ShowAll {
		return style.Render(m.help.View(m.keys))
	}
	return style.Render(m.shortBar(m.width - 2))
}

// shortBar puts the global keys on the right and as many of the others on the
// left as fit; the submodule keys are dropped first.
func (m Model) shortBar(w int) string {
	h := m.help
	h.Width = 0
	right := h.ShortHelpView(m.keys.Global())
	if hl := m.keys.Highlight(); len(hl) > 0 {
		var parts []string
		for _, b := range hl {
			parts = append(parts, b.Help().Key+" "+b.Help().Desc)
		}
		right += "  " + m.theme.Highlight().Render(" "+strings.Join(parts, "  ")+" ")
	}
	left := m.keys.Navigation()
	if m.actions {
		left = append(left[:len(left):len(left)], m.keys.Submodule()...)
	}
	for ; ; left = left[:len(left)-1] {
		l := h.ShortHelpView(left)
		if gap := w - lipgloss.Width(l) - lipgloss.Width(right); gap >= 2 || len(left) == 0 {
			if gap < 0 { // not even the global keys fit
				return ansi.Truncate(right, w, "…")
			}
			return l + strings.Repeat(" ", gap) + right
		}
	}
}
