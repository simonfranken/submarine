# UX guide

## Principles

1. **Attention follows severity.** Healthy submodules are quiet, problems are
   loud, dangers are loudest. Users should be able to scan a long list and
   only see what needs action.
2. **One word per row.** At most one state word in the tree (`+n` for more);
   the detail pane lists everything.
3. **Words over symbols.** States are short words that need no legend.
   Symbols are only used where they are conventional (`↑↓`, `▸▾`).
4. **Meaning never depends on color alone** (every state has a word).
5. **Follow the user's terminal theme.** Use ANSI 16 colors for meaning.

## Layout

```
 ~/coding/my_project  main        1 unpushed · 1 missing · 2 off pin · 2 changed · 13 submodules 
   docs                                  origin/main                     │ src/
 misc/tools/                                                             │ my_submodule_3
     my_submodule_1                      origin/main                     │
     my_submodule_2       missing        a1b2c3d                         │ unpushed  pin not pushed to any remote
 src/                                                                    │
     my_submodule_1       off pin        origin/main                     │ Ref       origin/main · via checked out branch
   ▸ my_submodule_2 [2]   changed ·2     origin/main                     │ pin · you ● b7c8d9e Hotfix null pointer
     my_submodule_3       unpushed       origin/main    ↑1               │           ┊ 1 commit
 libs/                                                                   │ tip       ● 6f5e4d3 Release 2.1.0
     logging                             origin/main    ↓3               │
 ↑↓/jk move • ←→/hl fold • ⇧←→/HL fold all • space toggle • ? help • q quit
```

| Area | Height / width |
|---|---|
| Header | 1 line |
| Tree | Remaining width |
| Detail pane | 40% of the width, at least 36 columns; hidden below 90 columns |
| Filter line | 1 line above the footer, only while filtering |
| Footer | 1 line (more with full help) |

### Key bar and palette

Keys come in three groups (`keyMap.Navigation`, `.Submodule`, `.Global`):

- **Navigation** (move, fold, toggle): left in the footer.
- **Global** (`?`, `q`): right-aligned in the footer, always kept.
- **Submodule** (act on the selected one, e.g. `c`): a palette on the last
  line of the detail pane. It is left out if the pane is shorter than 12
  lines. Whenever the palette isn't shown (pane hidden or too short), these
  keys move into the footer after the navigation keys.

If the footer is too narrow, keys are dropped from the left group's end
(submodule keys first); the global keys stay. Full help (`?`) lists all keys
in columns, whatever the size. Keys work whether or not they are shown.

### Tree row

```
indent ▸ name [n]   status          ref             delta
```

| Column | Width | Content |
|---|---|---|
| indent | 2 per depth | |
| toggle | 3 | `▸` collapsed, `▾` expanded, blank for leaves |
| name | fits the widest visible name | name, `[n]` = direct children (collapsed only) |
| status | 16 | issue word, `+n`, `●` staged pin, `·k` hidden issues per level (red, then yellow); blue `staged` or muted `in sync` without issues; spinner while loading |
| ref | widest ref, max 22 | resolved ref; muted short SHA if unresolved |
| delta | 6 | `↑n`, `↓n`, `↑n↓m` |

- The name column is only as wide as needed, so states stay close to names.
- Directory headers (`src/`, `modules/`) are muted and not selectable.
- Rows without issues: name and ref muted (except the selected row), status
  `in sync` in muted grey. An empty status cell would look like missing
  data; the muted word confirms the check ran without drawing attention.

### Detail pane

For the superproject (header selected), the `pin` and `you` roles become
`HEAD`, and `Children` becomes `Modules` (count and issues of all submodules). See
[Superproject](state-model.md#superproject).

1. Directory (muted) and name (bold); `fetched <age>` of the submodule,
   muted, right-aligned in the name's line (left out if the pane is too
   narrow, or the submodule is missing)
2. One line per issue: word (colored, 11 columns) + explanation;
   `in sync  nothing to do` if there are none and no pin is staged;
   `staged` and `detached HEAD on pin` as information
