// Package mock provides a submodule.Provider backed by fixture data. It
// covers every state the UI can display and simulates loading latency.
package mock

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"path"
	"strings"
	"sync"
	"time"

	"submarine/internal/submodule"
)

// Provider serves a fixed fixture tree.
type Provider struct {
	MinDelay, MaxDelay time.Duration

	mu       sync.RWMutex // Tree can run again (reload) while Status runs
	statuses map[string]*submodule.Status
}

func New() *Provider {
	return &Provider{
		MinDelay: 100 * time.Millisecond,
		MaxDelay: 800 * time.Millisecond,
	}
}

type node struct {
	relPath  string
	status   *submodule.Status
	children []node
}

func (p *Provider) Tree(ctx context.Context) (*submodule.Repo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statuses = map[string]*submodule.Status{}
	repo := &submodule.Repo{
		Path:      "~/coding/my_project",
		Branch:    "main",
		Behind:    2,
		Operation: "merging feat/v2",
	}
	repo.Root = &submodule.Submodule{Name: "my_project", URL: "git@github.com:acme/my_project.git"}
	p.statuses[""] = rootStatus()
	repo.Submodules = p.build(fixture(), nil)
	return repo, nil
}

func (p *Provider) build(nodes []node, parent *submodule.Submodule) []*submodule.Submodule {
	var out []*submodule.Submodule
	for _, n := range nodes {
		full := n.relPath
		if parent != nil {
			full = path.Join(parent.Path, n.relPath)
		}
		m := &submodule.Submodule{
			Name:    path.Base(n.relPath),
			Path:    full,
			RelPath: n.relPath,
			URL:     fmt.Sprintf("git@github.com:acme/%s.git", path.Base(n.relPath)),
			Parent:  parent,
		}
		m.Children = p.build(n.children, m)
		if n.status.Presence == submodule.PresenceOK && n.status.FetchedAt.IsZero() {
			n.status.FetchedAt = now.Add(-2 * time.Minute)
		}
		p.statuses[full] = n.status
		out = append(out, m)
	}
	return out
}

func (p *Provider) Status(ctx context.Context, path string) (*submodule.Status, error) {
	if err := p.sleep(ctx, p.MinDelay, p.MaxDelay); err != nil {
		return nil, err
	}
	p.mu.RLock()
	s, ok := p.statuses[path]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown submodule %q", path)
	}
	return s, nil
}

