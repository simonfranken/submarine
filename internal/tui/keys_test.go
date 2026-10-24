package tui

import "testing"

func TestUpdateReadsPullOnTheSuperproject(t *testing.T) {
	k := defaultKeys()
	if got := k.forSelection(true).Update.Help().Desc; got != "pull (fast-forward)" {
		t.Errorf("superproject: u reads %q", got)
	}
	if got := k.forSelection(false).Update.Help().Desc; got != "update pin" {
		t.Errorf("submodule: u reads %q", got)
	}
	if got := k.Update.Help().Desc; got != "update pin" {
		t.Errorf("forSelection changed the key map: u reads %q", got)
	}
}
