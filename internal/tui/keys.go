package tui

import (
	"github.com/charmbracelet/bubbles/key"

	"submarine/internal/tui/footer"
)

type keyMap struct {
	Up, Down, Left, Right, Toggle key.Binding
	CollapseAll, ExpandAll        key.Binding
	Home, End, PageUp, PageDown   key.Binding
	Fetch, Reload, Filter, Issues key.Binding
	Copy, Restore, Switch, Update key.Binding
	HardRestore, Resolve          key.Binding
	Help, Quit, QuitUpdate        key.Binding

	conflict bool // the selection has a pin conflict: Resolve leads the palette
}

func defaultKeys() keyMap {
	return keyMap{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:        key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse, parent")),
		Right:       key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand, first child")),
		Toggle:      key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		CollapseAll: key.NewBinding(key.WithKeys("shift+left", "ctrl+left", "H"), key.WithHelp("H/⇧←", "collapse all")),
		ExpandAll:   key.NewBinding(key.WithKeys("shift+right", "ctrl+right", "L"), key.WithHelp("L/⇧→", "expand all")),
		Home:        key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("home/g", "first")),
		End:         key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("end/G", "last")),
		PageUp:      key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown:    key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		Fetch:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fetch")),
		Reload:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Issues:      key.NewBinding(key.WithKeys("!"), key.WithHelp("!", "problems only")),
		Restore:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "restore to pin")),
		HardRestore: key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "restore committed pins")),
		Switch:      key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "switch branch")),
		Update:      key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "update pin")),
		Resolve:     key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "resolve pin conflict")),
		Copy:        key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy path")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		QuitUpdate:  key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit and update"), key.WithDisabled()),
	}
}

// SetUpdate announces the newer release v: q quits and updates to it, and the
// plain quit moves to Q (ctrl+c always quits without updating).
func (k *keyMap) SetUpdate(v string) {
	k.Quit = key.NewBinding(key.WithKeys("Q", "ctrl+c"), key.WithHelp("Q", "quit"))
	k.QuitUpdate = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit and update to "+v))
}

// Navigation returns the bindings that move the selection or fold the tree.
func (k keyMap) Navigation() []key.Binding {
	move := key.NewBinding(key.WithKeys("up"), key.WithHelp("↑↓/jk", "move"))
	fold := key.NewBinding(key.WithKeys("left"), key.WithHelp("←→/hl", "collapse/expand"))
	all := key.NewBinding(key.WithKeys("H"), key.WithHelp("⇧←→/HL", "all"))
	return []key.Binding{move, fold, all, k.Toggle}
}

// Tools returns the bindings that act on the whole list: problems only,
// filter and fetch, most useful first, since the footer drops them from the
// end. It shows them after the navigation keys.
func (k keyMap) Tools() []key.Binding { return []key.Binding{k.Issues, k.Filter, k.Fetch} }

// Submodule returns the bindings that act on the selected submodule. They
// are shown in the detail pane, or in the footer if the pane can't show them.
// Resolve is only among them, first, for a submodule with a pin conflict.
func (k keyMap) Submodule() []key.Binding {
	keys := []key.Binding{k.Copy, k.Update, k.Switch, k.Restore, k.HardRestore}
	if k.conflict {
		keys = append([]key.Binding{k.Resolve}, keys...)
	}
	return keys
}

// Global returns the bindings that don't depend on the selection.
func (k keyMap) Global() []key.Binding { return []key.Binding{k.Help, k.Quit} }

// fullUpdate is Highlight with a short description, for the full help.
func (k keyMap) fullUpdate() []key.Binding {
	if !k.QuitUpdate.Enabled() {
		return nil
	}
	return []key.Binding{key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit and update"))}
}

// Highlight returns the bindings that are shown after Global on a highlighted
// background: quit and update, if there is an update.
func (k keyMap) Highlight() []key.Binding {
	if !k.QuitUpdate.Enabled() {
		return nil
	}
	return []key.Binding{k.QuitUpdate}
}

// forSelection returns the keys as they read for the selection: on the
// superproject, u fast-forwards its branch (D29); a submodule with a pin
// conflict offers m first.
func (k keyMap) forSelection(root, conflict bool) keyMap {
	if root {
		k.Update.SetHelp("u", "pull (fast-forward)")
	}
	k.conflict = conflict
	return k
}

// HelpGroups returns the columns of the full help, each with a title.
func (k keyMap) HelpGroups() []footer.Group {
	help := key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "what words mean"))
	esc := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close"))
	return []footer.Group{
		{Title: "Move", Keys: []key.Binding{k.Up, k.Down, k.Left, k.Right, k.Toggle, k.CollapseAll, k.ExpandAll}},
		{Title: "Jump", Keys: []key.Binding{k.Home, k.End, k.PageUp, k.PageDown}},
		{Title: "Selected submodule", Keys: k.helpSubmodule()},
		{Title: "All submodules", Keys: []key.Binding{k.Fetch, k.Reload, k.Filter, k.Issues}},
		{Title: "App", Keys: append([]key.Binding{help, esc, k.Quit}, k.fullUpdate()...)},
	}
}

// helpSubmodule is the full help's column of submodule keys: all of them,
// m too, whatever is selected.
func (k keyMap) helpSubmodule() []key.Binding {
	if k.conflict {
		return k.Submodule() // m is first already
	}
	return append(k.Submodule(), k.Resolve)
}

// LegendKeys returns the column next to the legend, the help's last page;
// ? closes it too, but esc is the key to tell.
func (k keyMap) LegendKeys() footer.Group {
	esc := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close"))
	return footer.Group{Title: "App", Keys: append([]key.Binding{esc, k.Quit}, k.fullUpdate()...)}
}
