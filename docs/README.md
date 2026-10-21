# submarine — documentation

submarine is a terminal UI for git repositories with many (recursively nested)
submodules. It shows **only the submodules**, as a collapsible tree, and
describes each pin as a **branch plus distance** (`origin/main ↓1`) instead of a
bare commit hash.

## Documents

| Document | Contents |
|---|---|
| [requirements.md](requirements.md) | What the tool must do: functional and non-functional requirements |
| [state-model.md](state-model.md) | How a submodule's state is determined, summarized into issues, and how pins are resolved to refs |
| [ux-guide.md](ux-guide.md) | Layout, attention hierarchy, colors, wording, navigation and key bindings |
| [architecture.md](architecture.md) | Package layout, the provider boundary, async loading, testing |
| [decisions.md](decisions.md) | Decision log with rationale (ADR style) |

## Status

**Prototype.** The UI is complete for browsing and runs on mock data
(`internal/submodule/mock`). There is no git backend yet, and actions
(`u`, `b`, `f`, `/`, `!`) only show a "not implemented" message.

Next steps, roughly in order:

1. Git provider implementing `submodule.Provider` (read-only).
2. Actions: fetch, checkout pin (`submodule update`), bump pin to tip, init.
3. Filter (`/`) and "problems only" (`!`) views.
4. Refresh after actions and on demand.
