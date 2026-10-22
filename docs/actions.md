# Actions (draft)

**Status:** [Restore](#restore) is implemented (key `x`, decisions in
[D23](decisions.md)). [Stage pin](#stage-pin) is still a draft; its open
questions are [at the end](#open-questions).

## Principle

Offer as few actions as possible, but enough to work with submodules.
submarine shows what differs from what git records. The actions resolve such
a difference in one of two directions:

| Action | Direction | Key |
|---|---|---|
| [Restore](#restore) | The worktree follows the recorded state | `x` |
| [Stage pin](#stage-pin) | The recorded state follows the worktree | `s` |

There is nothing else for now. Committing, pushing, switching branches and
bumping a pin to the tip are done outside submarine. A bump is "check out
the tip in the submodule, then `s`".

## Restore

Makes the selected repository, and everything below it, exactly what git
records. It works on any submodule and on the superproject (header
selected). Restoring the superproject gives the exact project that the
current root commit specifies.

### Target

| Selected | Target commit | Branch |
|---|---|---|
| Superproject | `HEAD` | stays on the current branch |
| Submodule | The pin in the parent's **index** (what the UI shows as *Pinned*) | see [Branches](#branches) |
| Nested submodule, during the recursion | The gitlink in the restored parent's commit | as for a submodule |

Restoring the superproject resets its index too, so **staged pins are
discarded**: afterwards every pin is the one in `HEAD`. Restoring a submodule
does not touch the parent's index, so a staged pin of that submodule stays
and is the target.

### Postconditions

After a successful restore of repository R with target T:

1. **R's HEAD is T.** The index and the worktree equal T's tree. No
   modified, added, removed or conflicted files remain.
2. **No in-progress operation** remains in R: merge, rebase, cherry-pick,
   revert.
3. **No untracked files or directories** remain in R. This includes leftover
   directories of submodules that T no longer has (*untracked repository*).
   Ignored files are kept.
4. **Every submodule that T records is present and initialized**, even if
   it was never initialized, is inactive (`submodule.<name>.active`) or has
   `update = none / merge / rebase` configured. Its URL comes from T's
   `.gitmodules` (synced into the config).
5. **Each of those submodules is restored the same way**, with its gitlink
   in T as its target, recursively at any depth.

So the subtree shows `in sync` everywhere, except for states restore does
not change: `unpushed`, behind/ahead of the ref, and `detached HEAD on pin`.

The recursion follows **T**, not the tree loaded in the UI. A submodule that
is only in T (for example, after switching branches) is initialized. A
submodule that is only on disk is deleted as an untracked directory.

### Branches

- **Superproject:** stays on its branch. The branch is not moved, since the
  target is its own `HEAD`.
- **Submodule on the pin:** if HEAD is a branch that points at the target,
  the branch stays checked out.
- **Submodule off the pin:** HEAD is detached at the target. The branch that
  was checked out is **not moved**, so its commits stay on it.

Restore never moves, creates or deletes a branch.

### What can be lost

Restore discards work. The things it can lose:

| Lost | Where it comes from |
|---|---|
| Changed files | modified, added, removed, renamed files (`changed`) |
| Untracked files | `changed` (untracked) |
| Conflicts and in-progress operations | `conflict`; merge, rebase, cherry-pick, revert state |
| Commits | A detached HEAD off the pin whose commits are on no branch, tag or remote ref (only the reflog still has them) |
| Repositories | An untracked directory that is a repository with its **own** `.git` directory (a clone that was never added as a submodule). Its commits may exist nowhere else. A removed submodule whose git directory is absorbed into `.git/modules` loses only its worktree. Its objects stay |

Nothing else is lost. Branches, stashes, remote refs, ignored files and
absorbed submodule git directories stay.

### Confirmation

Restore asks **only if something would be lost**. Never asking is unsafe
(`git clean -ff` deletes repositories that exist nowhere else, and nothing
undoes a restore); always asking puts a question on the harmless, common
cases (`off pin`, `missing`, a staged pin) and teaches a reflexive `y`.

- **Nothing would be lost:** restore runs at once.
- **Something would be lost:** a line above the footer says what is
  discarded, summed over the whole subtree on disk:

  ```
  Restore src/app? Discards 7 changed files in 3 repositories, 2 commits on no branch, 1 untracked repository.   y restore · n cancel
  ```

  `y` runs it, any other key cancels.
- **The state can't be read** for a repository: it is counted as `n
  repositories of unknown state` and needs the same `y`.

The check reads the disk at the moment of the keypress (`RestoreLoss`), not
the loaded statuses, so loading or stale statuses don't matter. Nothing is
lost that is not in the table above; ignored files are never touched.

### Network

A restore may need objects that are not on disk: a submodule that was never
cloned (no git directory in `.git/modules`), or a target commit that was
never fetched. **Restore may clone and fetch, but only what is missing**,
non-interactively (`GIT_TERMINAL_PROMPT=0`, ssh `BatchMode`) and with a
timeout. If the commit is on no remote, restore fails with `commit … is not
available on any remote`. This is the one place besides `f` that uses the
network (decision D23).

### Running, progress and errors

- A line above the footer shows `checking …` and then `restoring …` with a
  spinner. Only one restore runs at a time; pressing `x` meanwhile flashes
  `wait for the restore of src/app to finish`. Everything else keeps working.
- A running fetch does not block a restore.
- Done: flash `restored src/app`; the app reloads the tree and all statuses,
  because the structure can have changed.
- Failure: the restore stops with git's message, shown in red above the
  footer (`restore of src/app failed: …`) until the next key. Restore is
  idempotent, so it can be run again after fixing the cause. Repositories
  that were restored before the failure stay restored.

### Implementation note

`git.Provider.Restore` (`internal/submodule/git/restore.go`) walks the
repositories itself, top-down; the children of a repository run
concurrently, bounded by the provider's semaphore. For each repository
(`T` = target) it runs, with `-c submodule.recurse=false`:

```sh
cat-file / fetch --all            # only if T is missing locally
rebase|cherry-pick|revert --quit  # whichever is in progress; --quit, not --abort: no ref moves
checkout --force --detach T       # only if HEAD is not T, so a branch is never moved
reset --hard                      # also ends a merge
clean -ffd                        # -ff: untracked repositories too; ignored files stay
submodule sync
submodule update --init --checkout --force -- <not yet initialized children>
# then every gitlink of T's tree (ls-tree -r T) is restored the same way
```

`--checkout` overrides `submodule.<name>.update`, and the explicit paths
override `submodule.<name>.active`, so no child module is skipped.

`RestoreLoss` (`lossOf`) follows the repositories on disk: per repository
`status` (without submodules), the in-progress state files, and
`rev-list --count HEAD --not --branches --tags --remotes` if HEAD is not the
target. A directory in the untracked list is searched for `.git`
*directories* (a standalone repository); an absorbed submodule leaves only a
`.git` file and loses just its worktree.

## Stage pin

Records the commit checked out in a submodule as its new pin in the
parent's index. It is the same as `git add <path>` in the parent, but only
when it is safe.

### Available when

All of:

- the selected node is a submodule, not the superproject;
- it is `off pin` (`Checkout == CheckoutDiffers`): HEAD differs from the
  pin in the parent's index;
- its worktree is **clean** (`Worktree == WorktreeClean`). Uncommitted
  changes, untracked files and conflicts count as dirty, the same rule as
  the `changed` issue. A moved nested pin counts as a change too, so nested
  submodules are staged bottom-up: first inside the submodule, then the
  submodule itself (after a commit in it, outside submarine).

Changes *inside* nested submodules don't count. Like the `changed` issue,
staging only looks at the submodule's own worktree.

### Effect

- The parent's index records the submodule's HEAD as the gitlink of its
  path. Nothing else changes: no worktree, no other index entry, no commit.
- The submodule then shows `staged` (or `●` next to an issue), and the
  `off pin` issue is gone.
- If the checkout equals the pin in the parent's `HEAD`, staging removes
  the staged pin instead (the index equals `HEAD` again).
- If the checked-out commit is not pushed, it is staged anyway. The row
  then shows `unpushed` (Danger), which is the warning.
- There is no confirmation: nothing is lost.
- Undo: there is no unstage action. `git restore --staged <path>` in the
  parent undoes it. Restoring the superproject also discards every staged
  pin, together with everything else.

### Implementation note

```sh
# in the submodule
git rev-parse HEAD                  # must still be the SHA shown in the UI, else abort
# in the parent
git update-index --cacheinfo 160000,<sha>,<path>
```

`update-index` stages exactly the SHA that was checked, independent of the
worktree. Flash: `staged pin of src/app` on success; git's message on
failure.

## Keys and palette

Both are submodule keys (`keyMap.Submodule`), shown in the detail pane's
palette, or in the footer when the pane can't show it. Restore is always
offered: it is idempotent, and the palette is static.

| Key | Action | Pressed when not possible (flash) |
|---|---|---|
| `x` restore | Restore the selection | `this data source can't restore` (mock); `wait for the restore of … to finish` |
| `s` stage pin | [Stage pin](#stage-pin), not implemented | `src/app is on its pin` / `src/app has uncommitted changes; commit or restore them first` / `the superproject has no pin` |

`R` would sit next to `r` (reload), a dangerous mix-up for a destructive
action, hence `x`.

## Testing

- Provider (`restore_test.go`), on fixtures from `scripts/make-fixture.sh`:
  a restored superproject is exact (every repository clean, `submodule status
  --recursive` without prefixes, `libs/missing` initialized), branches are
  not moved, ignored files survive, a merge and a rebase in progress end, a
  standalone clone and a removed submodule's directory are deleted, a missing
  submodule is cloned again, `update = none` and inactive submodules are
  restored, an unavailable commit is reported. The loss is empty for off pin,
  missing and unpushed submodules, counts files, nested changes, merges,
  detached commits (not those on a branch) and standalone repositories.
- UI (`internal/tui/restore_test.go`): restores at once without a loss, asks
  with one (`y` restores, anything else cancels), the mock can't restore.
- By hand in tmux on a fixture, see
  [architecture.md](architecture.md#testing).

## Open questions

Decided for restore: ignored files are kept (`clean` without `-x`); a
submodule is restored to the index gitlink, so a staged pin is its target and
survives; restore may clone and fetch what is missing; the key is `x`; it
asks only if something would be lost.

Open (stage pin):

1. **Key.** `s`?
2. **Palette.** Show it always (like restore), or only while it applies?
