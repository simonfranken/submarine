# Actions (draft)

**Status: draft, not implemented.** The spec for the first two write actions.
Open questions are listed [at the end](#open-questions). Once they are
answered, the decisions go into [decisions.md](decisions.md), and
[requirements.md](requirements.md) and [ux-guide.md](ux-guide.md) are
updated.

## Principle

Offer as few actions as possible, but enough to work with submodules.
submarine shows what differs from what git records. The actions resolve such
a difference in one of two directions:

| Action | Direction | Key (proposed) |
|---|---|---|
| [Restore](#restore) | The worktree follows the recorded state | `u` |
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
and is the target. (See open question 2.)

### Postconditions

After a successful restore of repository R with target T:

1. **R's HEAD is T.** The index and the worktree equal T's tree. No
   modified, added, removed or conflicted files remain.
2. **No in-progress operation** remains in R: merge, rebase, cherry-pick,
   revert.
3. **No untracked files or directories** remain in R. This includes leftover
   directories of submodules that T no longer has (*untracked repository*).
   Ignored files are kept (see open question 1).
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

- **Nothing would be lost** (only `off pin`, `missing`, a staged pin of the
  root): restore runs at once, without a question.
- **Something would be lost:** a confirmation line opens above the footer,
  like the filter line. It says what is discarded, summed over the subtree:

  ```
  Restore src/app? Discards 7 changed files in 3 submodules, 2 detached commits, 1 repository.   y restore · n cancel
  ```

  `y` runs it. `n`, `esc` or any other key cancels.
- **The state is not fully known** (a status in the subtree is still
  loading or failed to load): the line says so (`… · 2 submodules not
  loaded`) and needs the same `y`. Restore does not wait for the loading.
- Lost repositories and commits are shown in the Danger color, files in
  Warn.

The summary only covers the submodules that are loaded. Children of a
submodule that is not initialized are unknown. They are created fresh, so
nothing of them can be lost.

### Network

A restore may need objects that are not on disk:

- a submodule that was never cloned (`not initialized`, no git directory in
  `.git/modules`);
- a target commit that was never fetched.

Proposal: **restore may clone and fetch, but only for objects that are
missing**, and never prompts (`GIT_TERMINAL_PROMPT=0`, as for `Fetch`).
This changes the rule "the provider uses the network only in `Fetch`"
(AGENTS.md, N8). The alternative is to fail with "pinned commit … not
fetched" and leave cloning to the user (open question 3).

### Running, progress and errors

- The selected row (or the header) shows a spinner and `restoring` in the
  status column until the restore ends. The header shows `restoring src/app`
  where the fetch progress usually is.
- Only one write action runs at a time. Pressing `u` or `s` meanwhile
  flashes `wait for the restore of src/app to finish`. Navigation and
  everything else keep working.
- A running background fetch does not block a restore.
- Done: flash `restored src/app`. The watcher refreshes the affected rows.
  The app also reloads the subtree, because the structure can have changed.
- Failure: the restore stops at the first repository that fails. The footer
  flashes `restore failed in src/app/lib: <git's message>`, and the detail
  pane of the selected node shows the same message until the next restore.
  Restore is idempotent, so it can be run again after fixing the cause.

### Implementation note

For each repository, top-down (`T` = target):

```sh
git merge --abort / rebase --abort / cherry-pick --abort / revert --abort   # whichever is in progress
git checkout --detach --force T      # submodule off the pin; superproject: git reset --hard HEAD
git reset --hard T
git clean -ffd                       # -ff: also untracked nested repositories
git submodule sync
git submodule init                   # all submodules of T, including inactive ones
# for each gitlink in T: make sure the repo exists (clone/absorb), then recurse
```

The repository can be walked by submarine itself, not by `git submodule
update --recursive`, so progress, the loss check and errors are per
repository. `git submodule update --init --checkout --force` is an option for
the clone step. The loss check runs right before the restore, against the
current state, not the displayed one.

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
palette, or in the footer, **only while they apply** to the selection:

| Key | Shown when | Pressed when not applicable (flash) |
|---|---|---|
| `u` restore | The node or a loaded descendant has an issue restore fixes (`changed`, `conflict`, `missing`, `off pin`), or the root has a staged pin | `nothing to restore in src/app` |
| `s` stage pin | [Available](#available-when) | `src/app is on its pin` / `src/app has uncommitted changes; commit or restore them first` / `the superproject has no pin` |

Full help (`?`) always lists both.

## Testing

- Provider: tests on fixtures from `scripts/make-fixture.sh`, which builds
  every state. For restore: changed, untracked, untracked repository
  (absorbed and standalone), conflict, rebase in progress, off pin on a
  branch, off pin detached with unreachable commits, not initialized,
  inactive, `update = none`, a submodule added and one removed between two
  root commits, nested at depth ≥ 2. Assert the postconditions with
  `git status --porcelain --ignored=no` and `git submodule status
  --recursive`, and that branches are unchanged. For stage pin: index entry,
  refusal on a dirty worktree, refusal when HEAD moved.
- Loss summary: unit tests on `submodule` (no UI), like `Status.Issues()`.
- UI: run it on a fixture in tmux (see
  [architecture.md](architecture.md#testing)). Check the confirmation line,
  the spinner, the flash messages and the palette.

## Open questions

1. **Ignored files.** Should restore also delete ignored files (`git clean
   -x`)? That is the literal "no additional files", but it deletes `.env`,
   `node_modules`, build output and IDE settings in every submodule.
   *Proposal: keep ignored files.*
2. **Target of a submodule restore with a staged pin.** The staged pin
   (index, what the UI shows as *Pinned*, what `git submodule update` does)
   or the committed one (parent's `HEAD`, which also unstages)?
   *Proposal: the index. Unstaging happens only through a superproject
   restore.*
3. **Network.** May restore clone uninitialized submodules and fetch missing
   pins (non-interactive), or should it fail and ask for `f` / a manual
   clone? *Proposal: allow it, only for missing objects.*
4. **Keys.** `u` restore, `s` stage pin? `R` would sit next to `r` reload,
   which is a dangerous mix-up for a destructive action.
5. **Confirmation without loss.** Is "no question when nothing is lost" OK,
   or should restore always ask?
