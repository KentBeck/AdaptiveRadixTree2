// Package art is an Adaptive Radix Tree implementation.
//
// Keys are currently assumed to differ at byte 0. Path compression
// and lazy expansion will arrive in later slices.
//
// This file contains type definitions and node lifecycle (grow/shrink/addChild).
package art

import "bytes"

type nodeKind uint8

const (
	kindLeaf nodeKind = iota
	kindNode4
	kindNode16
	kindNode48
	kindNode256
)

const (
	node4Capacity   = 4
	node16Capacity  = 16
	node48Capacity  = 48
	node256Capacity = 256
)

type node interface {
	kind() nodeKind
}

// innerNode is implemented by all four inner node types (node4,
// node16, node48, node256) and defines the operations each must
// support. Tree.Put/Get/Delete/All dispatch through this interface so
// the per-node-type logic lives with each node rather than in a single
// switch.
type innerNode interface {
	node

	// Operations.
	put(key []byte, value any, depth int) (node, bool)
	get(key []byte, depth int) (any, bool)
	delete(key []byte, depth int) (node, bool)
	iterate(yield func([]byte, any) bool) bool

	// Structural.
	findChild(b byte) node
	removeChild(b byte)
	isEmpty() bool

	// Path compression accessors.
	getPrefix() []byte
	setPrefix([]byte)
	getTerminal() *leaf
	setTerminal(*leaf)
}

type leaf struct {
	key   []byte
	value any
}

func (*leaf) kind() nodeKind { return kindLeaf }

// node4 keeps keys[:numChildren] sorted ascending by edge byte. The
// prefix is consumed from the search key before branching. terminal,
// when non-nil, holds the value stored at this node's exact path (a
// key that ends after the prefix and does not branch further).
type node4 struct {
	prefix      []byte
	keys        [4]byte
	children    [4]node
	terminal    *leaf
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

// replaceChild swaps the child stored under edge byte b. Caller
// guarantees b is already present.
func (n *node4) replaceChild(b byte, child node) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			n.children[i] = child
			return
		}
	}
}

// removeChild removes the child stored under edge byte b, preserving
// the sorted order of the remaining keys. A no-op if b is absent.
func (n *node4) removeChild(b byte) {
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

func (n *node4) isEmpty() bool { return n.numChildren == 0 }

func (n *node4) getPrefix() []byte   { return n.prefix }
func (n *node4) setPrefix(p []byte)  { n.prefix = p }
func (n *node4) getTerminal() *leaf  { return n.terminal }
func (n *node4) setTerminal(t *leaf) { n.terminal = t }

// node16 keeps keys[:numChildren] sorted ascending by edge byte. Like
// node4, prefix is consumed from the search key before branching and
// terminal (when non-nil) holds the value stored at this node's exact
// path.
type node16 struct {
	prefix      []byte
	keys        [node16Capacity]byte
	children    [node16Capacity]node
	terminal    *leaf
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

// replaceChild swaps the child stored under edge byte b. Caller
// guarantees b is already present.
func (n *node16) replaceChild(b byte, child node) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			n.children[i] = child
			return
		}
	}
}

