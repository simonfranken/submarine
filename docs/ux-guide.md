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
 ~/coding/my_project  main      13 submodules · 1 unpushed · 1 missing · 2 off pin · …   fetched 2m ago
   docs                                  origin/main                     │ src/
 misc/tools/                                                             │ my_submodule_3
     my_submodule_1                      origin/main                     │
     my_submodule_2       missing        a1b2c3d                         │ unpushed  pin not pushed to any remote
 src/                                                                    │
     my_submodule_1       off pin        origin/main                     │ Ref       origin/main ↑1
   ▸ my_submodule_2 [2]   changed ·2     origin/main                     │           via checked out branch
     my_submodule_3       unpushed       origin/main    ↑1               │ Pinned    b7c8d9e Hotfix null pointer
 libs/                                                                   │ Checkout  b7c8d9e Hotfix null pointer
     logging                             origin/main    ↓3               │ Tip       6f5e4d3 Release 2.1.0
 ↑/↓ move • ←/→ collapse/expand • space toggle • u checkout pin • … • ? help • q quit
```

| Area | Height / width |
|---|---|
| Header | 1 line |
| Tree | Remaining width |
| Detail pane | 40% of the width, at least 36 columns; hidden below 90 columns |
| Footer | 1 line (more with full help) |

### Tree row

```
indent ▸ name [n]   status          ref             delta
```

| Column | Width | Content |
|---|---|---|
| indent | 2 per depth | |
| toggle | 3 | `▸` collapsed, `▾` expanded, blank for leaves |
| name | fits the widest visible name | name, `[n]` = direct children (collapsed only) |
| status | 14 | issue word, `+n`, `·k` hidden issues; muted `in sync` without issues; spinner while loading |
| ref | widest ref, max 22 | resolved ref; muted short SHA if unresolved |
| delta | 6 | `↑n`, `↓n`, `↑n↓m` |

- The name column is only as wide as needed, so states stay close to names.
- Directory headers (`src/`, `modules/`) are muted and not selectable.
- Rows without issues: name and ref muted (except the selected row), status
  `in sync` in muted grey. An empty status cell would look like missing
  data; the muted word confirms the check ran without drawing attention.

### Detail pane

1. Directory (muted) and name (bold)
2. One line per issue: word (colored, 10 columns) + explanation;
   `in sync  nothing to do` if there are none; `staged` and
   `detached HEAD on pin` as information
3. `Changes` (only with uncommitted changes), see
   [Change preview](#change-preview)
4. `Ref` with distance and `via <rule>`; `Fetched <age>` of the submodule;
   `fetch  failed: <git's message>` (yellow word) if the last fetch failed
5. `Pinned`, `Checkout`, `Tip`: SHA + subject; the checkout SHA is yellow if it
   differs from the pin
6. "Pin is n commits behind/ahead of …" / "Pin and … have diverged": commit list
7. `Children` (count and issues below), `URL`

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

`path  branch ↑↓ · n submodules · <count> <issue>… · n pin staged · n loading · <fetch>`

Issue counts are ordered by severity (unpushed, conflict, missing, off pin,
changed), each in its level color. Zero counts are omitted. `n loading`
counts only submodules whose status is still being loaded; ones whose load
failed (shown as `error`) are not counted. A detached
superproject shows `(detached)` as its branch.

`<fetch>` on the right:

| State | Shows |
|---|---|
| fetching | spinner + `fetching 12/174` (repositories done / total) |
| done | `fetched just now`, then the age (`fetched 5m ago`) |
| some failed | `fetched just now · 3 failed` (count in yellow) |
| never fetched | `fetched never` |

The age is the newer of the superproject's `FETCH_HEAD` and the end of the
application's own fetch.

## Colors

All colors are defined in `internal/tui/theme/theme.go`. Never hard-code
colors in components.

| Role | Color | Used for |
|---|---|---|
| Danger | ANSI `1` red, bold | `unpushed`, `conflict`, `error`, `↑n` of an unpushed pin |
| Warn | ANSI `3` yellow | `missing`, `off pin`, `changed`, differing checkout SHA |
| Info | ANSI `4` blue | `staged` |
| Primary | ANSI `6` cyan | branch names, commit hashes, spinner |
| Muted | ANSI `8` grey | quiet rows, directory headers, arrows, labels |
| Text | terminal default | normal text |
| Border | ANSI `8` grey | pane and help separators |
| Bar | adaptive 254 (light) / 235 (dark) | header background |
| Selected | adaptive 253 (light) / 237 (dark) | selected row background |

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
| `staged` | pin change staged, not committed |
| `in sync` | nothing to do |

## Navigation

| Key | Behavior |
|---|---|
| `↑` / `k`, `↓` / `j` | Previous / next submodule; directory headers are skipped |
| `space` | Toggle expand/collapse of the selected submodule |
| `→` / `l` | Collapsed with children: expand and select the first child. Expanded: select the first child. Leaf: nothing |
| `←` / `h` | Expanded: collapse it (stays selected). Otherwise: collapse the parent and select it. Top level: nothing |
| `ctrl+→` | Expand all; then follow the first child from the selection down to a submodule without children |
| `ctrl+←` | Collapse all (resets remembered expansion); select the top-level ancestor |
| `home` / `g`, `end` / `G` | First / last submodule |
| `pgup`, `pgdn` | Move by one screen |
| `?` | Toggle full help |
| `q`, `ctrl+c` | Quit |
| `f` | Fetch all repositories (does nothing while a fetch runs) |
| `r` | Reload everything (e.g. after editing files) |
| `u`, `b`, `/`, `!` | Planned actions (see requirements) |

"First child" always means the first child **as displayed** (children are
grouped by directory).

Collapsing an ancestor of the selection moves the selection to that ancestor.
The view scrolls so the selection is always visible; when the first submodule
is selected, headers above it are shown too.

### Mouse

- Click a row: select it.
- Click `▸` / `▾`: select and toggle.
- Scroll wheel: move the selection.

## Accessibility and terminals

- `SUBMARINE_ASCII=1`: ASCII symbols (`^ v > v . *`) instead of Unicode.
- Some terminals or multiplexers capture `ctrl+←/→`; letter aliases can be
  added if needed.
