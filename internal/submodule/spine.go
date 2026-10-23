package submodule

import "slices"

// Role is what a commit means for a submodule.
type Role int

// The order is the order in which roles are listed for a commit.
const (
	RolePin    Role = iota // the commit recorded in the superproject's HEAD
	RoleHead               // the superproject's HEAD, which takes the pin's place
	RoleStaged             // the pin in the index, if it differs from RolePin
	RoleTip                // the tip of the ref the pin was resolved to
	RoleYou                // the commit checked out in the submodule
)

// SpineNode is one commit on the spine and the roles that sit on it.
type SpineNode struct {
	Roles  []Role
	Commit *Commit
	// Below describes the history between this node and the next one down;
	// it is the zero Link for the last node.
	Below Link
}

// InSync reports whether the pin, the tip and the checkout are one commit.
func (n SpineNode) InSync() bool {
	return slices.Contains(n.Roles, RoleTip) &&
		(slices.Contains(n.Roles, RoleHead) || slices.Contains(n.Roles, RolePin) && slices.Contains(n.Roles, RoleYou))
}

// Link is the history between two spine nodes. Above counts the commits only
// the upper node has, Below those only the lower node has.
type Link struct{ Above, Below int }

// Linear reports whether the lower node is an ancestor of the upper one, so
// that Above is the number of commits between them.
func (l Link) Linear() bool { return l.Above > 0 && l.Below == 0 }

// Diverged reports whether the nodes are on different branches.
func (l Link) Diverged() bool { return l.Above > 0 && l.Below > 0 }

// spineItem is a commit with its roles on its way to becoming a node.
type spineItem struct {
	roles  []Role
	commit *Commit
	height int  // commits above (+) or below (−) the pin; unused if div
	div    bool // diverged from the pin
	own    int  // commits only this item has
	pin    int  // commits only the pin has
	isPin  bool
}

// Spine arranges the pin (and the staged pin, if any), the tip and the checkout as one line of history,
// newest on top. Roles on the same commit share a node. A tip or checkout
// that has diverged from the pin is placed outermost, above or below
// respectively, with a diverged Link. Missing commits are left out.
func (s *Status) Spine() []SpineNode {
	if s.Pinned == nil {
		return nil
	}
	pinRole := RolePin
	switch {
	case s.Root:
		pinRole = RoleHead
	case s.PinStaged:
		pinRole = RoleStaged // the committed pin gets its own node below
	}
	pin := spineItem{roles: []Role{pinRole}, commit: s.Pinned, isPin: true}

	var above, below []spineItem
	place := func(role Role, c *Commit, own, pinOnly int, divAbove bool) {
		it := spineItem{roles: []Role{role}, commit: c, own: own, pin: pinOnly}
		switch {
		case own == 0 && pinOnly == 0:
			pin.roles = append(pin.roles, role)
			return
		case own > 0 && pinOnly == 0:
			it.height = own
			above = append(above, it)
		case own == 0:
			it.height = -pinOnly
			below = append(below, it)
		default:
			it.div = true
			if divAbove {
				above = append(above, it)
			} else {
				below = append(below, it)
			}
		}
	}
	if s.Tip != nil {
		place(RoleTip, s.Tip, s.Pin.Behind, s.Pin.Ahead, true)
	}
	if s.PinStaged && s.Committed != nil {
		place(RolePin, s.Committed, s.CommittedDrift.Ahead, s.CommittedDrift.Behind, false)
	}
	switch {
	case s.CheckedOut == nil, s.Root: // HEAD is the superproject's pin and checkout
	case s.CheckedOut.SHA == s.Pinned.SHA:
		pin.roles = append(pin.roles, RoleYou)
	default:
		place(RoleYou, s.CheckedOut, s.Drift.Ahead, s.Drift.Behind, false)
	}

	// diverged items are outermost, the rest is ordered by height
	byHeight := func(a, b spineItem) int { return b.height - a.height }
	slices.SortStableFunc(above, func(a, b spineItem) int {
		if a.div != b.div {
			if a.div {
				return -1
			}
			return 1
		}
		return byHeight(a, b)
	})
	slices.SortStableFunc(below, func(a, b spineItem) int {
		if a.div != b.div {
			if a.div {
				return 1
			}
			return -1
		}
		return byHeight(a, b)
	})

	slices.Sort(pin.roles)
	items := append(append(above, pin), below...)
	var nodes []SpineNode
	var last spineItem
	for i, it := range items {
		// the same commit twice, e.g. a tip and a checkout side by side
		if i > 0 && !it.div && !last.div && it.height == last.height && it.commit.SHA == last.commit.SHA {
			n := &nodes[len(nodes)-1]
			n.Roles = append(n.Roles, it.roles...)
			slices.Sort(n.Roles)
			continue
		}
		if i > 0 {
			nodes[len(nodes)-1].Below = link(last, it)
		}
		nodes = append(nodes, SpineNode{Roles: it.roles, Commit: it.commit})
		last = it
	}
	return nodes
}

// link describes the history between an upper and a lower item.
func link(upper, lower spineItem) Link {
	switch {
	case upper.div && lower.isPin:
		return Link{Above: upper.own, Below: upper.pin}
	case lower.div && upper.isPin:
		return Link{Above: lower.pin, Below: lower.own}
	case upper.div || lower.div:
		return Link{} // unknown
	default:
		return Link{Above: upper.height - lower.height}
	}
}
