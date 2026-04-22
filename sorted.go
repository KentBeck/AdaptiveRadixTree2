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
func minLeafOf[V any](n node[V]) *leaf[V] {
	for n != nil {
		switch r := n.(type) {
		case *leaf[V]:
			return r
		case *node4[V]:
			if r.terminal != nil {
				return r.terminal
			}
			n = r.children[0]
		case *node16[V]:
			if r.terminal != nil {
				return r.terminal
			}
			n = r.children[0]
		case *node48[V]:
			if r.terminal != nil {
				return r.terminal
			}
			n = firstChildOfNode48[V](r)
		case *node256[V]:
			if r.terminal != nil {
				return r.terminal
			}
			n = firstChildOfNode256[V](r)
		default:
			return nil
		}
	}
	return nil
}

// maxLeafOf returns the leaf holding the largest key reachable from n,
// or nil if the subtree is empty. The largest key is reached by
// following the highest-byte child at every inner node; a terminal,
// when present, is always smaller than any child-extension and is used
// only when the node has no children.
func maxLeafOf[V any](n node[V]) *leaf[V] {
	for n != nil {
		switch r := n.(type) {
		case *leaf[V]:
			return r
		case *node4[V]:
			if r.numChildren > 0 {
				n = r.children[r.numChildren-1]
			} else {
				return r.terminal
			}
		case *node16[V]:
			if r.numChildren > 0 {
				n = r.children[r.numChildren-1]
			} else {
				return r.terminal
			}
		case *node48[V]:
			if c := lastChildOfNode48[V](r); c != nil {
				n = c
			} else {
				return r.terminal
			}
		case *node256[V]:
			if c := lastChildOfNode256[V](r); c != nil {
				n = c
			} else {
				return r.terminal
			}
		default:
			return nil
		}
	}
	return nil
}

// firstChildOfNode48 returns the child under the smallest occupied
// edge byte of n, or nil if n has no children.
func firstChildOfNode48[V any](n *node48[V]) node[V] {
	for edge := 0; edge < 256; edge++ {
		if slot := n.childIndex[byte(edge)]; slot != 0 {
			return n.children[slot-1]
		}
	}
	return nil
}

// lastChildOfNode48 returns the child under the largest occupied edge
// byte of n, or nil if n has no children.
func lastChildOfNode48[V any](n *node48[V]) node[V] {
	for edge := 255; edge >= 0; edge-- {
		if slot := n.childIndex[byte(edge)]; slot != 0 {
			return n.children[slot-1]
		}
	}
	return nil
}

// firstChildOfNode256 returns the child under the smallest occupied
// edge byte of n, or nil if n has no children.
func firstChildOfNode256[V any](n *node256[V]) node[V] {
	for edge := 0; edge < 256; edge++ {
		if c := n.children[edge]; c != nil {
			return c
		}
	}
	return nil
}

// lastChildOfNode256 returns the child under the largest occupied
// edge byte of n, or nil if n has no children.
func lastChildOfNode256[V any](n *node256[V]) node[V] {
	for edge := 255; edge >= 0; edge-- {
		if c := n.children[edge]; c != nil {
			return c
		}
	}
	return nil
}

// innerPrefixTerminal returns the prefix and terminal of an inner
// node. The second return is the terminal leaf (or nil).
func innerPrefixTerminal[V any](n node[V]) ([]byte, *leaf[V]) {
	switch r := n.(type) {
	case *node4[V]:
		return r.prefix, r.terminal
	case *node16[V]:
		return r.prefix, r.terminal
	case *node48[V]:
		return r.prefix, r.terminal
	case *node256[V]:
		return r.prefix, r.terminal
	}
	return nil, nil
}

// findInnerChild returns n's child under edge byte b, or nil.
func findInnerChild[V any](n node[V], b byte) node[V] {
	switch r := n.(type) {
	case *node4[V]:
		return r.findChild(b)
	case *node16[V]:
		return r.findChild(b)
	case *node48[V]:
		return r.findChild(b)
	case *node256[V]:
		return r.findChild(b)
	}
	return nil
}

// firstChildGT returns n's smallest child whose edge byte is strictly
// greater than b, or nil if none exists.
func firstChildGT[V any](n node[V], b byte) node[V] {
	switch r := n.(type) {
	case *node4[V]:
		for i := uint8(0); i < r.numChildren; i++ {
			if r.keys[i] > b {
				return r.children[i]
			}
		}
	case *node16[V]:
		for i := uint8(0); i < r.numChildren; i++ {
			if r.keys[i] > b {
				return r.children[i]
			}
		}
	case *node48[V]:
		for edge := int(b) + 1; edge < 256; edge++ {
			if slot := r.childIndex[byte(edge)]; slot != 0 {
				return r.children[slot-1]
			}
		}
	case *node256[V]:
		for edge := int(b) + 1; edge < 256; edge++ {
			if c := r.children[edge]; c != nil {
				return c
			}
		}
	}
	return nil
}

