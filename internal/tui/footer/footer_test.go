package footer

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"submarine/internal/tui/theme"
)

type testKeys struct{ highlight []key.Binding }

func bind(k, desc string) key.Binding { return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc)) }

func (testKeys) ShortHelp() []key.Binding  { return nil }
func (testKeys) FullHelp() [][]key.Binding { return nil }
func (testKeys) Navigation() []key.Binding {
	return []key.Binding{bind("j", "move"), bind("h", "fold")}
}
func (testKeys) Submodule() []key.Binding   { return nil }
func (testKeys) Global() []key.Binding      { return []key.Binding{bind("?", "help"), bind("Q", "quit")} }
func (k testKeys) Highlight() []key.Binding { return k.highlight }

func bar(w int, hl ...key.Binding) string {
	m := New(theme.Default(), testKeys{hl})
	m.SetWidth(w)
	return ansi.Strip(m.View())
}

func TestUpdateIsRightmost(t *testing.T) {
	got := strings.TrimRight(bar(80, bind("q", "quit and update to v1.2.3")), " ")
	if !strings.HasSuffix(got, "? help • Q quit   q quit and update to v1.2.3") {
		t.Errorf("order must be help, quit, quit and update:\n%q", got)
	}
	if w := lipgloss.Width(bar(80, bind("q", "quit and update to v1.2.3"))); w != 80 {
		t.Errorf("bar is %d wide, want 80", w)
	}
}

func TestNoUpdate(t *testing.T) {
	if got := bar(80); strings.Contains(got, "update") {
		t.Errorf("unexpected update hint:\n%q", got)
	}
}

func TestNavigationGivesWayToUpdate(t *testing.T) {
	got := bar(50, bind("q", "quit and update to v1.2.3"))
	if !strings.Contains(got, "q quit and update to v1.2.3") || !strings.Contains(got, "Q quit") {
		t.Errorf("the right group must be kept:\n%q", got)
	}
}
