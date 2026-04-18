// Package art is an Adaptive Radix Tree implementation.
//
// Keys are currently assumed to differ at byte 0. Path compression
// and lazy expansion will arrive in later slices.
package art

import "bytes"

type nodeKind uint8

const (
	kindLeaf nodeKind = iota
	kindNode4
	kindNode16
	kindNode48
	kindNode256
)

const (
	node4Capacity   = 4
	node16Capacity  = 16
	node48Capacity  = 48
	node256Capacity = 256
)

type node interface {
	kind() nodeKind
}

// innerNode is the interface satisfied by every non-leaf node. It
// exposes the subset of operations used by Tree.Delete so the caller
// can act uniformly across node4/16/48/256.
type innerNode interface {
	node
	findChild(b byte) node
	removeChild(b byte)
	isEmpty() bool
}

type leaf struct {
	key   []byte
	value any
}

func (*leaf) kind() nodeKind { return kindLeaf }

func newLeaf(key []byte, value any) *leaf {
	// Copy the key so callers may safely reuse their slice.
	return &leaf{
		key:   append([]byte(nil), key...),
		value: value,
	}
}

// node4 keeps keys[:numChildren] sorted ascending by edge byte. The
// prefix is consumed from the search key before branching. terminal,
// when non-nil, holds the value stored at this node's exact path (a
// key that ends after the prefix and does not branch further).
type node4 struct {
	prefix      []byte
	keys        [4]byte
	children    [4]node
	terminal    *leaf
	numChildren uint8
}

func (*node4) kind() nodeKind { return kindNode4 }

func (n *node4) findChild(b byte) node {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}

func (n *node4) addChild(b byte, child node) {
	i := uint8(0)
	for i < n.numChildren && n.keys[i] < b {
		i++
	}
	copy(n.keys[i+1:n.numChildren+1], n.keys[i:n.numChildren])
	copy(n.children[i+1:n.numChildren+1], n.children[i:n.numChildren])
	n.keys[i] = b
	n.children[i] = child
	n.numChildren++
}

// replaceChild swaps the child stored under edge byte b. Caller
// guarantees b is already present.
func (n *node4) replaceChild(b byte, child node) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			n.children[i] = child
			return
		}
	}
}

// removeChild removes the child stored under edge byte b, preserving
// the sorted order of the remaining keys. A no-op if b is absent.
func (n *node4) removeChild(b byte) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			copy(n.keys[i:], n.keys[i+1:n.numChildren])
			copy(n.children[i:], n.children[i+1:n.numChildren])
			n.numChildren--
			n.keys[n.numChildren] = 0
			n.children[n.numChildren] = nil
			return
		}
	}
}

func (n *node4) isEmpty() bool { return n.numChildren == 0 }

// longestCommonPrefix returns the leading slice of a that also
// prefixes b.
func longestCommonPrefix(a, b []byte) []byte {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// newNode4With returns a node4 whose prefix is the longest common tail
// of existing.key and newKey starting at depth. If one key is
// exhausted at that point it becomes the new node's terminal value;
// the other is attached as a branching child. If neither is exhausted
// both are attached as branching children on their first divergent
// byte. Caller guarantees the two keys are not equal.
func newNode4With(existing *leaf, newKey []byte, newValue any, depth int) *node4 {
	shared := longestCommonPrefix(existing.key[depth:], newKey[depth:])
	diverge := depth + len(shared)
	existingExhausted := diverge == len(existing.key)
	newExhausted := diverge == len(newKey)
	if existingExhausted && newExhausted {
		panic("art: newNode4With called with equal keys - invariant violation")
	}
	n := &node4{prefix: append([]byte(nil), shared...)}
	switch {
	case existingExhausted:
		n.terminal = existing
		n.addChild(newKey[diverge], newLeaf(newKey, newValue))
	case newExhausted:
		n.terminal = newLeaf(newKey, newValue)
		n.addChild(existing.key[diverge], existing)
	default:
		n.addChild(existing.key[diverge], existing)
		n.addChild(newKey[diverge], newLeaf(newKey, newValue))
	}
	return n
}

// node16 keeps keys[:numChildren] sorted ascending by edge byte.
type node16 struct {
	keys        [node16Capacity]byte
	children    [node16Capacity]node
	numChildren uint8
}

func (*node16) kind() nodeKind { return kindNode16 }

func (n *node16) findChild(b byte) node {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}

// insertChild inserts child under edge byte b. Caller guarantees b is
// not already present and that the node is not yet full.
func (n *node16) insertChild(b byte, child node) {
	i := uint8(0)
	for i < n.numChildren && n.keys[i] < b {
		i++
	}
	copy(n.keys[i+1:n.numChildren+1], n.keys[i:n.numChildren])
	copy(n.children[i+1:n.numChildren+1], n.children[i:n.numChildren])
	n.keys[i] = b
	n.children[i] = child
	n.numChildren++
}

// removeChild removes the child stored under edge byte b, preserving
// the sorted order of the remaining keys. A no-op if b is absent.
func (n *node16) removeChild(b byte) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			copy(n.keys[i:], n.keys[i+1:n.numChildren])
			copy(n.children[i:], n.children[i+1:n.numChildren])
			n.numChildren--
			n.keys[n.numChildren] = 0
			n.children[n.numChildren] = nil
			return
		}
	}
}

