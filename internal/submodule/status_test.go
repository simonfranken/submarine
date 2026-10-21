package submodule

import (
	"slices"
	"testing"
)

func kinds(is []Issue) []IssueKind {
	var out []IssueKind
	for _, i := range is {
		out = append(out, i.Kind)
	}
	return out
}

func TestIssues(t *testing.T) {
	pushed := PinInfo{Pushed: true}
	tests := []struct {
		name string
		s    *Status
		want []IssueKind
	}{
		{"loading", nil, nil},
		{"clean", &Status{Pin: pushed}, nil},
		{"behind, staged and detached are quiet", &Status{Pin: PinInfo{Pushed: true, Behind: 3}, PinStaged: true, Checkout: CheckoutDetached}, nil},
		{"not initialized", &Status{Presence: NotInitialized}, []IssueKind{IssueMissing}},
		{"dir missing", &Status{Presence: DirMissing}, []IssueKind{IssueMissing}},
		{"untracked is changed", &Status{Worktree: WorktreeUntracked, Pin: pushed}, []IssueKind{IssueChanged}},
		{"danger first", &Status{Checkout: CheckoutDiffers, Worktree: WorktreeModified}, []IssueKind{IssueUnpushed, IssueOffPin, IssueChanged}},
		{"conflict", &Status{Worktree: WorktreeConflict, Pin: pushed}, []IssueKind{IssueConflict}},
	}
	for _, tt := range tests {
		if got := kinds(tt.s.Issues()); !slices.Equal(got, tt.want) {
			t.Errorf("%s: Issues() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestHiddenIssues(t *testing.T) {
	ok := &Status{Pin: PinInfo{Pushed: true}}
	dirty := &Status{Worktree: WorktreeModified, Pin: PinInfo{Pushed: true}}
	unpushed := &Status{}
	grandchild := &Submodule{Status: unpushed}
	child := &Submodule{Status: dirty, Children: []*Submodule{grandchild}}
	loading := &Submodule{}
	root := &Submodule{Status: ok, Children: []*Submodule{child, {Status: ok}, loading}}

	count, worst := root.HiddenIssues()
	if count != 2 || worst != LevelDanger {
		t.Errorf("HiddenIssues() = %d, %v; want 2, LevelDanger", count, worst)
	}
}
