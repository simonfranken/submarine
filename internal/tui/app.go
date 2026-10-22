// Package tui is the Bubble Tea application. It only talks to repository
// data through submodule.Provider.
package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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

// Options configure the application.
type Options struct {
	Fetch bool // fetch all repositories in the background on startup
}

type Model struct {
	ctx      context.Context
	provider submodule.Provider
	fetcher  submodule.Fetcher // nil if the provider can't fetch
	watcher  submodule.Watcher // nil if the provider can't watch
	opts     Options
	theme    theme.Theme
	keys     keyMap

	repo   *submodule.Repo
	byPath map[string]*submodule.Submodule
	err    error

	loads          reloader
	reloadStatuses bool // the running or pending tree load also reloads every status
	changes        <-chan submodule.Change
	fetch          header.Fetch
	ticking        bool // a spinner tick is scheduled

	tree    tree.Model
	detail  detail.Model
	footer  footer.Model
	spinner spinner.Model
	filter  textinput.Model
	// filtering is true while the filter line is shown; the filter applies
	// exactly then. The input is focused while typing and blurred after enter.
	filtering bool

	width, height int
	showDetail    bool
}

func New(ctx context.Context, p submodule.Provider, opts Options) Model {
	t := theme.Default()
	k := defaultKeys()
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	fi := textinput.New()
	fi.Prompt = "/ "
	fi.PromptStyle = theme.Fg(t.Primary)
	fi.Cursor.Style = theme.Fg(t.Primary)
	fi.Placeholder = "filter by name or path"
	fi.PlaceholderStyle = theme.Fg(t.Muted)
	m := Model{
		ctx:      ctx,
		provider: p,
		opts:     opts,
		theme:    t,
		keys:     k,
		loads:    newReloader(),
		ticking:  true, // Init schedules the first tick
		tree:     tree.New(t),
		detail:   detail.New(t, k.Submodule()),
		footer:   footer.New(t, k),
		spinner:  sp,
		filter:   fi,
	}
	m.fetcher, _ = p.(submodule.Fetcher)
	m.watcher, _ = p.(submodule.Watcher)
	return m
}

