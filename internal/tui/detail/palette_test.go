package detail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"submarine/internal/tui/theme"
)

func TestPaletteWrapsInsteadOfCuttingKeys(t *testing.T) {
	keys := []key.Binding{
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy path")),
		key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "update pin")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "switch branch")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "restore")),
	}
	m := New(theme.Default(), keys)
	m.SetSize(36, 20)
	lines := m.palette()
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", lines)
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{"copy path", "update pin", "switch branch", "x restore"} {
		if !strings.Contains(all, want) {
			t.Errorf("palette lacks %q: %q", want, lines)
		}
	}
	for _, l := range lines {
		if w := lipgloss.Width(l); w > 36 {
			t.Errorf("line %q is %d wide", l, w)
		}
	}
	m.SetSize(120, 20)
	if lines := m.palette(); len(lines) != 1 {
		t.Errorf("wide pane: want 1 line, got %q", lines)
	}
}
