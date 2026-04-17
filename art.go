// Package art is an Adaptive Radix Tree implementation.
//
// This slice supports two keys that differ at byte 0. Path compression,
// lazy expansion, and the larger node variants (Node16/48/256) will
// arrive in later slices.
package art

import "bytes"

// nodeKind tags the concrete node variant behind a node pointer. Naming
// each variant keeps later slices honest when Node16/48/256 are
// introduced.
type nodeKind uint8

const (
	kindLeaf nodeKind = iota
	kindNode4
)

// node is the common interface every ART node variant satisfies.
type node interface {
	kind() nodeKind
}

// leaf stores the full key alongside its value. Inner nodes branch on
// key bytes with leaves below them.
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

// node4 is ART's smallest branching inner node (capacity 4). Children
// are kept sorted by their edge byte so iteration and later promotions
// (Node16, Node48) can rely on the same invariant.
type node4 struct {
	keys        [4]byte
	children    [4]node
	numChildren uint8
}

func (*node4) kind() nodeKind { return kindNode4 }

// findChild returns the child reached by edge byte b, or nil when none
// of the populated slots match.
func (n *node4) findChild(b byte) node {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}

// insertChildSorted inserts child under edge byte b while preserving
// the sorted-keys invariant. The caller guarantees b is not already
// present and that the node is not yet full.
func (n *node4) insertChildSorted(b byte, child node) {
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

// splitLeafIntoNode4 replaces an existing leaf with a node4 holding
// both the existing leaf and a new leaf, indexed by their first bytes.
// Only valid when the two keys differ at byte 0.
func splitLeafIntoNode4(existing *leaf, newKey []byte, newValue any) *node4 {
	n := &node4{}
	n.insertChildSorted(existing.key[0], existing)
	n.insertChildSorted(newKey[0], newLeaf(newKey, newValue))
	return n
}

// Tree is a sorted map backed by an Adaptive Radix Tree.
type Tree struct {
	root node
}

// New returns an empty Tree.
func New() *Tree {
	return &Tree{}
}

// Put associates value with key. This slice assumes keys have distinct
// first bytes; overwrite and shared-prefix handling arrive later.
func (t *Tree) Put(key []byte, value any) {
	if t.root == nil {
		t.root = newLeaf(key, value)
		return
	}
	switch r := t.root.(type) {
	case *leaf:
		t.root = splitLeafIntoNode4(r, key, value)
	case *node4:
		r.insertChildSorted(key[0], newLeaf(key, value))
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
		}
	}
	return nil, false
}
