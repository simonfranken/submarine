// Package tui is the Bubble Tea application. It only talks to repository
// data through submodule.Provider.
package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"submarine/internal/submodule"
	"submarine/internal/tui/detail"
	"submarine/internal/tui/footer"
	"submarine/internal/tui/header"
	"submarine/internal/tui/theme"
	"submarine/internal/tui/tree"
)

const (
	headerHeight     = 1
	minDetailWidth   = 36
	detailHideBelow  = 90 // terminal width below which the detail pane is hidden
	detailWidthRatio = 0.4
)

type Model struct {
	ctx      context.Context
	provider submodule.Provider
	theme    theme.Theme
	keys     keyMap

	repo    *submodule.Repo
	byPath  map[string]*submodule.Submodule
	loading int
	err     error

	tree    tree.Model
	detail  detail.Model
	footer  footer.Model
	spinner spinner.Model

	width, height int
	showDetail    bool
}

func New(ctx context.Context, p submodule.Provider) Model {
	t := theme.Default()
	k := defaultKeys()
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	return Model{
		ctx:      ctx,
		provider: p,
		theme:    t,
		keys:     k,
		tree:     tree.New(t),
		detail:   detail.New(t),
		footer:   footer.New(t, k),
		spinner:  sp,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadTree(), m.spinner.Tick)
}

func (m Model) loadTree() tea.Cmd {
	return func() tea.Msg {
		repo, err := m.provider.Tree(m.ctx)
		return treeLoadedMsg{repo, err}
	}
}

func (m Model) loadStatus(path string) tea.Cmd {
	return func() tea.Msg {
		st, err := m.provider.Status(m.ctx, path)
		return statusLoadedMsg{path, st, err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case treeLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.repo = msg.repo
		m.byPath = map[string]*submodule.Submodule{}
		var cmds []tea.Cmd
		submodule.Walk(m.repo.Submodules, func(s *submodule.Submodule) {
			m.byPath[s.Path] = s
			cmds = append(cmds, m.loadStatus(s.Path))
		})
		m.loading = len(cmds)
		m.tree.SetRoots(m.repo.Submodules)
		return m, tea.Batch(cmds...)

	case statusLoadedMsg:
		if s, ok := m.byPath[msg.path]; ok {
			s.Status, s.Err = msg.status, msg.err
			m.loading--
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.tree.SetSpinner(m.spinner.View())
		m.detail.SetSpinner(m.spinner.View())
		if m.repo != nil && m.loading == 0 {
			return m, nil // stop ticking once everything is loaded
		}
		return m, cmd

	case tea.MouseMsg:
		m.tree.Update(msg)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	m.footer.Update(msg)
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.footer.ToggleHelp()
		m.layout()
	case key.Matches(msg, k.Up):
		m.tree.Up()
	case key.Matches(msg, k.Down):
		m.tree.Down()
	case key.Matches(msg, k.Left):
		m.tree.Left()
	case key.Matches(msg, k.Right):
		m.tree.Right()
	case key.Matches(msg, k.Toggle):
		m.tree.Toggle()
	case key.Matches(msg, k.CollapseAll):
		m.tree.CollapseAll()
	case key.Matches(msg, k.ExpandAll):
		m.tree.ExpandAll()
	case key.Matches(msg, k.Home):
		m.tree.Home()
	case key.Matches(msg, k.End):
		m.tree.End()
	case key.Matches(msg, k.PageUp):
		m.tree.PageUp()
	case key.Matches(msg, k.PageDown):
		m.tree.PageDown()
	case key.Matches(msg, k.Update, k.Bump, k.Fetch, k.Filter, k.Issues):
		cmd := m.footer.Flash("“" + msg.String() + "” is not implemented in the prototype")
		m.layout()
		return m, cmd
	}
	return m, nil
}

// layout distributes the terminal size among the components.
func (m *Model) layout() {
	m.footer.SetWidth(m.width)
	bodyHeight := max(m.height-headerHeight-lipgloss.Height(m.footer.View()), 0)

	m.showDetail = m.width >= detailHideBelow
	treeWidth := m.width
	if m.showDetail {
		detailWidth := max(int(float64(m.width)*detailWidthRatio), minDetailWidth)
		treeWidth = m.width - detailWidth
		m.detail.SetSize(detailWidth-2, bodyHeight) // border + padding
	}
	m.tree.SetSize(treeWidth, bodyHeight)
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.err != nil {
		return theme.Fg(m.theme.Danger).Render("error: " + m.err.Error())
	}

	body := m.tree.View()
	if m.showDetail {
		pane := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(m.theme.Border).
			PaddingLeft(1).
			Render(m.detail.View(m.tree.Selected()))
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, pane)
	}

	return zone.Scan(lipgloss.JoinVertical(lipgloss.Left,
		header.View(m.theme, m.repo, m.width),
		body,
		m.footer.View(),
	))
}
