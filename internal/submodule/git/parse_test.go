package git

import (
	"slices"
	"strings"
	"testing"

	"submarine/internal/submodule"
)

func TestParseGitmodules(t *testing.T) {
	out := "submodule.src/core.path src/core\n" +
		"submodule.src/core.url ../core.git\n" +
		"submodule.lib.v2.path libs/v2\n" +
		"submodule.lib.v2.branch develop\n" +
		"submodule.lib.v2.url git@example.com:lib.git\n" +
		"submodule.nopath.url x\n"
	want := []gitmodule{
		{name: "src/core", path: "src/core", url: "../core.git"},
		{name: "lib.v2", path: "libs/v2", url: "git@example.com:lib.git", branch: "develop"},
	}
	if got := parseGitmodules(out); !slices.Equal(got, want) {
		t.Errorf("parseGitmodules() = %+v, want %+v", got, want)
	}
	if got := parseGitmodules(""); got != nil {
		t.Errorf("parseGitmodules(\"\") = %+v, want nil", got)
	}
}

func z(recs ...string) string { return strings.Join(recs, "\x00") + "\x00" }

func TestParseStatus(t *testing.T) {
	out := z(
		"1 .M N... 100644 100644 100644 a b src/my file.go",
		"1 A. N... 000000 100644 100644 a b new.go",
		"1 AD N... 000000 100644 000000 a b gone.go",
		"1 D. N... 100644 000000 000000 a b old.go",
		"1 .M S.M. 160000 160000 160000 a b libs/nested",
		"2 R. N... 100644 100644 100644 a b R100 dir/ü new.go", "dir/old.go",
		"u UU N... 100644 100644 100644 100644 a b c conflict.go",
		"? scratch dir/",
		"! ignored.o",
	)
	want := []entry{
		{path: "src/my file.go", kind: submodule.ChangeModified, tracked: true},
		{path: "new.go", kind: submodule.ChangeAdded, tracked: true},
		{path: "old.go", kind: submodule.ChangeRemoved, tracked: true},
		{path: "libs/nested", kind: submodule.ChangeModified, tracked: true, submodule: true},
		{path: "dir/ü new.go", orig: "dir/old.go", kind: submodule.ChangeRenamed, tracked: true},
		{path: "conflict.go", kind: submodule.ChangeConflicted, tracked: true},
		{path: "scratch dir/", kind: submodule.ChangeUntracked},
	}
	if got := parseStatus(out); !slices.Equal(got, want) {
		t.Errorf("parseStatus() =\n%+v\nwant\n%+v", got, want)
	}
	if got := parseStatus(""); got != nil {
		t.Errorf("parseStatus(\"\") = %+v, want nil", got)
	}
}

func TestWorktreeOf(t *testing.T) {
	untracked := entry{kind: submodule.ChangeUntracked}
	modified := entry{kind: submodule.ChangeModified, tracked: true}
	conflict := entry{kind: submodule.ChangeConflicted, tracked: true}
	tests := []struct {
		es   []entry
		want submodule.Worktree
	}{
		{nil, submodule.WorktreeClean},
		{[]entry{untracked}, submodule.WorktreeUntracked},
		{[]entry{untracked, modified}, submodule.WorktreeModified},
		{[]entry{modified, untracked, conflict}, submodule.WorktreeConflict},
	}
	for _, tt := range tests {
		if got := worktreeOf(tt.es); got != tt.want {
			t.Errorf("worktreeOf(%+v) = %v, want %v", tt.es, got, tt.want)
		}
	}
}

func TestParseNumstat(t *testing.T) {
	out := z("3\t1\tREADME.md", "-\t-\timg/logo.png", "0\t0\t", "old name.go", "new name.go", "5\t0\tsrc/a b.go")
	want := map[string]numstat{
		"README.md":    {added: 3, removed: 1},
		"img/logo.png": {binary: true},
		"new name.go":  {},
		"src/a b.go":   {added: 5},
	}
	got := parseNumstat(out)
	if len(got) != len(want) {
		t.Fatalf("parseNumstat() = %+v, want %+v", got, want)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("parseNumstat()[%q] = %+v, want %+v", k, got[k], w)
		}
	}
}

func TestParseLeftRight(t *testing.T) {
	if l, r, err := parseLeftRight("3\t1"); err != nil || l != 3 || r != 1 {
		t.Errorf("parseLeftRight = %d, %d, %v; want 3, 1, nil", l, r, err)
	}
	if _, _, err := parseLeftRight("3"); err == nil {
		t.Error("parseLeftRight(\"3\") should fail")
	}
}

func TestParseCommits(t *testing.T) {
	out := "abc1234\x1fFix it\x1fAda\x1f1700000000\x1e\n" +
		"def5678\x1fAdd\x1fBob\x1f1700000100\x1e\n" +
		"malformed\x1e\n"
	got := parseCommits(out)
	want := []string{"abc1234", "def5678"}
	if len(got) != len(want) {
		t.Fatalf("parseCommits() = %+v, want SHAs %v", got, want)
	}
	for i, c := range got {
		if c.SHA != want[i] {
			t.Errorf("commit %d: SHA %q, want %q", i, c.SHA, want[i])
		}
	}
	if got[0].Subject != "Fix it" || got[0].Author != "Ada" || got[0].When.Unix() != 1700000000 {
		t.Errorf("parseCommits()[0] = %+v", got[0])
	}
}

func TestParseGitlink(t *testing.T) {
	if got := parseGitlink("160000 0123abc 0\tsrc/core"); got != "0123abc" {
		t.Errorf("ls-files: got %q", got)
	}
	if got := parseGitlink("160000 commit 0123abc\tsrc/core"); got != "0123abc" {
		t.Errorf("ls-tree: got %q", got)
	}
	if got := parseGitlink("100644 0123abc 0\tREADME"); got != "" {
		t.Errorf("blob: got %q", got)
	}
}
