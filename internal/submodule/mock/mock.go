// Package mock provides a submodule.Provider backed by fixture data. It
// covers every state the UI can display and simulates loading latency.
package mock

import (
	"context"
	"fmt"
	"math/rand/v2"
	"path"
	"time"

	"submarine/internal/submodule"
)

// Provider serves a fixed fixture tree.
type Provider struct {
	MinDelay, MaxDelay time.Duration
	statuses           map[string]*submodule.Status
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
	p.statuses = map[string]*submodule.Status{}
	repo := &submodule.Repo{
		Path:      "~/coding/my_project",
		Branch:    "main",
		FetchedAt: time.Now().Add(-2 * time.Minute),
	}
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
		p.statuses[full] = n.status
		out = append(out, m)
	}
	return out
}

func (p *Provider) Status(ctx context.Context, path string) (*submodule.Status, error) {
	delay := p.MinDelay
	if span := p.MaxDelay - p.MinDelay; span > 0 {
		delay += rand.N(span)
	}
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s, ok := p.statuses[path]
	if !ok {
		return nil, fmt.Errorf("unknown submodule %q", path)
	}
	return s, nil
}

// --- fixture -----------------------------------------------------------------

var now = time.Now()

func commit(sha, subject, author string, ago time.Duration) *submodule.Commit {
	return &submodule.Commit{SHA: sha, Subject: subject, Author: author, When: now.Add(-ago)}
}

func inSync(ref string, c *submodule.Commit) *submodule.Status {
	return &submodule.Status{
		Pin:    submodule.PinInfo{Ref: ref, Pushed: true},
		Pinned: c, CheckedOut: c, Tip: c,
	}
}

func fixture() []node {
	c1 := commit("3f9a1c2", "Bump golangci-lint to v2", "anna", 26*time.Hour)
	c2 := commit("a1b2c3d", "Initial import", "ben", 40*24*time.Hour)

	srcSub1Pin := commit("5d6e7f8", "Add retry to client", "chris", 3*24*time.Hour)
	srcSub1Head := commit("0c1d2e3", "WIP: experiment with backoff", "you", 2*time.Hour)

	srcSub2 := inSync("origin/main", commit("77aa88b", "Merge PR #42", "dana", 5*time.Hour))
	srcSub2.Worktree = submodule.WorktreeModified

	subSubPin := commit("4e5f6a7", "Add feature flag", "eve", 30*time.Hour)
	subSubTip := commit("9f8e7d6", "Fix login timeout", "anna", 2*time.Hour)
	subSub := &submodule.Status{
		Checkout:   submodule.CheckoutDiffers,
		Pin:        submodule.PinInfo{Ref: "origin/feat/02", Behind: 1, Pushed: true},
		Pinned:     subSubPin,
		CheckedOut: subSubTip,
		Tip:        subSubTip,
		Between:    []submodule.Commit{*subSubTip},
	}

	schema := inSync("origin/main", commit("1a2b3c4", "Add v3 schema", "ben", 6*24*time.Hour))
	schema.Worktree = submodule.WorktreeUntracked

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
			Between:    []submodule.Commit{*unpushed},
		}},
		{relPath: "libs/logging", status: &submodule.Status{
			Pin:        submodule.PinInfo{Ref: "origin/main", Behind: 3, Pushed: true},
			Pinned:     logPin,
			CheckedOut: logPin,
			Tip:        logTip,
			Between: []submodule.Commit{
				*logTip,
				*commit("7d8e9f0", "Fix race in buffered writer", "dana", 3*24*time.Hour),
				*commit("6c7d8e9", "Update dependencies", "renovate", 5*24*time.Hour),
			},
		}},
		{relPath: "libs/http", status: &submodule.Status{
			Pin:        submodule.PinInfo{Ref: "origin/develop", Ahead: 1, Behind: 2, Pushed: true, Source: submodule.RefFromContains},
			Pinned:     httpPin,
			CheckedOut: httpPin,
			Tip:        httpTip,
			Between: []submodule.Commit{
				*httpTip,
				*commit("d0e1f2a", "Refactor transport", "frank", 2*24*time.Hour),
				*httpPin,
			},
		}},
		{relPath: "libs/crypto", status: crypto},
		{relPath: "third_party/zlib", status: zlib},
	}
}
