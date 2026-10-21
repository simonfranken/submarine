package tui

import "submarine/internal/submodule"

type treeLoadedMsg struct {
	repo *submodule.Repo
	err  error
}

type statusLoadedMsg struct {
	path   string
	status *submodule.Status
	err    error
}

// fetchDoneMsg reports one finished fetch. It only updates the fetch
// progress; refreshing is left to the watcher, which also sees fetches made
// outside the application.
type fetchDoneMsg struct {
	path string // "" is the superproject
	err  error
}

// changeMsg is a change reported by the provider's watcher.
type changeMsg struct {
	change submodule.Change
	closed bool // the watcher stopped
}
