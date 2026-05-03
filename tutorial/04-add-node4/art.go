// Package addnode4 is chapter 4 of the literate tutorial. It
// introduces a *second* inner-node type, *node4*, sized for nodes
// with at most four branching children. The other inner-node type,
// *node256*, is unchanged from chapter 3.
//
// Two node types means two cases in every dispatch: prefix lookup,
// child lookup, child insertion, child removal, reshape on Delete,
// iteration. Chapter 4 implements that dispatch with explicit
// type-switch helpers (`nodeFindChild`, `nodeAddOrGrowChild`, ...).
// At two cases the duplication is bearable; at four cases (chapter
// 6 adds node16, chapter 7 adds node48) it becomes intolerable.
// That tension is why chapter 5 introduces method polymorphism --
// before the third and fourth additions, not after.
//
// Read this chapter for two things: how a smaller inner node saves
// memory when fanout is low, and how the two-case switch reads.
// Hold on to that reading; chapter 5's diff will be exactly the
// switches collapsing to method calls.
package addnode4

import (
	"bytes"
	"iter"
)

// node is the same sum-type marker as chapters 2 and 3.
type node interface {
	isNode()
}

type leaf[V any] struct {
	key   []byte
	value V
}

func (*leaf[V]) isNode() {}

// node4 is the new small inner-node type. It carries up to four
// branching children, plus the same prefix and terminal slot as
// node256.
//
//   - keys[:numChildren] is sorted ascending by edge byte. Sorted
//     storage is load-bearing for All (children appear in byte
//     order without a separate sort) and for the bench-marker
//     ascending-iteration tests.
//   - children[i] is the child reached by edge keys[i].
//   - prefix and terminal carry the same meaning as on node256.
type node4[V any] struct {
	prefix      []byte
	keys        [4]byte
	children    [4]node
	terminal    *leaf[V]
	numChildren uint8
}

func (*node4[V]) isNode() {}

const node4Capacity = 4

// node256 is unchanged from chapter 3 except for the explicit
// numChildren counter. The counter is needed so reshape can
// demote a sparsely-populated node256 down to a node4 in O(1)
// rather than scanning all 256 slots.
type node256[V any] struct {
	prefix      []byte
	children    [256]node
	terminal    *leaf[V]
	numChildren uint16
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

// ---- per-type primitives ----------------------------------------------------
// A node4 has four primitives: findChild, addChild (sorted insert),
// replaceChild, removeChild. node256 has the equivalent operations
// implemented directly against its [256]node array. The dispatch
// helpers below pick which one to call based on the runtime type.

func (n *node4[V]) findChild(b byte) node {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}

// addChild inserts (b, child) keeping keys[:numChildren] sorted
// ascending. Caller guarantees b is not already present and
// numChildren < node4Capacity.
func (n *node4[V]) addChild(b byte, child node) {
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

func (n *node4[V]) replaceChild(b byte, child node) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			n.children[i] = child
			return
		}
	}
}