func (n *node16) isEmpty() bool { return n.numChildren == 0 }

// growToNode16 returns a node16 holding the same sorted children as n.
func growToNode16(n *node4) *node16 {
	grown := &node16{numChildren: n.numChildren}
	copy(grown.keys[:n.numChildren], n.keys[:n.numChildren])
	copy(grown.children[:n.numChildren], n.children[:n.numChildren])
	return grown
}

// shrinkToNode4 returns a node4 holding the same sorted children as n.
// Caller guarantees n.numChildren <= node4Capacity.
func shrinkToNode4(n *node16) *node4 {
	shrunk := &node4{numChildren: n.numChildren}
	copy(shrunk.keys[:n.numChildren], n.keys[:n.numChildren])
	copy(shrunk.children[:n.numChildren], n.children[:n.numChildren])
	return shrunk
}

// node48 maps edge bytes to children via a 256-entry index where a
// stored value of 0 means "absent" and any other value is a 1-based
// slot into children.
type node48 struct {
	childIndex  [256]byte
	children    [node48Capacity]node
	numChildren uint8
}

func (*node48) kind() nodeKind { return kindNode48 }

func (n *node48) findChild(b byte) node {
	slot := n.childIndex[b]
	if slot == 0 {
		return nil
	}
	return n.children[slot-1]
}

func (n *node48) addChild(newEdge byte, child node) {
	n.children[n.numChildren] = child
	n.childIndex[newEdge] = n.numChildren + 1
	n.numChildren++
}

// removeChild removes the child stored under edge byte b. To keep
// children[:numChildren] dense (which addChild relies on), the last
// live child is swapped into the vacated slot and its index entry is
// updated. A no-op if b is absent.
func (n *node48) removeChild(b byte) {
	slot := n.childIndex[b]
	if slot == 0 {
		return
	}
	last := n.numChildren
	if slot != last {
		for edge := 0; edge < 256; edge++ {
			if n.childIndex[byte(edge)] == last {
				n.children[slot-1] = n.children[last-1]
				n.childIndex[byte(edge)] = slot
				break
			}
		}
	}
	n.children[last-1] = nil
	n.childIndex[b] = 0
	n.numChildren--
}

func (n *node48) isEmpty() bool { return n.numChildren == 0 }

// growToNode48 returns a node48 holding the same children as n, with
// childIndex populated from n's sorted edge bytes.
func growToNode48(n *node16) *node48 {
	grown := &node48{numChildren: n.numChildren}
	for i := uint8(0); i < n.numChildren; i++ {
		grown.children[i] = n.children[i]
		grown.childIndex[n.keys[i]] = i + 1
	}
	return grown
}

// shrinkToNode16 returns a node16 holding the same children as n, with
// keys populated in ascending edge-byte order so node16's sort
// invariant is preserved. Caller guarantees n.numChildren <=
// node16Capacity.
func shrinkToNode16(n *node48) *node16 {
	shrunk := &node16{numChildren: n.numChildren}
	i := uint8(0)
	for edge := 0; edge < 256; edge++ {
		slot := n.childIndex[byte(edge)]
		if slot == 0 {
			continue
		}
		shrunk.keys[i] = byte(edge)
		shrunk.children[i] = n.children[slot-1]
		i++
	}
	return shrunk
}

