// Package nodeonly256 is chapter 1 of the literate tutorial: the
// simplest possible trie. There is exactly one node type. Every node
// holds:
//
//   - children[256]: a pointer to a child node for each possible byte
//     of the key alphabet, mostly nil.
//   - terminal: a pointer to a stored value (nil if no key ends at
//     this node's path).
//
// There is no leaf type, no path compression, no smaller node sizes.
// A key of length k always traverses exactly k inner nodes; an inner
// node always allocates 256 child slots no matter how many it uses.
//
// This is the disaster baseline. With 1 000 random 16-byte keys it
// allocates about 33 MB and runs Get at roughly 16 cache misses per
// key. Every later chapter takes one decision against this baseline
// and measures the savings.
package nodeonly256

import (
	"iter"
)

// node is the only node type in this stage. It carries every byte's
// worth of fanout (children[256]) plus an optional terminal value
// for the key that ends at this node's path. terminal is *V rather
// than V so the zero V is distinguishable from "no entry here".
type node[V any] struct {
	children [256]*node[V]
	terminal *V
}

// Tree is the public sorted map.
type Tree[V any] struct {
	root *node[V]
	size int
}

// New returns an empty Tree.
func New[V any]() *Tree[V] { return &Tree[V]{} }

// Len returns the number of (key, value) pairs.
func (t *Tree[V]) Len() int { return t.size }

// Put associates value with key, replacing any previous value.
//
// Walk the trie one byte at a time, allocating child nodes as
// needed. At the terminus, set or replace the terminal value.
func (t *Tree[V]) Put(key []byte, value V) {
	if t.root == nil {
		t.root = &node[V]{}
	}
	n := t.root
	for _, b := range key {
		if n.children[b] == nil {
			n.children[b] = &node[V]{}
		}
		n = n.children[b]
	}
	if n.terminal == nil {
		t.size++
	}
	v := value
	n.terminal = &v
}

// Get returns the value at key, if any.
//
// Walk the trie one byte at a time. If any edge is missing the key
// is absent. Otherwise the answer is the terminus node's terminal.
func (t *Tree[V]) Get(key []byte) (V, bool) {
	var zero V
	if t.root == nil {
		return zero, false
	}
	n := t.root
	for _, b := range key {
		n = n.children[b]
		if n == nil {
			return zero, false
		}
	}
	if n.terminal == nil {
		return zero, false
	}
	return *n.terminal, true
}

// Delete removes key, returning whether it was present.
//
// Walk to the terminus, clear its terminal, then walk back up
// pruning any node that is now both childless and terminal-less.
// The pruning is what makes Delete actually free memory; without
// it, deleted keys would leave behind 16-deep chains of empty
// node256 instances.
func (t *Tree[V]) Delete(key []byte) bool {
	if t.root == nil {
		return false
	}
	deleted, empty := deleteFrom(t.root, key, 0)
	if deleted {
		t.size--
	}
	if empty {
		t.root = nil
	}
	return deleted
}

func deleteFrom[V any](n *node[V], key []byte, depth int) (deleted, empty bool) {
	if depth == len(key) {
		if n.terminal == nil {
			return false, false
		}
		n.terminal = nil
		return true, isEmpty(n)
	}
	child := n.children[key[depth]]
	if child == nil {
		return false, false
	}
	deleted, childEmpty := deleteFrom(child, key, depth+1)
	if !deleted {
		return false, false
	}
	if childEmpty {
		n.children[key[depth]] = nil
	}
	return true, isEmpty(n)
}

func isEmpty[V any](n *node[V]) bool {
	if n.terminal != nil {
		return false
	}
	for _, c := range n.children {
		if c != nil {
			return false
		}
	}
	return true
}

// All yields every (key, value) pair in ascending byte-wise key
// order.
//
// Iterating children in ascending byte order at every node yields
// keys in sorted order for free: there is no comparison and no
// balancing. This is the property that makes a trie a sorted map.
func (t *Tree[V]) All() iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if t.root != nil {
			walk(t.root, nil, yield)
		}
	}
}

func walk[V any](n *node[V], path []byte, yield func([]byte, V) bool) bool {
	if n.terminal != nil {
		// Copy the path -- the caller may retain the key, and we
		// reuse the path buffer below.
		out := make([]byte, len(path))
		copy(out, path)
		if !yield(out, *n.terminal) {
			return false
		}
	}
	for b := 0; b < 256; b++ {
		c := n.children[b]
		if c == nil {
			continue
		}
		if !walk(c, append(path, byte(b)), yield) {
			return false
		}
	}
	return true
}

// CountNodes reports the number of inner nodes currently allocated
// in t. Every inner node, regardless of how many children it
// actually uses, occupies a full [256]*node[V] array (~2 KB on a
// 64-bit machine) plus the terminal pointer. Multiply by
// approximately 2 056 bytes/node to get the structural footprint
// shown in the bench addendum.
func (t *Tree[V]) CountNodes() int {
	if t.root == nil {
		return 0
	}
	return countNodes(t.root)
}

func countNodes[V any](n *node[V]) int {
	count := 1
	for _, c := range n.children {
		if c != nil {
			count += countNodes(c)
		}
	}
	return count
}
