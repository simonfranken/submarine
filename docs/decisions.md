# Decision log

Each entry: context, decision, consequences. Newest last.

## D1 — Show pins as ref + distance, not hashes

**Context.** Hashes don't tell users whether a pin is current.
**Decision.** Resolve each pin to a ref and show the distance (`origin/main ↓1`).
The resolution order is fixed and the applied rule is shown in the detail pane
(see [state-model.md](state-model.md#resolving-a-pin-to-a-ref)).
**Consequences.** Resolution is a heuristic; showing the rule keeps it
transparent.

## D2 — Compare three commits: pinned, checked out, tip

**Context.** The first mockup showed one branch for both the checkout and the
pin, so a mismatch was invisible.
**Decision.** Model all three commits. The tree's ref column always describes
the **pin**; checkout drift is an issue (`off pin`).
**Consequences.** The detail pane shows Pinned / Checkout / Tip rows.

## D3 — Raw dimensions in the domain, summarized issues in the UI

**Context.** States are not mutually exclusive (a submodule can be off pin and
dirty), but showing every dimension made rows noisy (14 symbols).
**Decision.** `Status` stores independent dimensions. `Status.Issues()`
summarizes them into five kinds with three levels. The tree shows only the
most severe issue (+n); the detail pane shows all.
**Consequences.** One place defines what a problem is; a git backend gets
the same behavior for free.

## D4 — Attention follows severity

**Context.** Healthy rows competed for attention with dangerous ones.
**Decision.** Three levels: quiet (muted row, no word), warn (yellow),
danger (red bold). Behind/ahead, staged pin and detached HEAD are quiet.
**Consequences.** A long list reads as grey, with problems standing out.

## D5 — Words instead of glyphs

**Context.** Symbols (`≠ ✚ ○ ⚠ ●`) were found unintuitive.
**Decision.** Short words: `unpushed`, `conflict`, `missing`, `off pin`,
`changed` (plus `error`). Symbols remain only for arrows and tree markers.
**Consequences.** Status column is 14 columns wide; no legend needed.

## D6 — "Unpushed pin" is the most severe state

**Context.** Committing a superproject whose pin isn't on any remote breaks
everyone else's checkout.
**Decision.** `unpushed` is Danger, listed first, and its `↑n` is red.

## D7 — Hidden issues bubble up to collapsed parents

**Context.** Only the first level is shown by default, so nested problems
would be invisible.
**Decision.** Collapsed parents show `·k` (issues in all descendants), colored
by the worst one. Problems in a child do **not** mark the parent itself as
`changed`.

## D8 — Collapsed by default, branch-first navigation

**Decision.** Only the first level is visible. `→` expands and enters the
first child, `←` collapses and goes to the parent, `space` toggles,
`ctrl+→/←` expand/collapse all (see [ux-guide.md](ux-guide.md#navigation)).
`←` on an expanded node collapses it in place, so a node can be closed
without leaving it. Remembered expansion is kept on collapse, but reset by
"collapse all".

## D9 — Directory headers are path groups, not nesting

**Decision.** Submodules are grouped by the directory of their path relative
to the parent repo. Headers (`src/`, `modules/`) are muted, non-selectable,
and skipped by navigation. Nesting is expressed by indentation under the
parent submodule.

## D10 — ANSI 16 colors

**Context.** Fixed 256-colors (e.g. violet 63) ignored the user's terminal
theme and were unreadable on light backgrounds.
**Decision.** Use ANSI 0–15 for all meaning (red, yellow, blue, cyan, grey,
default foreground). Only background shades use `AdaptiveColor`. Detect the
background before Bubble Tea starts.

## D11 — Provider boundary and async loading

**Decision.** The UI accesses data only through `submodule.Provider`:
`Tree()` for the fast structure, `Status(path)` per submodule, loaded
concurrently with a spinner per row. The prototype uses a mock provider with
artificial latency.
**Consequences.** A git backend can be added without UI changes; the UI
stays responsive with many submodules.

## D12 — No network on startup (superseded by D18)

**Decision.** Work from local refs and show their age; fetching is an
explicit action (`f`).

## D13 — Mouse support via bubblezone

**Decision.** Keep mouse support (click to select/toggle, wheel to move)
using bubblezone markers per row.

## D14 — Git provider shells out to the git CLI

**Context.** The provider can use a Go library (go-git) or the `git` binary.
**Decision.** Run `git -C <dir> …` with a fixed environment
(`GIT_TERMINAL_PROMPT=0`, `GIT_OPTIONAL_LOCKS=0`, `LC_ALL=C`) and parse
stable output formats (`--porcelain=v2`, custom `--format`).
**Consequences.** Behaves exactly like the user's git (config, `gitdir:`
files, worktrees, partial clones), no new dependency, fast on large repos.
Requires `git` on `PATH`; one process per query.

## D15 — Git provider semantics

**Decision.** The pin is the **index** gitlink, because that is what the
next commit records and what `submodule update` checks out; a difference to
HEAD is `PinStaged`. Worktree state uses `--ignore-submodules=dirty`, so dirt
in a nested submodule doesn't mark its parent (consistent with D7). A pin
whose commit isn't present locally is a loading `error`, not `unpushed`,
because without the commit nothing can be said about it.
`--mock` is no longer the default; `submarine [dir]` reads a real repository.

## D16 — Bounded concurrency inside the provider

**Context.** The app starts one `Status` command per submodule at once
(D11); with hundreds of submodules that would spawn thousands of git
processes.
**Decision.** The git provider limits concurrent `Status` calls to
`runtime.NumCPU()` with a semaphore that respects `ctx`.
**Consequences.** The UI stays unchanged; rows fill in steadily.

## D17 — Change preview: flat letter list against HEAD

**Context.** `changed` doesn't tell users what they changed or why; they
want to remember what they were doing in a submodule.
**Decision.** `Status.Changes` lists the changed files against HEAD (staged
and unstaged combined) with line counts. The detail pane shows them as a
flat list with a colored status letter in front, like IDE source-control
views, instead of group headers. Sorted by kind, then by size; the 10-line
budget keeps one line for every kind present and summarizes the rest.
Placed directly below the issue lines, because the pane is cut by height.
One summary line for all files, then strictly one line per file with its
own age: a second summary line ("+1 · edited 19m ago") read as if it
belonged to the first file.
Dirt inside nested submodules is ignored (D7); unstaged moves show as
removed + untracked.
**Consequences.** One extra `git diff` per dirty submodule, none for clean
ones. Not included: staged/unstaged markers, diff bodies, stash count.

## D18 — Background fetch on startup

**Context.** Without fetching, every answer is only as fresh as the user's
last `git fetch`, which they rarely run in every submodule.
**Decision.** Supersedes D12. Show the local state at once, then fetch the
superproject and every initialized submodule in the background (all
remotes, prune, no recursion: we fetch each repository ourselves, at most 8
at once). Fetches never prompt; they fail with git's message instead.
`--no-fetch` disables it, `f` repeats it.
**Consequences.** Startup still doesn't wait for the network (N1). A
failed fetch is shown per submodule and counted in the header; it is not an
issue (D3).

## D19 — Watch git metadata by polling

**Context.** The view should follow changes made outside the application.
File system events (fsnotify) need a dependency, one watch per directory
(hundreds of repositories × ref directories hit inotify limits) and don't
work on WSL's Windows drives or network file systems.
**Decision.** Poll every 2 s: compare names, mtimes and sizes of each
repository's git metadata (`FETCH_HEAD`, refs, `HEAD`, `index`, …),
split into a remote, a local and a structure part.
**Consequences.** Works everywhere, no dependency; a few `stat`s per
repository per poll; changes appear within about 2 s. Edits of worktree
files are not detected (that would mean running `git status` everywhere
every poll); `r` reloads on demand.

## D20 — Refresh is driven by the repositories, not by our fetch

**Decision.** A finished fetch only updates its progress. Reloading is
triggered solely by the watcher, so our own fetch and a fetch in another
shell take the same path. Reloads keep the old state visible, run at most
once per submodule at a time and coalesce bursts.
**Consequences.** One mechanism to test and trust. After our fetch the
data updates up to one poll interval later; the header shows "fetched just
now" right away.

## D21 — Distribution: local build, Gitea generic registry, curl | sh

**Decision.** Releases are built locally with `make` and uploaded with an
access token to the Gitea generic package registry. Installation is a
POSIX `install.sh`; `sub` is a symlink to `submarine`. Updates are
`submarine update`, driven by a `latest/VERSION` pointer file.
**Rationale.** The Gitea instance has no working runner. The generic
registry needs no extra hosting and is publicly readable. A symlink keeps
one binary to update. The pointer is uploaded last and lives in its own
package so versions stay clean.
**Consequences.** Integrity is a sha256 from the same host: it catches
corrupt or partial downloads, not a compromised registry (no signatures).
The update check is a deliberate, outside-the-provider network use: only
on `update` and a once-a-day background check that can be disabled.
