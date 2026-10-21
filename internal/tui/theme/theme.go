// Package theme holds the colors, glyphs and shared styles for the TUI.
package theme

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Theme uses the 16 ANSI colors for everything that carries meaning, so
// the TUI follows the user's terminal color scheme. Only background shades,
// which have no ANSI equivalent, use adaptive 256-color values.
type Theme struct {
	Primary  lipgloss.TerminalColor // accent: branch names, commit hashes
	Info     lipgloss.TerminalColor
	Warn     lipgloss.TerminalColor
	Danger   lipgloss.TerminalColor
	Muted    lipgloss.TerminalColor
	Text     lipgloss.TerminalColor
	Border   lipgloss.TerminalColor // pane separators, help separators
	Bar      lipgloss.TerminalColor // background of the header bar
	Selected lipgloss.TerminalColor // background of the selected row
	Glyphs   Glyphs
}

func Default() Theme {
	g := Unicode()
	if os.Getenv("SUBMARINE_ASCII") != "" {
		g = ASCII()
	}
	return Theme{
		Primary:  lipgloss.Color("6"), // cyan
		Info:     lipgloss.Color("4"), // blue
		Warn:     lipgloss.Color("3"), // yellow
		Danger:   lipgloss.Color("1"), // red
		Muted:    lipgloss.Color("8"), // bright black / grey
		Text:     lipgloss.NoColor{},  // terminal default foreground
		Border:   lipgloss.Color("8"),
		Bar:      lipgloss.AdaptiveColor{Light: "254", Dark: "235"},
		Selected: lipgloss.AdaptiveColor{Light: "253", Dark: "237"},
		Glyphs:   g,
	}
}

// Fg returns a style with the given foreground color.
func Fg(c lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

// Glyphs are the few symbols used next to the state words.
type Glyphs struct {
	Staged, Ahead, Behind, Collapsed, Expanded, Problems string
}

func Unicode() Glyphs {
	return Glyphs{
		Staged: "●", Ahead: "↑", Behind: "↓", Collapsed: "▸", Expanded: "▾", Problems: "·",
	}
}

func ASCII() Glyphs {
	return Glyphs{
		Staged: "*", Ahead: "^", Behind: "v", Collapsed: ">", Expanded: "v", Problems: ".",
	}
}
