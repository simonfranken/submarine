# Requirements

## Problem

In repositories with many submodules, `git submodule status` shows pinned
commit hashes. It doesn't answer the questions users actually have:

- Which branch is this submodule on, and is the pin current?
- Which submodules need my attention, and which are fine?
- Is anything dangerous, e.g. a pin that other people can't fetch?

## Goals

- Show **only submodules** (not files), recursively nested, as a tree; the
  superproject itself is shown by the header, which is selectable.
- Describe pins as **ref + distance** instead of a commit hash.
- Let **problems stand out** and keep healthy submodules quiet.
- Work comfortably with **dozens to hundreds** of submodules.

## Functional requirements

### Tree

- **F1** Show all submodules of the superproject, including nested submodules
  of submodules, at any depth.
- **F2** Group submodules by the directory of their path relative to the
  parent repository. A directory with submodules is shown as a non-selectable
  header line (e.g. `src/`, `modules/`). Submodules at the repository root
  have no header.
- **F3** Nested submodules are shown under their parent submodule. A
  directory header inside a submodule (e.g. `modules/`) means the nested
  submodule lives in that subdirectory of the parent.
- **F4** On startup only the first level is shown; every submodule with
  nested submodules is **collapsed**.
- **F5** A collapsed submodule shows the number of its **direct** children,
  e.g. `[2]`.
- **F6** A collapsed submodule shows the number of issues **hidden** in all
  its descendants (`·1 ·2`), one count per level: errors in red, then
  warnings in yellow. A staged pin below shows as a blue `●` before them.
- **F7** Expansion state of descendants is remembered when a parent is
  collapsed and restored when it is expanded again (except after
  "collapse all", which resets it).

### State per submodule

- **F10** Each submodule shows at most **one** state word: the most severe
  issue, plus `+n` if there are more. Submodules without issues show a
  muted `in sync`, or a blue `staged` if their pin change is staged. With
  issues, a staged pin adds a blue `●` after the word.
  See [state-model.md](state-model.md) for the full rules.
- **F11** Each submodule shows its pin as a ref (`origin/main`, a tag, or a
  short SHA as a fallback) and the distance to that ref (`↑n`, `↓n`, `↑n↓m`).
- **F12** A pin that is not reachable from any remote ref ("unpushed") is
  the most prominent state.
- **F13** States load asynchronously per submodule; a submodule shows a
  spinner until its state is known. Loading errors are shown as `error`.

### Detail pane

- **F20** Shows the selected submodule (or the superproject): full path, name, all issues in
  words, staged-pin and detached-HEAD notes.
- **F21** Shows the resolved ref, how it was resolved, and the **pin** (and
  the **staged** pin, if there is one), the
  **tip** and the checkout (**you**) as one line of history, newest on top:
  a node per commit (SHA + subject) with its roles, and the number of
  commits between two nodes. Roles on the same commit share a node. A
  checkout that differs from the pin is highlighted.
- **F22** *(dropped)* The commits between pin and tip are not listed; only
  their number is shown (F21).
- **F23** Shows child count, issues below, and the remote URL.
- **F24** For a submodule with uncommitted changes, previews the changed
  files so the user can remember what they were doing: a flat list with a
  colored letter per file (`M` modified, `A` added, `D` removed, `R`
  renamed, `?` untracked, `!` conflict), line counts (`+n −m`) per file and
  in total, and how long ago each file was edited. Only a handful of files are
  shown; the rest is summarized as "n more" with their kinds. Staged and
  unstaged changes are combined.

