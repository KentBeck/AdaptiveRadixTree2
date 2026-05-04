// Package lazyexpansion is chapter 2 of the literate tutorial. It
// keeps chapter 1's full-fanout 256-child inner node but introduces
// a *leaf* type so that a key with a unique tail is stored once,
// not as a chain of one-child node256s.
//
// The shape now has two kinds of node:
//
//   - *leaf[V]: stores a (key, value) pair directly. The key is a
//     full copy of what the caller passed in.
//   - *node256[V]: an inner node carrying a 256-slot children array
//     and an optional terminal *leaf[V] for the key (if any) that
//     ends exactly at this node's path.
//
// Both implement the empty-method `node` interface so a child slot
// (or a tree root) can hold either kind. Chapter 5 will give that
// interface real methods; for now it is a *sum-type marker* and
// every dispatch happens via type assertion in put/get/delete.
//
// What lazy expansion buys: when the new key has no sibling in the
// tree (the common case for sparse and URL workloads), Put creates a
// single leaf rather than a 16-deep chain of node256s. When two
// keys diverge, only the inner nodes needed to host the divergence
// are allocated.
//
// What it does not yet buy: a long chain of node256s with one child
// each remains. That is what path compression (chapter 3) removes.
package lazyexpansion

import (
	"bytes"
	"iter"
)

// node is the sum-type marker satisfied by both *leaf[V] and
// *node256[V]. The interface is deliberately empty: chapter 5
// promotes it (via a sibling interface) into a polymorphic dispatch
// surface for the four ART node sizes; here it just lets a child
// slot hold either a leaf or an inner node.
type node interface {
	isNode()
}

// leaf stores a (key, value) pair. The key is a copy of what the
// caller passed to Put; mutating the caller's slice afterward does
// not change anything in the tree.
type leaf[V any] struct {
	key   []byte
	value V
}

func (*leaf[V]) isNode() {}

// node256 is the only inner-node type at this stage. terminal holds
// the leaf for the key (if any) whose path ends exactly here;
// children[b] holds the subtree reachable by following edge byte b.
type node256[V any] struct {
	children [256]node
	terminal *leaf[V]
}

func (*node256[V]) isNode() {}

// Tree is the public sorted map.
type Tree[V any] struct {
	root node
	size int
}

// New returns an empty Tree.
func New[V any]() *Tree[V] { return &Tree[V]{} }

// Len returns the number of (key, value) pairs.
func (t *Tree[V]) Len() int { return t.size }

// Put associates value with key, replacing any previous value.
//
// The shape of the recursion: an empty subtree becomes a fresh leaf;
// an existing leaf either replaces its value (same key) or splits
// into a chain of node256s ending in a divergence node hosting both
// leaves; an existing inner node either sets its terminal (key
// ends here) or descends into the appropriate child slot.
func (t *Tree[V]) Put(key []byte, value V) {
	t.root = putInto(t.root, key, value, 0, &t.size)
}

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

// splitTwoLeaves builds the inner-node structure required to host
// two leaves whose keys agree from byte 0 through byte depth-1 and
// diverge somewhere from byte depth onward. The chain of node256s
// it returns is the *minimum* needed: one for every byte up to the
// first divergence.
//
// Without path compression, every shared byte still costs a fresh
// node256. Chapter 3 collapses this chain into a single prefix.
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

// Get returns the value at key, if any.
//
// Walk down. At a leaf, the answer is determined by a single full
// key compare (no further descent). At an inner node, either the
// key ended here (consult terminal) or descend to children[key[depth]].
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

// Delete removes key, returning whether it was present.
//
// The recursion has two new collapse cases compared with chapter 1:
//
//   - 0 children + nil terminal: drop this inner node (return nil)
//   - 0 children + 1 terminal: collapse to that terminal leaf
//   - 1 leaf child + nil terminal: hoist the leaf (it stores the
//     full key, so its position in the tree is irrelevant -- only
//     the bytes matter on lookup)
//   - 1 inner child + nil terminal: keep the chain (without prefix
//     compression, the parent cannot represent the skipped byte;
//     chapter 3 fixes this)
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

// reshape applies the post-Delete collapse rules. The result is the
// node that should occupy this slot in the parent (or as the root):
// the original n, the terminal leaf, the only child, or nil.
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

// Range returns an iter.Seq2 of all (key, value) pairs whose keys
// fall in the half-open interval [from, to), in ascending byte
// order. A nil bound is unbounded on that side, so Range(nil, nil)
// yields every entry.
//
// The pleasant change from chapter 1: leaves carry the full key, so
// Range can yield the leaf's key directly without building a path
// buffer along the way. That eliminates the per-yield allocation
// and copy that chapter 1 paid.
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

// CountInner returns the number of *node256[V] currently allocated
// in t. Each one occupies the same ~2 KB structural footprint as in
// chapter 1.
func (t *Tree[V]) CountInner() int { return countInner[V](t.root) }

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

// CountLeaves returns the number of *leaf[V] currently allocated in
// t (counting both branching leaves and the terminals attached to
// inner nodes). Equal to t.Len() in this stage; the helper exists
// to keep the bench side mechanical.
func (t *Tree[V]) CountLeaves() int { return countLeaves[V](t.root) }

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
