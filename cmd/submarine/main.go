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
	"submarine/internal/submodule/git"
	"submarine/internal/submodule/mock"
	"submarine/internal/tui"
)

func main() {
	useMock := flag.Bool("mock", false, "use built-in mock data instead of a real repository")
	noFetch := flag.Bool("no-fetch", false, "don't fetch remotes in the background on startup")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: submarine [--mock] [--no-fetch] [dir]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	var provider submodule.Provider
	if *useMock {
		provider = mock.New()
	} else {
		dir := "."
		if flag.NArg() == 1 {
			dir = flag.Arg(0)
		}
		p, err := git.New(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "submarine:", err)
			os.Exit(2)
		}
		provider = p
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Detect the background (for adaptive colors) before Bubble Tea takes
	// over stdin; the result is cached by lipgloss.
	lipgloss.HasDarkBackground()

	zone.NewGlobal()
	defer zone.Close()

	p := tea.NewProgram(tui.New(ctx, provider, tui.Options{Fetch: !*noFetch}), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
