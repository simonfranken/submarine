// Command submarine is a TUI for repositories with many git submodules.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"submarine/internal/submodule"
	"submarine/internal/submodule/git"
	"submarine/internal/submodule/mock"
	"submarine/internal/tui"
	"submarine/internal/update"
)

// version is set at build time (-ldflags "-X main.version=v1.2.3").
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "update":
			os.Exit(runUpdate(os.Args[2:]))
		case "version", "--version", "-version":
			fmt.Println("submarine", version)
			return
		}
	}

	useMock := flag.Bool("mock", false, "use built-in mock data instead of a real repository")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: submarine [--mock] [dir]\n       submarine update [--check] [--force]\n       submarine version")
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

	notice := checkForUpdate(ctx, *useMock)

	p := tea.NewProgram(tui.New(ctx, provider), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	select {
	case v := <-notice:
		if v != "" {
			fmt.Fprintf(os.Stderr, "submarine %s is available (you have %s), run: submarine update\n", v, version)
		}
	default:
	}
}

// checkForUpdate looks for a newer release in the background (at most once a
// day, cached) and delivers its version, or "" when there is none. Disabled
// for dev builds, --mock and SUBMARINE_NO_UPDATE_CHECK.
func checkForUpdate(ctx context.Context, off bool) <-chan string {
	out := make(chan string, 1)
	cache, err := os.UserCacheDir()
	if off || err != nil || !update.Release(version) || os.Getenv("SUBMARINE_NO_UPDATE_CHECK") != "" {
		out <- ""
		return out
	}
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		v, err := update.New().LatestCached(ctx, filepath.Join(cache, "submarine", "latest-version"))
		if err != nil || !update.Newer(version, v) {
			v = ""
		}
		out <- v
	}()
	return out
}

func runUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "only report whether a newer version exists")
	force := fs.Bool("force", false, "install the latest version even if not newer (also for dev builds)")
	fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c := update.New()
	latest, err := c.Latest(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "submarine: update check failed:", err)
		return 1
	}
	if !*force && !update.Newer(version, latest) {
		fmt.Printf("submarine %s is up to date\n", version)
		return 0
	}
	if *check {
		fmt.Printf("update available: %s (you have %s)\n", latest, version)
		return 0
	}
	if !update.Release(version) && !*force {
		fmt.Fprintf(os.Stderr, "submarine: this is a %q build; use --force to replace it with %s\n", version, latest)
		return 1
	}
	exe, err := c.Install(ctx, latest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "submarine: update failed:", err)
		return 1
	}
	fmt.Printf("updated %s: %s -> %s\n", exe, version, latest)
	return 0
}
