package theme

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/lipgloss"
)

// NewHelp returns a help renderer in the theme's colors.
func NewHelp(t Theme) help.Model {
	h := help.New()
	h.Styles.ShortKey = Fg(t.Text).Bold(true)
	h.Styles.ShortDesc = Fg(t.Muted)
	h.Styles.ShortSeparator = Fg(t.Border)
	h.Styles.FullKey = h.Styles.ShortKey
	h.Styles.FullDesc = h.Styles.ShortDesc
	h.Styles.FullSeparator = h.Styles.ShortSeparator
	return h
}

// Highlight is the style of a key hint that should catch the eye: dark text
// on the accent color, on the whole hint including its key.
func (t Theme) Highlight() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(t.Primary).Bold(true)
}