// removeChild removes the child stored under edge byte b, preserving
// the sorted order of the remaining keys. A no-op if b is absent.
func (n *node16) removeChild(b byte) {
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

func (n *node16) isEmpty() bool { return n.numChildren == 0 }

func (n *node16) getPrefix() []byte   { return n.prefix }
func (n *node16) setPrefix(p []byte)  { n.prefix = p }
func (n *node16) getTerminal() *leaf  { return n.terminal }
func (n *node16) setTerminal(t *leaf) { n.terminal = t }

// growToNode16 returns a node16 holding the same sorted children,
// prefix, and terminal as n.
func growToNode16(n *node4) *node16 {
	grown := &node16{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: n.numChildren,
	}
	copy(grown.keys[:n.numChildren], n.keys[:n.numChildren])
	copy(grown.children[:n.numChildren], n.children[:n.numChildren])
	return grown
}

// shrinkToNode4 returns a node4 holding the same sorted children,
// prefix, and terminal as n. Caller guarantees n.numChildren <=
// node4Capacity.
func shrinkToNode4(n *node16) *node4 {
	shrunk := &node4{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: n.numChildren,
	}
	copy(shrunk.keys[:n.numChildren], n.keys[:n.numChildren])
	copy(shrunk.children[:n.numChildren], n.children[:n.numChildren])
	return shrunk
}

// node48 maps edge bytes to children via a 256-entry index where a
// stored value of 0 means "absent" and any other value is a 1-based
// slot into children. Like the smaller inner nodes, prefix is consumed
// from the search key before branching and terminal (when non-nil)
// holds the value stored at this node's exact path.
type node48 struct {
	prefix      []byte
	childIndex  [256]byte
	children    [node48Capacity]node
	terminal    *leaf
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

// replaceChild swaps the child stored under edge byte b. Caller
// guarantees b is already present.
func (n *node48) replaceChild(b byte, child node) {
	slot := n.childIndex[b]
	if slot == 0 {
		return
	}
	n.children[slot-1] = child
}

// removeChild removes the child stored under edge byte b. To keep
// children[:numChildren] dense (which addChild relies on), the last
// live child is swapped into the vacated slot and its index entry is
// updated. A no-op if b is absent.
func (n *node48) removeChild(b byte) {
	slot := n.childIndex[b]
	if slot == 0 {
		return
	}
	last := n.numChildren
	if slot != last {
		for edge := 0; edge < 256; edge++ {
			if n.childIndex[byte(edge)] == last {
				n.children[slot-1] = n.children[last-1]
				n.childIndex[byte(edge)] = slot
				break
			}
		}
	}
	n.children[last-1] = nil
	n.childIndex[b] = 0
	n.numChildren--
}

func (n *node48) isEmpty() bool { return n.numChildren == 0 }

func (n *node48) getPrefix() []byte   { return n.prefix }
func (n *node48) setPrefix(p []byte)  { n.prefix = p }
func (n *node48) getTerminal() *leaf  { return n.terminal }
func (n *node48) setTerminal(t *leaf) { n.terminal = t }

// growToNode48 returns a node48 holding the same children, prefix, and
// terminal as n, with childIndex populated from n's sorted edge bytes.
func growToNode48(n *node16) *node48 {
	grown := &node48{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: n.numChildren,
	}
	for i := uint8(0); i < n.numChildren; i++ {
		grown.children[i] = n.children[i]
		grown.childIndex[n.keys[i]] = i + 1
	}
	return grown
}

// shrinkToNode16 returns a node16 holding the same children, prefix,
// and terminal as n, with keys populated in ascending edge-byte order
// so node16's sort invariant is preserved. Caller guarantees
// n.numChildren <= node16Capacity.
func shrinkToNode16(n *node48) *node16 {
	shrunk := &node16{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: n.numChildren,
	}
	i := uint8(0)
	for edge := 0; edge < 256; edge++ {
		slot := n.childIndex[byte(edge)]
		if slot == 0 {
			continue
		}
		shrunk.keys[i] = byte(edge)
		shrunk.children[i] = n.children[slot-1]
		i++
	}
	return shrunk
}

// node256 indexes children directly by edge byte; a nil slot means
// absent. numChildren tracks the count for fast emptiness checks.
// Like the smaller inner nodes, prefix is consumed from the search key
// before branching and terminal (when non-nil) holds the value stored
// at this node's exact path.
type node256 struct {
	prefix      []byte
	children    [node256Capacity]node
	terminal    *leaf
	numChildren uint16
}

func (*node256) kind() nodeKind { return kindNode256 }

func (n *node256) findChild(b byte) node {
	return n.children[b]
}

func (n *node256) addChild(b byte, child node) {
	n.children[b] = child
	n.numChildren++
}

// replaceChild swaps the child stored under edge byte b. Caller
// guarantees b is already present.
func (n *node256) replaceChild(b byte, child node) {
	if n.children[b] == nil {
		return
	}
	n.children[b] = child
}

// removeChild removes the child stored under edge byte b. A no-op if
// b is absent.
func (n *node256) removeChild(b byte) {
	if n.children[b] == nil {
		return
	}
	n.children[b] = nil
	n.numChildren--
}

func (n *node256) isEmpty() bool { return n.numChildren == 0 }

func (n *node256) getPrefix() []byte   { return n.prefix }
func (n *node256) setPrefix(p []byte)  { n.prefix = p }
func (n *node256) getTerminal() *leaf  { return n.terminal }
func (n *node256) setTerminal(t *leaf) { n.terminal = t }

// growToNode256 returns a node256 holding the same children, prefix,
// and terminal as n, indexed directly by edge byte.
func growToNode256(n *node48) *node256 {
	grown := &node256{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: uint16(n.numChildren),
	}
	for b := 0; b < 256; b++ {
		slot := n.childIndex[b]
		if slot != 0 {
			grown.children[b] = n.children[slot-1]
		}
	}
	return grown
}

// shrinkToNode48 returns a node48 holding the same children, prefix,
// and terminal as n, with childIndex populated from the occupied slots
// in n. Caller guarantees n.numChildren <= node48Capacity.
func shrinkToNode48(n *node256) *node48 {
	shrunk := &node48{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: uint8(n.numChildren),
	}
	slot := uint8(0)
	for b := 0; b < 256; b++ {
		if n.children[b] == nil {
			continue
		}
		shrunk.children[slot] = n.children[b]
		shrunk.childIndex[b] = slot + 1
		slot++
	}
	return shrunk
}

// --- node4 operations ---

// put writes (key, value) into n, handling the prefix-split, terminal,
// and branching-child cases. The shared prefix-match / terminal /
// exhausted-key logic is duplicated across all four inner node types;
// a future refactor may factor it out.
func (n *node4) put(key []byte, value any, depth int) (node, bool) {
	splitPoint := len(longestCommonPrefix(key[depth:], n.prefix))
	if splitPoint < len(n.prefix) {
		shared := n.prefix[:splitPoint]
		oldBranch := n.prefix[splitPoint]
		n.prefix = n.prefix[splitPoint+1:]
		return splitPrefixedInner(n, oldBranch, shared, key, value, depth, splitPoint), true
	}
	depth += len(n.prefix)
	if depth == len(key) {
		if n.terminal != nil {
			n.terminal.value = value
		} else {
			n.terminal = newLeaf(key, value)
		}
		return n, true
	}
	branch := key[depth]
	switch c := n.findChild(branch).(type) {
	case nil:
		return node4AddOrGrow(n, branch, newLeaf(key, value)), true
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return n, true
		}
		n.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return n, true
	case innerNode:
		newChild, _ := c.put(key, value, depth+1)
		n.replaceChild(branch, newChild)
		return n, true
	}
	return n, true
}

func (n *node4) get(key []byte, depth int) (any, bool) {
	end := depth + len(n.prefix)
	if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
		return nil, false
	}
	depth = end
	if depth == len(key) {
		if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
			return n.terminal.value, true
		}
		return nil, false
	}
	switch c := n.findChild(key[depth]).(type) {
	case nil:
		return nil, false
	case *leaf:
		if bytes.Equal(c.key, key) {
			return c.value, true
		}
		return nil, false
	case innerNode:
		return c.get(key, depth+1)
	}
	return nil, false
}

