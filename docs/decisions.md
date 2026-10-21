# Decision log

Each entry: context, decision, consequences. Newest last.

## D1 — Show pins as ref + distance, not hashes

**Context.** Hashes don't tell users whether a pin is current.
**Decision.** Resolve each pin to a ref and show the distance (`origin/main ↓1`).
The resolution order is fixed and the applied rule is shown in the detail pane
(see [state-model.md](state-model.md#resolving-a-pin-to-a-ref)).
**Consequences.** Resolution is a heuristic; showing the rule keeps it
transparent.

## D2 — Compare three commits: pinned, checked out, tip

**Context.** The first mockup showed one branch for both the checkout and the
pin, so a mismatch was invisible.
**Decision.** Model all three commits. The tree's ref column always describes
the **pin**; checkout drift is an issue (`off pin`).
**Consequences.** The detail pane shows Pinned / Checkout / Tip rows.

## D3 — Raw dimensions in the domain, summarized issues in the UI

**Context.** States are not mutually exclusive (a submodule can be off pin and
dirty), but showing every dimension made rows noisy (14 symbols).
**Decision.** `Status` stores independent dimensions. `Status.Issues()`
summarizes them into five kinds with three levels. The tree shows only the
most severe issue (+n); the detail pane shows all.
**Consequences.** One place defines what a problem is; a git backend gets
the same behavior for free.

## D4 — Attention follows severity

**Context.** Healthy rows competed for attention with dangerous ones.
**Decision.** Three levels: quiet (muted row, no word), warn (yellow),
danger (red bold). Behind/ahead, staged pin and detached HEAD are quiet.
**Consequences.** A long list reads as grey, with problems standing out.

## D5 — Words instead of glyphs

**Context.** Symbols (`≠ ✚ ○ ⚠ ●`) were found unintuitive.
**Decision.** Short words: `unpushed`, `conflict`, `missing`, `off pin`,
`changed` (plus `error`). Symbols remain only for arrows and tree markers.
**Consequences.** Status column is 14 columns wide; no legend needed.

## D6 — "Unpushed pin" is the most severe state

**Context.** Committing a superproject whose pin isn't on any remote breaks
everyone else's checkout.
**Decision.** `unpushed` is Danger, listed first, and its `↑n` is red.

## D7 — Hidden issues bubble up to collapsed parents

**Context.** Only the first level is shown by default, so nested problems
would be invisible.
**Decision.** Collapsed parents show `·k` (issues in all descendants), colored
by the worst one. Problems in a child do **not** mark the parent itself as
`changed`.

## D8 — Collapsed by default, branch-first navigation

**Decision.** Only the first level is visible. `→` expands and enters the
first child, `←` collapses and goes to the parent, `space` toggles,
`ctrl+→/←` expand/collapse all (see [ux-guide.md](ux-guide.md#navigation)).
`←` on an expanded node collapses it in place, so a node can be closed
without leaving it. Remembered expansion is kept on collapse, but reset by
"collapse all".

## D9 — Directory headers are path groups, not nesting

**Decision.** Submodules are grouped by the directory of their path relative
to the parent repo. Headers (`src/`, `modules/`) are muted, non-selectable,
and skipped by navigation. Nesting is expressed by indentation under the
parent submodule.

## D10 — ANSI 16 colors

**Context.** Fixed 256-colors (e.g. violet 63) ignored the user's terminal
theme and were unreadable on light backgrounds.
**Decision.** Use ANSI 0–15 for all meaning (red, yellow, blue, cyan, grey,
default foreground). Only background shades use `AdaptiveColor`. Detect the
background before Bubble Tea starts.

## D11 — Provider boundary and async loading

**Decision.** The UI accesses data only through `submodule.Provider`:
`Tree()` for the fast structure, `Status(path)` per submodule, loaded
concurrently with a spinner per row. The prototype uses a mock provider with
artificial latency.
**Consequences.** A git backend can be added without UI changes; the UI
stays responsive with many submodules.

## D12 — No network on startup

**Decision.** Work from local refs and show their age; fetching is an
explicit action (`f`).

## D13 — Mouse support via bubblezone

**Decision.** Keep mouse support (click to select/toggle, wheel to move)
using bubblezone markers per row.
