// Package nodeonly256 — chapter 2: the disaster baseline.
//
// One node type. No leaves, no prefix compression. ~31 KB per key
// on Sparse workloads. Exists to set the cost-to-beat that every
// later chapter measures itself against.
package nodeonly256

import (
	"bytes"
	"iter"
)

// --- types -----------------------------------------------------

type node[V any] struct {
	children [256]*node[V]
	terminal *V
}

type Tree[V any] struct {
	root *node[V]
	size int
}

// --- constructors ----------------------------------------------

func New[V any]() *Tree[V] { return &Tree[V]{} }

func (t *Tree[V]) Len() int { return t.size }

// --- read API --------------------------------------------------

func (t *Tree[V]) Get(key []byte) (V, bool) {
	var zero V
	n := t.root
	if n == nil {
		return zero, false
	}
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

// --- write API -------------------------------------------------

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

// --- iteration -------------------------------------------------

func (t *Tree[V]) Range(from, to []byte) iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if t.root == nil {
			return
		}
		walk(t.root, nil, func(k []byte, v V) bool {
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

// --- introspection (chapter-specific; used by bench addendum) --

func (t *Tree[V]) CountNodes() int {
	if t.root == nil {
		return 0
	}
	return countNodes(t.root)
}

// --- unexported helpers ----------------------------------------

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

func walk[V any](n *node[V], path []byte, yield func([]byte, V) bool) bool {
	if n.terminal != nil {
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

func countNodes[V any](n *node[V]) int {
	count := 1
	for _, c := range n.children {
		if c != nil {
			count += countNodes(c)
		}
	}
	return count
}
