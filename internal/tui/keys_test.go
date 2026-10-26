package tui

import "testing"

func TestUpdateReadsPullOnTheSuperproject(t *testing.T) {
	k := defaultKeys()
	if got := k.forSelection(true, false).Update.Help().Desc; got != "pull (fast-forward)" {
		t.Errorf("superproject: u reads %q", got)
	}
	if got := k.forSelection(false, false).Update.Help().Desc; got != "update pin" {
		t.Errorf("submodule: u reads %q", got)
	}
	if got := k.Update.Help().Desc; got != "update pin" {
		t.Errorf("forSelection changed the key map: u reads %q", got)
	}
}

func TestResolveLeadsThePaletteOnlyInAConflict(t *testing.T) {
	k := defaultKeys()
	if keys := k.forSelection(false, true).Submodule(); keys[0].Help().Key != "m" {
		t.Errorf("conflict: palette starts with %q", keys[0].Help().Key)
	}
	for _, b := range k.forSelection(false, false).Submodule() {
		if b.Help().Key == "m" {
			t.Error("no conflict: palette offers m")
		}
	}
}

func TestHelpListsResolveOnce(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		n := 0
		for _, b := range defaultKeys().forSelection(false, conflict).helpSubmodule() {
			if b.Help().Key == "m" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("conflict %v: m listed %d times", conflict, n)
		}
	}
}
