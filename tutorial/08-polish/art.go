// Package polish is chapter 8 of the literate tutorial. It is the
// final polish chapter: chapter 7's data structure plus three
// targeted refinements that close the gap to the production
// art.Tree, and one new feature (Range) that exercises the most
// interesting of those refinements.
//
// The polishes:
//
//  1. Inline-key buffer on *leaf. Short keys (<= 24 bytes) are
//     copied into a fixed array on the leaf itself, eliminating
//     the second allocation on Put. Long keys still go to the
//     heap.
//
//  2. Embedded innerHeader. The prefix and terminal fields and
//     their four trivial getter/setter methods used to be repeated
//     on every inner-node type (4 fields x 4 types = 16 method
//     bodies). Embedding a single innerHeader struct lets Go's
//     method-promotion rules satisfy the interface for free.
//
//  3. Range with a reused path buffer. iterateRange threads a
//     single []byte through the recursion, growing it as the
//     descent enters each prefix and shrinking it back on the way
//     out. Every other range-iteration approach allocates per
//     yield; this one is zero-alloc.
//
// Each polish is shown below as a small diff against chapter 7
// and benchmarked against it. The end of the chapter is a reading
// guide pointing at the corresponding parent-package files.
//
// There is no chapter 9. The production art.Tree extends this
// shape with Min/Max/Ceiling/Floor, a LockedTree concurrency
// wrapper, an artmap typed facade, and an industrial-scale fuzz
// harness. The reading guide tells you where each one lives.
package polish

import (
	"bytes"
	"iter"
)

// node is the sum-type marker for "anything storable in a child
// slot": a leaf or any inner-node kind.
type node interface {
	isNode()
}

// innerNode is the polymorphic interface implemented by every
// inner-node type (chapter 7 has *node4, *node16, *node48,
// *node256 -- the full ART ladder). Every operation that used to
// do a type switch in chapter 4 now goes through one of these
// methods.
//
// The methods deliberately avoid V: the interface is type-erased
// so the same set of inner-node implementations can serve any
// Tree[V]. Only leaf[V] and Tree[V] carry the value type.
//
// What's *not* on the interface is also a design choice: there is
// no `numChildren()` method, because no consumer calls it.
// reshape's per-type collapse rules need the count, but reshape is
// a method on each concrete type and accesses its own field. The
// interface stays narrower; chapter 6 / 7's drop-ins are simpler.
type innerNode interface {
	node
	getPrefix() []byte
	setPrefix(p []byte)
	getTerminal() node
	setTerminal(t node)
	findChild(b byte) node
	addOrGrowChild(b byte, child node) innerNode
	replaceChild(b byte, child node)
	removeChild(b byte)
	eachAscending(yield func(byte, node) bool) bool
	reshape() node
}

// inlineKeyMax is the longest key copied into the leaf's inline
// buffer. 24 bytes is a sweet spot: it covers most short keys
// (UUIDs, fixed-width ints, small enum strings) while keeping the
// leaf struct under a 64-byte cache line for V == int.
const inlineKeyMax = 24

// leaf stores a (key, value) pair. The key is always a copy of
// what the caller passed to Put; mutating the caller's slice
// afterward never changes anything in the tree. For keys up to
// inlineKeyMax bytes, the copy lives in the leaf's own `inline`
// array and `key` points at a sub-slice -- one allocation total
// instead of two.
type leaf[V any] struct {
	key    []byte
	value  V
	inline [inlineKeyMax]byte
}

func (*leaf[V]) isNode() {}

// newLeaf returns a freshly-allocated leaf carrying a copy of key
// and value. Short keys live in the leaf's inline buffer; long
// keys are heap-copied. Either way, callers may mutate or reuse
// their original slice safely.
func newLeaf[V any](key []byte, value V) *leaf[V] {
	l := &leaf[V]{value: value}
	if len(key) <= inlineKeyMax {
		n := copy(l.inline[:], key)
		l.key = l.inline[:n]
	} else {
		l.key = append([]byte(nil), key...)
	}
	return l
}

