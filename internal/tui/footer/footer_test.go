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

func (testKeys) Navigation() []key.Binding {
	return []key.Binding{bind("j", "move"), bind("h", "fold")}
}
func (testKeys) Tools() []key.Binding {
	return []key.Binding{bind("!", "problems only"), bind("/", "filter"), bind("f", "fetch")}
}
func (testKeys) Submodule() []key.Binding   { return []key.Binding{bind("x", "restore to pin")} }
func (testKeys) Global() []key.Binding      { return []key.Binding{bind("?", "help"), bind("Q", "quit")} }
func (k testKeys) Highlight() []key.Binding { return k.highlight }
func (testKeys) HelpGroups() []Group {
	return []Group{
		{Title: "Move", Keys: []key.Binding{bind("j", "down"), bind("k", "up")}},
		{Title: "Selected submodule", Keys: []key.Binding{bind("x", "restore to pin")}},
		{Title: "App", Keys: []key.Binding{bind("?", "what words mean"), bind("Q", "quit")}},
	}
}
func (testKeys) LegendKeys() Group {
	return Group{Title: "App", Keys: []key.Binding{bind("esc", "close")}}
}

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

func TestToolsFollowNavigation(t *testing.T) {
	got := bar(80)
	if !strings.Contains(got, "h fold • ! problems only • / filter • f fetch") {
		t.Errorf("tools must follow the navigation keys:\n%q", got)
	}
}

func TestToolsGiveWayBeforeNavigation(t *testing.T) {
	got := bar(40)
	if strings.Contains(got, "f fetch") || !strings.Contains(got, "j move") {
		t.Errorf("tools must go before navigation:\n%q", got)
	}
}

func TestSubmoduleKeysGoFirst(t *testing.T) {
	m := New(theme.Default(), testKeys{})
	m.SetActions(true)
	m.SetWidth(80)
	got := ansi.Strip(m.View())
	if strings.Contains(got, "restore to pin") || !strings.Contains(got, "f fetch") {
		t.Errorf("submodule keys must go before the tools:\n%q", got)
	}
	m.SetWidth(120)
	if got := ansi.Strip(m.View()); !strings.Contains(got, "f fetch • x restore to pin") {
		t.Errorf("submodule keys follow the tools:\n%q", got)
	}
}

func TestHelpPages(t *testing.T) {
	m := New(theme.Default(), testKeys{})
	m.SetWidth(120)
	m.CycleHelp()
	keys := ansi.Strip(m.View())
	for _, want := range []string{"Move", "Selected submodule", "App", "x restore to pin", "? what words mean"} {
		if !strings.Contains(keys, want) {
			t.Errorf("key page lacks %q:\n%s", want, keys)
		}
	}
	m.CycleHelp()
	if !m.Legend() {
		t.Fatal("second page must be the legend")
	}
	legend := ansi.Strip(m.View())
	for _, want := range []string{"Status", "off pin", "Symbols", "Commits", "esc close"} {
		if !strings.Contains(legend, want) {
			t.Errorf("legend lacks %q:\n%s", want, legend)
		}
	}
	for _, l := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(l); w > 120 {
			t.Errorf("legend line is %d wide: %q", w, ansi.Strip(l))
		}
	}
	m.CycleHelp()
	if got := ansi.Strip(m.View()); !strings.Contains(got, "? help") {
		t.Errorf("third ? must close the help:\n%q", got)
	}
}

func TestHelpLeavesOutColumnsThatDontFit(t *testing.T) {
	m := New(theme.Default(), testKeys{})
	m.SetWidth(30)
	m.CycleHelp()
	got := ansi.Strip(m.View())
	if !strings.Contains(got, "App") || strings.Contains(got, "Selected") || !strings.Contains(got, "…") {
		t.Errorf("want App kept, the columns before it left out with an ellipsis:\n%s", got)
	}
	for _, l := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(l); w > 30 {
			t.Errorf("line is %d wide: %q", w, ansi.Strip(l))
		}
	}
}
