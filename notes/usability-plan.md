# Usability plan (draft)

**Status:** items 1–8 and 11 are done ([D31](../docs/decisions.md),
[D32](../docs/decisions.md)); the rest is a proposal. Items that change
behavior need a decision in [decisions.md](../docs/decisions.md) and
updates to [ux-guide.md](../docs/ux-guide.md) /
[actions.md](../docs/actions.md) when they are taken up.

Done differently than proposed below: the labels are `x restore to pin` and
`X restore committed pins`; the tools read `! problems only • / filter • f
fetch` (most useful first, dropped from the end); `space toggle` stays, as
it now follows `←→/hl collapse/expand`.

## Goal

The target user is a developer who doesn't know git submodules well. They
should understand the screen and act safely **without reading a manual**.
At any moment the screen should answer three questions:

1. **What is wrong?** What does this state mean, in plain words?
2. **What can I do about it?** Which key fixes it?
3. **What will this key do?** What happens to my work, and can I undo it?

Today the tool answers (1) partly (the issue lines in the detail pane) and
(3) well *after* a key is pressed (loss question, flash). It hardly answers
(2), and it answers (3) only by trying. Most items below move the answers
from "after pressing" to "before pressing".

## Findings this is based on

From a walkthrough on the `make-fixture.sh` superproject (2026-10-24):

- `x restore` and `X hard restore` don't say the target ("to the pin"), the
  scope (the submodule and everything below it) or how they differ. "Hard"
  suggests "discards my changes", but both do that. The real difference,
  staged pins, can't be guessed.
- `X` drops a staged pin without asking (`restored libs/staged to the
  committed pins`), one Shift away from `x`.
- `f` fetch, `/` filter and `!` problems only appear only in `?`. The detail
  pane says `fetched never` but gives no hint how to fetch. `!` is the most
  useful view for many submodules, and nothing points to it.
- The words in the tree (`off pin`, `off target`, `staged`, `●`, `·1`, `↓2`)
  and on the spine (`pin`, `tip`, `you`, `base`) are explained nowhere in
  the UI.
- `u update pin` on the superproject actually fast-forwards the current
  branch; the label stays the same.
- The palette is static: `u` is offered when it can't do anything, and the
  user finds out from a flash.
- Mismatched words: the footer says "fold", the help "collapse/expand";
  `space toggle` doesn't say what it toggles.
- Good, and worth keeping: questions only when something would be lost, the
  loss listed exactly, flashes that say why and suggest another key.

## Overview

Effort: **S** = hours, mostly labels and docs; **M** = a day or two, new
logic in existing components; **L** = several days, new UI or new provider
calls.
Impact: how much it helps the target user's main flows (*understand state*,
*fix a submodule*, *find problems*, *stay safe*).

| # | Item | Effort | Impact | Flow |
|---|---|---|---|---|
| 1 ✓ | Clear names for restore and hard restore | S | High | fix, safe |
| 2 ✓ | Ask before `X` drops staged pins | S | High | safe |
| 3 ✓ | Show `/`, `!`, `f` in the footer | S | High | find |
| 4 ✓ | Status legend in `?` | S–M | High | understand |
| 5 ✓ | Groups with titles in the full help | S | Medium | understand |
| 6 ✓ | Plain-word fixes for wording | S | Low | understand |
| 7 ✓ | `u` label for the superproject | S | Medium | fix |
| 8 ✓ | Fetch hint at `fetched never` / old fetches | S | Medium | understand |
| 9 | Next step for each issue in the detail pane | M | **Highest** | understand, fix |
| 10 | Palette that fits the selection | M | High | fix |
| 11 ✓ | First-start hint | S | Medium | understand |
| 12 | Undo hint after a restore, switch or update | M | Medium | safe |
| 13 | Action menu on `enter` | M–L | High | fix, understand |
| 14 | "What would happen" preview | L | Medium | safe |

Quick view by quadrant:

|  | **Low effort** | **Medium effort** | **High effort** |
|---|---|---|---|
| **High impact** | 1, 2, 3, 4 | 9, 10, 13 | |
| **Medium impact** | 5, 7, 8, 11 | 12 | 14 |
| **Low impact** | 6 | | |

## Items

### Low effort

**1. Clear names for restore and hard restore.** *S · High*
The labels should give target and difference without the docs.
Proposal: `x restore to pin`, `X restore, unstage pins`. In the full help,
longer: `x restore to pin (with everything below)`, `X restore to committed
pins, drop staged ones`. The question lines already use "to the committed
pins"; keep the same words everywhere.
Alternative: rename the action to "reset". Rejected for now: `git reset`
moves branches, which restore never does (D23).
Docs: ux-guide key reference, actions.md "Keys and palette", README.

**2. Ask before `X` drops staged pins.** *S · High*
Amends D30 ("alone they don't make it ask"). A staged pin is a decision
the user made, and `X` sits next to `x`. If `X` would drop at least one
staged pin, ask: `Restore src/app to the committed pins? Unstages 2 pins.
y restore · n cancel`. The loss is already counted, so only the trigger
changes. D30's argument (the commit stays in the submodule) holds for an
expert, not for someone who doesn't know where to find it again.

**3. Show `/`, `!`, `f` in the footer.** *S · High*
Proposal: `/ filter • ! problems • f fetch` after the navigation keys,
before the global keys. The existing rule drops keys from the left group's
end when the footer is too narrow, so narrow terminals lose them first and
nothing else changes. While problems only is on, the footer could read
`! all` (the action the key does now). `r reload` stays in `?`.

