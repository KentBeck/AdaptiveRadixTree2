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

// node4 keeps keys[:numChildren] sorted ascending by edge byte.
type node4 struct {
	keys        [4]byte
	children    [4]node
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

// newNode4With returns a node4 branching an existing leaf and a new
// leaf on their first key byte. Only valid when the two keys differ at
// byte 0.
func newNode4With(existing *leaf, newKey []byte, newValue any) *node4 {
	n := &node4{}
	n.addChild(existing.key[0], existing)
	n.addChild(newKey[0], newLeaf(newKey, newValue))
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

// Put associates value with key. This slice handles overwrites of
// existing keys up to one node4 level; shared-prefix handling arrives
// later.
func (t *Tree) Put(key []byte, value any) {
	if t.root == nil {
		t.root = newLeaf(key, value)
		return
	}
	switch r := t.root.(type) {
	case *leaf:
		if bytes.Equal(r.key, key) {
			r.value = value
			return
		}
		t.root = newNode4With(r, key, value)
	case *node4:
		if existing, ok := r.findChild(key[0]).(*leaf); ok && bytes.Equal(existing.key, key) {
			existing.value = value
			return
		}
		if r.numChildren < node4Capacity {
			r.addChild(key[0], newLeaf(key, value))
			return
		}
		grown := growToNode16(r)
		grown.insertChild(key[0], newLeaf(key, value))
		t.root = grown
	case *node16:
		if existing, ok := r.findChild(key[0]).(*leaf); ok && bytes.Equal(existing.key, key) {
			existing.value = value
			return
		}
		if r.numChildren < node16Capacity {
			r.insertChild(key[0], newLeaf(key, value))
			return
		}
		grown := growToNode48(r)
		grown.addChild(key[0], newLeaf(key, value))
		t.root = grown
	case *node48:
		if existing, ok := r.findChild(key[0]).(*leaf); ok && bytes.Equal(existing.key, key) {
			existing.value = value
			return
		}
		if r.numChildren < node48Capacity {
			r.addChild(key[0], newLeaf(key, value))
			return
		}
		grown := growToNode256(r)
		grown.addChild(key[0], newLeaf(key, value))
		t.root = grown
	case *node256:
		if existing, ok := r.findChild(key[0]).(*leaf); ok && bytes.Equal(existing.key, key) {
			existing.value = value
			return
		}
		r.addChild(key[0], newLeaf(key, value))
	}
}

// Get returns the value previously stored under key, if any.
func (t *Tree) Get(key []byte) (value any, ok bool) {
	current := t.root
	for current != nil {
		switch n := current.(type) {
		case *leaf:
			if bytes.Equal(n.key, key) {
				return n.value, true
			}
			return nil, false
		case *node4:
			current = n.findChild(key[0])
		case *node16:
			current = n.findChild(key[0])
		case *node48:
			current = n.findChild(key[0])
		case *node256:
			current = n.findChild(key[0])
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
