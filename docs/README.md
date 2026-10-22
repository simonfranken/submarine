# submarine — documentation

submarine is a terminal UI for git repositories with many (recursively nested)
submodules. It shows **the submodules**, as a collapsible tree (plus the state
of the superproject itself in the header), and
describes each pin as a **branch plus distance** (`origin/main ↓1`) instead of a
bare commit hash.

## Documents

| Document | Contents |
|---|---|
| [requirements.md](requirements.md) | What the tool must do: functional and non-functional requirements |
| [state-model.md](state-model.md) | How a submodule's state is determined, summarized into issues, and how pins are resolved to refs |
| [ux-guide.md](ux-guide.md) | Layout, attention hierarchy, colors, wording, navigation and key bindings |
| [architecture.md](architecture.md) | Package layout, the provider boundary, async loading, testing |
| [actions.md](actions.md) | the write actions restore and switch branch (done) and stage pin (draft) |
| [decisions.md](decisions.md) | Decision log with rationale (ADR style) |

## Install

```sh
curl -fsSL https://git.simon-franken.de/simonfranken/submarine/raw/branch/main/install.sh | sh
```

Installs `submarine` and the alias `sub` (linux/macOS, amd64/arm64) to
`~/.local/bin`. Update with `submarine update`. Maintainers publish with
`GITEA_TOKEN=… make release` (see [architecture](architecture.md#distribution)).

## Status

**Prototype.** The UI is complete for browsing. It reads real repositories
through a git provider (`internal/submodule/git`): `submarine [dir]`. It
fetches only when asked (`f`) and follows changes made in other shells. `submarine --mock` shows
the built-in mock data (`internal/submodule/mock`). `!` shows only
submodules with problems. `x` restores the selection and `b` switches its
branch ([actions.md](actions.md)); stage pin is drafted there.

Next steps, roughly in order:

1. Stage pin, see [actions.md](actions.md). (Restore and switch branch are done.)
2. (done) "Problems only" (`!`) view, reusing the tree's filter.
