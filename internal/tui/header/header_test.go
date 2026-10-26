package header

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

func TestOperationReadyToCommit(t *testing.T) {
	for _, tt := range []struct {
		root  *submodule.Status
		ready bool
	}{
		{&submodule.Status{Root: true, Pin: submodule.PinInfo{Pushed: true}, Unmerged: 1}, false},
		{nil, false}, // still loading
		{&submodule.Status{Root: true, Pin: submodule.PinInfo{Pushed: true}}, true},
	} {
		repo := &submodule.Repo{Path: "~/p", Branch: "main", Operation: "merging feat",
			Root: &submodule.Submodule{Status: tt.root}}
		v := ansi.Strip(View(theme.Default(), repo, Fetch{}, "", 120, false))
		if !strings.Contains(v, "merging feat") {
			t.Errorf("no operation: %q", v)
		}
		if got := strings.Contains(v, "ready to commit"); got != tt.ready {
			t.Errorf("unmerged %+v: ready to commit %v, want %v: %q", tt.root, got, tt.ready, v)
		}
	}
}