func (n *node4[V]) removeChild(b byte) {
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

// ---- dispatch helpers -------------------------------------------------------
// Each helper inspects the concrete inner-node type and calls the
// type-specific primitive. Every operation file (Put, Get, Delete,
// All) dispatches through these. Chapter 5 will replace the
// switches with method calls on an `innerNode` interface.

func nodePrefix[V any](n node) []byte {
	switch r := n.(type) {
	case *node4[V]:
		return r.prefix
	case *node256[V]:
		return r.prefix
	}
	panic("nodePrefix: unknown inner-node type")
}

func setNodePrefix[V any](n node, p []byte) {
	switch r := n.(type) {
	case *node4[V]:
		r.prefix = p
	case *node256[V]:
		r.prefix = p
	default:
		panic("setNodePrefix: unknown inner-node type")
	}
}

func nodeTerminal[V any](n node) *leaf[V] {
	switch r := n.(type) {
	case *node4[V]:
		return r.terminal
	case *node256[V]:
		return r.terminal
	}
	panic("nodeTerminal: unknown inner-node type")
}

func setNodeTerminal[V any](n node, l *leaf[V]) {
	switch r := n.(type) {
	case *node4[V]:
		r.terminal = l
	case *node256[V]:
		r.terminal = l
	default:
		panic("setNodeTerminal: unknown inner-node type")
	}
}

func nodeFindChild[V any](n node, b byte) node {
	switch r := n.(type) {
	case *node4[V]:
		return r.findChild(b)
	case *node256[V]:
		return r.children[b]
	}
	panic("nodeFindChild: unknown inner-node type")
}

// nodeAddOrGrowChild adds (b, child) under n, returning n itself
// (or, if the existing n was a full node4, a freshly grown node256
// holding the same data plus the new child). Callers must replace
// their reference to n with the returned node.
func nodeAddOrGrowChild[V any](n node, b byte, child node) node {
	switch r := n.(type) {
	case *node4[V]:
		if r.numChildren < node4Capacity {
			r.addChild(b, child)
			return r
		}
		grown := growToNode256[V](r)
		grown.children[b] = child
		grown.numChildren++
		return grown
	case *node256[V]:
		r.children[b] = child
		r.numChildren++
		return r
	}
	panic("nodeAddOrGrowChild: unknown inner-node type")
}

func nodeReplaceChild[V any](n node, b byte, child node) {
	switch r := n.(type) {
	case *node4[V]:
		r.replaceChild(b, child)
	case *node256[V]:
		r.children[b] = child
	default:
		panic("nodeReplaceChild: unknown inner-node type")
	}
}

func nodeRemoveChild[V any](n node, b byte) {
	switch r := n.(type) {
	case *node4[V]:
		r.removeChild(b)
	case *node256[V]:
		if r.children[b] != nil {
			r.children[b] = nil
			r.numChildren--
		}
	default:
		panic("nodeRemoveChild: unknown inner-node type")
	}
}

func numChildren[V any](n node) int {
	switch r := n.(type) {
	case *node4[V]:
		return int(r.numChildren)
	case *node256[V]:
		return int(r.numChildren)
	}
	panic("numChildren: unknown inner-node type")
}

// eachAscending yields (edge, child) pairs in ascending edge-byte
// order. Used by All and the only-child reshape walk.
func eachAscending[V any](n node, yield func(byte, node) bool) bool {
	switch r := n.(type) {
	case *node4[V]:
		for i := uint8(0); i < r.numChildren; i++ {
			if !yield(r.keys[i], r.children[i]) {
				return false
			}
		}
		return true
	case *node256[V]:
		for b := 0; b < 256; b++ {
			if r.children[b] != nil {
				if !yield(byte(b), r.children[b]) {
					return false
				}
			}
		}
		return true
	}
	panic("eachAscending: unknown inner-node type")
}

// growToNode256 returns a node256 holding the same prefix,
// terminal, and children as the supplied node4. Used when a node4
// would exceed its 4-child capacity.
func growToNode256[V any](n *node4[V]) *node256[V] {
	grown := &node256[V]{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: uint16(n.numChildren),
	}
	for i := uint8(0); i < n.numChildren; i++ {
		grown.children[n.keys[i]] = n.children[i]
	}
	return grown
}

// shrinkToNode4 returns a node4 holding the same prefix, terminal,
// and children as the supplied node256. Used when reshape sees a
// node256 fall to <= 4 children. Caller guarantees n.numChildren <=
// node4Capacity. Children are inserted in ascending edge-byte order
// because we walk children[0..255] sequentially.
func shrinkToNode4[V any](n *node256[V]) *node4[V] {
	shrunk := &node4[V]{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: uint8(n.numChildren),
	}
	i := uint8(0)
	for b := 0; b < 256; b++ {
		if n.children[b] != nil {
			shrunk.keys[i] = byte(b)
			shrunk.children[i] = n.children[b]
			i++
		}
	}
	return shrunk
}

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

// consumePrefix matches a node's prefix against key[depth:].
// Returns the advanced depth on success and (0, false) on mismatch.
// Same short-circuited form as chapter 3.
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

// ---- Put --------------------------------------------------------------------

// Put associates value with key, replacing any previous value.
//
// The shape mirrors chapter 3 but every inner-node access goes
// through a dispatch helper.
func (t *Tree[V]) Put(key []byte, value V) {
	t.root = putInto[V](t.root, key, value, 0, &t.size)
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
	prefix := nodePrefix[V](current)
	common := longestCommonPrefix(prefix, key[depth:])
	if common < len(prefix) {
		return splitPrefixedNode(current, key, value, depth, common, size)
	}
	depth += common
	if depth == len(key) {
		if t := nodeTerminal[V](current); t == nil {
			*size++
			setNodeTerminal[V](current, &leaf[V]{key: append([]byte(nil), key...), value: value})
		} else {
			t.value = value
		}
		return current
	}
	b := key[depth]
	child := nodeFindChild[V](current, b)
	if child == nil {
		newLeaf := &leaf[V]{key: append([]byte(nil), key...), value: value}
		*size++
		return nodeAddOrGrowChild[V](current, b, newLeaf)
	}
	newChild := putInto[V](child, key, value, depth+1, size)
	if newChild != child {
		nodeReplaceChild[V](current, b, newChild)
	}
	return current
}

// splitTwoLeaves builds a new node4 parent (always start small)
// hosting two leaves that share an optional prefix.
func splitTwoLeaves[V any](existing *leaf[V], newKey []byte, newValue V, depth int, size *int) node {
	a := existing.key[depth:]
	b := newKey[depth:]
	diverge := longestCommonPrefix(a, b)

	parent := &node4[V]{prefix: append([]byte(nil), a[:diverge]...)}

	*size++
	newLeaf := &leaf[V]{key: append([]byte(nil), newKey...), value: newValue}
	cut := depth + diverge
	switch {
	case cut == len(existing.key):
		parent.terminal = existing
		parent.addChild(newKey[cut], newLeaf)
	case cut == len(newKey):
		parent.terminal = newLeaf
		parent.addChild(existing.key[cut], existing)
	default:
		parent.addChild(existing.key[cut], existing)
		parent.addChild(newKey[cut], newLeaf)
	}
	return parent
}

// splitPrefixedNode handles a partial prefix match. The new parent
// is a node4 hosting the (shortened) old node and the new leaf.
func splitPrefixedNode[V any](n node, key []byte, value V, depth, common int, size *int) node {
	oldPrefix := nodePrefix[V](n)
	sharedPrefix := append([]byte(nil), oldPrefix[:common]...)
	oldBranch := oldPrefix[common]
	setNodePrefix[V](n, append([]byte(nil), oldPrefix[common+1:]...))

	parent := &node4[V]{prefix: sharedPrefix}
	parent.addChild(oldBranch, n)

	*size++
	newLeaf := &leaf[V]{key: append([]byte(nil), key...), value: value}
	cut := depth + common
	if cut == len(key) {
		parent.terminal = newLeaf
	} else {
		parent.addChild(key[cut], newLeaf)
	}
	return parent
}

// ---- Get --------------------------------------------------------------------

// Get returns the value at key, if any.
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
		next, ok := consumePrefix(nodePrefix[V](current), key, depth)
		if !ok {
			return zero, false
		}
		depth = next
		if depth == len(key) {
			t := nodeTerminal[V](current)
			if t == nil {
				return zero, false
			}
			return t.value, true
		}
		current = nodeFindChild[V](current, key[depth])
		depth++
	}
	return zero, false
}