// Fetchable is true for the superproject and initialized submodules.
func (p *Provider) Fetchable(path string) bool {
	if path == "" {
		return true
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	s, ok := p.statuses[path]
	return ok && s.Presence == submodule.PresenceOK
}

// Fetch pretends to fetch; one submodule always fails. It changes no data.
func (p *Provider) Fetch(ctx context.Context, path string) error {
	if err := p.sleep(ctx, 300*time.Millisecond, 2*time.Second); err != nil {
		return err
	}
	if path == "src/my_submodule_3" {
		return errors.New("git@github.com: Permission denied (publickey).")
	}
	return nil
}

// sleep waits a random duration in [min, max) or until ctx is done.
func (p *Provider) sleep(ctx context.Context, min, max time.Duration) error {
	d := min
	if span := max - min; span > 0 {
		d += rand.N(span)
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- fixture -----------------------------------------------------------------

var now = time.Now()

func commit(sha, subject, author string, ago time.Duration) *submodule.Commit {
	return &submodule.Commit{SHA: sha, Subject: subject, Author: author, When: now.Add(-ago)}
}

func inSync(ref string, c *submodule.Commit) *submodule.Status {
	pin := submodule.PinInfo{Ref: ref, Pushed: true}
	if strings.HasPrefix(ref, "origin/") {
		pin.Target = ref // on its target branch
	}
	return &submodule.Status{
		Pin:    pin,
		Pinned: c, CheckedOut: c, Tip: c,
	}
}

// changes builds a Changes value like the git provider: sorted, with
// totals. The last file was edited age ago, earlier ones increasingly
// longer ago.
func changes(age time.Duration, files []submodule.FileChange) *submodule.Changes {
	c := &submodule.Changes{Files: files, Total: len(files)}
	for i := range files {
		f := &files[i]
		if f.Kind != submodule.ChangeRemoved {
			n := time.Duration(len(files) - 1 - i)
			f.Modified = time.Now().Add(-age - n*n*90*time.Minute)
		}
		c.Added += f.Added
		c.Removed += f.Removed
	}
	c.Sort()
	return c
}

// rootStatus is a superproject that is behind its upstream and still has a
// clone of a submodule the current branch no longer has.
func rootStatus() *submodule.Status {
	tip := commit("e4f5a6b", "Move docs into a submodule", "anna", 3*time.Hour)
	head := commit("0a1b2c3", "Update README", "ben", 2*24*time.Hour)
	st := &submodule.Status{
		Root:       true,
		Worktree:   submodule.WorktreeModified,
		Pin:        submodule.PinInfo{Ref: "origin/main", Behind: 2, Pushed: true, Source: submodule.RefFromCurrentBranch},
		Pinned:     head,
		CheckedOut: head,
		Tip:        tip,
		FetchedAt:  now.Add(-2 * time.Minute),
	}
	st.Changes = changes(15*time.Minute, []submodule.FileChange{
		{Path: "README.md", Kind: submodule.ChangeModified, Added: 4, Removed: 1},
		{Path: "old_module/", Kind: submodule.ChangeUntracked, Repo: true},
	})
	return st
}

func fixture() []node {
	c1 := commit("3f9a1c2", "Bump golangci-lint to v2", "anna", 26*time.Hour)
	c2 := commit("a1b2c3d", "Initial import", "ben", 40*24*time.Hour)

	srcSub1Pin := commit("5d6e7f8", "Add retry to client", "chris", 3*24*time.Hour)
	srcSub1Head := commit("0c1d2e3", "WIP: experiment with backoff", "you", 2*time.Hour)

	srcSub2 := inSync("origin/main", commit("77aa88b", "Merge PR #42", "dana", 5*time.Hour))
	srcSub2.Worktree = submodule.WorktreeModified
	srcSub2.Changes = changes(20*time.Minute, []submodule.FileChange{
		{Path: "internal/client/retry.go", Kind: submodule.ChangeModified, Added: 84, Removed: 12},
		{Path: "internal/client/backoff.go", Kind: submodule.ChangeModified, Added: 41, Removed: 3},
		{Path: "internal/server/handler.go", Kind: submodule.ChangeModified, Added: 17, Removed: 17},
		{Path: "README.md", Kind: submodule.ChangeModified, Added: 6, Removed: 2},
		{Path: "go.mod", Kind: submodule.ChangeModified, Added: 2, Removed: 2},
		{Path: "go.sum", Kind: submodule.ChangeModified, Added: 9, Removed: 4},
		{Path: "Makefile", Kind: submodule.ChangeModified, Added: 1, Removed: 1},
		{Path: "internal/client/retry_test.go", Kind: submodule.ChangeAdded, Added: 63},
		{Path: "docs/design/backoff-notes.md", Kind: submodule.ChangeAdded, Added: 28},
		{Path: "internal/legacy/old_client.go", Kind: submodule.ChangeRemoved, Removed: 212},
		{Path: "assets/logo.png", Kind: submodule.ChangeModified, Binary: true},
		{Path: "internal/client/transport.go", OrigPath: "internal/net/transport.go", Kind: submodule.ChangeRenamed, Added: 5, Removed: 1},
		{Path: "scratch.txt", Kind: submodule.ChangeUntracked},
		{Path: "tmp/", Kind: submodule.ChangeUntracked},
		{Path: "profile.out", Kind: submodule.ChangeUntracked},
	})

	subSubPin := commit("4e5f6a7", "Add feature flag", "eve", 30*time.Hour)
	subSubTip := commit("9f8e7d6", "Fix login timeout", "anna", 2*time.Hour)
	subSub := &submodule.Status{
		Checkout:   submodule.CheckoutDiffers,
		Pin:        submodule.PinInfo{Ref: "origin/feat/02", Behind: 1, Pushed: true},
		Pinned:     subSubPin,
		CheckedOut: subSubTip,
		Tip:        subSubTip,
		Drift:      submodule.Distance{Ahead: 1},
	}

	schema := inSync("origin/main", commit("1a2b3c4", "Add v3 schema", "ben", 6*24*time.Hour))
	schema.Worktree = submodule.WorktreeUntracked
	schema.Changes = changes(40*time.Minute, []submodule.FileChange{
		{Path: "v3/user.schema.json", Kind: submodule.ChangeUntracked},
		{Path: "v3/order.schema.json", Kind: submodule.ChangeUntracked},
	})

	unpushed := commit("b7c8d9e", "Hotfix null pointer in parser", "you", 20*time.Minute)
	sub3Tip := commit("6f5e4d3", "Release 2.1.0", "chris", 4*24*time.Hour)

	logPin := commit("2b3c4d5", "Add structured fields", "dana", 9*24*time.Hour)
	logTip := commit("8e9f0a1", "Support slog handler", "eve", 1*24*time.Hour)

	httpPin := commit("c4d5e6f", "Experimental HTTP/3 support", "frank", 8*24*time.Hour)
	httpTip := commit("e1f2a3b", "Drop Go 1.21 support", "anna", 12*time.Hour)

	cryptoNew := commit("d2e3f4a", "Release v1.4.2", "ben", 3*24*time.Hour)
	crypto := inSync("v1.4.2", cryptoNew)
	crypto.Pin.Source = submodule.RefFromTag
	crypto.PinStaged = true
	crypto.Committed = commit("e0f1a2b", "Release v1.4.1", "ben", 40*24*time.Hour)
	crypto.CommittedDrift = submodule.Distance{Behind: 3}

	// libs/parser: both sides of the merge moved the pin; main has merged
	// the feature since
	parserOurs := commit("3c4d5e6", "Faster tokenizer", "dana", 2*24*time.Hour)
	parserTheirs := commit("7a8b9c0", "Parse v2 syntax", "eve", 3*24*time.Hour)
	parser := inSync("origin/main", parserOurs)
	parser.PinConflict = &submodule.PinConflict{
		Base: commit("1b2c3d4", "Release 2.0.0", "dana", 9*24*time.Hour),
		Ours: submodule.ConflictSide{Name: "main", SHA: "3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d", Commit: parserOurs,
			Pin: submodule.PinInfo{Ref: "origin/main", Target: "origin/main", Behind: 3, Pushed: true}},
		Theirs: submodule.ConflictSide{Name: "feat/v2", SHA: "7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b", Commit: parserTheirs,
			Pin: submodule.PinInfo{Ref: "origin/feat/v2", Target: "origin/main", Pushed: true, Source: submodule.RefFromContains}},
		Drift:     submodule.Distance{Ahead: 4, Behind: 2},
		MergeBase: commit("1b2c3d4", "Release 2.0.0", "dana", 9*24*time.Hour),
		Candidates: []submodule.Candidate{{
			SHA: "d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3", Commit: commit("d4e5f6a", "Merge feat/v2", "dana", 1*24*time.Hour),
			Ref: "origin/main", Behind: 2,
		}},
	}
	parser.Pin.Behind = 3

	zlib := inSync("v1.3.1", commit("51f2c3e", "zlib 1.3.1", "madler", 300*24*time.Hour))
	zlib.Pin.Source = submodule.RefFromTag
	zlib.Checkout = submodule.CheckoutDetached

	return []node{
		{relPath: "docs", status: inSync("origin/main", c2)},
		{relPath: "misc/tools/my_submodule_1", status: inSync("origin/main", c1)},
		{relPath: "misc/tools/my_submodule_2", status: &submodule.Status{
			Presence: submodule.NotInitialized,
			Pin:      submodule.PinInfo{Pushed: true, Source: submodule.RefFromSHA},
			Pinned:   &submodule.Commit{SHA: "a1b2c3d"},
		}},
		{relPath: "src/my_submodule_1", status: &submodule.Status{
			Checkout:   submodule.CheckoutDiffers,
			Pin:        submodule.PinInfo{Ref: "origin/main", Pushed: true},
			Pinned:     srcSub1Pin,
			CheckedOut: srcSub1Head,
			Tip:        srcSub1Pin,
			Drift:      submodule.Distance{Ahead: 2},
		}},
		{relPath: "src/my_submodule_2", status: srcSub2, children: []node{
			{relPath: "modules/my_sub_submodule", status: subSub},
			{relPath: "modules/protocol", status: inSync("origin/main", commit("aa11bb2", "Regenerate stubs", "frank", 50*time.Hour)), children: []node{
				{relPath: "vendor/schema", status: schema},
			}},
		}},
		{relPath: "src/my_submodule_3", status: &submodule.Status{
			Pin:        submodule.PinInfo{Ref: "origin/main", Ahead: 1, Pushed: false, Source: submodule.RefFromCurrentBranch},
			Pinned:     unpushed,
			CheckedOut: unpushed,
			Tip:        sub3Tip,
		}},
		{relPath: "libs/logging", status: &submodule.Status{
			Pin:         submodule.PinInfo{Ref: "origin/feat/slog", Source: submodule.RefFromContains, Target: "origin/main", Behind: 3, Pushed: true},
			Pinned:      logPin,
			CheckedOut:  logPin,
			Tip:         logTip,
			TargetTip:   commit("0f1e2d3", "Bump dependencies", "renovate", 30*time.Hour),
			TargetDrift: submodule.Distance{Ahead: 2, Behind: 4},
			Bases:       submodule.Bases{Target: commit("9a8b7c6", "Release 0.8.0", "dana", 14*24*time.Hour)},
		}},
		{relPath: "libs/http", status: &submodule.Status{
			Pin:        submodule.PinInfo{Ref: "origin/develop", Target: "origin/develop", Ahead: 1, Behind: 2, Source: submodule.RefFromGitmodules},
			Bases:      submodule.Bases{Tip: commit("a9b8c7d", "Release 1.9.0", "frank", 20*24*time.Hour)},
			Pinned:     httpPin,
			CheckedOut: httpPin,
			Tip:        httpTip,
		}},
		{relPath: "libs/crypto", status: crypto},
		{relPath: "libs/parser", status: parser},
		{relPath: "third_party/zlib", status: zlib},
	}
}