// node256 indexes children directly by edge byte; a nil slot means
// absent. numChildren tracks the count for fast emptiness checks.
type node256 struct {
	children    [node256Capacity]node
	numChildren uint16
}

func (*node256) kind() nodeKind { return kindNode256 }

func (n *node256) findChild(b byte) node {
	return n.children[b]
}

func (n *node256) addChild(b byte, child node) {
	n.children[b] = child
	n.numChildren++
}

// removeChild removes the child stored under edge byte b. A no-op if
// b is absent.
func (n *node256) removeChild(b byte) {
	if n.children[b] == nil {
		return
	}
	n.children[b] = nil
	n.numChildren--
}

func (n *node256) isEmpty() bool { return n.numChildren == 0 }

// growToNode256 returns a node256 holding the same children as n,
// indexed directly by edge byte.
func growToNode256(n *node48) *node256 {
	grown := &node256{numChildren: uint16(n.numChildren)}
	for b := 0; b < 256; b++ {
		slot := n.childIndex[b]
		if slot != 0 {
			grown.children[b] = n.children[slot-1]
		}
	}
	return grown
}

// shrinkToNode48 returns a node48 holding the same children as n, with
// childIndex populated from the occupied slots in n. Caller guarantees
// n.numChildren <= node48Capacity.
func shrinkToNode48(n *node256) *node48 {
	shrunk := &node48{numChildren: uint8(n.numChildren)}
	slot := uint8(0)
	for b := 0; b < 256; b++ {
		if n.children[b] == nil {
			continue
		}
		shrunk.children[slot] = n.children[b]
		shrunk.childIndex[b] = slot + 1
		slot++
	}
	return shrunk
}

// collapseToOnlyChild returns the single remaining child of n. Caller
// guarantees n.numChildren == 1.
func collapseToOnlyChild(n *node4) node {
	return n.children[0]
}

// Tree is a sorted map backed by an Adaptive Radix Tree.
type Tree struct {
	root node
}

// New returns an empty Tree.
func New() *Tree {
	return &Tree{}
}

// Put associates value with key, replacing any previous value. A
// node4's prefix is consumed from the key as traversal descends;
// keys that end at a node4's exact path are stored in that node's
// terminal slot.
func (t *Tree) Put(key []byte, value any) {
	t.root = putInto(t.root, key, value, 0)
}

func putInto(current node, key []byte, value any, depth int) node {
	if current == nil {
		return newLeaf(key, value)
	}
	switch r := current.(type) {
	case *leaf:
		if bytes.Equal(r.key, key) {
			r.value = value
			return r
		}
		return newNode4With(r, key, value, depth)
	case *node4:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			return splitNode4Prefix(r, key, value, depth, splitPoint)
		}
		return putIntoNode4(r, key, value, depth+len(r.prefix))
	case *node16:
		branch := key[depth]
		if existing, ok := r.findChild(branch).(*leaf); ok && bytes.Equal(existing.key, key) {
			existing.value = value
			return r
		}
		if r.numChildren < node16Capacity {
			r.insertChild(branch, newLeaf(key, value))
			return r
		}
		grown := growToNode48(r)
		grown.addChild(branch, newLeaf(key, value))
		return grown
	case *node48:
		branch := key[depth]
		if existing, ok := r.findChild(branch).(*leaf); ok && bytes.Equal(existing.key, key) {
			existing.value = value
			return r
		}
		if r.numChildren < node48Capacity {
			r.addChild(branch, newLeaf(key, value))
			return r
		}
		grown := growToNode256(r)
		grown.addChild(branch, newLeaf(key, value))
		return grown
	case *node256:
		branch := key[depth]
		if existing, ok := r.findChild(branch).(*leaf); ok && bytes.Equal(existing.key, key) {
			existing.value = value
			return r
		}
		r.addChild(branch, newLeaf(key, value))
		return r
	}
	return current
}

// splitNode4Prefix handles the case where key[depth:] shares only a
// proper prefix of r.prefix. A new parent node4 takes that shared
// prefix and adopts r (with its prefix shortened past the divergence
// byte) as one branching child. If key is exhausted exactly at the
// split point it becomes the parent's terminal value; otherwise the
// new leaf is attached as the second branching child.
func splitNode4Prefix(r *node4, key []byte, value any, depth, splitPoint int) *node4 {
	parent := &node4{prefix: append([]byte(nil), r.prefix[:splitPoint]...)}
	oldBranch := r.prefix[splitPoint]
	r.prefix = r.prefix[splitPoint+1:]
	parent.addChild(oldBranch, r)
	if depth+splitPoint == len(key) {
		parent.terminal = newLeaf(key, value)
	} else {
		parent.addChild(key[depth+splitPoint], newLeaf(key, value))
	}
	return parent
}

