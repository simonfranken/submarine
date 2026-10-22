# submarine

**A terminal UI for git repositories with many submodules.**
See what needs your attention. Ignore the rest.

`git submodule status` gives you a wall of commit hashes. submarine answers
the questions you actually have:

- Which branch is this submodule on, and is the pin current?
- Which submodules need me, and which are fine?
- Is anything dangerous, like a pin nobody else can fetch?

![submarine showing a tree with nested submodules and the detail pane of an off-pin submodule that is one commit behind its branch](docs/images/overview.png)

## Why you might like it

- **Only submodules, as a tree.** Nested submodules at any depth, grouped by
  directory, collapsed until you open them.
- **Branch plus distance, not hashes.** `origin/main ↓2` says more than
  `b7c8d9e`. A pin on a feature branch shows that branch.
- **Problems are loud, healthy is quiet.** Submodules without issues are
  muted `in sync`; a staged pin shows as blue `staged` (or a `●` after an
  issue word). A collapsed parent counts the problems hidden below it
  (`·1`) and shows `●` if a staged pin is hidden.
- **Words, not symbols.** Every state is a short word (`unpushed`, `conflict`,
  `missing`, `off pin`, `changed`), so nothing depends on color alone.
- **Live.** Follows changes you make in other shells within about two
  seconds. It never fetches on its own: the header shows when you last did,
  `f` fetches everything.
- **Built for dozens to hundreds of submodules.** Each status loads
  asynchronously, so the UI never freezes.

## Install

```sh
curl -fsSL https://git.simon-franken.de/simonfranken/submarine/raw/branch/main/install.sh | sh
```

Installs `sub` and `submarine` (linux/macOS, amd64/arm64) to `~/.local/bin`.
Both names start the same program; the examples below use the shorter `sub`.
Update later with `sub update`.

## Try it

```sh
sub                  # the repository in the current directory
sub ~/code/project   # any directory inside a repository
sub --mock           # built-in sample data, no repository needed
```

`submarine` works everywhere `sub` does, e.g. `submarine --mock`.

## Examples

### Find what needs attention

Problems stand out: `unpushed` in red, `off pin` and `changed` in yellow,
everything else muted. Select a submodule to see the details on the right:
every issue in words, the resolved ref and why, the pinned, checked-out and
tip commits, and a preview of uncommitted changes.

```
 src/                                    │ src/
     core        off pin   origin/main   │ net
   ▸ net [2]     unpushed  origin/main ↑1│
     ui          in sync   origin/main   │ unpushed  pin not pushed to any remote
                                         │
                                         │ Ref       origin/main ↑1
                                         │           via checked out branch
                                         │ Pinned    b7c8d9e Hotfix null pointer
                                         │ Checkout  b7c8d9e Hotfix null pointer
                                         │ Tip       6f5e4d3 Release 2.1.0
```

### Jump to a submodule with `/`

Press `/` and type. Only matching submodules (and the parents that lead to
them) stay. Your selection moves to the next match if it was filtered out.

```
 libs/
   ▾ log          in sync   origin/main
     modules/
         nested   changed   origin/main
 / nest                                          enter navigate • esc close
```

`enter` keeps the filter and gives you the normal keys back, `esc` removes
it and leaves your selection where it is.

### Copy a path

Select a submodule and press `c`. Its absolute directory lands in your
clipboard, ready for `cd <paste>`. It works over SSH too (OSC 52; in tmux,
`set -g set-clipboard on`).

### Fetch and reload

`f` fetches all repositories again, `r` reloads everything after you edited
files.

## Keys

| Key | Action |
|---|---|
| `↑` `↓` / `j` `k` | Move |
| `←` `→` / `h` `l` | Fold / unfold, parent / first child |
| `H` `L` / `⇧←` `⇧→` | Collapse / expand all |
| `space` | Toggle the selected submodule |
| `home` `end` / `g` `G`, `pgup` `pgdn` | Jump, page |
| `/` | Filter |
| `!` | Problems only |
| `c` | Copy path |
| `f` / `r` | Fetch / reload |
| `?` | Full help |
| `q` | Quit |

The mouse works too: click to select, click `▸` to toggle, wheel to move.

## Status

A prototype. Browsing is complete; the actions `u` (check out the pin) and `b` (bump the pin to the tip) are
planned.

## Documentation

Requirements, state rules, UX guide, architecture and the decision log are in
[docs/](docs/README.md).

## Development

```sh
go run ./cmd/submarine --mock   # run on mock data
scripts/make-fixture.sh DIR     # build a test superproject with every state
go vet ./... && go test ./...
```
