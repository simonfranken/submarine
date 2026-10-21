package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// gitDirs returns the git directory of the worktree at dir and its common
// directory (they differ only for linked worktrees). A submodule's .git is
// a file "gitdir: <path>".
func gitDirs(dir string) (gitDir, commonDir string, err error) {
	dotGit := filepath.Join(dir, ".git")
	fi, err := os.Stat(dotGit)
	if err != nil {
		return "", "", err
	}
	gitDir = dotGit
	if !fi.IsDir() {
		b, err := os.ReadFile(dotGit)
		if err != nil {
			return "", "", err
		}
		p, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
		if !ok {
			return "", "", errors.New(dotGit + ": not a gitdir file")
		}
		gitDir = joinPath(dir, strings.TrimSpace(p))
	}
	commonDir = gitDir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		commonDir = joinPath(gitDir, strings.TrimSpace(string(b)))
	}
	return gitDir, commonDir, nil
}

func joinPath(base, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}