// putIntoNode4 writes (key, value) into r given that r.prefix has
// already been consumed from key (the caller passes the advanced
// depth). The decision reads as: key exhausted? → terminal. Else
// switch on the child at key[depth]: absent → add/grow; leaf same
// key → overwrite; leaf different key → nested node4; inner node →
// recurse.
func putIntoNode4(r *node4, key []byte, value any, depth int) node {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.value = value
		} else {
			r.terminal = newLeaf(key, value)
		}
		return r
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		return node4AddOrGrow(r, branch, newLeaf(key, value))
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return r
		}
		r.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return r
	default:
		r.replaceChild(branch, putInto(c, key, value, depth+1))
		return r
	}
}

// node4AddOrGrow adds child under edge byte b, growing to a node16
// when r is already full. Growth is not yet supported for node4s
// that carry a terminal value; that lands in Slice 11.
func node4AddOrGrow(r *node4, b byte, child node) node {
	if r.numChildren < node4Capacity {
		r.addChild(b, child)
		return r
	}
	if r.terminal != nil {
		panic("art: growing node4 with non-nil terminal - see Slice 11 (terminal on node16)")
	}
	grown := growToNode16(r)
	grown.insertChild(b, child)
	return grown
}

// Get returns the value previously stored under key, if any.
func (t *Tree) Get(key []byte) (value any, ok bool) {
	current := t.root
	depth := 0
	for current != nil {
		switch n := current.(type) {
		case *leaf:
			if bytes.Equal(n.key, key) {
				return n.value, true
			}
			return nil, false
		case *node4:
			end := depth + len(n.prefix)
			if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
				return nil, false
			}
			depth = end
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return nil, false
			}
			current = n.findChild(key[depth])
			depth++
		case *node16:
			current = n.findChild(key[depth])
			depth++
		case *node48:
			current = n.findChild(key[depth])
			depth++
		case *node256:
			current = n.findChild(key[depth])
			depth++
		}
	}
	return nil, false
}

// Delete removes key from the tree, returning whether it was present.
// This slice assumes keys differ at byte 0, so the root is either the
// matching leaf or an inner node whose child at key[0] is the leaf.
// After a successful remove the root is demoted to a smaller node type
// (or collapsed to its only child) whenever its child count crosses
// the next-smaller capacity.
func (t *Tree) Delete(key []byte) bool {
	if t.root == nil {
		return false
	}
	if leafRoot, ok := t.root.(*leaf); ok {
		if bytes.Equal(leafRoot.key, key) {
			t.root = nil
			return true
		}
		return false
	}
	inner := t.root.(innerNode)
	if !isLeafWithKey(inner.findChild(key[0]), key) {
		return false
	}
	inner.removeChild(key[0])
	if inner.isEmpty() {
		t.root = nil
		return true
	}
	t.root = shrunkenOrSame(inner)
	return true
}

// shrunkenOrSame returns the smaller-kinded replacement for n if its
// child count has dropped to the next-smaller capacity (or to a single
// child, in node4's case), otherwise n itself.
func shrunkenOrSame(n innerNode) node {
	switch m := n.(type) {
	case *node256:
		if m.numChildren == node48Capacity {
			return shrinkToNode48(m)
		}
	case *node48:
		if m.numChildren == node16Capacity {
			return shrinkToNode16(m)
		}
	case *node16:
		if m.numChildren == node4Capacity {
			return shrinkToNode4(m)
		}
	case *node4:
		if m.numChildren == 1 {
			return collapseToOnlyChild(m)
		}
	}
	return n
}

// isLeafWithKey reports whether child is a leaf whose full key equals
// key. It returns false for nil and for inner-node children (which
// this slice's keys-differ-at-byte-0 assumption keeps out of reach).
func isLeafWithKey(child node, key []byte) bool {
	l, ok := child.(*leaf)
	return ok && bytes.Equal(l.key, key)
}