// lastChildLT returns n's largest child whose edge byte is strictly
// less than b, or nil if none exists.
func lastChildLT[V any](n node[V], b byte) node[V] {
	switch r := n.(type) {
	case *node4[V]:
		for i := int(r.numChildren) - 1; i >= 0; i-- {
			if r.keys[i] < b {
				return r.children[i]
			}
		}
	case *node16[V]:
		for i := int(r.numChildren) - 1; i >= 0; i-- {
			if r.keys[i] < b {
				return r.children[i]
			}
		}
	case *node48[V]:
		for edge := int(b) - 1; edge >= 0; edge-- {
			if slot := r.childIndex[byte(edge)]; slot != 0 {
				return r.children[slot-1]
			}
		}
	case *node256[V]:
		for edge := int(b) - 1; edge >= 0; edge-- {
			if c := r.children[edge]; c != nil {
				return c
			}
		}
	}
	return nil
}

// ceilingLeafOf returns the leaf holding the smallest key >= target
// reachable from n, or nil if no such leaf exists. depth is the
// number of target bytes already consumed by the enclosing traversal.
// When target's bytes diverge from n's compressed prefix, the
// divergent byte alone decides whether the whole subtree sorts above
// target (Min of subtree is the answer) or below (no ceiling here).
func ceilingLeafOf[V any](n node[V], target []byte, depth int) *leaf[V] {
	switch r := n.(type) {
	case nil:
		return nil
	case *leaf[V]:
		if bytes.Compare(r.key, target) >= 0 {
			return r
		}
		return nil
	}
	prefix, terminal := innerPrefixTerminal[V](n)
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
		if terminal != nil {
			return terminal
		}
		return minLeafOf[V](firstChildOfInner[V](n))
	}
	b := target[newDepth]
	if child := findInnerChild[V](n, b); child != nil {
		if result := ceilingLeafOf[V](child, target, newDepth+1); result != nil {
			return result
		}
	}
	if sib := firstChildGT[V](n, b); sib != nil {
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
func floorLeafOf[V any](n node[V], target []byte, depth int) *leaf[V] {
	switch r := n.(type) {
	case nil:
		return nil
	case *leaf[V]:
		if bytes.Compare(r.key, target) <= 0 {
			return r
		}
		return nil
	}
	prefix, terminal := innerPrefixTerminal[V](n)
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
		return terminal
	}
	b := target[newDepth]
	if child := findInnerChild[V](n, b); child != nil {
		if result := floorLeafOf[V](child, target, newDepth+1); result != nil {
			return result
		}
	}
	if sib := lastChildLT[V](n, b); sib != nil {
		return maxLeafOf[V](sib)
	}
	return terminal
}

// firstChildOfInner returns n's smallest-edge child, or nil.
func firstChildOfInner[V any](n node[V]) node[V] {
	switch r := n.(type) {
	case *node4[V]:
		if r.numChildren > 0 {
			return r.children[0]
		}
	case *node16[V]:
		if r.numChildren > 0 {
			return r.children[0]
		}
	case *node48[V]:
		return firstChildOfNode48[V](r)
	case *node256[V]:
		return firstChildOfNode256[V](r)
	}
	return nil
}

// cloneNode returns a structural copy of n. Inner-node instances are
// newly allocated (so child-slot writes on either copy do not affect
// the other); leaves are freshly allocated with shared value semantics
// and their key bytes copied through newLeaf.
func cloneNode[V any](n node[V]) node[V] {
	switch r := n.(type) {
	case nil:
		return nil
	case *leaf[V]:
		return newLeaf(r.key, r.value)
	case *node4[V]:
		cp := &node4[V]{prefix: r.prefix, keys: r.keys, numChildren: r.numChildren}
		if r.terminal != nil {
			cp.terminal = newLeaf(r.terminal.key, r.terminal.value)
		}
		for i := uint8(0); i < r.numChildren; i++ {
			cp.children[i] = cloneNode[V](r.children[i])
		}
		return cp
	case *node16[V]:
		cp := &node16[V]{prefix: r.prefix, keys: r.keys, numChildren: r.numChildren}
		if r.terminal != nil {
			cp.terminal = newLeaf(r.terminal.key, r.terminal.value)
		}
		for i := uint8(0); i < r.numChildren; i++ {
			cp.children[i] = cloneNode[V](r.children[i])
		}
		return cp
	case *node48[V]:
		cp := &node48[V]{
			prefix:      r.prefix,
			childIndex:  r.childIndex,
			childEdge:   r.childEdge,
			numChildren: r.numChildren,
		}
		if r.terminal != nil {
			cp.terminal = newLeaf(r.terminal.key, r.terminal.value)
		}
		for i := uint8(0); i < r.numChildren; i++ {
			cp.children[i] = cloneNode[V](r.children[i])
		}
		return cp
	case *node256[V]:
		cp := &node256[V]{prefix: r.prefix, numChildren: r.numChildren}
		if r.terminal != nil {
			cp.terminal = newLeaf(r.terminal.key, r.terminal.value)
		}
		for edge := 0; edge < 256; edge++ {
			if r.children[edge] != nil {
				cp.children[edge] = cloneNode[V](r.children[edge])
			}
		}
		return cp
	}
	return nil
}
