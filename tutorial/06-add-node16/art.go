// Package addnode16 is chapter 6 of the literate tutorial. It is
// chapter 5's payoff: adding a third inner-node type now lands as
// a new struct (and its eleven innerNode methods) plus two
// surgical edits -- node4.addOrGrowChild now grows to node16, and
// node256.reshape gains a demote-to-node16 case. Put, Get, Delete,
// All, splitTwoLeaves, splitPrefixedNode, consumePrefix,
// longestCommonPrefix, collapseEmpty, and mergePrefixIntoChild are
// unchanged from chapter 5.
//
// node16 fills the fanout band 5–16. Without it, a node4 that
// gained its fifth child jumped straight to node256 (a ~36x size
// jump). With it, the ladder steps 4 -> 16 -> 256, and many more
// inner nodes settle at a size that matches their actual fanout.
//
// The promote / demote thresholds:
//
//	node4    full at 4 children   -> grows to node16
//	node16   full at 16 children  -> grows to node256
//	node256  falls to <= 16       -> demotes to node16
//	node16   falls to <= 4        -> demotes to node4
//
// The chapter's measurable impact is on the URL workload, where
// many inner nodes have 6–10 branching children (host divergence
// = 6, path divergence ≈ 10). Those nodes were forced into
// node256 in chapter 5; in chapter 6 they settle into node16.
package addnode16

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
// inner-node type (chapter 6 has *node4, *node16, *node256;
// chapter 7 will add *node48). Every operation that used to do
// a type switch in chapter 4 now goes through one of these methods.
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

// leaf stores a (key, value) pair.
type leaf[V any] struct {
	key   []byte
	value V
}

func (*leaf[V]) isNode() {}

// node4 is the small inner node from chapter 4. Field types:
// terminal is now `node` (the interface), not `*leaf[V]`. Storing
// the terminal as `node` keeps the inner-node struct V-erased so
// it can implement innerNode without itself being parameterised
// by V. (At point-of-use the consuming code asserts to *leaf[V].)
type node4[V any] struct {
	prefix      []byte
	keys        [4]byte
	children    [4]node
	terminal    node
	numChildren uint8
}

const node4Capacity = 4

func (*node4[V]) isNode() {}

func (n *node4[V]) getPrefix() []byte  { return n.prefix }
func (n *node4[V]) setPrefix(p []byte) { n.prefix = p }
func (n *node4[V]) getTerminal() node  { return n.terminal }
func (n *node4[V]) setTerminal(t node) { n.terminal = t }

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
	prefix      []byte
	children    [256]node
	terminal    node
	numChildren uint16
}

func (*node256[V]) isNode() {}

func (n *node256[V]) getPrefix() []byte  { return n.prefix }
func (n *node256[V]) setPrefix(p []byte) { n.prefix = p }
func (n *node256[V]) getTerminal() node  { return n.terminal }
func (n *node256[V]) setTerminal(t node) { n.terminal = t }

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
	if n.numChildren <= node16Capacity {
		return shrinkToNode16[V](n)
	}
	return n
}

// growToNode256 returns a node256 holding the same prefix,
// terminal, and children as the supplied node16.
func growToNode256[V any](n *node16[V]) *node256[V] {
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
// and children as the supplied node16. Caller guarantees count <=
// node4Capacity. Both n.keys and shrunk.keys are sorted ascending,
// so the copy preserves the sort.
func shrinkToNode4[V any](n *node16[V]) *node4[V] {
	shrunk := &node4[V]{
		prefix:      n.prefix,
		terminal:    n.terminal,
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
	prefix      []byte
	keys        [node16Capacity]byte
	children    [node16Capacity]node
	terminal    node
	numChildren uint8
}

const node16Capacity = 16

func (*node16[V]) isNode() {}

func (n *node16[V]) getPrefix() []byte  { return n.prefix }
func (n *node16[V]) setPrefix(p []byte) { n.prefix = p }
func (n *node16[V]) getTerminal() node  { return n.terminal }
func (n *node16[V]) setTerminal(t node) { n.terminal = t }

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
	grown := growToNode256[V](n)
	grown.children[b] = child
	grown.numChildren++
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
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: n.numChildren,
	}
	for i := uint8(0); i < n.numChildren; i++ {
		grown.keys[i] = n.keys[i]
		grown.children[i] = n.children[i]
	}
	return grown
}

// shrinkToNode16 demotes a node256 into a node16. Walks
// children[0..255] in ascending order so the resulting node16's
// keys array is sorted automatically.
func shrinkToNode16[V any](n *node256[V]) *node16[V] {
	shrunk := &node16[V]{
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
		return &leaf[V]{key: append([]byte(nil), key...), value: value}
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
		n.setTerminal(&leaf[V]{key: append([]byte(nil), key...), value: value})
		return n
	}
	b := key[depth]
	child := n.findChild(b)
	if child == nil {
		*size++
		return n.addOrGrowChild(b, &leaf[V]{key: append([]byte(nil), key...), value: value})
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

func splitPrefixedNode[V any](n innerNode, key []byte, value V, depth, common int, size *int) node {
	oldPrefix := n.getPrefix()
	sharedPrefix := append([]byte(nil), oldPrefix[:common]...)
	oldBranch := oldPrefix[common]
	n.setPrefix(append([]byte(nil), oldPrefix[common+1:]...))

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

// ---- footprint / count helpers ----------------------------------------------

func (t *Tree[V]) CountInner() int {
	c4, c16, c256 := countByKind[V](t.root)
	return c4 + c16 + c256
}

// CountByKind reports how many of each inner-node type are live.
// The bench addendum prints this so the per-stage table can show
// the inner-node mix shifting as new sizes are added.
func (t *Tree[V]) CountByKind() (n4, n16, n256 int) {
	return countByKind[V](t.root)
}

func countByKind[V any](n node) (n4, n16, n256 int) {
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
	case *node256[V]:
		n256 = 1
	}
	r.eachAscending(func(_ byte, c node) bool {
		a, b, d := countByKind[V](c)
		n4 += a
		n16 += b
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
