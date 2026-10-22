package submodule

import (
	"errors"
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
	dirty := &Status{Worktree: WorktreeModified, Pin: PinInfo{Pushed: true}, PinStaged: true}
	unpushed := &Status{}
	grandchild := &Submodule{Status: unpushed}
	child := &Submodule{Status: dirty, Children: []*Submodule{grandchild}}
	loading := &Submodule{}
	root := &Submodule{Status: ok, Children: []*Submodule{child, {Status: ok}, loading}}

	if got := root.HiddenIssues(); got != (Hidden{Warn: 1, Danger: 1, Staged: 1}) {
		t.Errorf("HiddenIssues() = %+v; want 1 warn, 1 danger, 1 staged", got)
	}
}

func TestUntrackedRepositoryWording(t *testing.T) {
	repo := FileChange{Path: "lib/x/", Kind: ChangeUntracked, Repo: true}
	file := FileChange{Path: "a.txt", Kind: ChangeUntracked}
	for _, c := range []struct {
		files []FileChange
		want  string
	}{
		{[]FileChange{repo}, "untracked repository"},
		{[]FileChange{repo, file}, "untracked files"},
		{[]FileChange{file}, "untracked files"},
	} {
		s := &Status{Worktree: WorktreeUntracked, Changes: &Changes{Total: len(c.files), Files: c.files}}
		got := s.Issues()
		if len(got) == 0 || got[len(got)-1].Detail != c.want {
			t.Errorf("%v: got %v, want %q", c.files, got, c.want)
		}
	}
}

func TestHiddenIssuesCountLoadErrors(t *testing.T) {
	bad := &Submodule{Err: errors.New("boom")}
	loading := &Submodule{}
	parent := &Submodule{Children: []*Submodule{bad, loading}}
	if got := parent.HiddenIssues(); got != (Hidden{Danger: 1}) {
		t.Errorf("got %+v; want 1 danger", got)
	}
}

func TestRootIssueWording(t *testing.T) {
	st := &Status{Root: true}
	got := st.Issues()
	if len(got) != 1 || got[0].Kind != IssueUnpushed || got[0].Level != LevelDanger || got[0].Detail != "HEAD not pushed to any remote" {
		t.Errorf("Issues() = %+v", got)
	}
	if !(&Submodule{}).IsRoot() || (&Submodule{Path: "a"}).IsRoot() {
		t.Error("IsRoot should be true exactly for the empty path")
	}
}
