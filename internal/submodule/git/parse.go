package git

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"submarine/internal/submodule"
)

// gitmodule is one entry of a .gitmodules file.
type gitmodule struct {
	name, path, url, branch string
}

// parseGitmodules parses the output of
// `git config -f .gitmodules --get-regexp '^submodule\..*\.(path|url|branch)$'`.
// Entries keep the order of first appearance; entries without a path are
// dropped.
func parseGitmodules(out string) []gitmodule {
	var order []string
	byName := map[string]*gitmodule{}
	for _, line := range strings.Split(out, "\n") {
		key, value, _ := strings.Cut(line, " ")
		key, ok := strings.CutPrefix(key, "submodule.")
		if !ok {
			continue
		}
		dot := strings.LastIndexByte(key, '.')
		if dot < 0 {
			continue
		}
		name, attr := key[:dot], key[dot+1:]
		g := byName[name]
		if g == nil {
			g = &gitmodule{name: name}
			byName[name] = g
			order = append(order, name)
		}
		switch attr {
		case "path":
			g.path = value
		case "url":
			g.url = value
		case "branch":
			g.branch = value
		}
	}
	var mods []gitmodule
	for _, name := range order {
		if g := byName[name]; g.path != "" {
			mods = append(mods, *g)
		}
	}
	return mods
}

// entry is one changed path of `git status --porcelain=v2 -z`.
type entry struct {
	path, orig string
	kind       submodule.ChangeKind
	tracked    bool // false for untracked files
	submodule  bool // a nested submodule pointer changed
}

// parseStatus parses `git status --porcelain=v2 -z`. Entries that don't
// differ from HEAD (e.g. added to the index, then deleted again) are
// dropped.
func parseStatus(out string) []entry {
	var es []entry
	recs := strings.Split(out, "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if len(rec) < 3 {
			continue
		}
		switch rec[0] {
		case '?':
			es = append(es, entry{path: rec[2:], kind: submodule.ChangeUntracked})
		case 'u': // u XY sub m1 m2 m3 mW h1 h2 h3 path
			if f := strings.SplitN(rec, " ", 11); len(f) == 11 {
				es = append(es, entry{path: f[10], kind: submodule.ChangeConflicted, tracked: true})
			}
		case '1': // 1 XY sub mH mI mW hH hI path
			if f := strings.SplitN(rec, " ", 9); len(f) == 9 {
				if e, ok := trackedEntry(f[1], f[2], f[8], ""); ok {
					es = append(es, e)
				}
			}
		case '2': // 2 XY sub mH mI mW hH hI Xscore path NUL origPath
			if f := strings.SplitN(rec, " ", 10); len(f) == 10 && i+1 < len(recs) {
				i++
				if e, ok := trackedEntry(f[1], f[2], f[9], recs[i]); ok {
					es = append(es, e)
				}
			}
		}
	}
	return es
}

func trackedEntry(xy, sub, path, orig string) (entry, bool) {
	if len(xy) != 2 {
		return entry{}, false
	}
	x, y := xy[0], xy[1]
	e := entry{path: path, orig: orig, tracked: true, submodule: strings.HasPrefix(sub, "S")}
	switch {
	case x == 'R' || y == 'R':
		e.kind = submodule.ChangeRenamed
	case x == 'A' && y == 'D':
		return entry{}, false
	case x == 'A' || y == 'A' || x == 'C' || y == 'C':
		e.kind = submodule.ChangeAdded
	case x == 'D' || y == 'D':
		e.kind = submodule.ChangeRemoved
	default:
		e.kind = submodule.ChangeModified
	}
	return e, true
}

// worktreeOf summarizes entries: a conflict outranks tracked changes, which
// outrank untracked files.
func worktreeOf(es []entry) submodule.Worktree {
	w := submodule.WorktreeClean
	for _, e := range es {
		switch {
		case e.kind == submodule.ChangeConflicted:
			return submodule.WorktreeConflict
		case e.tracked:
			w = submodule.WorktreeModified
		case w == submodule.WorktreeClean:
			w = submodule.WorktreeUntracked
		}
	}
	return w
}

// numstat holds the line counts of one file.
type numstat struct {
	added, removed int
	binary         bool
}

// parseNumstat parses `git diff --numstat -z -M`, keyed by the (new) path.
func parseNumstat(out string) map[string]numstat {
	m := map[string]numstat{}
	recs := strings.Split(out, "\x00")
	for i := 0; i < len(recs); i++ {
		f := strings.SplitN(recs[i], "\t", 3)
		if len(f) != 3 {
			continue
		}
		path := f[2]
		if path == "" { // rename: the next two records are old and new path
			if i+2 >= len(recs) {
				break
			}
			path = recs[i+2]
			i += 2
		}
		n := numstat{binary: f[0] == "-"}
		n.added, _ = strconv.Atoi(f[0])
		n.removed, _ = strconv.Atoi(f[1])
		m[path] = n
	}
	return m
}

// parseLeftRight parses `git rev-list --left-right --count a...b`.
func parseLeftRight(out string) (left, right int, err error) {
	f := strings.Fields(out)
	if len(f) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output %q", out)
	}
	if left, err = strconv.Atoi(f[0]); err != nil {
		return 0, 0, err
	}
	if right, err = strconv.Atoi(f[1]); err != nil {
		return 0, 0, err
	}
	return left, right, nil
}

// commitFormat is the `git log --format` matching parseCommits: fields
// separated by US, records terminated by RS.
const commitFormat = "--format=%h%x1f%s%x1f%an%x1f%ct%x1e"

// parseCommits parses `git log` output produced with commitFormat.
func parseCommits(out string) []submodule.Commit {
	var cs []submodule.Commit
	for _, rec := range strings.Split(out, "\x1e") {
		f := strings.Split(strings.TrimLeft(rec, "\n"), "\x1f")
		if len(f) != 4 {
			continue
		}
		c := submodule.Commit{SHA: f[0], Subject: f[1], Author: f[2]}
		if sec, err := strconv.ParseInt(f[3], 10, 64); err == nil {
			c.When = time.Unix(sec, 0)
		}
		cs = append(cs, c)
	}
	return cs
}

// parseGitlink returns the SHA of a gitlink (mode 160000) entry in
// `git ls-files -s` or `git ls-tree` output, or "" if there is none.
func parseGitlink(out string) string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) >= 3 && f[0] == "160000" && f[1] == "commit": // ls-tree
			return f[2]
		case len(f) >= 2 && f[0] == "160000": // ls-files -s
			return f[1]
		}
	}
	return ""
}
