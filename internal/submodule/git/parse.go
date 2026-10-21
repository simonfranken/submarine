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

// parseWorktree summarizes `git status --porcelain=v2` output. A conflict
// outranks modifications, which outrank untracked files.
func parseWorktree(out string) submodule.Worktree {
	w := submodule.WorktreeClean
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "u "):
			return submodule.WorktreeConflict
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			w = submodule.WorktreeModified
		case strings.HasPrefix(line, "? ") && w == submodule.WorktreeClean:
			w = submodule.WorktreeUntracked
		}
	}
	return w
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
