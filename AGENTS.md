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
scripts/make-fixture.sh DIR   # build a test superproject in DIR
make dist                     # cross-compile into dist/
make release                  # publish a tagged release to GitHub (outward-facing; run by the developer)
go vet ./...
go test ./...
gofmt -l .                    # must print nothing
```

## Releasing

Use the `release` skill (`/release`, `.claude/skills/release/SKILL.md`): it
picks the next semver, runs a check, tags and pushes, asks the developer to
run `make release`, then verifies the GitHub release and writes its notes.
Never run `make release` or push tags outside of it.

## Rules

- **Keep docs in sync.** A change to behavior, keys, colors, wording or
  architecture updates the matching document in `docs/`. A new significant
  decision gets an entry in `docs/decisions.md`.
- **Layering.** `internal/submodule` has no UI imports and decides what a
  state means (`Status.Issues()`). `internal/tui/theme` decides how it looks.
  Components never classify states or define colors themselves.
- **Data access** only through `submodule.Provider` (and its optional
  `Fetcher` / `Watcher` / `Restorer` / `Switcher` / `Resolver`). The git
  provider only uses the network in `Fetch` and, for missing objects, in
  `Restore` (and the switch and resolve that reuse it); git must never
  prompt, and the provider writes only on a user's action.
- **Colors** only from `theme.Theme`, ANSI 16 colors for meaning (see
  `docs/ux-guide.md#colors`). Render segments on a background with a `base`
  style (`LevelStyle(level, base)`, `Delta(pin, base)`).
- **Tree navigation** stays in plain methods on `tree.Model` with unit tests
  in `internal/tui/tree/tree_test.go`.
- **Verify UI changes** by running the app, e.g. headless in tmux (see
  `docs/architecture.md#testing`), not only by tests.
