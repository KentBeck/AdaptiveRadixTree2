// Package art is an Adaptive Radix Tree implementation.
//
// This slice supports two keys that differ at byte 0. Path compression,
// lazy expansion, and the larger node variants (Node16/48/256) will
// arrive in later slices.
package art

import "bytes"

type nodeKind uint8

const (
	kindLeaf nodeKind = iota
	kindNode4
)

type node interface {
	kind() nodeKind
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

// insertChild inserts child under edge byte b. Caller guarantees b is
// not already present and that the node is not yet full.
func (n *node4) insertChild(b byte, child node) {
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

// newNode4With returns a node4 branching an existing leaf and a new
// leaf on their first key byte. Only valid when the two keys differ at
// byte 0.
func newNode4With(existing *leaf, newKey []byte, newValue any) *node4 {
	n := &node4{}
	n.insertChild(existing.key[0], existing)
	n.insertChild(newKey[0], newLeaf(newKey, newValue))
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
		r.insertChild(key[0], newLeaf(key, value))
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
