package detail

import (
	"fmt"
	"testing"

	"submarine/internal/submodule"
)

func files(kind submodule.ChangeKind, n int) []submodule.FileChange {
	var fs []submodule.FileChange
	for i := range n {
		fs = append(fs, submodule.FileChange{Path: fmt.Sprintf("%d-%d", kind, i), Kind: kind})
	}
	return fs
}

func TestPickPreview(t *testing.T) {
	t.Run("fits", func(t *testing.T) {
		all := append(files(submodule.ChangeModified, 2), files(submodule.ChangeUntracked, 1)...)
		shown, hidden := pickPreview(all, 5)
		if len(shown) != 3 || len(hidden) != 0 {
			t.Errorf("shown %d, hidden %d; want 3, 0", len(shown), len(hidden))
		}
	})

	t.Run("one over the budget shows all", func(t *testing.T) {
		shown, hidden := pickPreview(files(submodule.ChangeModified, 6), 5)
		if len(shown) != 6 || len(hidden) != 0 {
			t.Errorf("shown %d, hidden %d; want 6, 0", len(shown), len(hidden))
		}
	})

	t.Run("every kind keeps a line", func(t *testing.T) {
		all := files(submodule.ChangeModified, 8)
		all = append(all, files(submodule.ChangeRemoved, 1)...)
		all = append(all, files(submodule.ChangeUntracked, 3)...)
		shown, hidden := pickPreview(all, 4)
		if len(shown) != 4 || len(hidden) != 8 {
			t.Fatalf("shown %d, hidden %d; want 4, 8", len(shown), len(hidden))
		}
		want := []string{"1-0", "1-1", "3-0", "5-0"} // sort order is kept
		for i, f := range shown {
			if f.Path != want[i] {
				t.Errorf("shown[%d] = %q, want %q (all: %+v)", i, f.Path, want[i], shown)
			}
		}
		if n := (&submodule.Changes{}).Counts(hidden); n[submodule.ChangeModified] != 6 || n[submodule.ChangeUntracked] != 2 {
			t.Errorf("hidden counts = %v", n)
		}
	})
}

func TestSplitPath(t *testing.T) {
	for _, tt := range []struct{ in, name, dir string }{
		{"README.md", "README.md", ""},
		{"src/a/b.go", "b.go", "src/a/"},
		{"tmp/", "tmp/", ""},
		{"src/tmp/", "tmp/", "src/"},
	} {
		if name, dir := splitPath(tt.in); name != tt.name || dir != tt.dir {
			t.Errorf("splitPath(%q) = %q, %q; want %q, %q", tt.in, name, dir, tt.name, tt.dir)
		}
	}
}