func (m Model) Init() tea.Cmd {
	m.loads.start(treeKey)
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

// reloadTree loads the tree again, unless a load is running (then it runs
// once more afterwards). With statuses, every status is reloaded too.
func (m *Model) reloadTree(statuses bool) tea.Cmd {
	m.reloadStatuses = m.reloadStatuses || statuses
	if !m.loads.start(treeKey) {
		return nil
	}
	return m.loadTree()
}

// reloadStatus loads one status again; the old one stays visible meanwhile.
func (m *Model) reloadStatus(path string) tea.Cmd {
	if !m.loads.start(path) {
		return nil
	}
	return m.loadStatus(path)
}

func (m Model) waitForChange() tea.Cmd {
	ch := m.changes
	return func() tea.Msg {
		c, ok := <-ch
		return changeMsg{change: c, closed: !ok}
	}
}

// startFetch fetches the superproject and every initialized submodule in
// the background. It does nothing while a fetch is running.
func (m *Model) startFetch() tea.Cmd {
	if m.fetcher == nil || m.fetch.Running || m.repo == nil {
		return nil
	}
	paths := []string{""}
	submodule.Walk(m.repo.Submodules, func(s *submodule.Submodule) {
		if m.fetcher.Fetchable(s.Path) {
			paths = append(paths, s.Path)
		}
	})
	m.fetch = header.Fetch{Running: true, Total: len(paths), At: m.fetch.At}
	cmds := []tea.Cmd{m.tick()}
	for _, p := range paths {
		f, ctx := m.fetcher, m.ctx
		cmds = append(cmds, func() tea.Msg { return fetchDoneMsg{p, f.Fetch(ctx, p)} })
	}
	return tea.Batch(cmds...)
}

// busy reports whether something shows a spinner.
func (m Model) busy() bool {
	if m.repo == nil || m.fetch.Running {
		return true
	}
	loading := m.repo.Root != nil && m.repo.Root.Status == nil && m.repo.Root.Err == nil
	submodule.Walk(m.repo.Submodules, func(s *submodule.Submodule) {
		loading = loading || (s.Status == nil && s.Err == nil)
	})
	return loading
}

// tick schedules a spinner tick if none is scheduled.
func (m *Model) tick() tea.Cmd {
	if m.ticking {
		return nil
	}
	m.ticking = true
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case treeLoadedMsg:
		return m.treeLoaded(msg)

	case statusLoadedMsg:
		if s, ok := m.byPath[msg.path]; ok {
			if msg.err != nil {
				s.Err = msg.err
			} else {
				s.Status, s.Err = msg.status, nil
			}
			m.tree.Refit()
		}
		if m.loads.done(msg.path) {
			return m, m.loadStatus(msg.path)
		}
		return m, nil

	case changeMsg:
		return m.changed(msg)

	case fetchDoneMsg:
		m.fetch.Done++
		if msg.err != nil {
			m.fetch.Failed++
		}
		if s, ok := m.byPath[msg.path]; ok {
			s.FetchErr = msg.err
		}
		if m.fetch.Done == m.fetch.Total {
			m.fetch.Running = false
			m.fetch.At = time.Now()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.tree.SetSpinner(m.spinner.View())
		m.detail.SetSpinner(m.spinner.View())
		if !m.busy() {
			m.ticking = false // stop ticking until there is work again
			return m, nil
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

// treeLoaded installs a (re)loaded tree. On a reload the known statuses are
// carried over by path, so rows don't fall back to a spinner.
func (m Model) treeLoaded(msg treeLoadedMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if m.loads.done(treeKey) {
		cmds = append(cmds, m.loadTree())
	}
	initial := m.repo == nil
	if msg.err != nil {
		if initial {
			m.err = msg.err
			return m, nil
		}
		cmds = append(cmds, m.footer.Flash("reload failed: "+msg.err.Error()))
		return m, tea.Batch(cmds...)
	}

	old := m.byPath
	m.repo = msg.repo
	m.byPath = map[string]*submodule.Submodule{}
	reload := initial || m.reloadStatuses
	m.reloadStatuses = false
	track := func(s *submodule.Submodule) {
		m.byPath[s.Path] = s
		if o, ok := old[s.Path]; ok {
			s.Status, s.Err, s.FetchErr = o.Status, o.Err, o.FetchErr
		}
		if reload || s.Status == nil {
			cmds = append(cmds, m.reloadStatus(s.Path))
		}
	}
	if m.repo.Root != nil {
		track(m.repo.Root)
	}
	submodule.Walk(m.repo.Submodules, track)
	m.tree.SetRoot(m.repo.Root)

	if initial {
		m.tree.SetRoots(m.repo.Submodules)
		if m.watcher != nil {
			m.changes = m.watcher.Watch(m.ctx)
			cmds = append(cmds, m.waitForChange())
		}
		if m.opts.Fetch {
			cmds = append(cmds, m.startFetch())
		}
	} else {
		m.tree.ReplaceRoots(m.repo.Submodules)
	}
	cmds = append(cmds, m.tick())
	return m, tea.Batch(cmds...)
}

// changed reacts to a change the watcher saw, whoever made it.
func (m Model) changed(msg changeMsg) (tea.Model, tea.Cmd) {
	if msg.closed {
		return m, nil
	}
	cmds := []tea.Cmd{m.waitForChange()}
	c := msg.change
	switch {
	case c.Path == "" || c.Structure:
		// superproject: pins of all submodules may have moved (local), or
		// only its header (remote); .gitmodules: the structure changed
		cmds = append(cmds, m.reloadTree(c.Local))
		if c.Path == "" {
			// the root's own state: files, HEAD, and (for a fetch) its upstream
			cmds = append(cmds, m.reloadStatus(""))
		}
	default:
		s, ok := m.byPath[c.Path]
		if !ok {
			break
		}
		cmds = append(cmds, m.reloadStatus(s.Path))
		if c.Local { // the children's pins live in this repository
			for _, child := range s.Children {
				cmds = append(cmds, m.reloadStatus(child.Path))
			}
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	if m.filter.Focused() {
		return m.typeFilter(msg)
	}
	switch {
	case m.filtering && msg.Type == tea.KeyEsc:
		m.closeFilter()
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
	case key.Matches(msg, k.Fetch):
		var flash string
		switch {
		case m.fetcher == nil:
			flash = "this data source can't fetch"
		case m.fetch.Running:
			flash = "already fetching"
		}
		if flash != "" {
			cmd := m.footer.Flash(flash)
			m.layout()
			return m, cmd
		}
		return m, m.startFetch()
	case key.Matches(msg, k.Reload):
		return m, m.reloadTree(true)
	case key.Matches(msg, k.Copy):
		return m.copyPath()
	case key.Matches(msg, k.Filter):
		m.filtering = true
		m.layout()
		return m, m.filter.Focus()
	case key.Matches(msg, k.Issues):
		m.tree.SetProblemsOnly(!m.tree.ProblemsOnly())
		m.layout()
	case key.Matches(msg, k.Update, k.Bump):
		cmd := m.footer.Flash("“" + msg.String() + "” is not implemented in the prototype")
		m.layout()
		return m, cmd
	}
	return m, nil
}

// typeFilter handles a key while the filter input has the focus: text edits
// the filter, enter hands the keys back to the tree (the filter stays), esc
// closes the filter. Arrows still move the selection.
func (m Model) typeFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.closeFilter()
	case tea.KeyEnter:
		m.filter.Blur()
	case tea.KeyUp:
		m.tree.Up()
	case tea.KeyDown:
		m.tree.Down()
	case tea.KeyPgUp:
		m.tree.PageUp()
	case tea.KeyPgDown:
		m.tree.PageDown()
	default:
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.tree.SetFilter(m.filter.Value())
		return m, cmd
	}
	return m, nil
}

// closeFilter hides the filter line and removes the filter; the selection stays.
func (m *Model) closeFilter() {
	m.filtering = false
	m.filter.Blur()
	m.filter.Reset()
	m.tree.SetFilter("")
	m.layout()
}

// filterView is the line above the footer: the input and, right-aligned, the
// keys that apply.
func (m Model) filterView() string {
	hint := "enter navigate • esc close"
	if !m.filter.Focused() {
		hint = "/ edit • esc close"
	}
	if m.tree.ProblemsOnly() {
		hint = "problems only (!) • " + hint
	}
	hint = theme.Fg(m.theme.Muted).Render(hint)
	m.filter.Width = max(m.width-lipgloss.Width(hint)-6, 1)
	in := m.filter.View()
	gap := max(m.width-2-lipgloss.Width(in)-lipgloss.Width(hint), 1)
	return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(in + strings.Repeat(" ", gap) + hint)
}

// extraLine reports whether a line above the footer is shown: the filter, or
// the notice that only problems are shown.
func (m Model) extraLine() bool { return m.filtering || m.tree.ProblemsOnly() }

// problemsView is the line above the footer while only problems are shown, so
// a short list is not mistaken for a healthy repository.
func (m Model) problemsView() string {
	left := theme.Fg(m.theme.Warn).Render("problems only")
	right := theme.Fg(m.theme.Muted).Render("! show all")
	gap := max(m.width-2-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(left + strings.Repeat(" ", gap) + right)
}

// copyPath copies the absolute directory of the selected submodule to the
// clipboard, or its path in the superproject if the provider can't tell.
func (m Model) copyPath() (tea.Model, tea.Cmd) {
	s := m.tree.Selected()
	if s == nil {
		return m, nil
	}
	path := s.Path
	if s.IsRoot() && m.repo != nil {
		path = m.repo.Path
	}
	if l, ok := m.provider.(submodule.Locator); ok {
		if dir := l.Dir(s.Path); dir != "" {
			path = dir
		}
	}
	cmd := tea.Batch(copyToClipboard(path), m.footer.Flash("copied "+path))
	m.layout()
	return m, cmd
}

// layout distributes the terminal size among the components.
func (m *Model) layout() {
	m.footer.SetWidth(m.width)
	bodyHeight := m.height - headerHeight - lipgloss.Height(m.footer.View())
	if m.extraLine() {
		bodyHeight--
	}
	bodyHeight = max(bodyHeight, 0)

	m.showDetail = m.width >= detailHideBelow
	treeWidth := m.width
	if m.showDetail {
		detailWidth := max(int(float64(m.width)*detailWidthRatio), minDetailWidth)
		treeWidth = m.width - detailWidth
		m.detail.SetSize(detailWidth-2, bodyHeight) // border + padding
	}
	m.footer.SetActions(!m.showDetail || !m.detail.PaletteVisible())
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
			Render(m.detail.View(m.tree.Selected(), m.repo))
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, pane)
	}

	head := header.View(m.theme, m.repo, m.fetch, m.spinner.View(), m.width, m.tree.RootSelected())
	parts := []string{zone.Mark(m.tree.RootZone(), head), body}
	if m.filtering {
		parts = append(parts, m.filterView())
	} else if m.tree.ProblemsOnly() {
		parts = append(parts, m.problemsView())
	}
	return zone.Scan(lipgloss.JoinVertical(lipgloss.Left, append(parts, m.footer.View())...))
}