// innerHeader holds the (prefix, terminal) state shared by every
// inner-node type. It is embedded by value at the top of node4,
// node16, node48, and node256 so the four trivial getter/setter
// methods on the innerNode interface are satisfied by Go's
// method-promotion rules. Without this, each of the four node
// types repeats the same four method bodies (16 total) -- the
// embedding deletes them.
type innerHeader struct {
	prefix   []byte
	terminal node
}

func (h *innerHeader) getPrefix() []byte  { return h.prefix }
func (h *innerHeader) setPrefix(p []byte) { h.prefix = p }
func (h *innerHeader) getTerminal() node  { return h.terminal }
func (h *innerHeader) setTerminal(t node) { h.terminal = t }

// node4 is the small inner node from chapter 4. Stores up to 4
// branching children in a sorted array; promotes to node16 on the
// fifth child.
type node4[V any] struct {
	innerHeader
	keys        [4]byte
	children    [4]node
	numChildren uint8
}

const node4Capacity = 4

func (*node4[V]) isNode() {}

func (n *node4[V]) findChild(b byte) node {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}

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

func (n *node4[V]) addOrGrowChild(b byte, child node) innerNode {
	if n.numChildren < node4Capacity {
		n.addChild(b, child)
		return n
	}
	grown := growToNode16[V](n)
	grown.addChild(b, child)
	return grown
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

func (n *node4[V]) eachAscending(yield func(byte, node) bool) bool {
	for i := uint8(0); i < n.numChildren; i++ {
		if !yield(n.keys[i], n.children[i]) {
			return false
		}
	}
	return true
}

// reshape applies the post-Delete collapse rules to a node4. The
// type-specific logic that used to live in chapter 4's free function
// is now a method on each inner-node type.
func (n *node4[V]) reshape() node {
	if n.numChildren == 0 {
		return collapseEmpty(n.terminal)
	}
	if n.numChildren == 1 && n.terminal == nil {
		only := n.children[0]
		if l, ok := only.(*leaf[V]); ok {
			return l
		}
		return mergePrefixIntoChild(n.prefix, n.keys[0], only.(innerNode))
	}
	return n
}

// node256 is the full-fanout inner node.
type node256[V any] struct {
	innerHeader
	children    [256]node
	numChildren uint16
}

func (*node256[V]) isNode() {}

func (n *node256[V]) findChild(b byte) node { return n.children[b] }

func (n *node256[V]) addOrGrowChild(b byte, child node) innerNode {
	n.children[b] = child
	n.numChildren++
	return n
}

func (n *node256[V]) replaceChild(b byte, child node) {
	n.children[b] = child
}

func (n *node256[V]) removeChild(b byte) {
	if n.children[b] != nil {
		n.children[b] = nil
		n.numChildren--
	}
}

func (n *node256[V]) eachAscending(yield func(byte, node) bool) bool {
	for b := 0; b < 256; b++ {
		if n.children[b] != nil {
			if !yield(byte(b), n.children[b]) {
				return false
			}
		}
	}
	return true
}

func (n *node256[V]) reshape() node {
	if n.numChildren == 0 {
		return collapseEmpty(n.terminal)
	}
	if n.numChildren == 1 && n.terminal == nil {
		var only node
		var onlyByte byte
		for b := 0; b < 256; b++ {
			if n.children[b] != nil {
				only = n.children[b]
				onlyByte = byte(b)
				break
			}
		}
		if l, ok := only.(*leaf[V]); ok {
			return l
		}
		return mergePrefixIntoChild(n.prefix, onlyByte, only.(innerNode))
	}
	if n.numChildren <= node48Capacity {
		return shrinkToNode48[V](n)
	}
	return n
}

// growToNode256 returns a node256 holding the same prefix,
// terminal, and children as the supplied node48. node256 indexes
// directly by edge byte, so the densely-packed children array is
// re-projected into the 256-slot array.
func growToNode256[V any](n *node48[V]) *node256[V] {
	grown := &node256[V]{
		innerHeader: n.innerHeader,
		numChildren: uint16(n.numChildren),
	}
	for i := uint8(0); i < n.numChildren; i++ {
		grown.children[n.childEdge[i]] = n.children[i]
	}
	return grown
}

// shrinkToNode4 returns a node4 holding the same prefix, terminal,
// and children as the supplied node16. Caller guarantees count <=
// node4Capacity. Both n.keys and shrunk.keys are sorted ascending,
// so the copy preserves the sort.
func shrinkToNode4[V any](n *node16[V]) *node4[V] {
	shrunk := &node4[V]{
		innerHeader: n.innerHeader,
		numChildren: n.numChildren,
	}
	for i := uint8(0); i < n.numChildren; i++ {
		shrunk.keys[i] = n.keys[i]
		shrunk.children[i] = n.children[i]
	}
	return shrunk
}

// node16 is the new chapter-6 inner-node type. It mirrors node4's
// shape (sorted keys array + parallel children array) at a larger
// capacity. Stage 5's node4 implementation reads as the template
// for everything below: the differences are the array sizes (16
// instead of 4), the capacity constant, and the demote/promote
// targets in addOrGrowChild and reshape.
type node16[V any] struct {
	innerHeader
	keys        [node16Capacity]byte
	children    [node16Capacity]node
	numChildren uint8
}

const node16Capacity = 16

func (*node16[V]) isNode() {}

func (n *node16[V]) findChild(b byte) node {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}

func (n *node16[V]) addChild(b byte, child node) {
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

func (n *node16[V]) addOrGrowChild(b byte, child node) innerNode {
	if n.numChildren < node16Capacity {
		n.addChild(b, child)
		return n
	}
	grown := growToNode48[V](n)
	grown.addChild(b, child)
	return grown
}

func (n *node16[V]) replaceChild(b byte, child node) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			n.children[i] = child
			return
		}
	}
}

