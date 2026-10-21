package tui

import (
	"os"

	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
)

// copyToClipboard sets the system clipboard with the OSC 52 escape sequence,
// which works over SSH and in terminals that allow it; others ignore it.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		seq := osc52.New(text)
		if os.Getenv("TMUX") != "" {
			seq = seq.Tmux()
		}
		_, _ = seq.WriteTo(os.Stderr)
		return nil
	}
}
