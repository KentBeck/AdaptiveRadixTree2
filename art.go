// Package art is an Adaptive Radix Tree implementation.
//
// Keys are currently assumed to differ at byte 0. Path compression,
// lazy expansion, and the Node256 variant will arrive in later slices.
package art

import "bytes"

type nodeKind uint8

const (
	kindLeaf nodeKind = iota
	kindNode4
	kindNode16
	kindNode48
)

const (
	node4Capacity  = 4
	node16Capacity = 16
	node48Capacity = 48
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

// growToNode16 returns a node16 holding the same sorted children as n.
func growToNode16(n *node4) *node16 {
	grown := &node16{numChildren: n.numChildren}
	copy(grown.keys[:n.numChildren], n.keys[:n.numChildren])
	copy(grown.children[:n.numChildren], n.children[:n.numChildren])
	return grown
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
		}
	}
	return nil, false
}