- **F25** The superproject is selectable (the header; selected on startup)
  and has a state of its own, so a stale checkout shows up: files changed
  or untracked, untracked repositories (e.g. a submodule clone the current
  branch no longer has), conflicts, and HEAD not pushed. Its HEAD is compared
  with the upstream of the branch. Submodules do not count for it, their rows
  show them. The detail pane shows it like a submodule (HEAD and tip instead
  of pin and you, a summary of all submodules instead of Children).
  See [state-model.md](state-model.md#superproject).

### Header and footer

- **F30** The header shows the superproject path, its branch and
  ahead/behind, its own state word (**F25**), the number of submodules, a count per issue kind (most
  severe first), staged pins, how many are still loading, and the time since
  the last fetch.
- **F31** The footer shows key hints; `?` toggles the full help. It
  temporarily shows short messages (e.g. "copied <path>").

### Navigation

- **F40** Keyboard navigation as specified in
  [ux-guide.md](ux-guide.md#navigation).
- **F41** Mouse: click a row to select it, click `▸`/`▾` to toggle, scroll
  wheel moves the selection.

### Fetch and refresh

- **F50** Nothing is fetched on startup. The detail pane shows
  when the selected repository was last fetched (`FETCH_HEAD`), whoever did it.
  `f` fetches the superproject and every initialized submodule in the
  background (all remotes, with prune).
- **F51** While fetching, the header shows progress (`fetching 12/174`)
  instead of the fetch age; afterwards the age and the number of failed
  fetches. The detail pane shows each submodule's last fetch and, if it
  failed, git's message.
- **F52** The view follows changes in the repositories, no matter who made
  them: a fetch, commit, checkout or `git add` in another shell shows up
  within about two seconds. Refreshing is driven by the repositories, not by
  the application's own fetch.
- **F53** `f` fetches again; `r` reloads everything (e.g. after editing
  files, which is not watched).

- **F54** `/` filters the tree by name/path (case-insensitive substring).
  A text input above the footer takes the term; `esc` closes it and removes
  the filter. A selection that is filtered out moves to the next visible
  submodule (none if nothing matches); closing the filter keeps the
  selection (the superproject if none is left). See [Filter](ux-guide.md#filter).

- **F55** `!` toggles "problems only": the tree shows only submodules with
  an issue (**F10**) or an unknown state (loading, `error`), plus their
  ancestors, expanded. It combines with the filter (**F54**) and stays when
  the filter closes. A line above the footer says `problems only`, so a short
  list is not mistaken for a healthy repository. See
  [Problems only](ux-guide.md#problems-only).

### Actions

- **F56** `x` restores the selection (a submodule, or the superproject when
  the header is selected): its worktree becomes exactly what git records, and
  so do all submodules below it, recursively. Staged pins are kept at every
  depth. For a submodule with a staged pin it asks whether to restore to the
  staged or the committed pin (the latter unstages it). It asks for
  confirmation only if something would be lost. See [actions.md](actions.md#restore).

- **F57** `b` opens a popup in the middle of the screen with a search line
  and the local and remote branches of the selection (a submodule, or the
  superproject). `↑` / `↓` select, `enter` switches, `esc` cancels. For a
  submodule the branch tip becomes its staged pin and it is restored to it,
  with everything below; HEAD ends on the branch. It asks for confirmation
  only if something would be lost. See [actions.md](actions.md#switch-branch).

Stage pin is drafted in [actions.md](actions.md#stage-pin).

## Usage

```sh
submarine [dir]     # the repository containing dir (default: current directory)
submarine --mock    # built-in mock data
```

## Non-functional requirements

- **N1** Startup must not block on the network. The tool shows the local
  state at once and uses the network only when the user asks (F50).
- **N2** Loading many submodules must not freeze the UI (async, per
  submodule).
- **N3** Colors follow the user's terminal theme (ANSI 16 colors); the UI
  works on dark and light backgrounds.
- **N4** Meaning never depends on color alone: every state has a word.
- **N5** Works without a Nerd Font. `SUBMARINE_ASCII=1` replaces the few
  remaining Unicode symbols with ASCII.
- **N6** Works in narrow terminals (detail pane hides below 90 columns).
- **N7** The UI is independent of the data source: the mock and the git
  provider are interchangeable without UI changes.
- **N8** The git provider only uses the network in `Fetch` and, for objects
  that are missing, in `Restore` and `Switch`. It never prompts (no password, passphrase
  or host-key questions; those fail with git's message). It uses the `git`
  command line, so the user's git configuration applies. Apart from fetching
  and the actions of [actions.md](actions.md), it is read-only: it writes
  only when the user asks.
