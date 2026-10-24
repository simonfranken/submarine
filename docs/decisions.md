# Decision log

Each entry: context, decision, consequences. Newest last.

## D1 — Show pins as ref + distance, not hashes

**Context.** Hashes don't tell users whether a pin is current.
**Decision.** Resolve each pin to a ref and show the distance (`origin/main ↓1`).
The resolution order is fixed and the applied rule is shown in the detail pane
(see [state-model.md](state-model.md#resolving-a-pin-to-a-ref); amended:
the closest branch wins, see "Pin resolves to the closest branch").
**Consequences.** Resolution is a heuristic; showing the rule keeps it
transparent.

## D2 — Compare three commits: pinned, checked out, tip

**Context.** The first mockup showed one branch for both the checkout and the
pin, so a mismatch was invisible.
**Decision.** Model all three commits. The tree's ref column always describes
the **pin**; checkout drift is an issue (`off pin`).
**Consequences.** The detail pane shows all three (see D25).

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
**Decision.** Collapsed parents show `·k` (issues in all descendants), one
count per level: red for danger, then yellow for warn (amended: a single
count in the worst color made 1 error + 1 warning look like 2 errors; the
status column grew from 14 to 16 for the second count). Problems in a child
do **not** mark the parent itself as `changed`.

## D8 — Collapsed by default, branch-first navigation

**Decision.** Only the first level is visible. `→` expands and enters the
first child, `←` collapses and goes to the parent, `space` toggles,
`shift+→/←`, `L`/`H` (or `ctrl+→/←`) expand/collapse all (see [ux-guide.md](ux-guide.md#navigation)).
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

## D18 — Background fetch on startup (superseded by D22)

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
on `update` and a background check at startup that can be disabled (it
was once a day; see D27).

## Copy path with `c` via OSC 52

`c` copies the selected submodule's absolute directory (optional
`submodule.Locator`; the relative path as fallback). The absolute path is
what a shell needs (`cd <paste>`). OSC 52 instead of a clipboard tool
(`xclip`, `pbcopy`): no external dependency and it works over SSH. The cost:
terminals that block OSC 52 copy nothing silently; the footer still says
`copied`, since the application can't know.

## Key groups: global, navigation, submodule

Keys that act on the selected submodule (copy path, later checkout, bump) are
shown in a palette in the detail pane, next to what they act on; global keys
(`?`, `q`) sit right in the footer. Where the pane can't show the palette
(hidden below 90 columns, or under 12 lines) the footer shows these keys
instead, so small terminals need no extra UI (no overlay or action menu).
Keys never depend on what is visible.

## Pin resolves to the closest branch (superseded by D26)

A pin on a pushed feature branch showed as diverged from `origin/main`,
because `branch = main` in `.gitmodules` won before any distance was looked
at. Now the closest branch containing the pin wins; the `.gitmodules`
branch, the upstream and the remote default branch only break ties (and are
always compared, even past the 16-branch cap). An unpushed pin compares only
those preferred refs, by commits on either side: scanning every branch's
merge base would be too slow. Cost: a pin on main far behind its tip shows a
younger branch that contains it if that branch's tip is closer, so "n behind
main" is no longer visible there; `via <rule>` still tells which ref it is.

## Filter: matches with ancestors, lives as long as the input

`/` filters by case-insensitive substring of the submodule path. Matches
keep their ancestors, shown expanded, because a nested submodule alone
would lose its context; the user's fold state is not touched, so closing the
filter restores it (plus the selection's ancestors, so the selection stays
visible). The filter exists exactly while the input line is open, and `esc`
closes both, so there is no hidden filter that makes the list look
incomplete. While typing, `j`/`k`/`q` are text; `enter` returns the keys to
the tree and keeps the filter, so a filtered list can be navigated with the
usual keys. A filtered-out selection moves to the next visible submodule
(previous if none follows) instead of the first, so the cursor stays near
where it was. This adds the `bubbles/textinput` dependency (and its
`atotto/clipboard` import).

## Staged pin is shown in the tree, but is not an issue

A staged pin was only visible in the detail pane, so the header's `1 pin
staged` could not be traced to a row, and `in sync  nothing to do` was
wrong for a submodule whose pin change still has to be committed. The tree
now shows it in the status column: blue `staged` replaces `in sync`; next to
an issue word it is a blue `●` (an issue wins the word, since the cell holds
one word, D3). It is not counted in `+n`, because `+n` and the header count
issues and a staged pin stays quiet (D4). A collapsed row shows `●` if a
staged pin is hidden below it (`Hidden.Staged`), placed before the `·k`
counts so truncation of the 16-column cell cuts the counts first.

## Superproject is selectable in the header

The state of the root repository was invisible: a submodule the current
branch no longer has stays on disk as an untracked repository, root files
can be modified, HEAD can be unpushed. The root becomes a node with an
empty path (the convention `Fetcher`/`Change` already use), selected on
startup, and the **header** is its row: it already holds the root's path,
branch and ahead/behind, and a tree row would cost a line and an indent
level for an item that is not part of the tree. Selecting the header shows
the usual detail pane; the header also shows the root's state word, so a
problem is visible while a submodule is selected.

- *Changes ignore submodules* (`--ignore-submodules=all`): off-pin and staged
  pins already have their rows; counting them again would make the root
  `changed` in most real repositories.
- *Unpushed stays Danger*: same issue kind and level as a submodule, one
  rule for "work that exists only here" (D4).
- *The ref is the upstream*, not the closest branch: the header shows
  ahead/behind of the upstream, and "up to date" means relative to the
  tracked branch. A nearer feature branch would make the two disagree.
  Without an upstream the usual resolution applies.
- The root is not in `Walk`, so counts, fetch and loading logic stay
  submodule-only; the app tracks it by the path `""`.

## Problems only reuses the filter's tree reduction

`!` shows only submodules with issues. Instead of a second view, the tree's
filter got a second criterion: a submodule matches if its path contains the
term *and*, with problems only, it has an issue. Ancestors of matches stay as
context, expanded, so a danger deep in the tree is reachable without
unfolding anything. Unlike the filter text it is a mode that outlives the
filter line, so it has its own notice line above the footer: a short list
must not look like a healthy repository (D4). Loading and failed submodules
count as problems because their state is unknown (D7 counts `error` as danger
too); the cost is that rows vanish as statuses arrive, so the selection is
refitted after each status. Staged pins are not shown by it, since they are
not issues.

## D22 — Fetch only on request

**Context.** D18 fetched every repository on startup. With hundreds of
submodules that is hundreds of network round trips (and credential or
host-key failures) every time the tool opens, whether or not the user wants
fresh remote data.
**Decision.** Supersedes D18. Nothing is fetched on startup. The detail pane
shows the last fetch of the selected repository (`FETCH_HEAD`, also from fetches made in
other shells), so the user can judge how stale the remote refs are and press
`f` to fetch. The `--no-fetch` flag is removed with it (it still had the
side effect of switching off the update check; that has
`SUBMARINE_NO_UPDATE_CHECK`).
**Consequences.** Right after startup, `unpushed`, `↓n` and the like are as
fresh as the last fetch. `fetched never` appears for a repository that was
never fetched. The progress display and per-submodule fetch errors stay as
they are, they now belong to `f`.

## D23 — Restore: exact worktree, asks only for losses, may fetch what is missing

**Context.** The first write action had to give "the exact version of the
project my root commit specifies" in one key, without destroying work by
accident.
**Decision.** `x` restores the selection: reset to the target (HEAD for the
superproject, the index gitlink for a submodule, so a staged pin is kept and
used), remove untracked files and repositories, end merges/rebases, and do
the same for every submodule of the target tree, initializing missing ones.
- *Ignored files stay* (`clean` without `-x`): `.env`, `node_modules` and
  IDE settings are not "additional files" worth deleting in every submodule.
- *Branches never move.* A checkout that is not on the target is detached
  first, so commits stay on their branch. (A submodule is then put on a
  branch at the target if there is one, see D28.)
- *It asks only if something is lost* (changed or untracked files,
  operations in progress, commits on no ref, standalone repositories).
  Never asking would delete repositories that exist nowhere else; always
  asking would train a reflexive `y` on the harmless cases. The loss is read
  from the disk when the key is pressed, not from the loaded statuses.
- *Network.* Supersedes the "network only in `Fetch`" rule (N8): restore may
  clone a missing submodule and fetch a missing commit, non-interactively and
  with a timeout, because "no child module missing" cannot be met offline.
- Key `x`, not `R`, which sits next to `r` (reload).
- *Staged pins are kept, at every depth.* The first version reset the
  superproject's index and so dropped staged pins silently (they were not
  counted as a loss). A staged pin is a decision, not drift: each
  repository's staged pins are put back after its reset and are the targets
  of its submodules. Unstaging gets a place instead: for a submodule with a
  staged pin, `x` asks *staged* (`s`) or *committed* (`c`, unstages it). That
  asks only where the intent is ambiguous and needs no extra key. Staged pins
  of a repository that moves to another commit can't be carried over and
  count as a loss.
**Consequences.** `Restorer` is a new optional provider interface; the mock
does not implement it. Restore is idempotent and can be repeated after a
failure. It takes the time of its slowest clone; there is no per-repository
progress yet.

## D24 — Switch branch: stage the tip, then restore

**Context.** Switching branches was left to the shell. Users of a repository
with many submodules want to move one to another branch and have the
superproject record it.
**Decision.** `b` opens a popup (search line, local then remote branches).
Enter on a branch of a submodule stages its tip as the pin in the parent's
index and restores the submodule to it (D23, recursion included); then HEAD
is put on the branch. On the superproject it checks the branch out and
restores below, without staging.
- *Key `b`*, not `s`, which stays reserved for stage pin (dropped, D29).
- *Remote branch.* A local branch of that name is created, tracking it. If
  one exists at another commit it is **not moved**: HEAD stays detached at
  the tip and the flash says so (restore never moves branches, D23).
- *Same loss rules as restore.* The loss is read from the disk; the question
  appears only if something is lost. Staged first, so a failed restore can be
  repeated with `x`. The pin question of restore (`s` / `c`) is skipped: the
  staged pin is being replaced.
- *What the picker showed is what is staged.* The branch carries its SHA; a
  fetch may move the ref meanwhile, then the switch aborts
  (`origin/feat moved; open the list again`).
- *Network.* Extends D23: a tip often pins nested commits that are not on
  disk, so switch may clone and fetch what is missing, like restore. The
  list itself uses none; `f` refreshes remote branches.
- *Popup.* The first overlay: drawn over the finished view (after
  `zone.Scan`, so no zone marker is cut); the keys go to the picker while it
  is open, and so does the mouse: the popup remembers where it was drawn and
  tests clicks itself (bubblezone can't see it, it is drawn after the scan).
  A click on a branch switches, like `enter`; a click outside closes. The backdrop is dimmed: stripped of its styles and drawn in
  `Backdrop`, an adaptive 256-color grey (238 dark / 252 light). `Muted`
  (ANSI 8) was too bright to set the popup apart, and ANSI faint (SGR 2) is
  ignored by some terminals. Like `Bar` and `Selected` it carries no meaning,
  so it may leave the 16 ANSI colors.
**Consequences.** `Switcher` is a new optional provider interface; the mock
does not implement it. The switch runs as the restore does (one write
operation at a time, same lines above the footer).

## D25 — Detail pane: a spine instead of three commit rows and a list

**Context.** `Pinned` / `Checkout` / `Tip` were three rows of SHAs to
compare, the distance sat on the `Ref` line, and a list of up to 50 commits
(two lines each) pushed the rest out of view. Where the pin, the checkout and
the tip are had to be worked out by reading.
**Decision.** Draw them as one line of history, newest on top: a node per
commit, the roles in the label column, the number of commits between two
nodes in a gap row. Roles on one commit share a node. The commit list is
dropped (no cap, no popup): the gap count says how far, not what changed.
The `Ref` line loses the distance suffix (the gap says it) and keeps
`via <rule>`.
**Consequences.** `Status.Between` is gone; `Status.Drift` (checkout ↔ pin,
one `rev-list --left-right --count`, only when off pin) and `Status.Spine()`
are new. Author and age of commits are no longer shown anywhere in the pane.
Diverged tip or checkout show `diverged ↑n ↓m` in the gap; drawing a real
fork needs the merge base and is left for later.

**Amendment (with D25).** The header no longer shows `fetched <age>`. It was
the superproject's own fetch, which the tree's `↓n` do not depend on, and read
as the age of everything. The age lives only in the detail pane, right-aligned
in the name's line (the superproject's when the header is selected); the
header keeps the progress and `n failed`. `Repo.FetchedAt` is gone.

**Amendment (with D25).** A staged pin shows both pins on the spine: `staged`
(the index) and `pin` (the superproject's HEAD, `Status.Committed`), so a
bump reads as a distance. Before, only the index pin was shown at all.

**Amendment (with D25): forks.** A diverged tip, checkout or committed pin is
drawn as a second lane that joins the main one at a `base` node (SHA +
subject of `git merge-base`), instead of a straight line with
`diverged ↑n ↓m`. The base was chosen over a bare join mark: where the split
happened is what decides between a merge and a rebase. Cost: one
`merge-base` and one `log -1` per diverged commit, only when it is diverged.
Only one fork is drawn; with two diverged commits (rare) the old line with
the diverged gap remains, since nodes on three lanes would need the
merge bases between all of them.

## D26 — The ref is the target branch; a pin off it is `off target`

**Context.** The main use is checking that a superproject points at the right
commits of its submodules, ideally the tips of `main`. D1's "closest branch"
(see "Pin resolves to the closest branch") answered *where the pin is*, not
*where it should be*: a pin on a feature branch showed that branch, `in sync`
and quiet, because `unpushed` (D6) is satisfied by any remote branch. The
closest branch also changed with the branch checked out in the submodule, so
two machines could show different refs for the same superproject commit.
**Decision.** Supersedes "Pin resolves to the closest branch". The ref is the
target: the `branch =` of `.gitmodules` (no fallback to the default branch if
it does not exist locally), else the remote's default branch (`HEAD`, `main`,
`master`). Without a target the checked-out branch's upstream, a tag or the
SHA are shown for orientation, but cannot make a pin off target. A pushed pin
with commits its target lacks (`Ahead > 0`) is `off target`: a new issue with
the new level `LevelInfo` (blue), listed last. The other branches that contain
the pin are named in its text.
**Alternatives.** Severity by the superproject's branch (strict on `main`,
quiet elsewhere): rejected, the severity should not depend on the branch.
Matching the superproject's branch name in submodules: rejected, feature
branches need not be named alike. Warn level: not now; behind the target is
quiet and `off target` is information, since a feature branch of the
superproject legitimately pins feature commits.
**Consequences.** `RefFromContains` and the 16-branch scan are gone;
`Pin.Branches` replaces them. `off target` is counted in the header and below
collapsed rows (blue `·k`) and is part of "problems only". The issue word
needs 10 columns, so the detail pane's label column is 11 wide.

**Amendment (with D26): target and ref are separate.** Checking a real
repository, a stale `branch = marco-dev` in `.gitmodules` made every pin
"↑52↓12 from marco-dev" although the pin was on `main`, one commit behind. The
target (`PinInfo.Target`) now only decides `off target`; the ref shown is the
target if the pin is on it, else the branch containing the pin that is closest
to it, with its real distance. The closest branch is the one with the fewest
commits after the pin; ties go to the upstream of the branch the submodule is
on, then alphabetically. The default branch is no favorite: if
`.gitmodules` targets another branch, `main` is just another branch. The
tie-break must be fixed, or the same superproject commit would show
different branches on different machines. The text of the issue names both
(`pin not on origin/marco-dev, on origin/fix/x +1`), so a stale declaration
is visible. `RefFromContains` is back; `RefSource.IsTarget` is gone.

**Amendment (with D26): the target on the spine.** An `off target` row only
said that the pin is elsewhere, not how far from the target. The target's tip
is now a node (`target`) next to the pin, the tip of the shown ref and the
checkout, so a feature pin reads as a distance from main. The shown ref
contains the pin, so only the target can fork from it, and the fork code
needs no new case. Cost: `log -1`, one `rev-list --left-right --count` and,
if diverged, one `merge-base` per off-target submodule.

**Amendment (with D26): labels.** The target's tip is labelled with its short
branch name (`marco-dev`) instead of the word `target`: it is usually short,
and the name is what people know. Long names are cut at 16 columns, so the
label column cannot grow without bound. In a label every role word has its
own style, and `you` is bold in the default color: it was lost between the
muted words, and it is the one the user looks for.

## D27 — Update check on every start, offered as "quit and update"

**Decision.** The registry is asked on every start, in the background, with
no cache. A newer release shows up in the footer as `q quit and update to
vX`, right of help and quit, on a highlighted background (black on cyan).
While it is shown, `q` quits and updates, and plain quit is `Q`. `ctrl+c`
always quits without updating. The update runs after the TUI has exited.
**Rationale.** A once-a-day check with a notice printed after exit was
easy to miss. The hint is only useful where the user is looking, and the
key they use to leave is the natural place. Running the install after the
TUI exits keeps the terminal clean and reuses `submarine update`'s code.
**Consequences.** Every start makes one small request to the registry (3 s
timeout, failures are silent), which supersedes the once-a-day rule of D21.
N1 still holds: the TUI never waits for it. `q` changes meaning while an
update is available, so muscle memory that exits with `q` now updates; the
bar says so. Typing `q` in the filter or a popup is unchanged.

## D28 — Restore puts a submodule on a branch at its pin

**Context.** After a restore (for example after the superproject's branch
changed) every submodule was a detached HEAD, so IDEs showed only hashes.
**Decision.** Amends D23. After restoring a submodule, at any depth, a
detached HEAD is put on a branch whose tip is the pin, in this order: (1) a
local branch tracking the shown ref (or named like it), (2) the ref itself
as a new local tracking branch, if no local branch has its name, (3) any
other local branch, alphabetically, (4) any other remote branch whose name
no local branch has, as a new tracking branch, the ref's remote first. A
HEAD already on a branch stays there. If git refuses a branch (checked out
in another worktree, a name conflict), the next one is tried; if none fits,
HEAD stays detached and the restore still succeeds. Submodules that were in
sync are included. The superproject is not affected.
**Rationale.** Attaching a branch whose tip is the pin changes no commit and
no file, so nothing can be lost and the loss question stays the same. The
ref comes first because it is what the UI shows; a local branch beats a new
one so restore adds as few branches as possible.
**Alternatives.** Fast-forwarding a local branch that is behind its remote
branch at the pin: rejected, restore still never moves a branch. Staying
detached when several branches fit: rejected, the order above always gives
one answer, the same on every run.
**Consequences.** Restore now may create local branches (only for remote
branches at the pin whose name has none). Per detached submodule it costs a
`symbolic-ref` and one `for-each-ref --points-at`; only if a branch is at
the pin, the ref is resolved (as for `Status`) and one `switch` runs. No
network. `detached HEAD on pin` is now left only where no branch is at the
pin. Switch keeps its own rule for the repository it switches; the
submodules below it follow this one.

## D29 — Update pin instead of stage pin; switch fast-forwards a branch behind

**Context.** Stage pin (record the checkout as the pin) was drafted, but a
commit made in a submodule is on a branch, and switching to that branch
(`b`) already stages it. The common job is a different one: a pin that is
behind its branch (`origin/main ↓2`) should move to the tip. With `b` that
is a search in the popup, and it often ended detached, because the local
`main` sat at the old pin and was never moved (D24).
**Decision.** `u` updates the pin of a submodule to the tip of its ref, only
when the pin is behind it and not ahead. It is a switch to that branch (D24)
with the tip the row shows: nothing is fetched, and if the ref moved
meanwhile nothing happens. Switch (and so `u`) now fast-forwards a local
branch that is an ancestor of the tip, unless a worktree has it checked out;
`update-ref` with the old tip makes the move fail if the branch moved. Stage
pin is dropped.
- *No fetch.* The user sees `↓2` and gets those two commits; a fetch first
  would update to something the screen never showed. `f` is separate.
- *Only behind.* A diverged pin has commits the ref lacks; moving it would
  drop them from the record, so that is a deliberate `b`.
- *The superproject too.* Its pin is HEAD and its ref the upstream, so `u`
  there is `git pull --ff-only` plus a restore of the submodules. Switch
  keeps a checked-out branch that tracks the chosen remote branch under
  another name (`dev` tracking `origin/main`), so an update never changes
  the branch.
- *One at a time.* Updating several at once is left for later.
- *Fast-forward only.* A local branch with commits of its own is never
  moved, as before. Restore (D23, D28) still never moves a branch: it goes
  back to the recorded pin, where moving branches has no place.
**Alternatives.** Keeping stage pin: it covers only a commit on no branch,
which then is `unpushed` anyway. `u` fetching first (a real pull): rejected,
see above. Resetting a local branch with commits of its own: loses them from
the branch.
**Consequences.** Amends D24 ("not moved") and D28's note that switch keeps
its own rule. No new provider interface: `u` is `Branches`, `SwitchLoss` and
`Switch`. Since `Status.Tip.SHA` is abbreviated, the UI compares it as a
prefix of the branch's full SHA.

## D30 — Hard restore drops staged pins at every depth

**Context.** `x` keeps staged pins at every depth (D23), except in a
repository the restore moves. Its `c` answer unstages only the selected
submodule's pin. Staged pins of the superproject, or of nested repositories
that stay on their pin, could only be dropped one by one.
**Decision.** Amends D23. `X` is a hard restore: the selected submodule goes
to its committed pin without the `s` / `c` question, and every restored
repository's index is reset to `HEAD` (the superproject's too), so all
staged pins below are dropped and the submodules follow the committed
trees. Dropped staged pins are counted in the loss and listed if the
question appears, but alone they don't make it appear. `x` and its question
stay as they are.
**Rationale.** A separate key keeps the common `x` free of a further
question and makes the stronger action deliberate. A staged pin is only a
recorded SHA; its commit stays in the submodule, and if HEAD leaves a
commit no ref reaches, `commits on no branch` still asks.
**Alternatives.** A third answer in the `s` / `c` question: only reachable
for a submodule with a staged pin itself, not for the superproject or a
clean parent. Dropping the question from `x`: rejected, the choice between
staged and committed pin is still useful there.
**Consequences.** `RestoreTarget` gets `RestoreHard`, which also matters for
the superproject. `restoreRepo` and `lossOf` take whether to keep staged
pins; switch always keeps them. Without a committed pin (an added
submodule), `X` uses the index pin.

## D31 — Hard restore asks before dropping staged pins

**Context.** D30 let `X` drop staged pins without a question: pressing `X`
was the choice. In a usability walkthrough (see
[notes/usability-plan.md](../notes/usability-plan.md)) `X` on `libs/staged`
unstaged its pin silently. `X` is one Shift away from `x`, and users new to
submodules don't know where to find the dropped commit again.
**Decision.** Amends D30. Dropped staged pins are a loss like any other and
make `X` ask: `Restore libs/staged to the committed pins? Discards 1 staged
pin.` `RestoreLoss` with `RestoreHard` now also counts the selected
submodule's own staged pin (in its parent's index), which `lossOf` doesn't
see. The `c` answer of `x` stays silent: the user picked it in a question.
**Consequences.** `X` asks more often, but only where it changes what is
recorded. Without staged pins or other losses it still runs at once.

## D32 — The screen explains itself

**Context.** The target user is a developer who doesn't know submodules
well and doesn't read manuals. The walkthrough found labels that don't say
what a key does (`x restore`, `X hard restore`), important keys found only
in `?` (`!`, `/`, `f`), and status words explained nowhere.
**Decision.**
- Labels name the target: `x restore to pin`, `X restore committed pins`.
  "Hard" suggested "discards changes", which both do.
- A fourth key group, Tools (`!`, `/`, `f`), in the footer after the
  navigation keys, dropped before them.
- On the header, `u` reads `pull (fast-forward)`: the label follows the
  selection, the key doesn't.
- The full help gets titled columns, and a second page, a legend of the
  status words, symbols and spine roles in their colors. Issue levels move
  to `IssueKind.Level()` so the legend can't drift from the tree. `esc`
  closes either page: cycling back with `?` alone was not obvious.
- A start hint above the footer until the first key.
- `fetched <age> · f fetch` in the detail pane when the last fetch is older
  than a day.
**Alternatives.** A first-run screen or tutorial: needs state, and is
skipped. Contextual palettes that grey out impossible keys: larger, kept for
later (item 10 of the usability plan).
**Consequences.** `footer.Keys` drops `help.KeyMap` for `Tools`,
`HelpGroups` and `LegendKeys`; the footer renders the full help itself.