func (n *node16[V]) removeChild(b byte) {
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

func (n *node16[V]) eachAscending(yield func(byte, node) bool) bool {
	for i := uint8(0); i < n.numChildren; i++ {
		if !yield(n.keys[i], n.children[i]) {
			return false
		}
	}
	return true
}

// reshape on a node16 has both ends of the ladder: at zero
// children it collapses, at exactly one inner-or-leaf child with
// no terminal it hoists or merges, and at <= 4 children it
// demotes to node4.
func (n *node16[V]) reshape() node {
	if n.numChildren == 0 {
		return collapseEmpty(n.terminal)
	}
	if n.numChildren == 1 && n.terminal == nil {
		only := n.children[0]
		if l, ok := only.(*leaf[V]); ok {
			return l
		}
		return mergePrefixIntoChild(n.prefix, n.keys[0], only.(innerNode))
	}
	if n.numChildren <= node4Capacity {
		return shrinkToNode4[V](n)
	}
	return n
}

// growToNode16 promotes a full node4 into a fresh node16,
// preserving the sorted-keys invariant by copying in order.
func growToNode16[V any](n *node4[V]) *node16[V] {
	grown := &node16[V]{
		innerHeader: n.innerHeader,
		numChildren: n.numChildren,
	}
	for i := uint8(0); i < n.numChildren; i++ {
		grown.keys[i] = n.keys[i]
		grown.children[i] = n.children[i]
	}
	return grown
}

// shrinkToNode16 demotes a node48 into a node16. Walks the
// node48's childIndex in ascending byte order so the resulting
// node16's keys array is sorted automatically. Caller guarantees
// n.numChildren <= node16Capacity.
func shrinkToNode16[V any](n *node48[V]) *node16[V] {
	shrunk := &node16[V]{
		innerHeader: n.innerHeader,
		numChildren: uint8(n.numChildren),
	}
	i := uint8(0)
	for b := 0; b < 256; b++ {
		slot := n.childIndex[byte(b)]
		if slot == 0 {
			continue
		}
		shrunk.keys[i] = byte(b)
		shrunk.children[i] = n.children[slot-1]
		i++
	}
	return shrunk
}

// node48 is the new chapter-7 inner-node type. It fills the
// 17–48 fanout band with a different layout from its neighbours:
//
//   - childIndex[b] is a 1-based slot into children where the
//     subtree under edge byte b lives (0 means "absent"). Zero is
//     the sentinel for absence so the array can be implicitly
//     zero-initialised.
//   - children[:numChildren] is the densely-packed child array.
//     addChild appends; removeChild swaps the last live entry into
//     the freed slot to keep the prefix dense.
//   - childEdge[i] is the edge byte that children[i] holds.
//     Needed by removeChild when it swaps slots: after the swap,
//     childIndex for the swapped-in entry must be updated, and
//     childEdge tells us which entry that was.
//
// findChild is O(1): one childIndex lookup. eachAscending walks
// childIndex in 0..255 order, yielding only occupied slots --
// linear in 256 but no key compares.
type node48[V any] struct {
	innerHeader
	childIndex  [256]byte
	children    [node48Capacity]node
	childEdge   [node48Capacity]byte
	numChildren uint8
}

const node48Capacity = 48

func (*node48[V]) isNode() {}

func (n *node48[V]) findChild(b byte) node {
	slot := n.childIndex[b]
	if slot == 0 {
		return nil
	}
	return n.children[slot-1]
}

// addChild places child at the next free slot, recording the slot
// in childIndex (1-based) and the edge byte in childEdge.
func (n *node48[V]) addChild(b byte, child node) {
	n.children[n.numChildren] = child
	n.childEdge[n.numChildren] = b
	n.childIndex[b] = n.numChildren + 1
	n.numChildren++
}

func (n *node48[V]) addOrGrowChild(b byte, child node) innerNode {
	if n.numChildren < node48Capacity {
		n.addChild(b, child)
		return n
	}
	grown := growToNode256[V](n)
	grown.children[b] = child
	grown.numChildren++
	return grown
}

func (n *node48[V]) replaceChild(b byte, child node) {
	slot := n.childIndex[b]
	if slot == 0 {
		return
	}
	n.children[slot-1] = child
}

// removeChild keeps children[:numChildren] dense by swapping the
// last live child into the vacated slot and updating its
// childIndex entry. childEdge tells us which edge byte the
// swapped-in child carried so we can rewrite that entry.
func (n *node48[V]) removeChild(b byte) {
	slot := n.childIndex[b]
	if slot == 0 {
		return
	}
	last := n.numChildren
	if slot != last {
		lastEdge := n.childEdge[last-1]
		n.children[slot-1] = n.children[last-1]
		n.childEdge[slot-1] = lastEdge
		n.childIndex[lastEdge] = slot
	}
	n.children[last-1] = nil
	n.childEdge[last-1] = 0
	n.childIndex[b] = 0
	n.numChildren--
}

// eachAscending yields children in ascending edge-byte order by
// scanning childIndex from 0 to 255.
func (n *node48[V]) eachAscending(yield func(byte, node) bool) bool {
	for b := 0; b < 256; b++ {
		slot := n.childIndex[byte(b)]
		if slot == 0 {
			continue
		}
		if !yield(byte(b), n.children[slot-1]) {
			return false
		}
	}
	return true
}

func (n *node48[V]) reshape() node {
	if n.numChildren == 0 {
		return collapseEmpty(n.terminal)
	}
	if n.numChildren == 1 && n.terminal == nil {
		// Find the single edge byte by scanning childIndex.
		var only node
		var onlyByte byte
		for b := 0; b < 256; b++ {
			if slot := n.childIndex[byte(b)]; slot != 0 {
				only = n.children[slot-1]
				onlyByte = byte(b)
				break
			}
		}
		if l, ok := only.(*leaf[V]); ok {
			return l
		}
		return mergePrefixIntoChild(n.prefix, onlyByte, only.(innerNode))
	}
	if n.numChildren <= node16Capacity {
		return shrinkToNode16[V](n)
	}
	return n
}

// growToNode48 promotes a full node16 into a fresh node48.
// node16 stores keys sorted in keys[]; node48 indexes by edge
// byte. The conversion populates childIndex (1-based) from each
// of the 16 sorted entries.
func growToNode48[V any](n *node16[V]) *node48[V] {
	grown := &node48[V]{
		innerHeader: n.innerHeader,
		numChildren: n.numChildren,
	}
	for i := uint8(0); i < n.numChildren; i++ {
		grown.children[i] = n.children[i]
		grown.childEdge[i] = n.keys[i]
		grown.childIndex[n.keys[i]] = i + 1
	}
	return grown
}

// shrinkToNode48 demotes a node256 into a node48. Walks
// children[0..255] in ascending order, packing into the 48-slot
// dense array and recording slot indices in childIndex.
// Caller guarantees n.numChildren <= node48Capacity.
func shrinkToNode48[V any](n *node256[V]) *node48[V] {
	shrunk := &node48[V]{
		innerHeader: n.innerHeader,
		numChildren: uint8(n.numChildren),
	}
	slot := uint8(0)
	for b := 0; b < 256; b++ {
		if n.children[b] == nil {
			continue
		}
		shrunk.children[slot] = n.children[b]
		shrunk.childEdge[slot] = byte(b)
		shrunk.childIndex[b] = slot + 1
		slot++
	}
	return shrunk
}

// collapseEmpty is the shared "no children" case for reshape: when
// an inner node has no branching children it is replaced by its
// terminal leaf (if any) or dropped from the tree.
func collapseEmpty(terminal node) node {
	if terminal != nil {
		return terminal
	}
	return nil
}

// mergePrefixIntoChild rewrites child's prefix to parentPrefix ||
// branchByte || child's old prefix and returns child for use as
// the replacement of its collapsed parent.
func mergePrefixIntoChild(parentPrefix []byte, branchByte byte, child innerNode) node {
	old := child.getPrefix()
	merged := make([]byte, 0, len(parentPrefix)+1+len(old))
	merged = append(merged, parentPrefix...)
	merged = append(merged, branchByte)
	merged = append(merged, old...)
	child.setPrefix(merged)
	return child
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

// consumePrefix matches a node's prefix against key[depth:]. The
// length-zero short-circuit avoids a bytes.Equal call (and slice-
// header construction) for empty prefixes; same idiom as production.
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

// Tree is the public sorted map.
type Tree[V any] struct {
	root node
	size int
}

func New[V any]() *Tree[V]  { return &Tree[V]{} }
func (t *Tree[V]) Len() int { return t.size }

// ---- Put --------------------------------------------------------------------

func (t *Tree[V]) Put(key []byte, value V) {
	t.root = putInto[V](t.root, key, value, 0, &t.size)
}

func putInto[V any](current node, key []byte, value V, depth int, size *int) node {
	if current == nil {
		*size++
		return newLeaf[V](key, value)
	}
	if l, ok := current.(*leaf[V]); ok {
		if bytes.Equal(l.key, key) {
			l.value = value
			return l
		}
		return splitTwoLeaves(l, key, value, depth, size)
	}
	n := current.(innerNode)
	prefix := n.getPrefix()
	common := longestCommonPrefix(prefix, key[depth:])
	if common < len(prefix) {
		return splitPrefixedNode[V](n, key, value, depth, common, size)
	}
	depth += common
	if depth == len(key) {
		if t := n.getTerminal(); t != nil {
			t.(*leaf[V]).value = value
			return n
		}
		*size++
		n.setTerminal(newLeaf[V](key, value))
		return n
	}
	b := key[depth]
	child := n.findChild(b)
	if child == nil {
		*size++
		return n.addOrGrowChild(b, newLeaf[V](key, value))
	}
	newChild := putInto[V](child, key, value, depth+1, size)
	if newChild != child {
		n.replaceChild(b, newChild)
	}
	return n
}

func splitTwoLeaves[V any](existing *leaf[V], newKey []byte, newValue V, depth int, size *int) node {
	a := existing.key[depth:]
	b := newKey[depth:]
	diverge := longestCommonPrefix(a, b)

	parent := &node4[V]{innerHeader: innerHeader{prefix: append([]byte(nil), a[:diverge]...)}}

	*size++
	nl := newLeaf[V](newKey, newValue)
	cut := depth + diverge
	switch {
	case cut == len(existing.key):
		parent.terminal = existing
		parent.addChild(newKey[cut], nl)
	case cut == len(newKey):
		parent.terminal = nl
		parent.addChild(existing.key[cut], existing)
	default:
		parent.addChild(existing.key[cut], existing)
		parent.addChild(newKey[cut], nl)
	}
	return parent
}

func splitPrefixedNode[V any](n innerNode, key []byte, value V, depth, common int, size *int) node {
	oldPrefix := n.getPrefix()
	sharedPrefix := append([]byte(nil), oldPrefix[:common]...)
	oldBranch := oldPrefix[common]
	n.setPrefix(append([]byte(nil), oldPrefix[common+1:]...))

	parent := &node4[V]{innerHeader: innerHeader{prefix: sharedPrefix}}
	parent.addChild(oldBranch, n)

	*size++
	nl := newLeaf[V](key, value)
	cut := depth + common
	if cut == len(key) {
		parent.terminal = nl
	} else {
		parent.addChild(key[cut], nl)
	}
	return parent
}

// ---- Get --------------------------------------------------------------------

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
		n := current.(innerNode)
		next, ok := consumePrefix(n.getPrefix(), key, depth)
		if !ok {
			return zero, false
		}
		depth = next
		if depth == len(key) {
			term := n.getTerminal()
			if term == nil {
				return zero, false
			}
			return term.(*leaf[V]).value, true
		}
		current = n.findChild(key[depth])
		depth++
	}
	return zero, false
}

