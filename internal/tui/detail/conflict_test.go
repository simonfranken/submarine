package detail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"submarine/internal/submodule"
	"submarine/internal/tui/theme"
)

func TestPinConflictView(t *testing.T) {
	m := New(theme.Default(), nil)
	m.SetSize(80, 40)
	ours := &submodule.Commit{SHA: "88f9593", Subject: "Fix alpha"}
	theirs := &submodule.Commit{SHA: "05122d7", Subject: "Add feature"}
	st := &submodule.Status{
		Pinned: ours, CheckedOut: ours, Checkout: submodule.CheckoutDetached,
		Pin: submodule.PinInfo{Ref: "origin/main", Behind: 3, Pushed: true},
		PinConflict: &submodule.PinConflict{
			Ours:   submodule.ConflictSide{Name: "main", SHA: "88f9593aaa", Commit: ours, Pin: submodule.PinInfo{Ref: "origin/main", Behind: 3, Pushed: true}},
			Theirs: submodule.ConflictSide{Name: "feat", SHA: "05122d7bbb", Commit: theirs, Pin: submodule.PinInfo{Ref: "origin/feat"}},
			Drift:  submodule.Distance{Ahead: 1, Behind: 1}, MergeBase: &submodule.Commit{SHA: "cb4e95c", Subject: "Base"},
			Candidates: []submodule.Candidate{{SHA: "b8dfd28ccc", Commit: &submodule.Commit{SHA: "b8dfd28", Subject: "Merge feat"}, Ref: "origin/main", Behind: 1}},
		},
	}
	view := ansi.Strip(m.View(&submodule.Submodule{Name: "alpha", Path: "libs/alpha", Status: st}, nil))
	for _, want := range []string{
		"pin conflict main and feat both moved the pin",
		"main       88f9593 ours · origin/main ↓3",
		"feat       05122d7 theirs · unpushed",
		"feat         ● 05122d7 Add feature",
		"Has both   b8dfd28 origin/main ↓1 · Merge feat",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Ref ", "detached HEAD", "staged"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("view has %q during a pin conflict:\n%s", unwanted, view)
		}
	}

	st.PinConflict.Theirs.Commit = nil
	st.PinConflict.Candidates = nil
	view = ansi.Strip(m.View(&submodule.Submodule{Name: "alpha", Path: "libs/alpha", Status: st}, nil))
	if !strings.Contains(view, "feat       theirs · not fetched · f fetch") || strings.Contains(view, "Has both") {
		t.Errorf("theirs not fetched:\n%s", view)
	}
}
