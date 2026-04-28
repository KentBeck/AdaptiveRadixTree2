package art

import "bytes"

// Min returns the smallest key in the tree, its value, and ok=true. If
// the tree is empty, ok is false and the returned key and value are
// their zero values.
func (t *Tree[V]) Min() (key []byte, value V, ok bool) {
	if l := minLeafOf[V](t.root); l != nil {
		return l.key, l.value, true
	}
	var zero V
	return nil, zero, false
}

// Max returns the largest key in the tree, its value, and ok=true. If
// the tree is empty, ok is false and the returned key and value are
// their zero values.
func (t *Tree[V]) Max() (key []byte, value V, ok bool) {
	if l := maxLeafOf[V](t.root); l != nil {
		return l.key, l.value, true
	}
	var zero V
	return nil, zero, false
}

// Ceiling returns the smallest key that compares byte-wise >= target,
// along with its value. If no such key exists, ok is false. A nil
// target and an empty-slice target are equivalent (both represent the
// empty key), so Ceiling of either returns the tree's Min when the
// tree is non-empty.
func (t *Tree[V]) Ceiling(target []byte) (key []byte, value V, ok bool) {
	if l := ceilingLeafOf[V](t.root, target, 0); l != nil {
		return l.key, l.value, true
	}
	var zero V
	return nil, zero, false
}

// Floor returns the largest key that compares byte-wise <= target,
// along with its value. If no such key exists, ok is false. A nil
// target and an empty-slice target are equivalent (both represent the
// empty key); Floor of either returns the empty key's entry when
// present, and otherwise ok is false.
func (t *Tree[V]) Floor(target []byte) (key []byte, value V, ok bool) {
	if l := floorLeafOf[V](t.root, target, 0); l != nil {
		return l.key, l.value, true
	}
	var zero V
	return nil, zero, false
}

// Clone returns a structural copy of t. Writes to t or to the returned
// tree do not affect each other. Key bytes may be shared between the
// two trees, matching the read-only-key contract of [Tree.All] and
// [Tree.Range].
func (t *Tree[V]) Clone() *Tree[V] {
	return &Tree[V]{root: cloneNode[V](t.root), size: t.size}
}

// Clear removes every entry from the tree in O(1) by dropping the root
// reference. After Clear, [Tree.Len] returns 0 and subsequent Put
// calls behave as on a newly constructed tree.
func (t *Tree[V]) Clear() {
	t.root = nil
	t.size = 0
}

// minLeafOf returns the leaf holding the smallest key reachable from
// n, or nil if the subtree is empty. A node's terminal (when set) is
// the smallest key in its subtree: the shorter key sorts before any
// longer key that extends the same prefix.
func minLeafOf[V any](n node) *leaf[V] {
	for n != nil {
		if l, ok := n.(*leaf[V]); ok {
			return l
		}
		r := n.(innerNode)
		if tl, ok := r.getTerminal().(*leaf[V]); ok {
			return tl
		}
		n = firstChildOf(r)
	}
	return nil
}

// maxLeafOf returns the leaf holding the largest key reachable from n,
// or nil if the subtree is empty. The largest key is reached by
// following the highest-byte child at every inner node; a terminal,
// when present, is always smaller than any child-extension and is used
// only when the node has no children.
func maxLeafOf[V any](n node) *leaf[V] {
	for n != nil {
		if l, ok := n.(*leaf[V]); ok {
			return l
		}
		r := n.(innerNode)
		if c := lastChildOf(r); c != nil {
			n = c
			continue
		}
		tl, _ := r.getTerminal().(*leaf[V])
		return tl
	}
	return nil
}

// firstChildOf returns n's smallest-edge child, or nil if n has no
// children. Implemented as an early-exit eachAscending walk so node48
// and node256 share the bounded scan they would do anyway.
func firstChildOf(n innerNode) node {
	var first node
	n.eachAscending(func(_ byte, c node) bool {
		first = c
		return false
	})
	return first
}

// lastChildOf returns n's largest-edge child, or nil if n has no
// children.
func lastChildOf(n innerNode) node {
	var last node
	n.eachDescending(func(_ byte, c node) bool {
		last = c
		return false
	})
	return last
}

// firstChildGT returns n's smallest child whose edge byte is strictly
// greater than b, or nil if none exists.
func firstChildGT(n innerNode, b byte) node {
	var found node
	n.eachAscending(func(edge byte, c node) bool {
		if edge > b {
			found = c
			return false
		}
		return true
	})
	return found
}

