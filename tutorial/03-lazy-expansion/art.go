// Package lazyexpansion — chapter 3: leaves alongside node256.
//
// One leaf type plus the chapter-2 node256. A key with a unique
// tail is stored as a single leaf instead of a chain of one-child
// inner nodes; divergent keys allocate inner nodes only down to
// the byte where they part. Path compression is still future work.
package lazyexpansion

import (
	"bytes"
	"iter"
)

// --- types -----------------------------------------------------

type node interface {
	isNode()
}

type leaf[V any] struct {
	key   []byte
	value V
}

func (*leaf[V]) isNode() {}

type node256[V any] struct {
	children [256]node
	terminal *leaf[V]
}

func (*node256[V]) isNode() {}

type Tree[V any] struct {
	root node
	size int
}

// --- constructors ----------------------------------------------

func New[V any]() *Tree[V] { return &Tree[V]{} }

func (t *Tree[V]) Len() int { return t.size }

// --- read API --------------------------------------------------

func (t *Tree[V]) Get(key []byte) (V, bool) {
	var zero V
	current := t.root
	depth := 0
	for current != nil {
		if l, ok := current.(*leaf[V]); ok {
			if bytes.Equal(l.key, key) {
				return l.value, true
			}
			return zero, false
		}
		n := current.(*node256[V])
		if depth == len(key) {
			if n.terminal == nil {
				return zero, false
			}
			return n.terminal.value, true
		}
		current = n.children[key[depth]]
		depth++
	}
	return zero, false
}

// --- write API -------------------------------------------------

func (t *Tree[V]) Put(key []byte, value V) {
	t.root = putInto(t.root, key, value, 0, &t.size)
}

func (t *Tree[V]) Delete(key []byte) bool {
	if t.root == nil {
		return false
	}
	newRoot, deleted := deleteFrom[V](t.root, key, 0, &t.size)
	if deleted {
		t.root = newRoot
	}
	return deleted
}

// --- iteration -------------------------------------------------

func (t *Tree[V]) Range(from, to []byte) iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if t.root == nil {
			return
		}
		iterate(t.root, func(k []byte, v V) bool {
			if from != nil && bytes.Compare(k, from) < 0 {
				return true
			}
			if to != nil && bytes.Compare(k, to) >= 0 {
				return true
			}
			return yield(k, v)
		})
	}
}

// --- introspection ---------------------------------------------

func (t *Tree[V]) CountInner() int { return countInner[V](t.root) }

func (t *Tree[V]) CountLeaves() int { return countLeaves[V](t.root) }

// --- unexported helpers ----------------------------------------

func putInto[V any](current node, key []byte, value V, depth int, size *int) node {
	if current == nil {
		*size++
		return &leaf[V]{key: append([]byte(nil), key...), value: value}
	}

	if l, ok := current.(*leaf[V]); ok {
		if bytes.Equal(l.key, key) {
			l.value = value
			return l
		}
		return splitTwoLeaves(l, key, value, depth, size)
	}

	n := current.(*node256[V])
	if depth == len(key) {
		if n.terminal == nil {
			*size++
			n.terminal = &leaf[V]{key: append([]byte(nil), key...), value: value}
		} else {
			n.terminal.value = value
		}
		return n
	}
	b := key[depth]
	n.children[b] = putInto(n.children[b], key, value, depth+1, size)
	return n
}

// splitTwoLeaves builds the chain of node256s needed to host the
// existing leaf and the new key, branching at the first byte where
// they differ. One leaf may sit in a terminal if its key is a
// prefix of the other.
func splitTwoLeaves[V any](existing *leaf[V], newKey []byte, newValue V, depth int, size *int) node {
	diverge := depth
	for diverge < len(existing.key) && diverge < len(newKey) && existing.key[diverge] == newKey[diverge] {
		diverge++
	}

	head := &node256[V]{}
	n := head
	for i := depth; i < diverge; i++ {
		next := &node256[V]{}
		n.children[existing.key[i]] = next
		n = next
	}

	*size++
	newLeaf := &leaf[V]{key: append([]byte(nil), newKey...), value: newValue}

	switch {
	case diverge == len(existing.key):
		n.terminal = existing
		n.children[newKey[diverge]] = newLeaf
	case diverge == len(newKey):
		n.terminal = newLeaf
		n.children[existing.key[diverge]] = existing
	default:
		n.children[existing.key[diverge]] = existing
		n.children[newKey[diverge]] = newLeaf
	}
	return head
}

func deleteFrom[V any](current node, key []byte, depth int, size *int) (node, bool) {
	if l, ok := current.(*leaf[V]); ok {
		if bytes.Equal(l.key, key) {
			*size--
			return nil, true
		}
		return l, false
	}
	n := current.(*node256[V])
	if depth == len(key) {
		if n.terminal == nil || !bytes.Equal(n.terminal.key, key) {
			return n, false
		}
		n.terminal = nil
		*size--
		return reshape(n), true
	}
	b := key[depth]
	if n.children[b] == nil {
		return n, false
	}
	newChild, deleted := deleteFrom[V](n.children[b], key, depth+1, size)
	if !deleted {
		return n, false
	}
	n.children[b] = newChild
	return reshape(n), true
}

// reshape applies the post-Delete collapse rules: 0 children + no
// terminal → nil; 0 children + terminal → the terminal as a leaf;
// 1 leaf child + no terminal → hoist that leaf. Inner-node-only
// chains stay until chapter 4 introduces path compression.
func reshape[V any](n *node256[V]) node {
	var only node
	count := 0
	for _, c := range n.children {
		if c != nil {
			count++
			only = c
		}
	}
	if count == 0 {
		if n.terminal != nil {
			return n.terminal
		}
		return nil
	}
	if count == 1 && n.terminal == nil {
		if l, ok := only.(*leaf[V]); ok {
			return l
		}
	}
	return n
}

func iterate[V any](n node, yield func([]byte, V) bool) bool {
	if l, ok := n.(*leaf[V]); ok {
		return yield(l.key, l.value)
	}
	r := n.(*node256[V])
	if r.terminal != nil {
		if !yield(r.terminal.key, r.terminal.value) {
			return false
		}
	}
	for b := 0; b < 256; b++ {
		c := r.children[b]
		if c == nil {
			continue
		}
		if !iterate(c, yield) {
			return false
		}
	}
	return true
}

func countInner[V any](n node) int {
	r, ok := n.(*node256[V])
	if !ok {
		return 0
	}
	count := 1
	for _, c := range r.children {
		if c != nil {
			count += countInner[V](c)
		}
	}
	return count
}

func countLeaves[V any](n node) int {
	if n == nil {
		return 0
	}
	if _, ok := n.(*leaf[V]); ok {
		return 1
	}
	r := n.(*node256[V])
	count := 0
	if r.terminal != nil {
		count++
	}
	for _, c := range r.children {
		count += countLeaves[V](c)
	}
	return count
}