**4. Status legend in `?`.** *S–M · High*
The full help gets a second block, "What the words mean", with one line per
status word and symbol, using the same colors as the tree:
`off pin  the checkout is not the commit the parent records`, `staged`,
`●`, `↑n ↓n`, `·k`, and the spine roles (`pin`, `tip`, `you`, `base`,
target). Wording comes from state-model.md; the explanations the detail
pane already uses are a good source. Layout: below the key columns if there
is room, otherwise `?` pages between keys and legend (`? more`).

**5. Groups with titles in the full help.** *S · Medium*
The help columns are already grouped, but without titles. Add short titles
on top: `Move`, `Selected submodule`, `All repositories`, `App`. Then it is
clear that `x` acts on the selection and `f` on everything.

**6. Plain-word fixes.** *S · Low*
- Footer `fold` → `collapse/expand`; `⇧←→/HL fold all` → `H/L all`.
- `space toggle` → `space open/close`.
- `c copy path` → say which path (`copy path` is fine if it is the
  relative path; check).
- Header `pin staged` vs tree `staged`: one word.

**7. `u` label for the superproject.** *S · Medium*
With the header selected the palette says `u pull (fast-forward)`, since
that is what it does (D29). Labels that change with the selection need
item 10's mechanism, but this one is simple on its own: one check for the
header.

**8. Fetch hint.** *S · Medium*
`fetched never` (and an age above a threshold, e.g. a day) gets a muted
`· f fetches`. Users who don't know that the arrows (`↓2`) are only as fresh
as the last fetch otherwise trust old numbers.

**11. First-start hint.** *S · Medium*
On start, the footer's flash line shows `? explains keys and states` until
the first key. No config file, no state: it is shown every start, and it is
just a muted line. Makes item 4 discoverable.

### Medium effort

**9. Next step for each issue in the detail pane.** *M · Highest*
Each issue line ends with what to do about it, as keys:

```
off pin    checkout is not the pin        x back to pin · b record a branch
changed    3 files changed                commit outside, or x discard
missing    not initialized                x restore
off target pin is not on origin/main      b switch branch
```

A pin that is only behind (`↓2`, no issue) gets the same kind of hint on
its `Ref` line: `u update pin`.

This is the single biggest step for the target user: they no longer need to
know what to do, only to read. Layering: which action resolves which issue
is meaning, not looks, so it belongs in `internal/submodule` (e.g. a
`Remedies()` on the issue, returning action kinds) and the words in
`theme`. It must agree with what the actions really allow (e.g. `u` only
when behind and not ahead, D29), which item 10 needs anyway.

**10. Palette that fits the selection.** *M · High*
Amends actions.md ("the palette is static"). The palette keeps the same keys
in the same places (muscle memory), but:
- keys that can't do anything for the selection are drawn muted (`u` when
  up to date, `b` on a missing submodule); pressing them still flashes why;
- the key that item 9 recommends for the most severe issue is highlighted.
The checks for `u` already exist as flash reasons; they are cheap and need
only the status, so this is a matter of calling them for the palette too.
Restore stays always active (it is idempotent).

**12. Undo hint after a restore, switch or update.** *M · Medium*
The success flash names the old commit when a repository moved:
`restored src/app (was 1a2b3c4)`. A flash only lasts a moment, so the detail
pane could keep `previous  1a2b3c4` until the next reload. For a user who
doesn't know `git reflog`, this is the difference between "lost" and
"recoverable". No new provider call if the SHA is taken before the action.

**13. Action menu on `enter`.** *M–L · High*
`enter` on a row opens a small popup (reusing the branch popup's frame)
listing the actions for this submodule, each with one line of explanation
and its key; the recommended one first and selected. `enter` runs it, `esc`
closes. Users who know the keys never need it; users who don't need no
memory at all. `enter` is unused in the tree today.

### High effort

**14. "What would happen" preview.** *L · Medium*
In the action menu (13), the selected entry shows the effect before
running it: `moves the pin e510d9b → 906f119 (2 commits)`, `discards 3
changed files`. The loss check exists, but it is async git work, so it runs
when the entry is selected, with a spinner. Valuable, but the loss question
already covers the dangerous cases, so this comes last.

## Order

1. **Labels and safety** (1, 2, 3, 5, 6, 7): only labels, docs and one
   question trigger. Fixes the worst risk (`X`) and the hidden keys at once.
2. **Explain the screen** (4, 8, 11): the legend and the hints that point
   to it.
3. **Guide to the fix** (9, 10): issue → key, and a palette that agrees.
   Needs a decision on where remedies live (layering).
4. **Menus and previews** (12, 13, 14).

## How to check

Give someone who hasn't used submarine the fixture (`make-fixture.sh`) and
these tasks, without the docs:

- Find every submodule that needs attention.
- Get `ui` up to date with its branch.
- Put `core` back where the superproject wants it.
- Find out whether the arrows are current.
- Explain what `staged` on `libs/staged` means.

Note keys tried, wrong actions and questions asked. Run it before phase 1
and after each phase.

## Open questions

- Item 1: are "restore to pin" / "restore, unstage pins" the right words, or
  should `X` move under a different key that is not one Shift away?
- Item 2: ask for every dropped staged pin, or only for the selection's own?
- Item 9: are external actions (`commit outside submarine`) good hints, or
  only keys?
- Item 13: does `enter` stay free for something else (e.g. opening a shell
  in the submodule)?