// lastChildLT returns n's largest child whose edge byte is strictly
// less than b, or nil if none exists.
func lastChildLT(n innerNode, b byte) node {
	var found node
	n.eachDescending(func(edge byte, c node) bool {
		if edge < b {
			found = c
			return false
		}
		return true
	})
	return found
}

// ceilingLeafOf returns the leaf holding the smallest key >= target
// reachable from n, or nil if no such leaf exists. depth is the
// number of target bytes already consumed by the enclosing traversal.
// When target's bytes diverge from n's compressed prefix, the
// divergent byte alone decides whether the whole subtree sorts above
// target (Min of subtree is the answer) or below (no ceiling here).
func ceilingLeafOf[V any](n node, target []byte, depth int) *leaf[V] {
	if n == nil {
		return nil
	}
	if l, ok := n.(*leaf[V]); ok {
		if bytes.Compare(l.key, target) >= 0 {
			return l
		}
		return nil
	}
	r := n.(innerNode)
	prefix := r.getPrefix()
	tl, _ := r.getTerminal().(*leaf[V])
	remaining := len(target) - depth
	m := len(prefix)
	if m > remaining {
		m = remaining
	}
	for i := 0; i < m; i++ {
		if prefix[i] < target[depth+i] {
			return nil
		}
		if prefix[i] > target[depth+i] {
			return minLeafOf[V](n)
		}
	}
	if remaining < len(prefix) {
		return minLeafOf[V](n)
	}
	newDepth := depth + len(prefix)
	if newDepth == len(target) {
		if tl != nil {
			return tl
		}
		return minLeafOf[V](firstChildOf(r))
	}
	b := target[newDepth]
	if child := r.findChild(b); child != nil {
		if result := ceilingLeafOf[V](child, target, newDepth+1); result != nil {
			return result
		}
	}
	if sib := firstChildGT(r, b); sib != nil {
		return minLeafOf[V](sib)
	}
	return nil
}

// floorLeafOf returns the leaf holding the largest key <= target
// reachable from n, or nil if no such leaf exists. depth is the
// number of target bytes already consumed by the enclosing traversal.
// When target's bytes diverge from n's compressed prefix, the
// divergent byte alone decides whether the whole subtree sorts above
// target (no floor here) or below (Max of subtree is the answer).
func floorLeafOf[V any](n node, target []byte, depth int) *leaf[V] {
	if n == nil {
		return nil
	}
	if l, ok := n.(*leaf[V]); ok {
		if bytes.Compare(l.key, target) <= 0 {
			return l
		}
		return nil
	}
	r := n.(innerNode)
	prefix := r.getPrefix()
	tl, _ := r.getTerminal().(*leaf[V])
	remaining := len(target) - depth
	m := len(prefix)
	if m > remaining {
		m = remaining
	}
	for i := 0; i < m; i++ {
		if prefix[i] > target[depth+i] {
			return nil
		}
		if prefix[i] < target[depth+i] {
			return maxLeafOf[V](n)
		}
	}
	if remaining < len(prefix) {
		return nil
	}
	newDepth := depth + len(prefix)
	if newDepth == len(target) {
		return tl
	}
	b := target[newDepth]
	if child := r.findChild(b); child != nil {
		if result := floorLeafOf[V](child, target, newDepth+1); result != nil {
			return result
		}
	}
	if sib := lastChildLT(r, b); sib != nil {
		return maxLeafOf[V](sib)
	}
	return tl
}

// cloneNode returns a structural copy of n. Inner-node instances are
// freshly allocated via [innerNode.shallow] (so child-slot writes on
// either copy do not affect the other) and then their child slots are
// recursively replaced with deep copies; leaves are freshly allocated
// with shared value semantics and their key bytes copied through
// newLeaf.
func cloneNode[V any](n node) node {
	if n == nil {
		return nil
	}
	if l, ok := n.(*leaf[V]); ok {
		return newLeaf(l.key, l.value)
	}
	src := n.(innerNode)
	cp := src.shallow()
	if tl, ok := cp.getTerminal().(*leaf[V]); ok {
		cp.setTerminal(newLeaf(tl.key, tl.value))
	}
	cp.eachAscending(func(b byte, child node) bool {
		cp.replaceChild(b, cloneNode[V](child))
		return true
	})
	return cp
}
