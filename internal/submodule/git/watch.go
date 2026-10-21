package git

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"submarine/internal/submodule"
)

// fingerprint summarizes the git metadata of one repository. Each part is
// a list of file names with modification time and size; a change in a part
// means something in that area changed.
type fingerprint struct {
	remote    string // FETCH_HEAD, remote-tracking branches, tags
	local     string // HEAD, index, local branches, packed refs, merge state
	structure string // .gitmodules and whether the repository is checked out
}

// Watch polls the git metadata of the superproject and all submodules every
// WatchInterval and reports the repositories that changed. Polling (instead
// of file system events) works on every file system, including WSL's
// Windows drives, and needs no watch handles.
func (p *Provider) Watch(ctx context.Context) <-chan submodule.Change {
	ch := make(chan submodule.Change, 16)
	go func() {
		defer close(ch)
		prev := p.fingerprints()
		t := time.NewTicker(p.WatchInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			cur := p.fingerprints()
			for path, f := range cur {
				old, ok := prev[path]
				if !ok || old == f {
					continue // new repositories only get a baseline
				}
				c := submodule.Change{
					Path:      path,
					Local:     old.local != f.local || old.structure != f.structure,
					Structure: old.structure != f.structure,
				}
				select {
				case ch <- c:
				case <-ctx.Done():
					return
				}
			}
			prev = cur
		}
	}()
	return ch
}

// fingerprints returns the fingerprint of every known repository, keyed by
// submodule path ("" is the superproject).
func (p *Provider) fingerprints() map[string]fingerprint {
	dirs := map[string]string{"": p.root}
	p.mu.RLock()
	for path, in := range p.mods {
		dirs[path] = in.dir
	}
	p.mu.RUnlock()

	fps := make(map[string]fingerprint, len(dirs))
	for path, dir := range dirs {
		fps[path] = fingerprintOf(dir)
	}
	return fps
}

func fingerprintOf(dir string) fingerprint {
	var f fingerprint
	var structure strings.Builder
	stamp(&structure, filepath.Join(dir, ".gitmodules"))
	gitDir, common, err := gitDirs(dir)
	if err != nil {
		f.structure = structure.String() + "not checked out"
		return f
	}
	f.structure = structure.String()

	var remote, local strings.Builder
	stamp(&remote, filepath.Join(gitDir, "FETCH_HEAD"))
	stampTree(&remote, filepath.Join(common, "refs", "remotes"))
	stampTree(&remote, filepath.Join(common, "refs", "tags"))

	for _, name := range []string{"HEAD", "index", "MERGE_HEAD"} {
		stamp(&local, filepath.Join(gitDir, name))
	}
	stamp(&local, filepath.Join(common, "packed-refs"))
	stampTree(&local, filepath.Join(common, "refs", "heads"))
	stampTree(&local, filepath.Join(common, "reftable")) // reftable ref storage
	f.remote, f.local = remote.String(), local.String()
	return f
}

// stamp appends the name, modification time and size of a file; nothing if
// it doesn't exist.
func stamp(b *strings.Builder, name string) {
	if fi, err := os.Stat(name); err == nil {
		fmt.Fprintf(b, "%s %d %d\n", name, fi.ModTime().UnixNano(), fi.Size())
	}
}

// stampTree stamps every file below dir.
func stampTree(b *strings.Builder, dir string) {
	filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			stamp(b, name)
		}
		return nil
	})
}
