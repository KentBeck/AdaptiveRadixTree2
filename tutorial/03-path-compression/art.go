// Package pathcompression is chapter 3 of the literate tutorial. It
// adds a `prefix []byte` field to the inner node so a single node
// can consume an arbitrarily long run of bytes that don't branch.
//
// Lazy expansion (chapter 2) eliminated the chain of one-child
// node256s for *unique tails* by storing the whole tail as a leaf.
// It did not eliminate the chain for *shared* runs of bytes: two
// URL keys that agree on `"https://api.example.com/v1/"` still
// allocate one node256 per shared byte, even though those nodes
// have only one child each.
//
// Path compression collapses each such chain into a single node
// whose `prefix` is the run of bytes consumed before branching.
// The headline win is on URL workloads: ~1 800 B/key in chapter 2
// drops to roughly the per-leaf overhead.
package pathcompression

import (
	"bytes"
	"iter"
)

// node is the same sum-type marker as chapter 2.
type node interface {
	isNode()
}

type leaf[V any] struct {
	key   []byte
	value V
}

func (*leaf[V]) isNode() {}

// node256 now carries a `prefix` field. The semantics:
//
//   - prefix is consumed from the search key *before* this node's
//     children are consulted. A node at path "https://" with
//     prefix=[]byte("api.example") represents the path
//     "https://api.example".
//   - children[b] holds the subtree reached by following one byte
//     b past the prefix.
//   - terminal holds the leaf for the key (if any) ending exactly
//     at the path-plus-prefix point of this node.
//
// An empty prefix is legal and common (e.g., the root of a tree
// with no shared leading bytes).
type node256[V any] struct {
	prefix   []byte
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

// longestCommonPrefix returns the length of the longest shared
// leading-byte run of a and b.
func longestCommonPrefix(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// consumePrefix matches a node's prefix against key[depth:]. Returns
// the advanced depth on success and (0, false) on mismatch.
//
// The length-zero short-circuit is load-bearing for performance: a
// `bytes.Equal(empty, empty)` function call is otherwise made at every
// inner node, and slice-header creation for `key[depth:end]` where
// end == depth costs another few cycles. On Sparse keys, where
// almost every prefix is empty, the difference between this guarded
// version and a naive `if !bytes.Equal(...)` is roughly 6x on Get.
// Production code in the parent package uses the same idiom for the
// same reason.
func consumePrefix(prefix, key []byte, depth int) (int, bool) {
	if len(prefix) == 0 {
		return depth, true
	}
	end := depth + len(prefix)
	if end > len(key) || !bytes.Equal(prefix, key[depth:end]) {
		return 0, false
	}
	return end, true
}

// Put associates value with key, replacing any previous value.
//
// The new wrinkle vs chapter 2 is the prefix-match step on every
// inner node. Three outcomes:
//
//   - prefix matches in full → descend past it (terminal or child).
//   - prefix matches partially and the key ran out → split the
//     prefix at the cut, new parent's terminal = the key.
//   - prefix matches partially and the key kept going on a
//     different byte → split the prefix at the divergence, new
//     parent has two branching children.
//
// The "split" cases are handled by splitPrefixedNode below.
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
	common := longestCommonPrefix(n.prefix, key[depth:])
	if common < len(n.prefix) {
		return splitPrefixedNode(n, key, value, depth, common, size)
	}
	depth += common
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

// splitTwoLeaves creates a single parent node256 to host two leaves
// that share a (possibly empty) prefix. Compared with chapter 2's
// implementation, no chain is needed: the shared bytes go into the
// new parent's prefix.
func splitTwoLeaves[V any](existing *leaf[V], newKey []byte, newValue V, depth int, size *int) node {
	a := existing.key[depth:]
	b := newKey[depth:]
	diverge := longestCommonPrefix(a, b)

	parent := &node256[V]{prefix: append([]byte(nil), a[:diverge]...)}

	*size++
	newLeaf := &leaf[V]{key: append([]byte(nil), newKey...), value: newValue}
	cut := depth + diverge
	switch {
	case cut == len(existing.key):
		parent.terminal = existing
		parent.children[newKey[cut]] = newLeaf
	case cut == len(newKey):
		parent.terminal = newLeaf
		parent.children[existing.key[cut]] = existing
	default:
		parent.children[existing.key[cut]] = existing
		parent.children[newKey[cut]] = newLeaf
	}
	return parent
}

// splitPrefixedNode handles the case where the new key matched only
// `common` bytes of n.prefix and then either ran out (terminal at
// the new parent) or diverged on a different byte (second branching
// child at the new parent).
//
// n's prefix is shortened in place to start past the (formerly
// shared) bytes plus the branch byte under which n hangs from the
// new parent.
func splitPrefixedNode[V any](n *node256[V], key []byte, value V, depth, common int, size *int) node {
	sharedPrefix := append([]byte(nil), n.prefix[:common]...)
	oldBranch := n.prefix[common]
	n.prefix = append([]byte(nil), n.prefix[common+1:]...)

	parent := &node256[V]{prefix: sharedPrefix}
	parent.children[oldBranch] = n

	*size++
	newLeaf := &leaf[V]{key: append([]byte(nil), key...), value: value}
	cut := depth + common
	if cut == len(key) {
		parent.terminal = newLeaf
	} else {
		parent.children[key[cut]] = newLeaf
	}
	return parent
}

// Get returns the value at key, if any.
//
// At every inner node, first verify that the node's prefix matches
// the next bytes of the key (a single bytes.Equal). A mismatch
// proves the key is absent without further descent. A length
// shortfall ("the key ran out mid-prefix") likewise proves absence.
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
		next, ok := consumePrefix(n.prefix, key, depth)
		if !ok {
			return zero, false
		}
		depth = next
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
// The reshape rules pick up a new collapse case: an inner node
// with one *inner* child and no terminal can now merge -- the
// child's prefix gains "n.prefix + branchByte" out front, and the
// child replaces n in its parent. Without prefix compression
// (chapter 2) this collapse was impossible because there was
// nowhere to record the byte that n consumed.
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
	next, ok := consumePrefix(n.prefix, key, depth)
	if !ok {
		return n, false
	}
	depth = next
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

func reshape[V any](n *node256[V]) node {
	var only node
	var onlyByte byte
	count := 0
	for b, c := range n.children {
		if c != nil {
			count++
			only = c
			onlyByte = byte(b)
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
		// Only child is an inner node: merge its prefix with ours
		// and the branch byte that linked them.
		child := only.(*node256[V])
		merged := make([]byte, 0, len(n.prefix)+1+len(child.prefix))
		merged = append(merged, n.prefix...)
		merged = append(merged, onlyByte)
		merged = append(merged, child.prefix...)
		child.prefix = merged
		return child
	}
	return n
}

// All yields every (key, value) pair in ascending byte-wise key
// order.
//
// Same shape as chapter 2: iterate the terminal first (its key is
// a strict prefix of every child's key) then walk children in
// ascending byte order. The prefix doesn't appear here -- leaves
// carry their own keys, and at iteration time we simply yield them.
func (t *Tree[V]) All() iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if t.root != nil {
			iterate(t.root, yield)
		}
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

// CountInner returns the number of *node256[V] currently allocated.
// Each one still occupies the same ~2 KB structural footprint as in
// chapters 1 and 2 -- the prefix lives in a separately-allocated
// []byte.
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

// CountLeaves returns the number of *leaf[V] currently allocated
// (counting both branching leaves and terminals attached to inner
// nodes). Equal to t.Len() in this stage.
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

// PrefixBytes returns the total number of bytes held in inner-node
// prefix slices. Reported alongside structural footprint so the
// bench output makes the new cost visible.
func (t *Tree[V]) PrefixBytes() int { return prefixBytes[V](t.root) }

func prefixBytes[V any](n node) int {
	r, ok := n.(*node256[V])
	if !ok {
		return 0
	}
	total := len(r.prefix)
	for _, c := range r.children {
		total += prefixBytes[V](c)
	}
	return total
}