// ---- Delete -----------------------------------------------------------------

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
	n := current.(innerNode)
	next, ok := consumePrefix(n.getPrefix(), key, depth)
	if !ok {
		return current, false
	}
	depth = next
	if depth == len(key) {
		term, _ := n.getTerminal().(*leaf[V])
		if term == nil || !bytes.Equal(term.key, key) {
			return current, false
		}
		n.setTerminal(nil)
		*size--
		return n.reshape(), true
	}
	b := key[depth]
	child := n.findChild(b)
	if child == nil {
		return current, false
	}
	newChild, deleted := deleteFrom[V](child, key, depth+1, size)
	if !deleted {
		return current, false
	}
	if newChild == nil {
		n.removeChild(b)
	} else {
		n.replaceChild(b, newChild)
	}
	return n.reshape(), true
}

// ---- All --------------------------------------------------------------------

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
	r := n.(innerNode)
	if term, ok := r.getTerminal().(*leaf[V]); ok {
		if !yield(term.key, term.value) {
			return false
		}
	}
	return r.eachAscending(func(_ byte, c node) bool {
		return iterate[V](c, yield)
	})
}

// ---- Range ------------------------------------------------------------------

// Range yields every (key, value) pair whose key lies in the
// half-open interval [start, end), in ascending byte-wise key
// order. A nil bound is unbounded on that side.
//
// The implementation prunes whole subtrees whose key range falls
// entirely outside [start, end). Pruning needs the byte path
// consumed from the root to the current node, and we maintain it
// in a single reusable buffer that grows on the way down and
// shrinks on the way back up. Result: zero allocations per yielded
// key, which is the polish that justifies this chapter's
// existence.
func (t *Tree[V]) Range(start, end []byte) iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if t.root == nil {
			return
		}
		path := make([]byte, 0, 32)
		iterateRange(t.root, &path, start, end, yield)
	}
}

