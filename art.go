// Package art is an Adaptive Radix Tree implementation.
//
// This slice only has what is needed for a single round-trip (Put then
// Get). The full ART node hierarchy, path compression, and lazy
// expansion will arrive in later slices.
package art

import "bytes"

// nodeKind tags the concrete node variant behind the root pointer. Only
// one kind exists today, but naming it now keeps later slices honest
// when Node4/16/48/256 are introduced.
type nodeKind uint8

const (
	kindLeaf nodeKind = iota
)

// node is the common interface every ART node variant will satisfy.
// Today that is only *leaf.
type node interface {
	kind() nodeKind
}

// leaf stores the full key alongside its value. In later slices, inner
// nodes will branch on key bytes with leaves below them; for now a
// single leaf directly under the root is enough.
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

// Tree is a sorted map backed by an Adaptive Radix Tree.
type Tree struct {
	root node
}

// New returns an empty Tree.
func New() *Tree {
	return &Tree{}
}

// Put associates value with key.
func (t *Tree) Put(key []byte, value any) {
	t.root = newLeaf(key, value)
}

// Get returns the value previously stored under key, if any.
func (t *Tree) Get(key []byte) (value any, ok bool) {
	if t.root == nil {
		return nil, false
	}
	if l, isLeaf := t.root.(*leaf); isLeaf && bytes.Equal(l.key, key) {
		return l.value, true
	}
	return nil, false
}