// ---- Delete -----------------------------------------------------------------

// Delete removes key, returning whether it was present.
//
// The reshape rules are the same as chapter 3 plus one: a node256
// with <= 4 children is demoted to a node4. Demotion preserves the
// keys-sorted-ascending invariant because shrinkToNode4 walks the
// node256's children in ascending byte order.
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
	next, ok := consumePrefix(nodePrefix[V](current), key, depth)
	if !ok {
		return current, false
	}
	depth = next
	if depth == len(key) {
		t := nodeTerminal[V](current)
		if t == nil || !bytes.Equal(t.key, key) {
			return current, false
		}
		setNodeTerminal[V](current, nil)
		*size--
		return reshape[V](current), true
	}
	b := key[depth]
	child := nodeFindChild[V](current, b)
	if child == nil {
		return current, false
	}
	newChild, deleted := deleteFrom[V](child, key, depth+1, size)
	if !deleted {
		return current, false
	}
	if newChild == nil {
		nodeRemoveChild[V](current, b)
	} else {
		nodeReplaceChild[V](current, b, newChild)
	}
	return reshape[V](current), true
}

// reshape applies the post-Delete collapse rules to current. The
// new chapter-4 case is the demotion: a node256 with <= 4 children
// becomes a node4. The collapse rules from chapter 3 (drop empty,
// hoist sole leaf, merge sole inner-child prefix) still apply and
// are dispatched type-by-type.
func reshape[V any](current node) node {
	count := numChildren[V](current)
	terminal := nodeTerminal[V](current)
	if count == 0 {
		if terminal != nil {
			return terminal
		}
		return nil
	}
	if count == 1 && terminal == nil {
		var only node
		var onlyByte byte
		eachAscending[V](current, func(b byte, c node) bool {
			only = c
			onlyByte = b
			return false
		})
		if l, ok := only.(*leaf[V]); ok {
			return l
		}
		// Only child is an inner node: merge prefixes.
		parentPrefix := nodePrefix[V](current)
		childPrefix := nodePrefix[V](only)
		merged := make([]byte, 0, len(parentPrefix)+1+len(childPrefix))
		merged = append(merged, parentPrefix...)
		merged = append(merged, onlyByte)
		merged = append(merged, childPrefix...)
		setNodePrefix[V](only, merged)
		return only
	}
	// Demote node256 to node4 once it falls to the smaller type's
	// capacity. This is the new chapter-4 case.
	if r, ok := current.(*node256[V]); ok && r.numChildren <= node4Capacity {
		return shrinkToNode4[V](r)
	}
	return current
}

