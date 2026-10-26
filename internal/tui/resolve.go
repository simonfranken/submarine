package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"submarine/internal/submodule"
	"submarine/internal/tui/branches"
)

// resolveChoice is a pin that resolves a pin conflict: sha ("" removes the
// submodule) and how the footer calls it.
type resolveChoice struct{ sha, label string }

// startResolve opens the picker of the pins that resolve the pin conflict of
// the selection: ours, theirs and the commits that contain both, the first
// of those preselected.
func (m Model) startResolve() (tea.Model, tea.Cmd) {
	s := m.tree.Selected()
	_, ok := m.provider.(submodule.Resolver)
	var flash string
	switch {
	case s == nil:
		return m, nil
	case !ok:
		flash = "this data source can't resolve pin conflicts"
	case m.restore != nil:
		flash = "wait for the restore of " + m.restore.name + " to finish"
	case s.Status == nil || s.Status.PinConflict == nil:
		flash = restoreName(s) + " has no pin conflict"
	}
	if flash != "" {
		cmd := m.footer.Flash(flash)
		m.layout()
		return m, cmd
	}

	items, notes, choices, cursor := resolveItems(s.Status.PinConflict)
	m.picker = branches.NewChoices(m.theme, "Resolve", s.Path, items, notes, cursor)
	m.pickerPath, m.pickerResolve = s.Path, choices
	return m, nil
}

// resolveItems lists the pins that resolve c, as picker items with a note
// each, and which one to preselect: the first commit that contains both
// sides, else ours.
func resolveItems(c *submodule.PinConflict) (items []submodule.Branch, notes []string, choices []resolveChoice, cursor int) {
	add := func(b submodule.Branch, note, label string) {
		items, notes = append(items, b), append(notes, note)
		choices = append(choices, resolveChoice{b.SHA, label})
	}
	cursor = -1
	for _, side := range []struct {
		s    submodule.ConflictSide
		role string
	}{{c.Ours, "ours"}, {c.Theirs, "theirs"}} {
		if side.s.Deleted() {
			add(submodule.Branch{Name: "remove", Ref: "remove"}, side.s.Name+" removed the submodule", "removed")
			continue
		}
		b := submodule.Branch{Name: side.s.Name, Ref: side.role, SHA: side.s.SHA}
		note := side.role
		switch {
		case side.s.Commit == nil:
			note += " · not fetched"
		case !side.s.Pin.Pushed:
			b.When = side.s.Commit.When
			note += " · unpushed"
		default:
			b.When = side.s.Commit.When
			if side.s.Pin.Ref != "" {
				note += " · " + side.s.Pin.Ref + distance(side.s.Pin)
			}
		}
		for _, cand := range c.Candidates {
			if cand.Side == side.role {
				note += " · has both"
				cursor = len(items)
			}
		}
		add(b, note, side.s.Name+" "+short(side.s.SHA))
	}
	for _, cand := range c.Candidates {
		if cand.Side != "" {
			continue // the side above
		}
		b := submodule.Branch{Ref: cand.SHA, SHA: cand.SHA, When: cand.Commit.When}
		note := "has both"
		if cand.Checkout {
			b.Name, note = "checkout", "your checkout, has both"
		} else {
			b.Name, b.Remote = cand.Ref, true
			if cand.Behind > 0 {
				note += fmt.Sprintf(" · %d behind the tip", cand.Behind)
			}
		}
		if cursor < 0 {
			cursor = len(items)
		}
		add(b, note, b.Name+" "+short(cand.SHA))
	}
	return items, notes, choices, max(cursor, 0)
}

// distance is " ↑n↓m" for the pin's distance to its ref, "" if on its tip.
func distance(p submodule.PinInfo) string {
	var s string
	if p.Ahead > 0 {
		s += fmt.Sprintf("↑%d", p.Ahead)
	}
	if p.Behind > 0 {
		s += fmt.Sprintf("↓%d", p.Behind)
	}
	if s == "" {
		return ""
	}
	return " " + s
}

func short(sha string) string { return sha[:min(7, len(sha))] }
