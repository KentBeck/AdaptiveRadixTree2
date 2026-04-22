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
		switch r := n.(type) {
		case *leaf[V]:
			return r
		case *node4:
			if tl, ok := r.terminal.(*leaf[V]); ok {
				return tl
			}
			n = r.children[0]
		case *node16:
			if tl, ok := r.terminal.(*leaf[V]); ok {
				return tl
			}
			n = r.children[0]
		case *node48:
			if tl, ok := r.terminal.(*leaf[V]); ok {
				return tl
			}
			n = firstChildOfNode48(r)
		case *node256:
			if tl, ok := r.terminal.(*leaf[V]); ok {
				return tl
			}
			n = firstChildOfNode256(r)
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
func maxLeafOf[V any](n node) *leaf[V] {
	for n != nil {
		switch r := n.(type) {
		case *leaf[V]:
			return r
		case *node4:
			if r.numChildren > 0 {
				n = r.children[r.numChildren-1]
			} else {
				tl, _ := r.terminal.(*leaf[V])
				return tl
			}
		case *node16:
			if r.numChildren > 0 {
				n = r.children[r.numChildren-1]
			} else {
				tl, _ := r.terminal.(*leaf[V])
				return tl
			}
		case *node48:
			if c := lastChildOfNode48(r); c != nil {
				n = c
			} else {
				tl, _ := r.terminal.(*leaf[V])
				return tl
			}
		case *node256:
			if c := lastChildOfNode256(r); c != nil {
				n = c
			} else {
				tl, _ := r.terminal.(*leaf[V])
				return tl
			}
		default:
			return nil
		}
	}
	return nil
}

// firstChildOfNode48 returns the child under the smallest occupied
// edge byte of n, or nil if n has no children.
func firstChildOfNode48(n *node48) node {
	for edge := 0; edge < 256; edge++ {
		if slot := n.childIndex[byte(edge)]; slot != 0 {
			return n.children[slot-1]
		}
	}
	return nil
}

// lastChildOfNode48 returns the child under the largest occupied edge
// byte of n, or nil if n has no children.
func lastChildOfNode48(n *node48) node {
	for edge := 255; edge >= 0; edge-- {
		if slot := n.childIndex[byte(edge)]; slot != 0 {
			return n.children[slot-1]
		}
	}
	return nil
}

// firstChildOfNode256 returns the child under the smallest occupied
// edge byte of n, or nil if n has no children.
func firstChildOfNode256(n *node256) node {
	for edge := 0; edge < 256; edge++ {
		if c := n.children[edge]; c != nil {
			return c
		}
	}
	return nil
}

// lastChildOfNode256 returns the child under the largest occupied
// edge byte of n, or nil if n has no children.
func lastChildOfNode256(n *node256) node {
	for edge := 255; edge >= 0; edge-- {
		if c := n.children[edge]; c != nil {
			return c
		}
	}
	return nil
}

// innerPrefixTerminal returns the prefix and terminal of an inner
// node. The second return is the terminal leaf (or nil) after
// asserting the node-interface terminal to *leaf[V].
func innerPrefixTerminal[V any](n node) ([]byte, *leaf[V]) {
	var tl *leaf[V]
	switch r := n.(type) {
	case *node4:
		tl, _ = r.terminal.(*leaf[V])
		return r.prefix, tl
	case *node16:
		tl, _ = r.terminal.(*leaf[V])
		return r.prefix, tl
	case *node48:
		tl, _ = r.terminal.(*leaf[V])
		return r.prefix, tl
	case *node256:
		tl, _ = r.terminal.(*leaf[V])
		return r.prefix, tl
	}
	return nil, nil
}

// findInnerChild returns n's child under edge byte b, or nil.
func findInnerChild(n node, b byte) node {
	switch r := n.(type) {
	case *node4:
		return r.findChild(b)
	case *node16:
		return r.findChild(b)
	case *node48:
		return r.findChild(b)
	case *node256:
		return r.findChild(b)
	}
	return nil
}

// firstChildGT returns n's smallest child whose edge byte is strictly
// greater than b, or nil if none exists.
func firstChildGT(n node, b byte) node {
	switch r := n.(type) {
	case *node4:
		for i := uint8(0); i < r.numChildren; i++ {
			if r.keys[i] > b {
				return r.children[i]
			}
		}
	case *node16:
		for i := uint8(0); i < r.numChildren; i++ {
			if r.keys[i] > b {
				return r.children[i]
			}
		}
	case *node48:
		for edge := int(b) + 1; edge < 256; edge++ {
			if slot := r.childIndex[byte(edge)]; slot != 0 {
				return r.children[slot-1]
			}
		}
	case *node256:
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
func lastChildLT(n node, b byte) node {
	switch r := n.(type) {
	case *node4:
		for i := int(r.numChildren) - 1; i >= 0; i-- {
			if r.keys[i] < b {
				return r.children[i]
			}
		}
	case *node16:
		for i := int(r.numChildren) - 1; i >= 0; i-- {
			if r.keys[i] < b {
				return r.children[i]
			}
		}
	case *node48:
		for edge := int(b) - 1; edge >= 0; edge-- {
			if slot := r.childIndex[byte(edge)]; slot != 0 {
				return r.children[slot-1]
			}
		}
	case *node256:
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
func ceilingLeafOf[V any](n node, target []byte, depth int) *leaf[V] {
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
		return minLeafOf[V](firstChildOfInner(n))
	}
	b := target[newDepth]
	if child := findInnerChild(n, b); child != nil {
		if result := ceilingLeafOf[V](child, target, newDepth+1); result != nil {
			return result
		}
	}
	if sib := firstChildGT(n, b); sib != nil {
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
	if child := findInnerChild(n, b); child != nil {
		if result := floorLeafOf[V](child, target, newDepth+1); result != nil {
			return result
		}
	}
	if sib := lastChildLT(n, b); sib != nil {
		return maxLeafOf[V](sib)
	}
	return terminal
}

// firstChildOfInner returns n's smallest-edge child, or nil.
func firstChildOfInner(n node) node {
	switch r := n.(type) {
	case *node4:
		if r.numChildren > 0 {
			return r.children[0]
		}
	case *node16:
		if r.numChildren > 0 {
			return r.children[0]
		}
	case *node48:
		return firstChildOfNode48(r)
	case *node256:
		return firstChildOfNode256(r)
	}
	return nil
}

// cloneNode returns a structural copy of n. Inner-node instances are
// newly allocated (so child-slot writes on either copy do not affect
// the other); leaves are freshly allocated with shared value semantics
// and their key bytes copied through newLeaf.
func cloneNode[V any](n node) node {
	switch r := n.(type) {
	case nil:
		return nil
	case *leaf[V]:
		return newLeaf(r.key, r.value)
	case *node4:
		cp := &node4{prefix: r.prefix, keys: r.keys, numChildren: r.numChildren}
		if tl, ok := r.terminal.(*leaf[V]); ok {
			cp.terminal = newLeaf(tl.key, tl.value)
		}
		for i := uint8(0); i < r.numChildren; i++ {
			cp.children[i] = cloneNode[V](r.children[i])
		}
		return cp
	case *node16:
		cp := &node16{prefix: r.prefix, keys: r.keys, numChildren: r.numChildren}
		if tl, ok := r.terminal.(*leaf[V]); ok {
			cp.terminal = newLeaf(tl.key, tl.value)
		}
		for i := uint8(0); i < r.numChildren; i++ {
			cp.children[i] = cloneNode[V](r.children[i])
		}
		return cp
	case *node48:
		cp := &node48{
			prefix:      r.prefix,
			childIndex:  r.childIndex,
			childEdge:   r.childEdge,
			numChildren: r.numChildren,
		}
		if tl, ok := r.terminal.(*leaf[V]); ok {
			cp.terminal = newLeaf(tl.key, tl.value)
		}
		for i := uint8(0); i < r.numChildren; i++ {
			cp.children[i] = cloneNode[V](r.children[i])
		}
		return cp
	case *node256:
		cp := &node256{prefix: r.prefix, numChildren: r.numChildren}
		if tl, ok := r.terminal.(*leaf[V]); ok {
			cp.terminal = newLeaf(tl.key, tl.value)
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
