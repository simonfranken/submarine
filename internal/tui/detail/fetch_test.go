package detail

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

func TestFetchHintForOldFetches(t *testing.T) {
	m := New(theme.Default(), nil)
	m.SetSize(60, 20)
	for _, c := range []struct {
		fetched time.Time
		hint    bool
	}{
		{time.Time{}, true},
		{time.Now().Add(-48 * time.Hour), true},
		{time.Now().Add(-time.Hour), false},
	} {
		s := &submodule.Submodule{Path: "src/app", Name: "app", Status: &submodule.Status{
			Presence: submodule.PresenceOK, FetchedAt: c.fetched, Pin: submodule.PinInfo{Pushed: true},
		}}
		got := ansi.Strip(m.View(s, nil))
		if strings.Contains(got, "f fetch") != c.hint {
			t.Errorf("fetched %s: hint %v, want %v:\n%s", theme.Ago(c.fetched), !c.hint, c.hint, got)
		}
		if !strings.Contains(got, "fetched "+theme.Ago(c.fetched)) {
			t.Errorf("the age is missing:\n%s", got)
		}
	}

	// too narrow for the hint: the age alone
	m.SetSize(24, 20)
	s := &submodule.Submodule{Path: "src/app", Name: "app", Status: &submodule.Status{Presence: submodule.PresenceOK}}
	if got := ansi.Strip(m.View(s, nil)); strings.Contains(got, "f fetch") || !strings.Contains(got, "fetched never") {
		t.Errorf("narrow pane:\n%s", got)
	}
}