func (n *node4) delete(key []byte, depth int) (node, bool) {
	end := depth + len(n.prefix)
	if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
		return n, false
	}
	depth = end
	if depth == len(key) {
		if n.terminal == nil || !bytes.Equal(n.terminal.key, key) {
			return n, false
		}
		n.terminal = nil
		return postDeleteReshape(n), true
	}
	branch := key[depth]
	child := n.findChild(branch)
	if child == nil {
		return n, false
	}
	var (
		newChild node
		deleted  bool
	)
	switch c := child.(type) {
	case *leaf:
		if !bytes.Equal(c.key, key) {
			return n, false
		}
		newChild, deleted = nil, true
	case innerNode:
		newChild, deleted = c.delete(key, depth+1)
	}
	if !deleted {
		return n, false
	}
	if newChild == nil {
		n.removeChild(branch)
	} else {
		n.replaceChild(branch, newChild)
	}
	return postDeleteReshape(n), true
}

func (n *node4) iterate(yield func([]byte, any) bool) bool {
	if n.terminal != nil && !yield(n.terminal.key, n.terminal.value) {
		return false
	}
	for i := uint8(0); i < n.numChildren; i++ {
		switch c := n.children[i].(type) {
		case *leaf:
			if !yield(c.key, c.value) {
				return false
			}
		case innerNode:
			if !c.iterate(yield) {
				return false
			}
		}
	}
	return true
}

// --- node16 operations ---

