package theme

import "github.com/charmbracelet/bubbles/help"

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
