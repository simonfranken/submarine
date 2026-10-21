package tui

// treeKey is the reloader key for loading the tree; submodules use their
// path.
const treeKey = "\x00tree"

// reloader allows at most one load per key at a time. A request while a
// load is running marks the key dirty, and the load runs once more after
// the current one finishes, so the last result always reflects the latest
// change and results never arrive out of order.
type reloader struct {
	running map[string]bool
	dirty   map[string]bool
}

func newReloader() reloader {
	return reloader{running: map[string]bool{}, dirty: map[string]bool{}}
}

// start reports whether a load for key should start now. If one is already
// running, the key is marked dirty instead.
func (r reloader) start(key string) bool {
	if r.running[key] {
		r.dirty[key] = true
		return false
	}
	r.running[key] = true
	return true
}

// done marks the load for key as finished. It reports whether the load must
// run again because it was requested meanwhile; the key then stays running.
func (r reloader) done(key string) (again bool) {
	if r.dirty[key] {
		delete(r.dirty, key)
		return true
	}
	delete(r.running, key)
	return false
}

// busy reports whether any load is running.
func (r reloader) busy() bool { return len(r.running) > 0 }