func (n *node16) put(key []byte, value any, depth int) (node, bool) {
	splitPoint := len(longestCommonPrefix(key[depth:], n.prefix))
	if splitPoint < len(n.prefix) {
		shared := n.prefix[:splitPoint]
		oldBranch := n.prefix[splitPoint]
		n.prefix = n.prefix[splitPoint+1:]
		return splitPrefixedInner(n, oldBranch, shared, key, value, depth, splitPoint), true
	}
	depth += len(n.prefix)
	if depth == len(key) {
		if n.terminal != nil {
			n.terminal.value = value
		} else {
			n.terminal = newLeaf(key, value)
		}
		return n, true
	}
	branch := key[depth]
	switch c := n.findChild(branch).(type) {
	case nil:
		return node16AddOrGrow(n, branch, newLeaf(key, value)), true
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return n, true
		}
		n.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return n, true
	case innerNode:
		newChild, _ := c.put(key, value, depth+1)
		n.replaceChild(branch, newChild)
		return n, true
	}
	return n, true
}

func (n *node16) get(key []byte, depth int) (any, bool) {
	end := depth + len(n.prefix)
	if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
		return nil, false
	}
	depth = end
	if depth == len(key) {
		if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
			return n.terminal.value, true
		}
		return nil, false
	}
	switch c := n.findChild(key[depth]).(type) {
	case nil:
		return nil, false
	case *leaf:
		if bytes.Equal(c.key, key) {
			return c.value, true
		}
		return nil, false
	case innerNode:
		return c.get(key, depth+1)
	}
	return nil, false
}

func (n *node16) delete(key []byte, depth int) (node, bool) {
	end := depth + len(n.prefix)
	if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
		return n, false
	}
	depth = end
	if depth == len(key) {
		if n.terminal == nil || !bytes.Equal(n.terminal.key, key) {
			return n, false
		}
		n.terminal = nil
		return postDeleteReshape(n), true
	}
	branch := key[depth]
	child := n.findChild(branch)
	if child == nil {
		return n, false
	}
	var (
		newChild node
		deleted  bool
	)
	switch c := child.(type) {
	case *leaf:
		if !bytes.Equal(c.key, key) {
			return n, false
		}
		newChild, deleted = nil, true
	case innerNode:
		newChild, deleted = c.delete(key, depth+1)
	}
	if !deleted {
		return n, false
	}
	if newChild == nil {
		n.removeChild(branch)
	} else {
		n.replaceChild(branch, newChild)
	}
	return postDeleteReshape(n), true
}

func (n *node16) iterate(yield func([]byte, any) bool) bool {
	if n.terminal != nil && !yield(n.terminal.key, n.terminal.value) {
		return false
	}
	for i := uint8(0); i < n.numChildren; i++ {
		switch c := n.children[i].(type) {
		case *leaf:
			if !yield(c.key, c.value) {
				return false
			}
		case innerNode:
			if !c.iterate(yield) {
				return false
			}
		}
	}
	return true
}

// --- node48 operations ---

func (n *node48) put(key []byte, value any, depth int) (node, bool) {
	splitPoint := len(longestCommonPrefix(key[depth:], n.prefix))
	if splitPoint < len(n.prefix) {
		shared := n.prefix[:splitPoint]
		oldBranch := n.prefix[splitPoint]
		n.prefix = n.prefix[splitPoint+1:]
		return splitPrefixedInner(n, oldBranch, shared, key, value, depth, splitPoint), true
	}
	depth += len(n.prefix)
	if depth == len(key) {
		if n.terminal != nil {
			n.terminal.value = value
		} else {
			n.terminal = newLeaf(key, value)
		}
		return n, true
	}
	branch := key[depth]
	switch c := n.findChild(branch).(type) {
	case nil:
		return node48AddOrGrow(n, branch, newLeaf(key, value)), true
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return n, true
		}
		n.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return n, true
	case innerNode:
		newChild, _ := c.put(key, value, depth+1)
		n.replaceChild(branch, newChild)
		return n, true
	}
	return n, true
}

func (n *node48) get(key []byte, depth int) (any, bool) {
	end := depth + len(n.prefix)
	if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
		return nil, false
	}
	depth = end
	if depth == len(key) {
		if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
			return n.terminal.value, true
		}
		return nil, false
	}
	switch c := n.findChild(key[depth]).(type) {
	case nil:
		return nil, false
	case *leaf:
		if bytes.Equal(c.key, key) {
			return c.value, true
		}
		return nil, false
	case innerNode:
		return c.get(key, depth+1)
	}
	return nil, false
}