func iterateRange[V any](n node, path *[]byte, start, end []byte, yield func([]byte, V) bool) bool {
	if l, ok := n.(*leaf[V]); ok {
		if keyInRange(l.key, start, end) {
			return yield(l.key, l.value)
		}
		return true
	}
	r := n.(innerNode)
	before := len(*path)
	*path = append(*path, r.getPrefix()...)
	nodeLen := len(*path)
	if term, ok := r.getTerminal().(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
		if !yield(term.key, term.value) {
			*path = (*path)[:before]
			return false
		}
	}
	cont := r.eachAscending(func(b byte, child node) bool {
		// Skip subtrees whose entire byte range falls below start
		// or above end without ever materialising their full path.
		if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
			return true
		}
		if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
			return false
		}
		*path = append((*path)[:nodeLen], b)
		return iterateRange[V](child, path, start, end, yield)
	})
	*path = (*path)[:before]
	return cont
}

// keyInRange reports whether key lies in [start, end), where a
// nil bound imposes no constraint on its side.
func keyInRange(key, start, end []byte) bool {
	if start != nil && bytes.Compare(key, start) < 0 {
		return false
	}
	if end != nil && bytes.Compare(key, end) >= 0 {
		return false
	}
	return true
}

// subtreeBeforeWithByte reports whether every key beginning with
// (nodePath ++ extra) is strictly less than bound. Equivalent to
// subtreeBefore(concatByte(nodePath, extra), bound) but does not
// allocate.
func subtreeBeforeWithByte(nodePath []byte, extra byte, bound []byte) bool {
	if bound == nil {
		return false
	}
	if len(bound) <= len(nodePath) {
		return bytes.Compare(nodePath[:len(bound)], bound) < 0
	}
	c := bytes.Compare(nodePath, bound[:len(nodePath)])
	if c != 0 {
		return c < 0
	}
	return extra < bound[len(nodePath)]
}

