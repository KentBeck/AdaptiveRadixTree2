package art

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

type node[V any] interface {
	kind() nodeKind
}

// innerNode is the interface satisfied by every non-leaf node. It
// exposes the subset of operations used by Tree.Delete so the caller
// can act uniformly across node4/16/48/256.
type innerNode[V any] interface {
	node[V]
	findChild(b byte) node[V]
	removeChild(b byte)
}

const inlineKeyMax = 24

type leaf[V any] struct {
	key    []byte
	value  V
	inline [inlineKeyMax]byte
}

func (*leaf[V]) kind() nodeKind { return kindLeaf }

// node4 keeps keys[:numChildren] sorted ascending by edge byte. The
// prefix is consumed from the search key before branching. terminal,
// when non-nil, holds the value stored at this node's exact path (a
// key that ends after the prefix and does not branch further).
type node4[V any] struct {
	prefix      []byte
	keys        [4]byte
	children    [4]node[V]
	terminal    *leaf[V]
	numChildren uint8
}

func (*node4[V]) kind() nodeKind { return kindNode4 }

func (n *node4[V]) findChild(b byte) node[V] {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}

func (n *node4[V]) addChild(b byte, child node[V]) {
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
func (n *node4[V]) replaceChild(b byte, child node[V]) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			n.children[i] = child
			return
		}
	}
}

// removeChild removes the child stored under edge byte b, preserving
// the sorted order of the remaining keys. A no-op if b is absent.
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

// node16 keeps keys[:numChildren] sorted ascending by edge byte. Like
// node4, prefix is consumed from the search key before branching and
// terminal (when non-nil) holds the value stored at this node's exact
// path.
type node16[V any] struct {
	prefix      []byte
	keys        [node16Capacity]byte
	children    [node16Capacity]node[V]
	terminal    *leaf[V]
	numChildren uint8
}

func (*node16[V]) kind() nodeKind { return kindNode16 }

func (n *node16[V]) findChild(b byte) node[V] {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}

// insertChild inserts child under edge byte b. Caller guarantees b is
// not already present and that the node is not yet full.
func (n *node16[V]) insertChild(b byte, child node[V]) {
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
func (n *node16[V]) replaceChild(b byte, child node[V]) {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			n.children[i] = child
			return
		}
	}
}

// removeChild removes the child stored under edge byte b, preserving
// the sorted order of the remaining keys. A no-op if b is absent.
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

// growToNode16 returns a node16 holding the same sorted children,
// prefix, and terminal as n.
func growToNode16[V any](n *node4[V]) *node16[V] {
	grown := &node16[V]{
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
func shrinkToNode4[V any](n *node16[V]) *node4[V] {
	shrunk := &node4[V]{
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
type node48[V any] struct {
	prefix      []byte
	childIndex  [256]byte
	children    [node48Capacity]node[V]
	childEdge   [node48Capacity]byte
	terminal    *leaf[V]
	numChildren uint8
}

func (*node48[V]) kind() nodeKind { return kindNode48 }

func (n *node48[V]) findChild(b byte) node[V] {
	slot := n.childIndex[b]
	if slot == 0 {
		return nil
	}
	return n.children[slot-1]
}

func (n *node48[V]) addChild(newEdge byte, child node[V]) {
	n.children[n.numChildren] = child
	n.childEdge[n.numChildren] = newEdge
	n.childIndex[newEdge] = n.numChildren + 1
	n.numChildren++
}

// replaceChild swaps the child stored under edge byte b. Caller
// guarantees b is already present.
func (n *node48[V]) replaceChild(b byte, child node[V]) {
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

// growToNode48 returns a node48 holding the same children, prefix, and
// terminal as n, with childIndex populated from n's sorted edge bytes.
func growToNode48[V any](n *node16[V]) *node48[V] {
	grown := &node48[V]{
		prefix:      n.prefix,
		terminal:    n.terminal,
		numChildren: n.numChildren,
	}
	for i := uint8(0); i < n.numChildren; i++ {
		grown.children[i] = n.children[i]
		grown.childEdge[i] = n.keys[i]
		grown.childIndex[n.keys[i]] = i + 1
	}
	return grown
}

// shrinkToNode16 returns a node16 holding the same children, prefix,
// and terminal as n, with keys populated in ascending edge-byte order
// so node16's sort invariant is preserved. Caller guarantees
// n.numChildren <= node16Capacity.
func shrinkToNode16[V any](n *node48[V]) *node16[V] {
	shrunk := &node16[V]{
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
type node256[V any] struct {
	prefix      []byte
	children    [node256Capacity]node[V]
	terminal    *leaf[V]
	numChildren uint16
}

func (*node256[V]) kind() nodeKind { return kindNode256 }

func (n *node256[V]) findChild(b byte) node[V] {
	return n.children[b]
}

func (n *node256[V]) addChild(b byte, child node[V]) {
	n.children[b] = child
	n.numChildren++
}

// replaceChild swaps the child stored under edge byte b. Caller
// guarantees b is already present.
func (n *node256[V]) replaceChild(b byte, child node[V]) {
	if n.children[b] == nil {
		return
	}
	n.children[b] = child
}

// removeChild removes the child stored under edge byte b. A no-op if
// b is absent.
func (n *node256[V]) removeChild(b byte) {
	if n.children[b] == nil {
		return
	}
	n.children[b] = nil
	n.numChildren--
}

// growToNode256 returns a node256 holding the same children, prefix,
// and terminal as n, indexed directly by edge byte.
func growToNode256[V any](n *node48[V]) *node256[V] {
	grown := &node256[V]{
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
func shrinkToNode48[V any](n *node256[V]) *node48[V] {
	shrunk := &node48[V]{
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
		shrunk.childEdge[slot] = byte(b)
		shrunk.childIndex[b] = slot + 1
		slot++
	}
	return shrunk
}

// Tree is a sorted map from []byte keys to V values, backed by an
// Adaptive Radix Tree.
//
// A Tree is not safe for concurrent use by multiple goroutines when
// any goroutine is writing. Callers that need concurrent access should
// guard a Tree with their own sync.RWMutex.
type Tree[V any] struct {
	root node[V]
	size int
}

// New returns an empty Tree.
func New[V any]() *Tree[V] {
	return &Tree[V]{}
}

// Len returns the number of key-value pairs in the tree. It runs in
// O(1).
func (t *Tree[V]) Len() int { return t.size }
