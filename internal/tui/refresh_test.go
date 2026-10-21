package tui

import "testing"

func TestReloader(t *testing.T) {
	r := newReloader()
	if !r.start("a") {
		t.Fatal("first start: want true")
	}
	if r.start("a") || r.start("a") {
		t.Fatal("start while running: want false")
	}
	if !r.start("b") {
		t.Fatal("other key: want true")
	}
	if !r.done("a") {
		t.Fatal("done after requests while running: want again")
	}
	if r.done("a") {
		t.Fatal("second done: requests were coalesced into one rerun")
	}
	if r.done("b") {
		t.Fatal("done without requests: want no rerun")
	}
	if r.busy() {
		t.Fatal("busy after all done")
	}
	if !r.start("a") {
		t.Fatal("start after done: want true")
	}
}
