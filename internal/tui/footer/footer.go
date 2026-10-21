// Package footer renders key hints and short-lived status messages.
package footer

import (
	"time"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"submarine/internal/tui/theme"
)

const flashDuration = 2 * time.Second

type clearFlashMsg struct{ id int }

type Model struct {
	theme   theme.Theme
	help    help.Model
	keys    help.KeyMap
	width   int
	flash   string
	flashID int
}

func New(t theme.Theme, keys help.KeyMap) Model {
	h := help.New()
	h.Styles.ShortKey = theme.Fg(t.Text).Bold(true)
	h.Styles.ShortDesc = theme.Fg(t.Muted)
	h.Styles.ShortSeparator = theme.Fg(t.Border)
	h.Styles.FullKey = h.Styles.ShortKey
	h.Styles.FullDesc = h.Styles.ShortDesc
	h.Styles.FullSeparator = h.Styles.ShortSeparator
	return Model{theme: t, help: h, keys: keys}
}

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
	return style.Render(m.help.View(m.keys))
}
