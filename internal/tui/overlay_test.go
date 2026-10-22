package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/charmbracelet/x/ansi"
)

func TestOverlayCentersThePopup(t *testing.T) {
	bg := strings.Repeat(strings.Repeat(".", 10)+"\n", 4) + strings.Repeat(".", 10)
	got := strings.Split(ansi.Strip(overlay(bg, "ab\ncd", 4, 1, lipgloss.NewStyle())), "\n")
	want := []string{"..........", "....ab....", "....cd....", "..........", ".........."}
	if len(got) != len(want) {
		t.Fatalf("got %d lines", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestOverlayDimsTheBackdrop(t *testing.T) {
	bg := "\x1b[31mredredred\x1b[0m"
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	got := overlay(bg, "X", 4, 0, dim)
	if ansi.StringWidth(got) != 9 || ansi.Strip(got) != "redrXdred" {
		t.Errorf("got %q", ansi.Strip(got))
	}
	if strings.Contains(got, "\x1b[31m") {
		t.Errorf("the backdrop keeps its colors: %q", got)
	}
}