// ---- All --------------------------------------------------------------------

// All yields every (key, value) pair in ascending byte-wise key
// order. Same shape as chapters 2 and 3; the only change is that
// children-iteration goes through eachAscending so each node type
// can yield in its own native order.
func (t *Tree[V]) All() iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if t.root != nil {
			iterate[V](t.root, yield)
		}
	}
}

func iterate[V any](n node, yield func([]byte, V) bool) bool {
	if l, ok := n.(*leaf[V]); ok {
		return yield(l.key, l.value)
	}
	if t := nodeTerminal[V](n); t != nil {
		if !yield(t.key, t.value) {
			return false
		}
	}
	return eachAscending[V](n, func(_ byte, c node) bool {
		return iterate[V](c, yield)
	})
}

// ---- footprint / count helpers ----------------------------------------------

// CountInner returns the total number of inner nodes (node4 +
// node256) currently allocated.
func (t *Tree[V]) CountInner() int {
	c4, c256 := countByKind[V](t.root)
	return c4 + c256
}

// CountByKind returns (node4 count, node256 count) for the bench
// addendum. Lets the per-stage report show how the inner-node mix
// shifts after chapter 4.
func (t *Tree[V]) CountByKind() (n4, n256 int) {
	return countByKind[V](t.root)
}

func countByKind[V any](n node) (n4, n256 int) {
	switch r := n.(type) {
	case *node4[V]:
		n4 = 1
		for i := uint8(0); i < r.numChildren; i++ {
			a, b := countByKind[V](r.children[i])
			n4 += a
			n256 += b
		}
	case *node256[V]:
		n256 = 1
		for _, c := range r.children {
			if c != nil {
				a, b := countByKind[V](c)
				n4 += a
				n256 += b
			}
		}
	}
	return
}

// CountLeaves returns the number of leaves currently allocated.
func (t *Tree[V]) CountLeaves() int { return countLeaves[V](t.root) }

func countLeaves[V any](n node) int {
	if n == nil {
		return 0
	}
	if _, ok := n.(*leaf[V]); ok {
		return 1
	}
	count := 0
	if nodeTerminal[V](n) != nil {
		count++
	}
	eachAscending[V](n, func(_ byte, c node) bool {
		count += countLeaves[V](c)
		return true
	})
	return count
}

// PrefixBytes returns the total bytes held in inner-node prefix
// slices.
func (t *Tree[V]) PrefixBytes() int { return prefixBytes[V](t.root) }

func prefixBytes[V any](n node) int {
	if n == nil {
		return 0
	}
	if _, ok := n.(*leaf[V]); ok {
		return 0
	}
	total := len(nodePrefix[V](n))
	eachAscending[V](n, func(_ byte, c node) bool {
		total += prefixBytes[V](c)
		return true
	})
	return total
}
