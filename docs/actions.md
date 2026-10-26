# Actions (draft)

**Status:** [Restore](#restore) (key `x`, [D23](decisions.md); hard restore
`X`, [D30](decisions.md)),
[Switch branch](#switch-branch) (key `b`, [D24](decisions.md)) and
[Update pin](#update-pin) (key `u`, [D29](decisions.md)) and
[Resolve pin conflict](#resolve-pin-conflict) (key `m`, [D34](decisions.md))
are implemented.
Stage pin was dropped ([D29](decisions.md)).

## Principle

Offer as few actions as possible, but enough to work with submodules.
submarine shows what differs from what git records. The actions resolve such
a difference in one of two directions:

| Action | Direction | Key |
|---|---|---|
| [Restore](#restore) | The worktree follows the recorded state | `x`, `X` (hard) |
| [Switch branch](#switch-branch) | Both: the branch tip is recorded, then restored to | `b` |
| [Update pin](#update-pin) | Both: a switch to the tip of the pin's ref | `u` |
| [Resolve pin conflict](#resolve-pin-conflict) | Both: the chosen side (or a commit with both) is recorded, then restored to | `m` |

There is nothing else for now. Committing and pushing are done outside
submarine. Recording a commit made in a submodule is a switch to its branch
(`b`); a commit on no branch must get one first.

## Restore

Makes the selected repository, and everything below it, exactly what git
records. It works on any submodule and on the superproject (header
selected). Restoring the superproject gives the exact project that the
current root commit specifies, with the pins you have staged.

### Target

| Selected | Target commit | Branch |
|---|---|---|
| Superproject | `HEAD` | stays on the current branch |
| Submodule | The pin in the parent's **index** (what the UI shows as *Pinned*); with a staged pin you choose, see below | see [Branches](#branches) |
| Nested submodule, during the recursion | The gitlink in the restored parent's index | as for a submodule |

**Staged pins are kept, at every depth.** A staged pin is a decision, not
drift. Each repository's files are reset to its `HEAD`, but its staged pins
stay in its index, and the submodules below are restored to them. So a
nested submodule with a staged pin and uncommitted changes keeps the pin
and is reset to it; its changes are discarded (and listed in the question).

**Choosing the pin.** If the selected submodule has a staged pin itself, `x`
first asks which pin to restore to:

```
Restore libs/staged to  s staged pin · c committed pin (unstages it)     any other key cancels
```

- `s`: the staged pin, which stays staged.
- `c`: the committed pin (the gitlink in the parent's `HEAD`). The parent's
  index is set back to it, which is how a pin is unstaged in submarine.

The loss check then runs for the chosen pin (the two can differ: a staged
commit that exists only locally is lost by `c`, not by `s`).

Two exceptions, where a staged pin is not kept:

- **The repository moves.** If a repository is off its pin, restore checks
  out another commit; staged pins in its own index belonged to the old one.
  They are counted as a loss (`2 staged pins`), so the question appears. Its
  submodules follow the target commit's tree.
- **An added, not yet committed submodule** is no staged pin: its
  `.gitmodules` entry is reset with the other files. Its directory then
  counts as an untracked file.

### Hard restore

`X` restores to the **committed** pins, at every depth: what the commits
record, without any staged pin.

- The selected submodule goes to the committed pin, without the `s` / `c`
  question; a staged pin of it is unstaged (as with `c`). A submodule that
  was added but not committed yet has no committed pin and goes to the one
  in the index.
- Every restored repository's index is reset to its `HEAD`, the superproject
  included: its staged pins are dropped, and the submodules below follow the
  committed tree.

So `x` and `X` differ in the staged pins of repositories that the restore
does not move: `x` (`s` or `c`) keeps them, `X` drops them. This matters
for the superproject, for a selected submodule without a staged pin of its
own, and for nested repositories that are on their pin. A repository that
moves loses its staged pins either way (see above); then `x` → `c` and `X`
end the same, but `c` asks first.

The dropped staged pins are counted in the loss (`2 staged pins`), the
selected submodule's own staged pin included, and like any loss they make
restore ask (D31): `Restore libs/staged to the committed pins? Discards 1
staged pin.` The
commits of a dropped pin stay in the submodule; if HEAD leaves them and no
ref reaches them, they count as `commits on no branch`, as always.

### Postconditions

After a successful restore of repository R with target T:

1. **R's HEAD is T.** The index and the worktree equal T's tree, except for
   the staged pins that were kept. No modified, added, removed or conflicted
   files remain.
2. **No in-progress operation** remains in R: merge, rebase, cherry-pick,
   revert.
3. **No untracked files or directories** remain in R. This includes leftover
   directories of submodules that T no longer has (*untracked repository*).
   Ignored files are kept.
4. **Every submodule that R's index records is present and initialized**, even if
   it was never initialized, is inactive (`submodule.<name>.active`) or has
   `update = none / merge / rebase` configured. Its URL comes from T's
   `.gitmodules` (synced into the config).
5. **Each of those submodules is restored the same way**, with its gitlink
   in R's index (T's, or a kept staged pin) as its target, recursively at
   any depth.

So the subtree shows `in sync` everywhere, except for states restore does
not change: `unpushed`, behind/ahead of the ref, and `detached HEAD on pin`
where no branch is at the pin (see [Branches](#branches)).

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
- **Submodule detached at the target** (after the step above, or because it
  was already so, also when it was in sync): HEAD is put on a branch whose
  tip is the target, the first that fits of ([D28](decisions.md)):
  1. a local branch tracking the ref the UI shows (or named like it), e.g.
     local `main` tracking `origin/main`, also when the pin is behind or
     ahead of the ref;
  2. the ref itself, if its tip is the target and no local branch has its
     name: the local branch is created, tracking it;
  3. any other local branch, the first alphabetically;
  4. any other remote branch whose name no local branch has, created as a
     tracking branch; the ref's remote first, then alphabetically.

  If git refuses one (checked out in another worktree, a name conflict),
  the next is tried. If none fits, HEAD stays detached.

Restore never moves or deletes a branch. A local branch that is behind or
ahead of its remote branch is no candidate and is not fast-forwarded. The
only branches restore creates are tracking branches for remote branches at
the target, in steps 2 and 4.

### What can be lost

Restore discards work. The things it can lose:

| Lost | Where it comes from |
|---|---|
| Changed files | modified, added, removed, renamed files (`changed`) |
| Untracked files | `changed` (untracked) |
| Conflicts and in-progress operations | `conflict`; merge, rebase, cherry-pick, revert state |
| Commits | A detached HEAD off the pin whose commits are on no branch, tag or remote ref (only the reflog still has them) |
| Staged pins | Staged pins in a repository that restore moves to another commit (see [Target](#target)); with `X` all staged pins |
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
update-index --cacheinfo …        # put the staged pins back (only if HEAD did not move)
clean -ffd                        # -ff: untracked repositories too; ignored files stay
submodule sync
submodule update --init --checkout --force -- <not yet initialized children>
# then every gitlink of the index (ls-files -s) is restored the same way
```

`--checkout` overrides `submodule.<name>.update`, and the explicit paths
override `submodule.<name>.active`, so no child module is skipped.

For a submodule, after `submodule sync`, `attachBranch` puts a detached HEAD
on a branch: `symbolic-ref -q HEAD` (on a branch: done), `for-each-ref
--points-at=T refs/heads refs/remotes` (no branch at T: done), then the ref
is resolved like `Status` does, and the first candidate (see
[Branches](#branches)) that `switch` / `switch -c <local> --track <remote>`
accepts wins. The branch checked out before the restore is passed to the
ref resolution. A child's `.gitmodules` branch is read once per parent.

With `c` and `X` the parent's index gets the committed pin first
(`update-index --cacheinfo` in the parent). With `X` (`RestoreHard`) the
staged pins are not put back, and the loss check takes the children's
targets from `HEAD`'s tree instead of the index.

`RestoreLoss` (`lossOf`) follows the repositories on disk: per repository
`status` (without submodules), the in-progress state files, and
`rev-list --count HEAD --not --branches --tags --remotes` if HEAD is not the
target. A directory in the untracked list is searched for `.git`
*directories* (a standalone repository); an absorbed submodule leaves only a
`.git` file and loses just its worktree.

## Switch branch

Moves the selected repository to another branch, in one step: the tip of the
branch becomes the pin, and the repository is restored to it.

### The popup

`b` opens a popup in the middle of the screen: a search line on top, below it
the branches of the selection, local ones first, then remote ones
(`origin/…`), each group with the newest tip first. Each row shows the name
(`*` marks the checked-out branch), the short SHA and the age of the tip.
The list is read from the disk; fetch (`f`) is what brings new remote
branches.

| Key | Effect |
|---|---|
| text | Filters: every word must be in the name, case-insensitive. The selection stays on its branch if it still matches |
| `↑` `↓`, `pgup` `pgdn` | Select |
| `enter` | Switch to the selected branch |
| `esc` | Close |

Mouse: a click on a branch switches to it (like `enter`), the wheel moves the
selection, a click outside the popup closes it; a click on the title, the
search line or an empty row does nothing. Everything else is ignored while the
popup is open. A header selection switches the superproject.

### Effect

For a **submodule**, with the tip `T` the list showed:

1. The parent's index records `T` as the pin (a staged pin; if `T` is the
   committed pin, the index equals `HEAD` again).
2. The submodule, and everything below it, is [restored](#restore) to `T`.
3. HEAD is put on the branch (the branch name is the local one):
   - a local branch: checked out;
   - a remote branch `origin/feat`: the local branch is the one checked out
     before if it tracks `origin/feat` (e.g. `dev`), else `feat`. It is
     created, tracking it, if it does not exist; if it exists at `T` it is
     checked out; if it is only
     **behind** `T` (an ancestor of it) it is fast-forwarded to `T` and
     checked out ([D29](decisions.md)); if it has commits of its own, or is
     checked out in another worktree, it is **not moved** and HEAD stays
     detached at `T`.

For the **superproject** step 1 is left out: the branch is checked out via the
restore, so its submodules follow the pins of the new commit (staged pins of
the old state are lost, and counted).

Staging comes first, so a failed restore can be repeated with `x`. If the
branch moved since the list was read (a fetch), the switch aborts with
`origin/feat moved; open the list again`; the staged SHA is always the one
shown.

### Confirmation, network, errors

As for restore: the loss is read from the disk and the question
(`Switch src/app to origin/feat? Discards …   y switch · n cancel`) appears
only if something would be lost. A submodule that is not initialized can't be
switched (`src/app is not initialized; restore it first`). Network: only what
is missing below, like restore. Lines above the footer: `checking …`,
`switching src/app to origin/feat…`; flash `switched src/app to feat` (or
`… to origin/feat (detached: local feat can't be fast-forwarded)`); failure
`switch of src/app to origin/feat failed: …`, until the next key. One
restore or switch runs at a time.

### Implementation note

`git.Provider.Switch` (`internal/submodule/git/switch.go`): verify the ref is
still `T`; `update-index --cacheinfo` in the parent; `restoreRepo` (see
restore); then `switch` / `switch -c <local> --track <remote-branch>`; a local branch
behind `T` is first moved with `merge-base --is-ancestor <local> T`, a look
at `worktree list --porcelain` and `update-ref refs/heads/<local> T <old>`
(which fails if the branch moved meanwhile). The restore leaves HEAD of the
switched repository alone (Switch puts it on the chosen branch); the
submodules below it are attached as in a restore.
`SwitchLoss` is `lossOf` with `T` as the target. `Branches` is one
`for-each-ref`, without the `origin/HEAD` symref.

## Update pin

Moves the pin of a submodule to the tip of its ref, the branch the row shows
(`origin/main ↓2`). It is a [switch](#switch-branch) to that branch, with the
tip the row shows: nothing is fetched, so `u` gives exactly the commits the
row counts. `f` is what brings newer ones.

On the **superproject** the pin is HEAD and the ref is the upstream of its
branch (the header's `main ↓1`): `u` fast-forwards the branch to it, like
`git pull --ff-only`, and restores the submodules to the pins of the new
commit.

### Available when

All of:

- it is initialized and its state is loaded;
- its ref is a remote branch (not a tag, a SHA or a local branch) and has a
  tip; for the superproject: its branch has an upstream;
- the pin is **behind** the ref and not ahead (`Behind > 0`, `Ahead == 0`).
  A diverged pin (`↑n↓m`) is left to `b`: moving it would drop the pin's
  own commits from the record.

### Effect

The same as [switching](#switch-branch) to the ref: the tip is staged
as the pin, the submodule and everything below it are restored to it, and
HEAD is put on the local branch. A local branch that is only behind is
fast-forwarded, so the common case (local `main` at the old pin) ends on
`main`. A checked-out branch that tracks the ref under another name (`dev`
tracking `origin/main`) is the one used, so the update stays on it. For the
superproject staged pins are lost, as in a switch (and counted).

If the ref no longer points at the tip the row showed (a fetch meanwhile),
nothing happens: `can't update src/app: origin/main moved; check the new
state`. The watcher has updated the row by then; `u` again takes the new
tip.

### Confirmation, lines, errors

As for switch: the question appears only if something would be lost
(`Update src/app to origin/main? Discards …   y update · n cancel`). Lines
above the footer: `checking src/app…`, `updating src/app to origin/main…`;
flash `updated src/app to main`; failure `update of src/app to origin/main
failed: …`.

### Implementation note

The UI only: `startPinUpdate` (`internal/tui/pinupdate.go`) reads the
branches (`Switcher.Branches`), takes the one named like `Pin.Ref`, checks
that its tip still starts with the abbreviated `Status.Tip.SHA`, and then
runs the switch flow with it (`SwitchLoss`, `Switch`).

## Resolve pin conflict

Ends a [pin conflict](state-model.md#pin-conflicts): the parent of the
selected submodule is in a merge (rebase, cherry-pick, revert) in which
both sides moved its pin. `m` opens a popup like `b` with the pins to
choose from:

| Item | Note |
|---|---|
| ours, named after its branch (`main`) | `ours · origin/main ↓3`, `· unpushed`, `· not fetched` |
| theirs (`feat`) | the same with `theirs` |
| each commit that contains both, named after its branch (`origin/main`) | `has both · 1 behind the tip` |
| the checkout, if it contains both | `your checkout, has both` |
| `remove`, instead of a side that removed the submodule | `<side> removed the submodule` |

The first commit with both is preselected (a side that contains the other
gets `· has both` and is preselected), else ours.

### Effect

1. The chosen SHA replaces the three stages in the parent's index
   (`update-index --cacheinfo`): the conflict is resolved, the pin staged.
2. The submodule and everything below it are restored to it, as by
   [Restore](#restore): a commit that isn't fetched is fetched (the only
   network use), HEAD is put on a branch at it ([Branches](#branches)).
   If the restore fails, the pin stays staged and `x` repeats it.

`remove` only removes the gitlink from the index (`rm --cached`); the
directory and the `.gitmodules` entry are left to the user.

submarine does not merge inside the submodule and does not commit the
superproject. If no commit has both sides yet, merge them in the
submodule (in a shell), then `m` offers the checkout. When no conflict is
left, the header says `merging feat · ready to commit`.

### Confirmation, lines, errors

Like a switch: `checking libs/alpha…`, the question only if something would
be lost (`Resolve libs/alpha to main 88f9593? Discards …   y resolve · n
cancel`), `resolving libs/alpha to origin/main 32b19cd…`, flash `resolved
libs/alpha to origin/main 32b19cd`; `remove` reads `by removing it`.
Failure: `resolve of libs/alpha to … failed: …`. If the conflict was
resolved meanwhile (in a shell), the check fails with `has no pin conflict
(resolved meanwhile?)`.

During a pin conflict `x`, `X` and `u` refuse with `libs/alpha has a pin
conflict; m chooses its pin`: there is no pin to restore to or to update.
`b` works: the branch tip it stages resolves the conflict too.

### Implementation note

`submodule.Resolver` (`ResolveLoss`, `Resolve`), implemented in
`internal/submodule/git/resolve.go` with the restore's `lossOf` and
`restoreRepo`. The UI (`internal/tui/resolve.go`) builds the items from
`Status.PinConflict` and reuses the branch picker (`branches.NewChoices`)
and the restore flow (`restoreState.resolve`).

## Keys and palette

They are submodule keys (`keyMap.Submodule`), shown in the detail
pane's palette, or in the footer when the pane can't show it. They are
always offered: restore is idempotent, and `u` says why it can't update.
`u`'s label follows the selection: on the header it reads `pull
(fast-forward)`. `m` is only in the palette, first, for a submodule with a
pin conflict; it is always in the full help.

| Key | Action | Pressed when not possible (flash) |
|---|---|---|
| `x` restore to pin, `X` restore committed pins | Restore the selection; `X` see [Hard restore](#hard-restore) | `this data source can't restore` (mock); `wait for the restore of … to finish`; `src/app has a pin conflict; m chooses its pin` |
| `b` switch branch | [Switch branch](#switch-branch) | `this data source can't switch branches`; `wait for the restore of … to finish`; `src/app is not initialized; restore it first` |
| `m` resolve pin conflict | [Resolve pin conflict](#resolve-pin-conflict) | `this data source can't resolve pin conflicts`; `wait for the restore of … to finish`; `src/app has no pin conflict` |
| `u` update pin (`pull (fast-forward)` on the header) | [Update pin](#update-pin) | `this data source can't update pins`; `src/app has a pin conflict; m chooses its pin`; `wait for the restore of … to finish`; `the superproject has no upstream branch`; `src/app is still loading`; `src/app is not initialized; restore it first`; `the pin of src/app is not on a branch`; `the pin of src/app has diverged from origin/main; b switches the branch`; `the pin of src/app is ahead of origin/main`; `src/app is up to date with origin/main` (the superproject without `the pin of`); after the check `can't update src/app: main is a local branch; b switches to it` |

`R` would sit next to `r` (reload), a dangerous mix-up for a destructive
action, hence `x`.

## Testing

- Provider (`restore_test.go`), on fixtures from `scripts/make-fixture.sh`:
  a restored superproject is exact (every repository clean, `submodule status
  --recursive` without prefixes, `libs/missing` initialized), branches are
  not moved, ignored files survive, a merge and a rebase in progress end, a
  standalone clone and a removed submodule's directory are deleted, a missing
  submodule is cloned again, `update = none` and inactive submodules are
  restored, an unavailable commit is reported. A detached submodule ends on
  the local branch tracking the ref, else on the ref created as a tracking
  branch (before other branches at the pin), else on the first local
  branch; a local branch elsewhere is not moved; a checked-out branch stays;
  a branch of another worktree is skipped; nested submodules are attached
  too. The loss is empty for off pin,
  missing and unpushed submodules, counts files, nested changes, merges,
  detached commits (not those on a branch) and standalone repositories.
- UI (`internal/tui/restore_test.go`): restores at once without a loss, asks
  with one (`y` restores, anything else cancels), the mock can't restore.
- Switch: provider (`switch_test.go`): list without `origin/HEAD`, local
  first; a remote pick stages the tip and creates a tracking branch; a
  local branch that is only behind is fast-forwarded; one with commits of
  its own, or checked out in another worktree, is not moved; a moved ref aborts; the
  superproject switch ends exact; the loss is the restore's. Picker
  (`branches_test.go`): filter, selection, scrolling. UI (`switch_test.go`):
  opens, filters, enter switches, asks with a loss, esc closes, the mock
  can't.
- Update pin: UI (`pinupdate_test.go`): switches to the tip of the ref, asks
  with a loss, aborts if the ref moved, refuses (with its flash) a pin that
  is up to date, ahead, diverged, on a tag, on a local branch or not
  initialized; updates the superproject, not one without an upstream.
  Provider: the superproject's branch is fast-forwarded; a checked-out
  branch that tracks the remote branch under another name is kept.
- By hand in tmux on a fixture, see
  [architecture.md](architecture.md#testing).

## Open questions

Decided for restore: ignored files are kept (`clean` without `-x`); a
submodule is restored to the index gitlink, so a staged pin is its target and
survives; restore may clone and fetch what is missing; the key is `x`; it
asks only if something would be lost.

Open (update pin): updating several submodules at once (on the superproject
or a collapsed parent), deliberately left out for now.
