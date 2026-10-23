# Architecture

Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea) (v1),
[Lip Gloss](https://github.com/charmbracelet/lipgloss),
[Bubbles](https://github.com/charmbracelet/bubbles) (key, help, spinner),
[bubblezone](https://github.com/lrstanley/bubblezone) (mouse hit areas).

## Package layout

```
cmd/submarine/main.go         wiring: choose provider (git or --mock), start the program
internal/submodule/           domain model; NO UI imports
  model.go                    Repo, Submodule, Status, PinInfo, Commit, enums
  status.go                   Issues(), HiddenIssues(), Walk()
  provider.go                 Provider interface; optional Fetcher, Watcher, Locator, Restorer, Switcher
internal/submodule/git/       provider using the git CLI (reads, fetch, watch)
  exec.go                     git runner (environment, errors)
  parse.go                    pure parsers for git output (status, numstat, …)
  provider.go                 Tree, Status, pin → ref resolution
  fetch.go                    Fetch (network), Fetchable
  watch.go                    Watch: polls git metadata for changes
  restore.go                  Restore, RestoreLoss (network: only what is missing)
  switch.go                   Branches, SwitchLoss, Switch (stage the tip, restore, attach the branch)
  gitdir.go                   resolves .git dirs and gitdir: files
internal/submodule/mock/      fixture provider with artificial latency
internal/update/              self-update from the package registry; NO UI imports
internal/tui/                 Bubble Tea application
  app.go                      root model: layout, message routing, loading
  keys.go                     key bindings (also feed the help view)
  restore.go, switch.go       restore / switch flows; a switch reuses restoreState
  overlay.go                  draws the picker over the finished view at the position the picker reports
  msgs.go                     tree/status/fetch/change messages
  refresh.go                  reloader: one load per key, coalesced reruns
  theme/                      colors, glyphs, issue words, change letters, level styles
  tree/                       tree component: rows, navigation, mouse, view
  detail/                     detail pane; changes.go: change preview
  header/                     header line
  branches/                   branch picker popup (search line + list)
  footer/                     key bar (navigation left, global right) and flash messages
```

### Layering rules

- `internal/submodule` knows nothing about the UI. Everything that decides
  **what** a state means (issue kinds, levels, hidden issues) lives here.
- `internal/tui/theme` decides **how** it looks: words, colors, symbols.
- Components (`tree`, `detail`, `header`, `footer`) use the theme; they never
  define colors or classify states themselves.
- The UI only accesses data through `submodule.Provider`.

## Provider boundary

```go
type Provider interface {
    Tree(ctx context.Context) (*Repo, error)                  // structure only, fast
    Status(ctx context.Context, path string) (*Status, error) // per submodule, may be slow
}
```

Optional capabilities; the app type-asserts them:

```go
type Fetcher interface {
    Fetchable(path string) bool                // initialized? ("" = superproject)
    Fetch(ctx context.Context, path string) error
}
type Watcher interface {
    Watch(ctx context.Context) <-chan Change   // {Path, Local, Structure}
}
```

```go
type Restorer interface {
    RestoreLoss(ctx context.Context, path string) (Loss, error) // what a restore would discard
    Restore(ctx context.Context, path string) error              // may clone/fetch what is missing
}
```

```go
type Switcher interface {
    Branches(ctx context.Context, path string) ([]Branch, error)          // local + remote, no network
    SwitchLoss(ctx context.Context, path string, b Branch) (Loss, error)  // as RestoreLoss
    Switch(ctx context.Context, path string, b Branch) (string, error)    // branch HEAD ends on, "" if detached
}
```

```go
type Locator interface {
    Dir(path string) string                    // absolute directory, "" if unknown
}
```

- `Tree` returns the superproject and all submodules with `Parent` and
  `Children` linked. `Status` fields are nil.
- `Status` is called once per submodule, concurrently. It must be safe to
  call concurrently and must respect `ctx`.
- `Submodule.Path` (relative to the superproject root) is the unique ID.

The mock provider (`internal/submodule/mock`) covers every state and sleeps
100–800 ms per `Status` call to exercise the loading path.

### Git provider

`internal/submodule/git` shells out to `git -C <dir> …` (see D14).

- Environment: `GIT_TERMINAL_PROMPT=0`, `GIT_OPTIONAL_LOCKS=0` (reads never
  take `index.lock`), `LC_ALL=C`. Only `Fetch` uses the network.
- `Tree` reads each repository's `.gitmodules` (`git config -f
  .gitmodules`) and recurses into initialized submodules. It records, per
  path, the parent repository and the `.gitmodules` branch for `Status`.
  `Name` is the last element of `RelPath`.
- `Status` runs about ten small git commands per submodule (see
  [state-model.md](state-model.md#git-commands)). At most `runtime.NumCPU()`
  `Status` calls run at once; the others wait on a semaphore and still
  respect `ctx` (D16).
- Superproject: branch from `symbolic-ref`, ahead/behind against
  `@{upstream}`. Its fetch age (mtime of `FETCH_HEAD`) is `Status.FetchedAt`
  of the root node, like a submodule's.
- `Fetch`: `git fetch --all --prune --no-recurse-submodules --quiet` (we
  fetch every submodule ourselves), at most 8 at once, 2 min timeout each.
  To never prompt, `GIT_SSH_COMMAND="ssh -o BatchMode=yes"` is set unless
  the user configured `GIT_SSH_COMMAND` or `core.sshCommand`. The error is
  git's first line of output.
- `Watch` polls every 2 s (`WatchInterval`) and compares a fingerprint per
  repository: names, mtimes and sizes of the git metadata files (D19).
  - remote part: `FETCH_HEAD`, `refs/remotes/**`, `refs/tags/**`
  - local part: `HEAD`, `index`, `MERGE_HEAD`, `packed-refs`,
    `refs/heads/**`, `reftable/**`
  - structure: the worktree `.gitmodules`, and whether `.git` exists

  Git dirs are resolved from `.git` (directory, or `gitdir:` file for
  submodules, plus `commondir` for worktrees) without running git. Our own
  reads write nothing (`GIT_OPTIONAL_LOCKS=0`), so they never trigger a
  change. Worktree file edits are not watched (`r` reloads).

## Loading flow

1. `Init` runs `Tree()` → `treeLoadedMsg`.
2. The app indexes submodules by path and starts one `tea.Cmd` per submodule
   calling `Status()` → `statusLoadedMsg`.
3. Each result is stored on the submodule (`Status` or `Err`); the view
   updates incrementally, including parents' hidden-issue counts.
4. After the first tree: the watcher starts (`changeMsg`). Nothing is
   fetched until the user presses `f`, which starts one `tea.Cmd` per
   fetchable repository (`fetchDoneMsg`).
5. The spinner ticks only while something loads or fetches.

### Fetch and refresh are decoupled (D20)

```
Fetch (network) ──writes refs──▶ .git/…  ◀──writes refs── user's shell
                                   │
                          Watch (polls every 2 s)
                                   │ Change{Path, Local, Structure}
                                   ▼
                          app reloads Status / Tree
```

- `fetchDoneMsg` only updates the progress and `Submodule.FetchErr`; it
  never reloads anything. The watcher sees the new refs and triggers the
  reload, exactly as for a fetch made in another shell.
- `changeMsg`:
  - submodule, remote only → reload its `Status`
  - submodule, local → reload it and its direct children (their pins live
    in its index)
  - superproject, or `.gitmodules` anywhere → reload the tree; if local,
    also every status
- A reloaded tree carries the old `Status`, `Err` and `FetchErr` over by
  path, and `tree.ReplaceRoots` keeps expansion and the cursor, so a reload
  doesn't flash spinners or move the selection.
- The `reloader` runs at most one load per key (a submodule path, or the
  tree). A request during a load marks it dirty; it runs once more
  afterwards. Results therefore never arrive out of order and bursts of
  changes (a fetch writing many refs) coalesce.

## Tree component

- State: roots, `expanded map[path]bool`, cursor (a `*Submodule`), scroll
  offset, size.
- `rows()` flattens the visible tree into directory-header rows and
  submodule rows. Directory grouping uses `path.Dir(RelPath)` and keeps the
  order of first appearance.
- Navigation methods (`Up`, `Down`, `Left`, `Right`, `Toggle`,
  `CollapseAll`, `ExpandAll`, `Home`, `End`, `PageUp`, `PageDown`) are plain
  methods without Bubble Tea dependencies, so they are unit-tested directly.
- Key mapping happens in `app.go`; the tree only handles mouse messages.
- Mouse zones: `toggle:<path>` and `row:<path>`, prefixed per instance.

## Rendering notes

- Each segment of a row is rendered with the row's `base` style
  (background), because an inner style's reset would otherwise clear the
  outer background.
- Widths are measured with `lipgloss.Width`; truncation uses
  `ansi.Truncate`, which keeps escape sequences and bubblezone markers intact.
- `zone.Scan` runs once, on the root view.

## Distribution

No CI runner: releases are built and published from a developer machine.

- `make dist` cross-compiles `CGO_ENABLED=0` binaries for linux/darwin ×
  amd64/arm64 into `dist/` plus `checksums.txt`; the version comes from
  `git describe` and is baked in with `-X main.version`.
- `make release` with `GITEA_TOKEN` set, e.g. in `.env` (copy `.env.template`; token needs `write:package`; HEAD must be a
  clean `vX.Y.Z` tag) runs tests, builds and uploads to the Gitea generic
  registry `https://git.simon-franken.de/api/packages/simonfranken/generic`:
  - `submarine/<version>/submarine_<os>_<arch>` and `checksums.txt`
  - `submarine-latest/latest/VERSION`, replaced **last**, so a failed upload
    never advertises a broken version.
- The `release` skill (`.claude/skills/release/SKILL.md`) drives a release:
  latest tag → next semver from the commits since → check (`vet`, `test`,
  `gofmt`, `make dist`) → tag and push → the developer runs `make release` →
  the skill verifies the registry's `VERSION` and creates the Gitea release
  with MCP (notes only; the binaries stay in the registry).
- `install.sh` (`curl -fsSL …/raw/branch/main/install.sh | sh`) detects
  OS/arch, verifies the sha256, installs `submarine` and a `sub` symlink to
  `~/.local/bin` (`SUBMARINE_INSTALL_DIR`, `SUBMARINE_VERSION`,
  `SUBMARINE_BASE_URL` override).
- `submarine update [--check] [--force]` (package `internal/update`) reads
  `VERSION`, downloads the asset for the running platform, verifies the
  checksum and atomically replaces the real executable (symlinks resolved,
  so `sub` works too). `SUBMARINE_UPDATE_URL` overrides the registry.
- On every start the TUI looks for a newer release in the background (3 s
  timeout, no cache, no blocking). `main` passes `tui.WithUpdateCheck` and
  the TUI gets the result as a message; the footer then offers `q quit and
  update to vX`. That key quits, and `main` installs the release once the
  terminal is restored (the same code as `submarine update`). The check is off
  for dev builds, `--mock` and `SUBMARINE_NO_UPDATE_CHECK=1`.

## Testing

```sh
go vet ./...
go test ./...
```

- `internal/submodule`: issue classification and hidden-issue aggregation.
- `internal/submodule/git`: parser unit tests, and integration tests
  against a fixture repository built by `scripts/make-fixture.sh` (skipped
  if `git` is not installed). Fetch and watch tests build their own
  fixture, since they change it; the fixture's remotes are local bare
  repositories, so fetching works offline.
- `internal/tui`: `reloader`.
- `internal/tui/tree`: navigation rules. Tests need `zone.NewGlobal()` (see
  `TestMain`).

Manual check in a headless terminal:

```sh
scripts/make-fixture.sh /tmp/subfix   # superproject with every state
go build -o /tmp/submarine ./cmd/submarine
tmux new-session -d -s sub -x 130 -y 24 "/tmp/submarine /tmp/subfix/super"
tmux send-keys -t sub Down Right
tmux capture-pane -pt sub        # add -e to see colors
tmux kill-session -t sub
```
