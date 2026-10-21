# AGENTS.md

submarine: a Go + Bubble Tea TUI for repositories with many git submodules.
Currently a prototype with a git provider (with background fetch) and mock data.

## Read first

The `docs/` folder is the source of truth for requirements, state rules, UX
and decisions. Read the relevant document before changing behavior:

- [docs/README.md](docs/README.md): overview, status, next steps
- [docs/requirements.md](docs/requirements.md): what the tool must do
- [docs/state-model.md](docs/state-model.md): states, issues, levels, pin → ref resolution
- [docs/ux-guide.md](docs/ux-guide.md): layout, colors, wording, key bindings
- [docs/architecture.md](docs/architecture.md): packages, provider boundary, loading, testing
- [docs/decisions.md](docs/decisions.md): decision log with rationale

## Commands

```sh
go run ./cmd/submarine [dir]  # run on a repository (default: cwd)
go run ./cmd/submarine --mock # run on mock data
go run ./cmd/submarine --no-fetch [dir]  # without background fetch
scripts/make-fixture.sh DIR   # build a test superproject in DIR
make dist                     # cross-compile into dist/
GITEA_TOKEN=… make release    # publish a tagged release (outward-facing)
go vet ./...
go test ./...
gofmt -l .                    # must print nothing
```

## Rules

- **Keep docs in sync.** A change to behavior, keys, colors, wording or
  architecture updates the matching document in `docs/`. A new significant
  decision gets an entry in `docs/decisions.md`.
- **Layering.** `internal/submodule` has no UI imports and decides what a
  state means (`Status.Issues()`). `internal/tui/theme` decides how it looks.
  Components never classify states or define colors themselves.
- **Data access** only through `submodule.Provider` (and its optional
  `Fetcher` / `Watcher`). The git provider only uses the network in
  `Fetch`, and git must never prompt.
- **Colors** only from `theme.Theme`, ANSI 16 colors for meaning (see
  `docs/ux-guide.md#colors`). Render segments on a background with a `base`
  style (`LevelStyle(level, base)`, `Delta(pin, base)`).
- **Tree navigation** stays in plain methods on `tree.Model` with unit tests
  in `internal/tui/tree/tree_test.go`.
- **Verify UI changes** by running the app, e.g. headless in tmux (see
  `docs/architecture.md#testing`), not only by tests.