3. `Changes` (only with uncommitted changes), see
   [Change preview](#change-preview)
4. `Ref` with `· via <rule>`, a blank line, then the [spine](#spine);
   `fetch  failed: <git's message>` (yellow word) if the last fetch failed
5. `Children` (count and issues below), `URL`

#### Spine

```
Ref       origin/main · via .gitmodules branch

tip       ● 3f1bf20 Fix: update package dependencies
          ┊ 4 commits
pin       ● 9ac01de Add ethercat node
          ┊ 2 commits
you       ● 48005ac Merge branch 'feat/ethercat…
```

- Newest on top, like `git log`: where each commit is, is read from its
  position, not by comparing SHAs. The commits between two nodes are not
  listed, only counted (`┊ 4 commits`; `1 commit` if adjacent).
- The label column holds the roles (`pin`, `tip`, `you`; `HEAD` for the
  superproject; the target's tip is labelled with the name of its branch, e.g.
  `marco-dev`, without the remote and cut at 16 columns; `target` if unknown), joined by ` · ` if they sit on one commit
  (`pin · tip`, `pin · you`). If all of them sit on one commit it says
  `in sync` instead, which also keeps the commit column in line with the
  other values; the column only grows for a longer label.
- With a staged pin the spine shows both: `staged` (blue label) is the pin
  in the index, `pin` the one in the superproject's HEAD, so the bump is
  visible as a distance between two nodes. The committed pin is left out if
  it is not on disk. If the staged pin is *older* than the committed one it
  sits below it, and the gap above it says `(staged pin moves back)` (blue).
- If the pin is `off target`, the target branch's tip is on the spine,
  labelled with the name of its branch (`marco-dev`, without the remote, cut
  at 16 columns), so the distance from main is visible: below the pin if the pin is
  on top of the target, a fork if both have commits of their own. `tip` is
  then the tip of the branch the pin is on (the `Ref`):

  ```
  marco-dev    ● d0bbbd7 updated submodules
               ┊ 12 commits
  tip        ● ┊ a523224 Remove stale entry
             ┊ ┊ 1 commit
  pin · you  ● ┊ 5a7df75 Merge branch 'ethercat…
             ┊ ┊ 52 commits
  base       ●─╯ 6707725 updated submodule
  ```
- The role words are muted, each word on its own: `you` is bold in the
  default text color, so the checkout stands out in `pin · you`. On its own
  node, i.e. off pin, it is yellow (word and SHA); `staged` is blue. Commit
  subjects use the default text color, the gap is muted.
- A commit that has **diverged** from the pin gets its own lane to the right
  of the main one. The two lanes join at a `base` node, the commit where the
  histories split; the counts say how many commits each side has since:

  ```
  tip         ● e1f2a3b Drop Go 1.21 support
              ┊ 2 commits
  pin · you ● ┊ c4d5e6f Experimental HTTP/3
            ┊ ┊ 1 commit
  base      ●─╯ a9b8c7d Release 1.9.0
  ```

  Only one diverged commit is drawn as a fork, and only if the base is known.
  Otherwise the nodes stay in a line and the gap says `┊ diverged ↑n ↓m`
  (arrows from the lower node's view, like the tree).
- The nodes are `●` (`o` with `SUBMARINE_ASCII`), the gap `┊` (`:`), the
  join `─╯` (`-/`).
- A missing submodule has only the pin node (SHA, no subject).

#### Change preview

```
Changes   5 files · 2M 1A 1D 1R · +37 −12
          ──────────────────────────────────────
          M  render.go  src/render/  +20 −4  3d ago
          M  README.md                +3 −1  20m ago
          A  notes.md  docs/           +14  2h ago
          D  old.go  src/               −8
          R  new.go  ← old.go                1d ago
          ?  scratch.txt                     5m ago
          …  3 more (2M 1?)
```

- A flat list, no group headers: IDEs (VS Code, JetBrains) do the same, and
  headers would use up the few lines the preview has. The letter is the
  meaning, the color only supports it (N4).
- Letters and colors: `!` red bold, `M` yellow, `A` green, `D` red, `R`
  cyan, `?` muted. Line counts: green `+n`, red `−n`; `bin` for binary
  files, `submodule` for a moved nested pin, `repository` for an untracked
  directory that is a git repository itself (cloned, never added as a
  submodule), nothing for other untracked files.
- Order: kind (`!`, `M`, `A`, `D`, `R`, `?`), then changed lines, largest
  first, because the biggest edits are the ones people remember.
- Budget: 10 files. Every kind that occurs gets at least one line, the
  rest is filled in sort order. `… n more (<kinds>)` summarizes the hidden
  files. With 11 files all are shown: "1 more" would take the same line as
  the file itself.
- The `Changes` line summarizes all files: count, files per kind, line
  totals. Everything below it describes one file per line, so no line can
  be mistaken for a file's detail. In a narrow pane the line totals are
  dropped rather than cut.
- A muted rule (`─`, `-` in ASCII mode) separates the summary from the
  files. It starts in the value column and is as wide as the summary or
  the file table, whichever is wider.
- The age (muted) is the file's modification time, so the most recently
  touched files stand out. Removed files have no age.
- The list starts in the value column (under `5 files`), like every other
  value in the pane.
- File name bold, directory muted. Counts form a right-aligned column
  directly after the widest path (max 48 columns), not at the pane's edge,
  so they stay next to their file in wide panes. If space is short the age
  column goes first, then the directory is cut from the left
  (`…ernal/client/`), the name last. For
  renames the muted part is `← old/path`.
- Untracked directories are shown as one entry (`tmp/`).
### Header

The header is the superproject's row: it is selected on startup and drawn
with the selected background (bold) while selected. After the branch it
shows the superproject's own state word like a tree row (`in sync`,
`changed`, `unpushed +1`, spinner, `error`). The counts that follow are
about submodules only.

`path  branch ↑↓ state            n error · <count> <issue>… · n pin staged · n loading · n submodules · <fetch>`

The summary and `<fetch>` form one group at the right edge; the path,
branch and state stay at the left. If the line is too narrow it is cut with
`…` at the right end, so the most important counts stay.

Issue counts are ordered by severity (unpushed, conflict, missing, off pin,
changed, off target), each in its level color. Zero counts are omitted. `n loading`
counts only submodules whose status is still being loaded; ones whose load
failed are counted separately as `n error` (red, before the issue counts). A detached
superproject shows `(detached)` as its branch.

`<fetch>` on the right:

| State | Shows |
|---|---|
| fetching | spinner + `fetching 12/174` (repositories done / total) |
| done | nothing |
| some failed | `3 failed` (count in yellow) |

The header has no fetch age: the age belongs to one repository, and a header
age would be read as the age of everything. It is in the detail pane, for
the selected repository (the superproject's own when the header is selected).

## Colors

All colors are defined in `internal/tui/theme/theme.go`. Never hard-code
colors in components.

| Role | Color | Used for |
|---|---|---|
| Danger | ANSI `1` red, bold | `unpushed`, `conflict`, `error`, `↑n` of an unpushed pin |
| Warn | ANSI `3` yellow | `missing`, `off pin`, `changed`, differing checkout SHA |
| Info | ANSI `4` blue | `staged`, the `●` staged marker, `off target` |
| Primary | ANSI `6` cyan | branch names, commit hashes, spinner |
| Muted | ANSI `8` grey | quiet rows, directory headers, arrows, labels |
| Text | terminal default | normal text |
| Border | ANSI `8` grey | pane and help separators |
| Bar | adaptive 254 (light) / 235 (dark) | header background |
| Selected | adaptive 253 (light) / 237 (dark) | selected row background |
| Backdrop | adaptive 252 (light) / 238 (dark) | the screen behind a popup |

- Background shades have no ANSI-16 equivalent, so they use `AdaptiveColor`.
  The background is detected once in `main` before Bubble Tea starts.
- Styles that render on a background (header, selected row) take a `base`
  style (`LevelStyle(level, base)`, `Delta(pin, base)`), so inner segments
  keep the background.

## Wording

| Word | Detail pane text |
|---|---|
| `unpushed` | pin not pushed to any remote |
| `conflict` | merge conflict |
| `missing` | not initialized / directory missing |
| `off pin` | checkout differs from pin |
| `changed` | uncommitted changes / untracked files |
| `off target` | pin not on `<target>`, on `<branch>` (+n more) |
| `staged` | pin change staged, not committed |
| `in sync` | nothing to do |

## Navigation

| Key | Behavior |
|---|---|
| `↑` / `k`, `↓` / `j` | Previous / next submodule; directory headers are skipped. `↑` from the first submodule selects the header (superproject) |
| `space` | Toggle expand/collapse of the selected submodule |
| `→` / `l` | Header selected: first submodule. Collapsed with children: expand and select the first child. Expanded: select the first child. Leaf: nothing |
| `←` / `h` | Expanded: collapse it (stays selected). Otherwise: collapse the parent and select it. Top level: select the header (superproject) |
| `L` / `shift+→` / `ctrl+→` | Expand all; then follow the first child from the selection down to a submodule without children |
| `H` / `shift+←` / `ctrl+←` | Collapse all (resets remembered expansion); select the top-level ancestor |
| `home` / `g`, `end` / `G` | Header (superproject) / last submodule |
| `pgup`, `pgdn` | Move by one screen |
| `?` | Toggle full help |
| `q`, `ctrl+c` | Quit |
| `f` | Fetch all repositories (does nothing while a fetch runs) |
| `r` | Reload everything (e.g. after editing files) |
| `c` | Copy the absolute directory of the selected submodule to the clipboard (OSC 52, so it also works over SSH; needs terminal support, in tmux `set -g set-clipboard on`). The footer confirms with `copied <path>`. Without a local provider (mock) the path relative to the superproject is copied |
| `/` | Open the filter, see [Filter](#filter) |
| `!` | Toggle problems only, see [Problems only](#problems-only) |
| `x` | Restore the selection, see [Restore](#restore) |

"First child" always means the first child **as displayed** (children are
grouped by directory).

Collapsing an ancestor of the selection moves the selection to that ancestor.
The view scrolls so the selection is always visible; when the first submodule
is selected, headers above it are shown too.

### Filter

`/` shows a text input above the footer (`/ term`). While it is open the
tree shows only the submodules whose path (which includes the name)
contains the term, ignoring case; their ancestors stay as context and are
shown expanded, whatever the user folded. Directory headers without a
match vanish. If nothing matches the tree says `no submodule matches`.

- **Selection.** If the selected submodule is filtered out, the next
  visible one (in display order) is selected, else the previous one; if none
  is left, nothing is selected (the detail pane is empty).
- **Typing.** The input has the focus: letters edit the term, so `j`, `q`
  etc. are text. `↑` `↓` `pgup` `pgdn` still move the selection. `enter`
  hands the keys back to the tree (the filter stays, all keys work again);
  `/` edits the term again.
- **`esc`** closes the input and removes the filter, in both modes. The
  selection stays; its ancestors are expanded so it is still visible.
- The superproject is never filtered out; if nothing matches it is selected.
- The filter line shows the keys that apply on its right.
- The header counts are not filtered.

### Problems only

`!` reduces the tree to what needs attention. It uses the filter's rules
(matches with their ancestors, shown expanded, see [Filter](#filter)), with
"match" meaning *has an issue* (`Status.Issues()`) instead of *path contains
the term*.

- A submodule that is still loading or failed to load (`error`) counts as a
  problem, since its state is unknown (as in the hidden counts, D7). Healthy
  submodules vanish when their status arrives.
- A staged pin is not an issue and does not show the row.
- It combines with the filter (both must match). Unlike the filter it is a
  mode, not an input: it stays until `!` is pressed again, also after `esc`
  closed the filter.
- A line above the footer shows `problems only` (yellow) and `! show all`.
  While the filter line is open, its hint starts with `problems only (!)`
  instead.
- With no problem the tree says `no problems`.
- The selection follows the filter's rule: if it stops being visible (also
  when a status changes), the next visible submodule is selected, else the
  previous one. The superproject stays selectable.
- The header counts are not filtered.

### Restore

`x` makes the selection, and everything below it, exactly what git records
(see [actions.md](actions.md#restore)). It uses the line above the footer:

| Phase | Line |
|---|---|
| the submodule has a staged pin | yellow `Restore src/app to`, `s staged pin · c committed pin (unstages it)`, right-aligned `any other key cancels` |
| reading what is lost | spinner, muted `checking src/app…` |
| something would be lost | yellow `Restore src/app?`, red `Discards 7 changed files in 3 repositories, 2 commits on no branch, 1 untracked repository.`, right-aligned `y restore · n cancel` |
| running | spinner, yellow `restoring src/app…` |
| failed | red `restore of src/app failed: <git's message>`, until the next key |

`y` restores, any other key cancels; the keys are not passed on while a
question is open. After `c` the name reads `src/app to the committed pin`. Without a loss there is no question. Success flashes
`restored src/app` in the footer. For the header the name is `the
superproject`. This line takes precedence over the filter line and the
problems-only notice.

### Switch branch

`b` opens a popup in the middle of the screen (see
[actions.md](actions.md#switch-branch)). The screen around it stays visible
but dimmed: its colors are dropped and it is drawn in `Backdrop`, darker
than `Muted`, so the popup has the focus.

```
╭──────────────────────────────────────────────────────────────────────╮
│ Switch libs/feature                                                  │
│ / filter branches                                                    │
│ ──────────────────────────────────────────────────────────────────── │
│ * main                                             92d1583  3d ago   │
│   origin/feat/search                               58a2bf1  2h ago   │
│ ↑↓ select · enter switch · esc cancel                                │
╰──────────────────────────────────────────────────────────────────────╯
```

- Border `Border`, title in `Primary` bold, search prompt `Primary`, the
  selected row on the `Selected` background, SHA and age `Muted`, the remote
  prefix `Muted`, the checked-out branch bold with `*`.
- Width at most 72 columns, height as the list (at most 14 rows), so
  filtering does not resize it. While reading: spinner and `reading
  branches…`; no match: `no branch matches`; error: red, inside the popup.
- Enter closes the popup and continues on the line above the footer like a
  [restore](#restore): `checking`, the question `Switch src/app to
  origin/feat? Discards …   y switch · n cancel`, `switching …`.

### Key reference

Every key in the UI, where it is shown. Update this
table when a key is added or moved. "Bar" is the short footer
bar, "Palette" the detail pane's palette (the footer takes its keys over
where the palette can't be shown, see [Key bar and palette](#key-bar-and-palette)),
"Help" the full help (`?`).

| Key | Action | Group | Shown in |
|---|---|---|---|
| `↑` / `k`, `↓` / `j` | Move selection | Navigation | Bar, Help |
| `←` / `h`, `→` / `l` | Fold / unfold, parent / first child | Navigation | Bar, Help |
| `H` / `⇧←`, `L` / `⇧→` | Collapse / expand all | Navigation | Bar, Help |
| `space` | Toggle expand/collapse | Navigation | Bar, Help |
| `home` / `g`, `end` / `G` | Header / last submodule | Navigation | Help |
| `pgup`, `pgdn` | Move by one screen | Navigation | Help |
| `c` | Copy path | Submodule | Palette, Help |
| `b` | Switch branch (popup) | Submodule | Palette, Help |
| `x` | Restore | Submodule | Palette, Help |
| `f` | Fetch all repositories | Global | Help |
| `r` | Reload everything | Global | Help |
| `/` | Filter | Global | Help |
| `!` | Problems only (toggle) | Global | Help |
| `?` | Toggle full help | Global | Bar, Help |
| `q`, `ctrl+c` | Quit | Global | Bar, Help |

### Mouse

- Click a row: select it. Click the header: select the superproject.
- Click `▸` / `▾`: select and toggle.
- Scroll wheel: move the selection.
- In the [branch popup](#switch-branch): click a branch to switch to it,
  wheel to move, click outside to close. The popup is hit-tested by its own
  position, not with bubblezone, since it is drawn after `zone.Scan`.

## Accessibility and terminals

- `SUBMARINE_ASCII=1`: ASCII symbols (`^ v > v . *`) instead of Unicode.
- Many setups capture `ctrl+←/→` (macOS Mission Control, some terminals and
  multiplexers), so expand/collapse all also work as `shift+→/←` and as `L` / `H`, the
  shifted vim keys for `→` / `←` (for terminals that don't report shift+arrows).
