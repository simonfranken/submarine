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
  provider.go                 Provider interface
internal/submodule/git/       read-only provider using the git CLI
  exec.go                     git runner (environment, errors)
  parse.go                    pure parsers for git output (status, numstat, …)
  provider.go                 Tree, Status, pin → ref resolution
internal/submodule/mock/      fixture provider with artificial latency
internal/tui/                 Bubble Tea application
  app.go                      root model: layout, message routing, loading
  keys.go                     key bindings (also feed the help view)
  msgs.go                     treeLoadedMsg, statusLoadedMsg
  theme/                      colors, glyphs, issue words, change letters, level styles
  tree/                       tree component: rows, navigation, mouse, view
  detail/                     detail pane; changes.go: change preview
  header/                     header line
  footer/                     help and flash messages
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
  take `index.lock`), `LC_ALL=C`. No command touches the network.
- `Tree` reads each repository's `.gitmodules` (`git config -f
  .gitmodules`) and recurses into initialized submodules. It records, per
  path, the parent repository and the `.gitmodules` branch for `Status`.
  `Name` is the last element of `RelPath`.
- `Status` runs about ten small git commands per submodule (see
  [state-model.md](state-model.md#git-commands)). At most `runtime.NumCPU()`
  `Status` calls run at once; the others wait on a semaphore and still
  respect `ctx` (D16).
- Superproject: branch from `symbolic-ref`, ahead/behind against
  `@{upstream}`, "fetched" from the mtime of `FETCH_HEAD`.

## Loading flow

1. `Init` runs `Tree()` → `treeLoadedMsg`.
2. The app indexes submodules by path and starts one `tea.Cmd` per submodule
   calling `Status()` → `statusLoadedMsg`.
3. Each result is stored on the submodule (`Status` or `Err`); the view
   updates incrementally, including parents' hidden-issue counts.
4. The spinner stops ticking when nothing is loading.

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

## Testing

```sh
go vet ./...
go test ./...
```

- `internal/submodule`: issue classification and hidden-issue aggregation.
- `internal/submodule/git`: parser unit tests, and integration tests
  against a fixture repository built by `scripts/make-fixture.sh` (skipped
  if `git` is not installed).
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
