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
