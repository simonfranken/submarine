package git

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"submarine/internal/submodule"
)

// operation describes what is in progress in the repository at dir: the
// name of HEAD's side, the name of the side being applied, and the words
// for the operation ("merging feat"); all "" if nothing is in progress
// except ours, which is always set.
func operation(ctx context.Context, dir string) (ours, theirs, op string) {
	gitDir, _, err := gitDirs(dir)
	if err != nil {
		return "", "", ""
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(gitDir, name))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}

	// a rebase swaps the roles: HEAD is the branch rebased onto, the
	// commits applied are the user's branch
	for _, d := range []string{"rebase-merge", "rebase-apply"} {
		if fi, err := os.Stat(filepath.Join(gitDir, d)); err != nil || !fi.IsDir() {
			continue
		}
		theirs = strings.TrimPrefix(read(filepath.Join(d, "head-name")), "refs/heads/")
		if theirs == "" || theirs == "detached HEAD" {
			theirs = nameOf(ctx, dir, read(filepath.Join(d, "orig-head")))
		}
		ours = nameOf(ctx, dir, read(filepath.Join(d, "onto")))
		return ours, theirs, "rebasing " + theirs
	}

	if b, ok, _ := try(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD"); ok && b != "" {
		ours = b
	} else {
		ours = nameOf(ctx, dir, "HEAD")
	}
	for _, o := range []struct{ head, verb string }{
		{"MERGE_HEAD", "merging"},
		{"CHERRY_PICK_HEAD", "cherry-picking"},
		{"REVERT_HEAD", "reverting"},
	} {
		if sha := read(o.head); sha != "" {
			sha, _, _ = strings.Cut(sha, "\n") // an octopus merge lists several
			theirs = nameOf(ctx, dir, sha)
			return ours, theirs, o.verb + " " + theirs
		}
	}
	return ours, "", ""
}

// nameOf names a commit of the repository at dir after a local branch
// ("feat", "feat~2"), else a remote branch ("origin/feat"), else its short
// SHA.
func nameOf(ctx context.Context, dir, rev string) string {
	if rev == "" {
		return ""
	}
	for _, refs := range []string{"refs/heads/*", "refs/remotes/*"} {
		if n, ok, _ := try(ctx, dir, "name-rev", "--name-only", "--no-undefined", "--refs="+refs, rev); ok && n != "" {
			return strings.TrimPrefix(n, "remotes/")
		}
	}
	if sha, ok, _ := try(ctx, dir, "rev-parse", "--short", rev); ok {
		return sha
	}
	return rev
}

// conflictSides describes a pin conflict as far as the parent at superDir
// knows it: the names and SHAs of the sides.
func conflictSides(ctx context.Context, superDir string, st stages) *submodule.PinConflict {
	ours, theirs, _ := operation(ctx, superDir)
	short := func(sha string) string { return sha[:min(7, len(sha))] }
	c := &submodule.PinConflict{
		Ours:   submodule.ConflictSide{Name: cmp.Or(ours, short(st[2]), "ours"), SHA: st[2]},
		Theirs: submodule.ConflictSide{Name: cmp.Or(theirs, short(st[3]), "theirs"), SHA: st[3]},
	}
	if st[1] != "" {
		c.Base = &submodule.Commit{SHA: short(st[1])}
	}
	return c
}

// conflictState fills what the submodule knows about the sides of c: their
// commits and refs, how they compare, and the commits that contain both.
// A side that is not fetched keeps only its SHA. st is the status, already
// filled for its pin (ours, or theirs if ours removed the submodule) if that
// is fetched; base is the full SHA of the base, head and branch are the
// checkout.
func conflictState(ctx context.Context, in info, st *submodule.Status, base, head, branch string) error {
	c := st.PinConflict
	exists := func(sha string) bool {
		_, ok, err := try(ctx, in.dir, "cat-file", "-e", sha+"^{commit}")
		return err == nil && ok
	}
	if base != "" && exists(base) {
		cm, err := commit(ctx, in.dir, base)
		if err != nil {
			return err
		}
		c.Base = cm
	}
	for _, side := range []*submodule.ConflictSide{&c.Ours, &c.Theirs} {
		if side.Deleted() || !exists(side.SHA) {
			continue
		}
		if st.Pinned != nil && strings.HasPrefix(side.SHA, st.Pinned.SHA) {
			side.Commit, side.Pin = st.Pinned, st.Pin
			continue
		}
		scratch := &submodule.Status{}
		if err := pinState(ctx, in, scratch, side.SHA, head, branch, nil); err != nil {
			return err
		}
		side.Commit, side.Pin = scratch.Pinned, scratch.Pin
	}
	if c.Ours.Commit == nil || c.Theirs.Commit == nil {
		return nil
	}

	out, err := run(ctx, in.dir, "rev-list", "--left-right", "--count", c.Theirs.SHA+"..."+c.Ours.SHA)
	if err != nil {
		return err
	}
	if c.Drift.Ahead, c.Drift.Behind, err = parseLeftRight(out); err != nil {
		return err
	}
	if c.Drift.Ahead > 0 && c.Drift.Behind > 0 {
		if c.MergeBase, err = mergeBase(ctx, in.dir, c.Ours.SHA, c.Theirs.SHA); err != nil {
			return err
		}
	}
	c.Candidates, err = candidates(ctx, in.dir, c, head)
	return err
}

