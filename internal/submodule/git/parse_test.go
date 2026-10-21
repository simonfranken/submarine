package git

import (
	"slices"
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

func TestParseWorktree(t *testing.T) {
	tests := []struct {
		out  string
		want submodule.Worktree
	}{
		{"", submodule.WorktreeClean},
		{"? new.txt", submodule.WorktreeUntracked},
		{"? new.txt\n1 .M N... 100644 100644 100644 a b README", submodule.WorktreeModified},
		{"2 R. N... 100644 100644 100644 a b R100 new\told", submodule.WorktreeModified},
		{"1 .M N... 100644 100644 100644 a b README\nu UU N... 1 2 3 4 a b c README", submodule.WorktreeConflict},
	}
	for _, tt := range tests {
		if got := parseWorktree(tt.out); got != tt.want {
			t.Errorf("parseWorktree(%q) = %v, want %v", tt.out, got, tt.want)
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
