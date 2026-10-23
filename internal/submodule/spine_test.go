package submodule

import (
	"slices"
	"testing"
)

func TestSpine(t *testing.T) {
	c := func(sha string) *Commit { return &Commit{SHA: sha} }
	tests := []struct {
		name  string
		st    Status
		nodes []string // "sha:roles"
		links []Link   // Below of every node but the last
	}{
		{
			name:  "in sync",
			st:    Status{Pinned: c("a"), CheckedOut: c("a"), Tip: c("a")},
			nodes: []string{"a:pin,tip,you"},
		},
		{
			name:  "pin behind tip, checkout on pin",
			st:    Status{Pinned: c("p"), CheckedOut: c("p"), Tip: c("t"), Pin: PinInfo{Behind: 4}},
			nodes: []string{"t:tip", "p:pin,you"},
			links: []Link{{Above: 4}},
		},
		{
			name:  "pin on tip, checkout behind",
			st:    Status{Pinned: c("p"), CheckedOut: c("y"), Tip: c("p"), Drift: Distance{Behind: 2}},
			nodes: []string{"p:pin,tip", "y:you"},
			links: []Link{{Above: 2}},
		},
		{
			name:  "all apart",
			st:    Status{Pinned: c("p"), CheckedOut: c("y"), Tip: c("t"), Pin: PinInfo{Behind: 4}, Drift: Distance{Behind: 2}},
			nodes: []string{"t:tip", "p:pin", "y:you"},
			links: []Link{{Above: 4}, {Above: 2}},
		},
		{
			name:  "checkout ahead of the pin",
			st:    Status{Pinned: c("p"), CheckedOut: c("y"), Tip: c("t"), Pin: PinInfo{Behind: 5}, Drift: Distance{Ahead: 2}},
			nodes: []string{"t:tip", "y:you", "p:pin"},
			links: []Link{{Above: 3}, {Above: 2}},
		},
		{
			name:  "checkout on the tip",
			st:    Status{Pinned: c("p"), CheckedOut: c("t"), Tip: c("t"), Pin: PinInfo{Behind: 3}, Drift: Distance{Ahead: 3}},
			nodes: []string{"t:tip,you", "p:pin"},
			links: []Link{{Above: 3}},
		},
		{
			name:  "pin ahead of its ref",
			st:    Status{Pinned: c("p"), CheckedOut: c("p"), Tip: c("t"), Pin: PinInfo{Ahead: 1}},
			nodes: []string{"p:pin,you", "t:tip"},
			links: []Link{{Above: 1}},
		},
		{
			name:  "pin diverged from tip",
			st:    Status{Pinned: c("p"), CheckedOut: c("p"), Tip: c("t"), Pin: PinInfo{Ahead: 2, Behind: 3}},
			nodes: []string{"t:tip", "p:pin,you"},
			links: []Link{{Above: 3, Below: 2}},
		},
		{
			name:  "checkout diverged from pin",
			st:    Status{Pinned: c("p"), CheckedOut: c("y"), Tip: c("p"), Drift: Distance{Ahead: 1, Behind: 2}},
			nodes: []string{"p:pin,tip", "y:you"},
			links: []Link{{Above: 2, Below: 1}},
		},
		{
			name: "staged pin above the committed one, checkout on the staged pin",
			st: Status{Pinned: c("s"), CheckedOut: c("s"), Tip: c("s"), PinStaged: true,
				Committed: c("p"), CommittedDrift: Distance{Behind: 3}},
			nodes: []string{"s:staged,tip,you", "p:pin"},
			links: []Link{{Above: 3}},
		},
		{
			name: "checkout stayed on the committed pin",
			st: Status{Pinned: c("s"), CheckedOut: c("p"), Tip: c("t"), PinStaged: true, Pin: PinInfo{Behind: 1},
				Committed: c("p"), CommittedDrift: Distance{Behind: 2}, Drift: Distance{Behind: 2}},
			nodes: []string{"t:tip", "s:staged", "p:pin,you"},
			links: []Link{{Above: 1}, {Above: 2}},
		},
		{
			name: "tip diverged from the pin: a fork",
			st: Status{Pinned: c("p"), CheckedOut: c("p"), Tip: c("t"), Pin: PinInfo{Ahead: 1, Behind: 2},
				Bases: Bases{Tip: c("b")}},
			nodes: []string{"t:tip", "p:pin,you", "b:base"},
			links: []Link{{Above: 2}, {Above: 1}},
		},
		{
			name: "checkout diverged from the pin: a fork",
			st: Status{Pinned: c("p"), CheckedOut: c("y"), Tip: c("p"), Drift: Distance{Ahead: 3, Behind: 2},
				Bases: Bases{Checkout: c("b")}},
			nodes: []string{"y:you", "p:pin,tip", "b:base"},
			links: []Link{{Above: 3}, {Above: 2}},
		},
		{
			name: "fork with the tip above the pin on the main lane",
			st: Status{Pinned: c("p"), CheckedOut: c("y"), Tip: c("t"), Pin: PinInfo{Behind: 2},
				Drift: Distance{Ahead: 1, Behind: 1}, Bases: Bases{Checkout: c("b")}},
			nodes: []string{"y:you", "t:tip", "p:pin", "b:base"},
			links: []Link{{Above: 1}, {Above: 2}, {Above: 1}},
		},
		{
			name: "something older than the base continues below it",
			st: Status{Pinned: c("p"), CheckedOut: c("y"), Tip: c("t"), Pin: PinInfo{Ahead: 1, Behind: 2},
				Bases: Bases{Tip: c("b")}, Drift: Distance{Behind: 3}},
			nodes: []string{"t:tip", "p:pin", "b:base", "y:you"},
			links: []Link{{Above: 2}, {Above: 1}, {Above: 2}},
		},
		{
			name: "the base is also the checkout",
			st: Status{Pinned: c("p"), CheckedOut: c("b"), Tip: c("t"), Pin: PinInfo{Ahead: 1, Behind: 2},
				Bases: Bases{Tip: c("b")}, Drift: Distance{Behind: 1}},
			nodes: []string{"t:tip", "p:pin", "b:you,base"},
			links: []Link{{Above: 2}, {Above: 1}},
		},
		{
			name: "two diverged commits are not drawn as a fork",
			st: Status{Pinned: c("p"), CheckedOut: c("y"), Tip: c("t"), Pin: PinInfo{Ahead: 1, Behind: 2},
				Drift: Distance{Ahead: 1, Behind: 1}, Bases: Bases{Tip: c("b"), Checkout: c("b")}},
			nodes: []string{"t:tip", "p:pin", "y:you"},
			links: []Link{{Above: 2, Below: 1}, {Above: 1, Below: 1}},
		},
		{
			name: "pin on a branch ahead of its target: the target is below the pin",
			st: Status{Pinned: c("p"), CheckedOut: c("p"), Tip: c("t"), Pin: PinInfo{Behind: 1},
				TargetTip: c("m"), TargetDrift: Distance{Behind: 3}},
			nodes: []string{"t:tip", "p:pin,you", "m:target"},
			links: []Link{{Above: 1}, {Above: 3}},
		},
		{
			name: "pin on a branch diverged from its target: a fork",
			st: Status{Pinned: c("p"), CheckedOut: c("p"), Tip: c("t"), Pin: PinInfo{Behind: 1},
				TargetTip: c("m"), TargetDrift: Distance{Ahead: 12, Behind: 52}, Bases: Bases{Target: c("b")}},
			nodes: []string{"m:target", "t:tip", "p:pin,you", "b:base"},
			links: []Link{{Above: 12}, {Above: 1}, {Above: 52}},
		},
		{
			name:  "no tip, checkout not loaded",
			st:    Status{Pinned: c("p")},
			nodes: []string{"p:pin"},
		},
		{
			name:  "superproject",
			st:    Status{Root: true, Pinned: c("h"), CheckedOut: c("h"), Tip: c("t"), Pin: PinInfo{Behind: 2}},
			nodes: []string{"t:tip", "h:head"},
			links: []Link{{Above: 2}},
		},
	}
	words := map[Role]string{RolePin: "pin", RoleStaged: "staged", RoleHead: "head", RoleTip: "tip", RoleTarget: "target", RoleYou: "you", RoleBase: "base"}
	for _, tt := range tests {
		var nodes []string
		var links []Link
		spine := tt.st.Spine()
		for i, n := range spine {
			s := n.Commit.SHA + ":"
			for j, r := range n.Roles {
				if j > 0 {
					s += ","
				}
				s += words[r]
			}
			nodes = append(nodes, s)
			if i < len(spine)-1 {
				links = append(links, n.Below)
			}
		}
		if !slices.Equal(nodes, tt.nodes) {
			t.Errorf("%s: nodes %v, want %v", tt.name, nodes, tt.nodes)
		}
		if !slices.Equal(links, tt.links) {
			t.Errorf("%s: links %v, want %v", tt.name, links, tt.links)
		}
	}
}

func TestSpineNodeInSync(t *testing.T) {
	tests := []struct {
		roles []Role
		want  bool
	}{
		{[]Role{RolePin, RoleTip, RoleYou}, true},
		{[]Role{RoleHead, RoleTip}, true},
		{[]Role{RolePin, RoleTip}, false},
		{[]Role{RolePin, RoleYou}, false},
		{[]Role{RoleTip}, false},
	}
	for _, tt := range tests {
		if got := (SpineNode{Roles: tt.roles}).InSync(); got != tt.want {
			t.Errorf("%v: InSync = %v, want %v", tt.roles, got, tt.want)
		}
	}
}
