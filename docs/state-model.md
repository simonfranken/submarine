# State model

Code: `internal/submodule/model.go`, `internal/submodule/status.go`.

## Three commits per submodule

Every submodule involves three commits. All state is derived from comparing
them.

| Commit | Source |
|---|---|
| **Pinned** | The gitlink recorded in the superproject index (differs from HEAD → `PinStaged`) |
| **Checked out** | The submodule's `HEAD` |
| **Tip** | The tip of the ref the pin was resolved to, e.g. `origin/main` |

This gives two independent questions:

- **Is the checkout on the pin?** → `off pin` if not.
- **Where is the pin relative to its ref?** → `↑n` / `↓n` / `↑n↓m`.

## Raw status dimensions

`submodule.Status` stores the raw, independent dimensions. A submodule can be
in several at once.

| Field | Values |
|---|---|
| `Presence` | `PresenceOK`, `NotInitialized`, `DirMissing` |
| `Checkout` | `CheckoutMatch`, `CheckoutDiffers`, `CheckoutDetached` (on the pin, detached HEAD; informational) |
| `Worktree` | `WorktreeClean`, `WorktreeModified`, `WorktreeUntracked`, `WorktreeConflict` |
| `PinStaged` | Pin changed in the superproject index but not committed |
| `Pin` | `Ref`, `Source`, `Ahead`, `Behind`, `Pushed` |
| `Pinned`, `CheckedOut`, `Tip` | Commits (SHA, subject, author, time) |
| `Drift` | Checkout compared with the pin: `Ahead` = commits only the checkout has, `Behind` = commits only the pin has |
| `Bases` | `Tip`, `Checkout`, `Committed`: `merge-base <pin> <that commit>` for each commit that is diverged from the pin (ahead and behind); nil otherwise or without a common ancestor |
| `Committed`, `CommittedDrift` | With a staged pin: the pin in the superproject's HEAD (nil if it is not on disk or the submodule was only added) and how it compares with the staged pin, like `Drift` |
| `Changes` | Changed files of the worktree (nil if clean), see below |
| `FetchedAt` | Last fetch of the submodule (`FETCH_HEAD` mtime); zero if never |

`Submodule.FetchErr` is set if the application's last fetch of the
submodule failed. Like loading errors, it is not an issue kind: a failed
fetch says nothing about the submodule's state.

`Status == nil` means "still loading". `Submodule.Err` is set if loading
failed.

## Changes

`Status.Changes` describes *what* is behind `Worktree`: one `FileChange` per
path with a `ChangeKind` (conflicted, modified, added, removed, renamed,
untracked), added/removed line counts, and flags for binary files and moved
nested submodule pointers. It compares the worktree **with HEAD**, so staged
and unstaged changes are combined (a file added to the index and edited
afterwards is one added file).

- `Files` is sorted by kind (that order), then by changed lines (largest
  first), then by path, and capped at `MaxChangeFiles` (200). `Total`,
  `Added` and `Removed` cover all files.
- `FileChange.Modified` is the file's modification time (zero for removed
  files).
- `Worktree` is derived from the same list: conflict, else any tracked
  change → modified, else untracked.
- It is a preview, not an issue: it adds no issue kinds and changes no levels.

## Issues: summarizing the dimensions

The UI never shows raw dimensions in the tree. `Status.Issues()` summarizes
them into **issue kinds**, each with a **level**. This is the single source of
truth for what counts as a problem.

| Issue (word) | Combines | Level |
|---|---|---|
| `unpushed` | Pin not reachable from any remote ref | Danger |
| `conflict` | Merge conflict in the worktree | Danger |
| `missing` | Not initialized; directory missing | Warn |
| `off pin` | Checkout differs from pin | Warn |
| `changed` | Uncommitted changes; untracked files (worded "untracked repository" if all of them are git repositories) | Warn |

Rules:

- If the submodule is **missing**, that is its only issue (nothing else can
  be determined).
- Issues are returned **most severe first**: unpushed, conflict, off pin,
  changed.
