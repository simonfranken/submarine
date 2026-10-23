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
