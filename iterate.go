// This file contains All and Range iteration logic across all node types.
package art

import (
	"bytes"
	"iter"
)

// All returns an iterator over every (key, value) pair in the tree in
// ascending byte-wise key order. Breaking out of the range stops the
// traversal immediately.
func (t *Tree) All() iter.Seq2[[]byte, any] {
	return func(yield func([]byte, any) bool) {
		iterate(t.root, yield)
	}
}

// iterate visits every (key, value) pair reachable from n in sorted
// key order, returning false as soon as yield does so the caller can
// short-circuit all the way up.
func iterate(n node, yield func([]byte, any) bool) bool {
	switch r := n.(type) {
	case nil:
		return true
	case *leaf:
		return yield(r.key, r.value)
	case *node4:
		if r.terminal != nil && !yield(r.terminal.key, r.terminal.value) {
			return false
		}
		for i := uint8(0); i < r.numChildren; i++ {
			if !iterate(r.children[i], yield) {
				return false
			}
		}
		return true
	case *node16:
		if r.terminal != nil && !yield(r.terminal.key, r.terminal.value) {
			return false
		}
		for i := uint8(0); i < r.numChildren; i++ {
			if !iterate(r.children[i], yield) {
				return false
			}
		}
		return true
	case *node48:
		if r.terminal != nil && !yield(r.terminal.key, r.terminal.value) {
			return false
		}
		for edge := 0; edge < 256; edge++ {
			slot := r.childIndex[byte(edge)]
			if slot == 0 {
				continue
			}
			if !iterate(r.children[slot-1], yield) {
				return false
			}
		}
		return true
	case *node256:
		if r.terminal != nil && !yield(r.terminal.key, r.terminal.value) {
			return false
		}
		for edge := 0; edge < 256; edge++ {
			child := r.children[edge]
			if child == nil {
				continue
			}
			if !iterate(child, yield) {
				return false
			}
		}
		return true
	}
	return true
}

// Range returns an iterator over every (key, value) pair whose key
// lies in the half-open interval [start, end), in ascending byte-wise
// key order. A nil bound is treated as unbounded on that side, so
// Range(nil, nil) is equivalent to All. Breaking out of the range
// stops the traversal immediately.
func (t *Tree) Range(start, end []byte) iter.Seq2[[]byte, any] {
	return func(yield func([]byte, any) bool) {
		if start != nil && end != nil && bytes.Compare(start, end) >= 0 {
			return
		}
		path := make([]byte, 0, 32)
		iterateRange(t.root, &path, start, end, yield)
	}
}

// iterateRange is the pruning analogue of iterate. *path is the byte
// sequence consumed from the root to n (before n's own prefix); the
// same backing buffer is reused across the recursion and every exit
// path restores *path to its length on entry. At each inner node the
// terminal is yielded first, then child edges are visited in
// ascending order; a subtree whose keys all fall outside [start, end)
// is skipped without materializing its path, and because edges are
// sorted, the first edge whose subtree is at-or-after end ends the
// walk of this node.
func iterateRange(n node, path *[]byte, start, end []byte, yield func([]byte, any) bool) bool {
	switch r := n.(type) {
	case nil:
		return true
	case *leaf:
		if keyInRange(r.key, start, end) {
			return yield(r.key, r.value)
		}
		return true
	case *node4:
		before := len(*path)
		*path = append(*path, r.prefix...)
		nodeLen := len(*path)
		if r.terminal != nil && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(r.terminal.key, r.terminal.value) {
				*path = (*path)[:before]
				return false
			}
		}
		for i := uint8(0); i < r.numChildren; i++ {
			b := r.keys[i]
			if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
				continue
			}
			if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
				*path = (*path)[:before]
				return true
			}
			*path = append((*path)[:nodeLen], b)
			if !iterateRange(r.children[i], path, start, end, yield) {
				*path = (*path)[:before]
				return false
			}
		}
		*path = (*path)[:before]
		return true
	case *node16:
		before := len(*path)
		*path = append(*path, r.prefix...)
		nodeLen := len(*path)
		if r.terminal != nil && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(r.terminal.key, r.terminal.value) {
				*path = (*path)[:before]
				return false
			}
		}
		for i := uint8(0); i < r.numChildren; i++ {
			b := r.keys[i]
			if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
				continue
			}
			if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
				*path = (*path)[:before]
				return true
			}
			*path = append((*path)[:nodeLen], b)
			if !iterateRange(r.children[i], path, start, end, yield) {
				*path = (*path)[:before]
				return false
			}
		}
		*path = (*path)[:before]
		return true
	case *node48:
		before := len(*path)
		*path = append(*path, r.prefix...)
		nodeLen := len(*path)
		if r.terminal != nil && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(r.terminal.key, r.terminal.value) {
				*path = (*path)[:before]
				return false
			}
		}
		for edge := 0; edge < 256; edge++ {
			slot := r.childIndex[byte(edge)]
			if slot == 0 {
				continue
			}
			b := byte(edge)
			if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
				continue
			}
			if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
				*path = (*path)[:before]
				return true
			}
			*path = append((*path)[:nodeLen], b)
			if !iterateRange(r.children[slot-1], path, start, end, yield) {
				*path = (*path)[:before]
				return false
			}
		}
		*path = (*path)[:before]
		return true
	case *node256:
		before := len(*path)
		*path = append(*path, r.prefix...)
		nodeLen := len(*path)
		if r.terminal != nil && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(r.terminal.key, r.terminal.value) {
				*path = (*path)[:before]
				return false
			}
		}
		for edge := 0; edge < 256; edge++ {
			child := r.children[edge]
			if child == nil {
				continue
			}
			b := byte(edge)
			if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
				continue
			}
			if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
				*path = (*path)[:before]
				return true
			}
			*path = append((*path)[:nodeLen], b)
			if !iterateRange(child, path, start, end, yield) {
				*path = (*path)[:before]
				return false
			}
		}
		*path = (*path)[:before]
		return true
	}
	return true
}

// keyInRange reports whether key lies in [start, end). A nil bound
// imposes no constraint on that side.
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
// (nodePath ++ extra) compares strictly less than bound. Equivalent
// to subtreeBefore(concatByte(nodePath, extra), bound) but does not
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

// subtreeAtOrAfterWithByte reports whether every key beginning with
// (nodePath ++ extra) is greater than or equal to bound. Equivalent
// to subtreeAtOrAfter(concatByte(nodePath, extra), bound) but does
// not allocate.
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
