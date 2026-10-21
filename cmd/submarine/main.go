// Command submarine is a TUI for repositories with many git submodules.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"submarine/internal/submodule"
	"submarine/internal/submodule/mock"
	"submarine/internal/tui"
)

func main() {
	useMock := flag.Bool("mock", true, "use built-in mock data instead of a real repository")
	flag.Parse()

	var provider submodule.Provider
	if *useMock {
		provider = mock.New()
	} else {
		fmt.Fprintln(os.Stderr, "only --mock is supported in the prototype")
		os.Exit(2)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Detect the background (for adaptive colors) before Bubble Tea takes
	// over stdin; the result is cached by lipgloss.
	lipgloss.HasDarkBackground()

	zone.NewGlobal()
	defer zone.Close()

	p := tea.NewProgram(tui.New(ctx, provider), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