func (n *node48) delete(key []byte, depth int) (node, bool) {
	end := depth + len(n.prefix)
	if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
		return n, false
	}
	depth = end
	if depth == len(key) {
		if n.terminal == nil || !bytes.Equal(n.terminal.key, key) {
			return n, false
		}
		n.terminal = nil
		return postDeleteReshape(n), true
	}
	branch := key[depth]
	child := n.findChild(branch)
	if child == nil {
		return n, false
	}
	var (
		newChild node
		deleted  bool
	)
	switch c := child.(type) {
	case *leaf:
		if !bytes.Equal(c.key, key) {
			return n, false
		}
		newChild, deleted = nil, true
	case innerNode:
		newChild, deleted = c.delete(key, depth+1)
	}
	if !deleted {
		return n, false
	}
	if newChild == nil {
		n.removeChild(branch)
	} else {
		n.replaceChild(branch, newChild)
	}
	return postDeleteReshape(n), true
}

func (n *node48) iterate(yield func([]byte, any) bool) bool {
	if n.terminal != nil && !yield(n.terminal.key, n.terminal.value) {
		return false
	}
	for edge := 0; edge < 256; edge++ {
		slot := n.childIndex[byte(edge)]
		if slot == 0 {
			continue
		}
		switch c := n.children[slot-1].(type) {
		case *leaf:
			if !yield(c.key, c.value) {
				return false
			}
		case innerNode:
			if !c.iterate(yield) {
				return false
			}
		}
	}
	return true
}

// --- node256 operations ---

func (n *node256) put(key []byte, value any, depth int) (node, bool) {
	splitPoint := len(longestCommonPrefix(key[depth:], n.prefix))
	if splitPoint < len(n.prefix) {
		shared := n.prefix[:splitPoint]
		oldBranch := n.prefix[splitPoint]
		n.prefix = n.prefix[splitPoint+1:]
		return splitPrefixedInner(n, oldBranch, shared, key, value, depth, splitPoint), true
	}
	depth += len(n.prefix)
	if depth == len(key) {
		if n.terminal != nil {
			n.terminal.value = value
		} else {
			n.terminal = newLeaf(key, value)
		}
		return n, true
	}
	branch := key[depth]
	switch c := n.findChild(branch).(type) {
	case nil:
		n.addChild(branch, newLeaf(key, value))
		return n, true
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return n, true
		}
		n.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return n, true
	case innerNode:
		newChild, _ := c.put(key, value, depth+1)
		n.replaceChild(branch, newChild)
		return n, true
	}
	return n, true
}

func (n *node256) get(key []byte, depth int) (any, bool) {
	end := depth + len(n.prefix)
	if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
		return nil, false
	}
	depth = end
	if depth == len(key) {
		if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
			return n.terminal.value, true
		}
		return nil, false
	}
	switch c := n.findChild(key[depth]).(type) {
	case nil:
		return nil, false
	case *leaf:
		if bytes.Equal(c.key, key) {
			return c.value, true
		}
		return nil, false
	case innerNode:
		return c.get(key, depth+1)
	}
	return nil, false
}

func (n *node256) delete(key []byte, depth int) (node, bool) {
	end := depth + len(n.prefix)
	if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
		return n, false
	}
	depth = end
	if depth == len(key) {
		if n.terminal == nil || !bytes.Equal(n.terminal.key, key) {
			return n, false
		}
		n.terminal = nil
		return postDeleteReshape(n), true
	}
	branch := key[depth]
	child := n.findChild(branch)
	if child == nil {
		return n, false
	}
	var (
		newChild node
		deleted  bool
	)
	switch c := child.(type) {
	case *leaf:
		if !bytes.Equal(c.key, key) {
			return n, false
		}
		newChild, deleted = nil, true
	case innerNode:
		newChild, deleted = c.delete(key, depth+1)
	}
	if !deleted {
		return n, false
	}
	if newChild == nil {
		n.removeChild(branch)
	} else {
		n.replaceChild(branch, newChild)
	}
	return postDeleteReshape(n), true
}

func (n *node256) iterate(yield func([]byte, any) bool) bool {
	if n.terminal != nil && !yield(n.terminal.key, n.terminal.value) {
		return false
	}
	for edge := 0; edge < 256; edge++ {
		switch c := n.children[edge].(type) {
		case nil:
			continue
		case *leaf:
			if !yield(c.key, c.value) {
				return false
			}
		case innerNode:
			if !c.iterate(yield) {
				return false
			}
		}
	}
	return true
}

// Tree is a sorted map backed by an Adaptive Radix Tree.
type Tree struct {
	root node
}

// New returns an empty Tree.
func New() *Tree {
	return &Tree{}
}
