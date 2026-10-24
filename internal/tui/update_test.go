package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"submarine/internal/submodule/mock"
)

func newUpdateModel(version string) tea.Model {
	check := func(context.Context) string { return version }
	var m tea.Model = New(context.Background(), mock.New(), WithUpdateCheck(check))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestUpdateCheckDeliversMessage(t *testing.T) {
	m := New(context.Background(), mock.New(), WithUpdateCheck(func(context.Context) string { return "v9.9.9" }))
	if m.lookForUpdate() == nil {
		t.Fatal("no update check was started")
	}
	if msg := m.lookForUpdate()(); msg != (updateMsg{"v9.9.9"}) {
		t.Errorf("got %v", msg)
	}
	if New(context.Background(), mock.New()).lookForUpdate() != nil {
		t.Error("an update check was started without the option")
	}
}

func TestQuitWithoutUpdate(t *testing.T) {
	m := newUpdateModel("")
	m, _ = m.Update(updateMsg{""})
	m, cmd := press(m, "q")
	if !isQuit(cmd) || m.(Model).UpdateTo() != "" {
		t.Errorf("q must quit without updating, got update to %q", m.(Model).UpdateTo())
	}
}

func TestQuitAndUpdate(t *testing.T) {
	m := newUpdateModel("v9.9.9")
	m, _ = m.Update(updateMsg{"v9.9.9"})
	if bar := m.(Model).footer.View(); !strings.Contains(bar, "q quit and update to v9.9.9") {
		t.Errorf("the footer does not offer the update:\n%s", bar)
	}
	m2, cmd := press(m, "q")
	if !isQuit(cmd) || m2.(Model).UpdateTo() != "v9.9.9" {
		t.Errorf("q must quit and update, got update to %q", m2.(Model).UpdateTo())
	}
	m2, cmd = press(m, "Q")
	if !isQuit(cmd) || m2.(Model).UpdateTo() != "" {
		t.Errorf("Q must quit without updating, got update to %q", m2.(Model).UpdateTo())
	}
	m2, cmd = pressKey(m, tea.KeyCtrlC)
	if !isQuit(cmd) || m2.(Model).UpdateTo() != "" {
		t.Errorf("ctrl+c must quit without updating, got update to %q", m2.(Model).UpdateTo())
	}
}

func TestHintUntilTheFirstKey(t *testing.T) {
	m := newUpdateModel("")
	if !m.(Model).extraLine() || !strings.Contains(m.View(), "new here?") {
		t.Fatal("no hint on start")
	}
	m, _ = press(m, "j")
	if m.(Model).extraLine() || strings.Contains(m.View(), "new here?") {
		t.Error("the hint must go away with the first key")
	}
}

func TestEscClosesTheHelp(t *testing.T) {
	for _, presses := range []int{1, 2} { // the keys page, the legend
		m := newUpdateModel("")
		for range presses {
			m, _ = press(m, "?")
		}
		if !m.(Model).footer.HelpOpen() {
			t.Fatalf("%d × ?: no help", presses)
		}
		m, _ = pressKey(m, tea.KeyEsc)
		if m.(Model).footer.HelpOpen() {
			t.Errorf("%d × ?: esc must close the help", presses)
		}
	}
}

func TestEscClosesTheHelpBeforeTheFilter(t *testing.T) {
	m := newUpdateModel("")
	m, _ = press(m, "/")
	m, _ = pressKey(m, tea.KeyEnter) // keys back to the tree, the filter stays
	m, _ = press(m, "?")
	m, _ = pressKey(m, tea.KeyEsc)
	if mm := m.(Model); mm.footer.HelpOpen() || !mm.filtering {
		t.Errorf("first esc: help open %v, filtering %v; want the help closed, the filter kept", mm.footer.HelpOpen(), mm.filtering)
	}
	m, _ = pressKey(m, tea.KeyEsc)
	if m.(Model).filtering {
		t.Error("second esc must close the filter")
	}
}
