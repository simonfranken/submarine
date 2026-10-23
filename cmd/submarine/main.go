// Command submarine is a TUI for repositories with many git submodules.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
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

	var opts []tui.Option
	if !*useMock && update.Release(version) && os.Getenv("SUBMARINE_NO_UPDATE_CHECK") == "" {
		opts = append(opts, tui.WithUpdateCheck(latestRelease))
	}

	p := tea.NewProgram(tui.New(ctx, provider, opts...), tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The user chose "quit and update": the terminal is restored by now.
	if m, ok := final.(tui.Model); ok && m.UpdateTo() != "" {
		fmt.Printf("updating to %s…\n", m.UpdateTo())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if code := install(ctx, update.New(), m.UpdateTo()); code != 0 {
			os.Exit(code)
		}
	}
}

// latestRelease returns the version of a newer release, or "" if there is
// none or the registry can't be reached within a few seconds.
func latestRelease(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	v, err := update.New().Latest(ctx)
	if err != nil || !update.Newer(version, v) {
		return ""
	}
	return v
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
	return install(ctx, c, latest)
}

// install replaces the running binary with release latest.
func install(ctx context.Context, c *update.Client, latest string) int {
	exe, err := c.Install(ctx, latest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "submarine: update failed:", err)
		return 1
	}
	fmt.Printf("updated %s: %s -> %s\n", exe, version, latest)
	return 0
}