- **Not issues** (quiet): in sync, pin behind/ahead/diverged but pushed,
  staged pin, detached HEAD on the pin. They are shown as information only.
  A staged pin is also visible in the tree (`staged` or a `●` marker), see
  [ux-guide.md](ux-guide.md#tree-row), but is never counted as an issue.
- A loading error is shown as `error` (Danger) in the UI; it is not an issue
  kind.

### Levels

| Level | Meaning | Presentation |
|---|---|---|
| `LevelQuiet` | Nothing to do | Whole row muted, no word |
| `LevelWarn` | Look at it | Yellow word |
| `LevelDanger` | Act now | Red bold word |

### Hidden issues

`Submodule.HiddenIssues()` counts the issues of **all** loaded descendants
per level (`Hidden{Warn, Danger}`). A collapsed submodule shows them as `·k`
per level, red first, then yellow, so a danger can never hide inside a
collapsed subtree and a warning is never mistaken for one.
A descendant whose load failed (`error`) counts as one danger.

## Superproject

The superproject is a node with an empty `Path` (`Repo.Root`, no children;
`Submodule.IsRoot()`) and a `Status` with `Root` set. It reuses the pin
fields with **pin = HEAD**: `Pinned` and `CheckedOut` are HEAD, `Checkout`
is never `CheckoutDiffers` (`CheckoutDetached` for a detached HEAD),
`PinStaged` is false. `Issues()` is the same code; only `unpushed` says
"HEAD not pushed to any remote" (Danger, like a submodule's pin).

- **Ref** is the upstream of the current branch (`RefFromCurrentBranch`), so
  ahead/behind agrees with the header. Closest-branch resolution is only the
  fallback without an upstream (detached, or no upstream configured).
- **Changes** ignore submodules (`status --ignore-submodules=all`): an off-pin
  or staged submodule is shown by its own row, not twice. A leftover
  directory of a submodule the branch no longer has is an untracked
  repository.
- `Walk` and the header counts do not include the root.

## Resolving a pin to a ref

The pin is displayed as a ref, not a hash. A commit can be on several
branches, on none, or deep in the history. The git provider shows the
**closest** branch and records which rule proposed it in `PinInfo.Source`.

Preferred refs, in this order (each only if it exists):

| Preference | Ref | `RefSource` |
|---|---|---|
| a | `branch = …` in `.gitmodules` | `RefFromGitmodules` |
| b | Upstream of the branch checked out in the submodule | `RefFromCurrentBranch` |
| c | Remote default branch (`<remote>/HEAD`) | `RefFromDefaultBranch` |

Resolution:

| # | Rule | `RefSource` |
|---|---|---|
| 1 | Pin pushed: the branch containing the pin with the fewest commits after it. Candidates are the preferred refs that contain the pin plus any remote branch containing it (`for-each-ref --contains`); a tie goes to the preferred ref | preferred ref's, else `RefFromContains` |
| 2 | Pin unpushed: the preferred ref with the fewest commits on either side (`↑n↓m`), ties by preference | preferred ref's |
| 3 | A tag (`git describe`) | `RefFromTag` |
| 4 | Short SHA | `RefFromSHA` |

So a `.gitmodules` branch is shown when it is as close as any other branch,
but a pin that sits on a feature branch shows that branch rather than
"diverged from main". The trade-off: a pin on main that a younger branch
also contains shows the younger branch if its tip is fewer commits away.

Distances: `git rev-list --left-right --count <pin>...<ref>` gives
`Ahead` (left) and `Behind` (right).

`Pushed` is true if the pin is reachable from any remote ref. A pin that is
ahead of its ref can still be pushed (e.g. on another branch); only
`Pushed == false` is dangerous.

## Git commands

How the git provider (`internal/submodule/git`) fills `Status`. All commands
are local; `<pin>` is the index gitlink.

| Field | Command (in the parent repo / in the submodule) |
|---|---|
| `Pinned`, `PinStaged` | parent: `ls-files -s -- <path>` vs `ls-tree HEAD -- <path>` |
| `Presence` | directory missing → `DirMissing`; no `.git` in it → `NotInitialized` |
| `Checkout` | `rev-parse HEAD`, `symbolic-ref -q HEAD` (no branch → detached) |
| `Worktree`, `Changes` kinds | `status --porcelain=v2 -z --untracked-files=normal --ignore-submodules=dirty` |
| `Changes` line counts | `diff HEAD --numstat -z -M --ignore-submodules=dirty`, only if a tracked file changed; failure leaves the counts at 0 |
| `FileChange.Modified` | `stat` of each changed file that still exists |
| `Pushed` | `for-each-ref --contains <pin> refs/remotes` is not empty |
| `Pin.Ref` | rules above; the remote is `origin`, else the first remote |
| `Ahead`, `Behind` | `rev-list --left-right --count <pin>...<ref>` |
| `Drift` | `rev-list --left-right --count <head>...<pin>`, only if the checkout differs from the pin |
| `Bases` | `merge-base <pin> <ref>` / `<head>` / `<committed>`, only when diverged; then `log -1` of it |
| `Committed`, `CommittedDrift` | `ls-tree HEAD` gitlink, if it differs from the pin and exists locally; `rev-list --left-right --count <committed>...<pin>` |

Details:

- Preference a: `branch = .` means the superproject's current branch. The
  ref counts only if `<remote>/<branch>` exists locally.
- Preference b uses the checked-out branch's upstream (`<branch>@{upstream}`).
- Rule 1 counts `rev-list --count <pin>..<ref>`. Besides the preferred refs
  it compares at most 16 containing branches (oldest tip first).
- Rule 2 counts `rev-list --count <pin>...<ref>`.
- Rule 4 shows the short SHA; there is no tip and no distance.
- `--ignore-submodules=dirty`: changes inside a nested submodule do not make
  its parent `changed` (D7), but a moved nested pin does.
- Renames are detected only for staged moves (`git mv`); an unstaged move
  is a removed file plus an untracked file. Untracked directories are one
  entry (`dir/`) without line counts.
- `--untracked-files=normal` overrides `status.showUntrackedFiles`, so the
  result doesn't depend on the user's config.
- A missing submodule only gets a `Pinned` SHA; nested submodules of an
  uninitialized submodule are unknown and not shown.
- If the pinned commit doesn't exist in the submodule (not fetched), loading
  fails with "pinned commit … not fetched" (shown as `error`) instead of
  guessing `unpushed`.

## Spine

`Status.Spine()` arranges the pin, the tip and the checkout as one line of
history, newest on top, for the detail pane. Each node is a commit with its
roles (`pin`, `tip`, `you`; `HEAD` for the superproject). It uses `Pin.Ahead` /
`Pin.Behind` (pin ↔ tip) and `Drift` (checkout ↔ pin).

- Roles on the same commit share a node (`pin · tip · you`);
  `SpineNode.InSync()` is true if all of them are on it (`HEAD` and `tip` for
  the superproject).
- Tip or checkout *ahead* of the pin (it is their ancestor) sit above it,
  *behind* below it; the `Link` between two nodes counts the commits
  separating them. The count is the distance, so a tip 4 commits ahead
  of the pin shows `4 commits`, and an adjacent one `1 commit`.
- A commit that has **diverged** from the pin (tip, checkout or committed
  pin) is drawn as a **fork** if it is the only one and its merge base is
  known (`Status.Bases`): it is the first node (`SpineNode.Fork`), with the
  number of commits down to the base as its `Below`; the main lane follows
  (pin and the linear nodes), and a `RoleBase` node where the two histories
  split. Linear nodes between the pin and the base stay on the main lane
  above it, older ones continue below it, and one that is the base commit
  shares its node (`you · base`). The pin-to-base distance is the pin's side
  of the divergence.
- Otherwise (two diverged commits, or no merge base) there is no fork: a
  diverged tip is placed above the pin, a diverged checkout below it, with a
  diverged `Link` (`Above` and `Below` count the commits only the upper or
  lower node has). The link between a diverged node and a node other than the
  pin is unknown (zero `Link`).
- With a staged pin the node of `Pinned` (the index) is labelled `staged`, and
  the committed pin gets its own `pin` node, placed by `CommittedDrift` like
  the checkout. Both can share nodes with `tip` and `you`.
- No tip (the pin resolved to a SHA) or no checkout commit: no node.

## Distance notation

| Display | Meaning |
|---|---|
| `↓n` | The ref has n commits the pin doesn't have; the pin is behind |
| `↑n` | The pin has n commits the ref doesn't have |
| `↑n↓m` | Diverged |

The arrows follow the convention of `git status`, lazygit and shell prompts.