// candidates lists the commits that contain both sides of c: a side that
// contains the other, else for each remote branch with both the oldest
// commit on it that has both, closest to its tip first; then the checkout,
// if it has both (e.g. after a merge in the submodule).
func candidates(ctx context.Context, dir string, c *submodule.PinConflict, head string) ([]submodule.Candidate, error) {
	ours, theirs := c.Ours.SHA, c.Theirs.SHA
	switch {
	case c.Drift.Ahead == 0:
		return []submodule.Candidate{{SHA: ours, Commit: c.Ours.Commit, Side: "ours"}}, nil
	case c.Drift.Behind == 0:
		return []submodule.Candidate{{SHA: theirs, Commit: c.Theirs.Commit, Side: "theirs"}}, nil
	}

	withOurs, err := remoteContaining(ctx, dir, ours)
	if err != nil {
		return nil, err
	}
	withTheirs, err := remoteContaining(ctx, dir, theirs)
	if err != nil {
		return nil, err
	}
	var out []submodule.Candidate
	seen := map[string]int{} // SHA → index in out
	compared := 0
	for _, b := range withOurs {
		if !slices.Contains(withTheirs, b) {
			continue
		}
		if compared++; compared > maxPinBranches {
			break
		}
		sha, err := oldestWithBoth(ctx, dir, ours, theirs, b)
		if err != nil {
			return nil, err
		}
		if sha == "" {
			continue
		}
		n, err := run(ctx, dir, "rev-list", "--count", sha+".."+b)
		if err != nil {
			return nil, err
		}
		var behind int
		fmt.Sscan(n, &behind)
		if i, ok := seen[sha]; ok {
			if behind < out[i].Behind || behind == out[i].Behind && b < out[i].Ref {
				out[i].Ref, out[i].Behind = b, behind
			}
			continue
		}
		cm, err := commit(ctx, dir, sha)
		if err != nil {
			return nil, err
		}
		seen[sha] = len(out)
		out = append(out, submodule.Candidate{SHA: sha, Commit: cm, Ref: b, Behind: behind})
	}
	slices.SortStableFunc(out, func(a, b submodule.Candidate) int {
		return cmp.Or(cmp.Compare(a.Behind, b.Behind), strings.Compare(a.Ref, b.Ref))
	})

	if head != "" && head != ours && head != theirs {
		if _, ok := seen[head]; !ok && isAncestor(ctx, dir, ours, head) && isAncestor(ctx, dir, theirs, head) {
			cm, err := commit(ctx, dir, head)
			if err != nil {
				return nil, err
			}
			out = append(out, submodule.Candidate{SHA: head, Commit: cm, Checkout: true})
		}
	}
	return out, nil
}

// oldestWithBoth returns the oldest commit on branch b that contains both
// ours and theirs: the first one, from the oldest, that descends from both.
func oldestWithBoth(ctx context.Context, dir, ours, theirs, b string) (string, error) {
	fromTheirs, err := run(ctx, dir, "rev-list", "--ancestry-path", theirs+".."+b)
	if err != nil {
		return "", err
	}
	desc := map[string]bool{}
	for _, sha := range strings.Fields(fromTheirs) {
		desc[sha] = true
	}
	fromOurs, err := run(ctx, dir, "rev-list", "--topo-order", "--reverse", "--ancestry-path", ours+".."+b)
	if err != nil {
		return "", err
	}
	for _, sha := range strings.Fields(fromOurs) {
		if desc[sha] {
			return sha, nil
		}
	}
	return "", nil
}

// isAncestor reports whether a is an ancestor of b (or b itself).
func isAncestor(ctx context.Context, dir, a, b string) bool {
	_, ok, err := try(ctx, dir, "merge-base", "--is-ancestor", a, b)
	return err == nil && ok
}