// subtreeAtOrAfterWithByte reports whether every key beginning
// with (nodePath ++ extra) is greater than or equal to bound.
func subtreeAtOrAfterWithByte(nodePath []byte, extra byte, bound []byte) bool {
	if bound == nil {
		return false
	}
	if len(bound) <= len(nodePath) {
		return bytes.Compare(nodePath[:len(bound)], bound) >= 0
	}
	c := bytes.Compare(nodePath, bound[:len(nodePath)])
	if c != 0 {
		return c > 0
	}
	bb := bound[len(nodePath)]
	if extra != bb {
		return extra > bb
	}
	return len(bound) <= len(nodePath)+1
}

// ---- footprint / count helpers ----------------------------------------------

func (t *Tree[V]) CountInner() int {
	c4, c16, c48, c256 := countByKind[V](t.root)
	return c4 + c16 + c48 + c256
}

// CountByKind reports how many of each inner-node type are live.
// The bench addendum prints this so the per-stage table can show
// the inner-node mix shifting as the ladder fills out.
func (t *Tree[V]) CountByKind() (n4, n16, n48, n256 int) {
	return countByKind[V](t.root)
}

func countByKind[V any](n node) (n4, n16, n48, n256 int) {
	if n == nil {
		return
	}
	if _, ok := n.(*leaf[V]); ok {
		return
	}
	r := n.(innerNode)
	switch r.(type) {
	case *node4[V]:
		n4 = 1
	case *node16[V]:
		n16 = 1
	case *node48[V]:
		n48 = 1
	case *node256[V]:
		n256 = 1
	}
	r.eachAscending(func(_ byte, c node) bool {
		a, b, c48Sub, d := countByKind[V](c)
		n4 += a
		n16 += b
		n48 += c48Sub
		n256 += d
		return true
	})
	return
}

func (t *Tree[V]) CountLeaves() int { return countLeaves[V](t.root) }

func countLeaves[V any](n node) int {
	if n == nil {
		return 0
	}
	if _, ok := n.(*leaf[V]); ok {
		return 1
	}
	r := n.(innerNode)
	count := 0
	if r.getTerminal() != nil {
		count++
	}
	r.eachAscending(func(_ byte, c node) bool {
		count += countLeaves[V](c)
		return true
	})
	return count
}

func (t *Tree[V]) PrefixBytes() int { return prefixBytes[V](t.root) }

func prefixBytes[V any](n node) int {
	if n == nil {
		return 0
	}
	if _, ok := n.(*leaf[V]); ok {
		return 0
	}
	r := n.(innerNode)
	total := len(r.getPrefix())
	r.eachAscending(func(_ byte, c node) bool {
		total += prefixBytes[V](c)
		return true
	})
	return total
}
