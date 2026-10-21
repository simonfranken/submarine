# Requirements

## Problem

In repositories with many submodules, `git submodule status` shows pinned
commit hashes. It doesn't answer the questions users actually have:

- Which branch is this submodule on, and is the pin current?
- Which submodules need my attention, and which are fine?
- Is anything dangerous, e.g. a pin that other people can't fetch?

## Goals

- Show **only submodules** (not files), recursively nested, as a tree.
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
  its descendants (`·2`), colored by the most severe hidden issue.
- **F7** Expansion state of descendants is remembered when a parent is
  collapsed and restored when it is expanded again (except after
  "collapse all", which resets it).

### State per submodule

- **F10** Each submodule shows at most **one** state word: the most severe
  issue, plus `+n` if there are more. Submodules without issues show no word.
  See [state-model.md](state-model.md) for the full rules.
- **F11** Each submodule shows its pin as a ref (`origin/main`, a tag, or a
  short SHA as a fallback) and the distance to that ref (`↑n`, `↓n`, `↑n↓m`).
- **F12** A pin that is not reachable from any remote ref ("unpushed") is
  the most prominent state.
- **F13** States load asynchronously per submodule; a submodule shows a
  spinner until its state is known. Loading errors are shown as `error`.

### Detail pane

- **F20** Shows the selected submodule: full path, name, all issues in
  words, staged-pin and detached-HEAD notes.
- **F21** Shows the resolved ref, how it was resolved, and three commits:
  **Pinned**, **Checkout** and **Tip** (SHA + subject). A checkout that
  differs from the pin is highlighted.
- **F22** Lists the commits between pin and tip (SHA, subject, author, age)
  with a title stating ahead/behind/diverged.
- **F23** Shows child count, issues below, and the remote URL.

### Header and footer

- **F30** The header shows the superproject path, its branch and
  ahead/behind, the number of submodules, a count per issue kind (most
  severe first), staged pins, how many are still loading, and the time since
  the last fetch.
- **F31** The footer shows key hints; `?` toggles the full help. It
  temporarily shows short messages (e.g. "not implemented").

### Navigation

- **F40** Keyboard navigation as specified in
  [ux-guide.md](ux-guide.md#navigation).
- **F41** Mouse: click a row to select it, click `▸`/`▾` to toggle, scroll
  wheel moves the selection.

### Actions (planned, not implemented)

| Key | Action |
|---|---|
| `u` | Check out the pinned commit (`git submodule update`) |
| `b` | Bump the pin to the tip of its ref |
| `f` | Fetch remotes |
| `/` | Filter by name/path |
| `!` | Show only submodules with issues |

## Non-functional requirements

- **N1** Startup must not block on the network. The tool works from local
  refs and shows how old they are ("fetched 2m ago"); fetching is explicit.
- **N2** Loading many submodules must not freeze the UI (async, per
  submodule).
- **N3** Colors follow the user's terminal theme (ANSI 16 colors); the UI
  works on dark and light backgrounds.
- **N4** Meaning never depends on color alone: every state has a word.
- **N5** Works without a Nerd Font. `SUBMARINE_ASCII=1` replaces the few
  remaining Unicode symbols with ASCII.
- **N6** Works in narrow terminals (detail pane hides below 90 columns).
- **N7** The UI is independent of the data source: replacing the mock
  provider with a git provider requires no UI changes.
