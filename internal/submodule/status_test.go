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
	conflict := &PinConflict{Ours: ConflictSide{Name: "main", SHA: "a"}, Theirs: ConflictSide{Name: "feat", SHA: "b"}}
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
		{"pushed, but not on the target", &Status{Pin: PinInfo{Pushed: true, Ref: "origin/feat", Target: "origin/main", Source: RefFromContains}}, []IssueKind{IssueOffTarget}},
		{"off target comes last", &Status{Checkout: CheckoutDiffers, Pin: PinInfo{Pushed: true, Ref: "origin/feat", Target: "origin/main"}}, []IssueKind{IssueOffPin, IssueOffTarget}},
		{"unpushed is not also off target", &Status{Pin: PinInfo{Ref: "origin/main", Target: "origin/main", Ahead: 1}}, []IssueKind{IssueUnpushed}},
		{"on the target, ahead of nothing else", &Status{Pin: PinInfo{Pushed: true, Ref: "origin/main", Target: "origin/main", Behind: 4}}, nil},
		{"no target, no judgement", &Status{Pin: PinInfo{Pushed: true, Ref: "origin/feat", Source: RefFromContains}}, nil},
		{"the superproject has no target", &Status{Root: true, Pin: PinInfo{Pushed: true, Ref: "origin/feat", Target: "origin/main"}}, nil},
		{"pin conflict hides what compares with the pin", &Status{PinConflict: conflict, Checkout: CheckoutDiffers, Pin: PinInfo{Ref: "origin/feat", Target: "origin/main"}}, []IssueKind{IssuePinConflict}},
		{"pin conflict keeps the checkout's issues", &Status{PinConflict: conflict, Worktree: WorktreeModified}, []IssueKind{IssuePinConflict, IssueChanged}},
		{"pin conflict on a missing submodule", &Status{PinConflict: conflict, Presence: NotInitialized}, []IssueKind{IssuePinConflict, IssueMissing}},
	}
	for _, tt := range tests {
		if got := kinds(tt.s.Issues()); !slices.Equal(got, tt.want) {
			t.Errorf("%s: Issues() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestConflictText(t *testing.T) {
	for _, c := range []struct {
		ours, theirs string
		want         string
	}{
		{"a", "b", "main and feat both moved the pin"},
		{"", "b", "main removed the submodule, feat moved its pin"},
		{"a", "", "feat removed the submodule, main moved its pin"},
	} {
		s := &Status{PinConflict: &PinConflict{Ours: ConflictSide{Name: "main", SHA: c.ours}, Theirs: ConflictSide{Name: "feat", SHA: c.theirs}}}
		if got := s.Issues()[0].Detail; got != c.want {
			t.Errorf("%q/%q: %q, want %q", c.ours, c.theirs, got, c.want)
		}
	}
}

func TestOffTargetText(t *testing.T) {
	s := &Status{Pin: PinInfo{Target: "origin/marco-dev", Ref: "origin/fix/x"}}
	for _, c := range []struct {
		branches []string
		want     string
	}{
		{nil, "pin not on origin/marco-dev, on origin/fix/x"},
		{[]string{"origin/main"}, "pin not on origin/marco-dev, on origin/fix/x +1"},
		{[]string{"origin/main", "origin/y"}, "pin not on origin/marco-dev, on origin/fix/x +2"},
	} {
		s.Pin.Branches = c.branches
		if got := s.offTargetText(); got != c.want {
			t.Errorf("%v: %q, want %q", c.branches, got, c.want)
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
