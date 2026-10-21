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
| `Between` | Commits that make up the pin ↔ tip difference |
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
and returns the most severe level. A collapsed submodule shows this as `·k`
in that level's color, so a danger can never hide inside a collapsed subtree.

## Resolving a pin to a ref

The pin is displayed as a ref, not a hash. A commit can be on several
branches, on none, or deep in the history, so the git provider resolves it by
this order and records which rule applied in `PinInfo.Source`:

| # | Rule | `RefSource` |
|---|---|---|
| 1 | `branch = …` in `.gitmodules` | `RefFromGitmodules` |
| 2 | Branch currently checked out in the submodule | `RefFromCurrentBranch` |
| 3 | Remote default branch, if it contains the pin | `RefFromDefaultBranch` |
| 4 | Any remote branch containing the pin, preferring the closest tip (`git branch -r --contains`) | `RefFromContains` |
| 5 | A tag (`git describe`) | `RefFromTag` |
| 6 | Short SHA | `RefFromSHA` |

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
| `Between` | `log <pin>...<ref>`, at most 50 commits |

Details:

- Rule 1: `branch = .` means the superproject's current branch. The rule
  applies only if `<remote>/<branch>` exists locally.
- Rule 2 uses the checked-out branch's upstream (`<branch>@{upstream}`).
- Rule 4 compares at most 16 containing branches (oldest tip first) and
  picks the one with the fewest commits after the pin.
- Rule 6 shows the short SHA; there is no tip and no distance.
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

## Distance notation

| Display | Meaning |
|---|---|
| `↓n` | The ref has n commits the pin doesn't have; the pin is behind |
| `↑n` | The pin has n commits the ref doesn't have |
| `↑n↓m` | Diverged |

The arrows follow the convention of `git status`, lazygit and shell prompts.
