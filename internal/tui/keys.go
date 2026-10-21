package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up, Down, Left, Right, Toggle       key.Binding
	CollapseAll, ExpandAll              key.Binding
	Home, End, PageUp, PageDown         key.Binding
	Update, Bump, Fetch, Filter, Issues key.Binding
	Help, Quit                          key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "move")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:        key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/→", "collapse/expand")),
		Right:       key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand, first child")),
		Toggle:      key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		CollapseAll: key.NewBinding(key.WithKeys("ctrl+left"), key.WithHelp("ctrl+←", "collapse all")),
		ExpandAll:   key.NewBinding(key.WithKeys("ctrl+right"), key.WithHelp("ctrl+→", "expand all")),
		Home:        key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("home/g", "first")),
		End:         key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("end/G", "last")),
		PageUp:      key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown:    key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		Update:      key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "checkout pin")),
		Bump:        key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "bump pin to tip")),
		Fetch:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fetch")),
		Filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Issues:      key.NewBinding(key.WithKeys("!"), key.WithHelp("!", "problems only")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Left, k.Toggle, k.Update, k.Bump, k.Fetch, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Toggle},
		{k.CollapseAll, k.ExpandAll},
		{k.Home, k.End, k.PageUp, k.PageDown},
		{k.Update, k.Bump, k.Fetch, k.Filter, k.Issues},
		{k.Help, k.Quit},
	}
}
