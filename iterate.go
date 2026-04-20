// This file contains the All/Range entry points and the iterateRange
// helper. Per-node-type iterate logic lives in iterate methods on each
// node type (see types.go).
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
		switch r := t.root.(type) {
		case nil:
			return
		case *leaf:
			yield(r.key, r.value)
		case innerNode:
			r.iterate(yield)
		}
	}
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
		iterateRange(t.root, nil, start, end, yield)
	}
}

// iterateRange is the pruning analogue of iterate. path is the byte
// sequence consumed from the root to n (before n's own prefix). At
// each inner node the terminal is yielded first, then child edges are
// visited in ascending order; a subtree whose keys all fall outside
// [start, end) is skipped, and because edges are sorted, the first
// edge whose subtree is at-or-after end ends the walk of this node.
func iterateRange(n node, path []byte, start, end []byte, yield func([]byte, any) bool) bool {
	switch r := n.(type) {
	case nil:
		return true
	case *leaf:
		if keyInRange(r.key, start, end) {
			return yield(r.key, r.value)
		}
		return true
	case *node4:
		nodePath := concatPrefix(path, r.prefix)
		if r.terminal != nil && keyInRange(nodePath, start, end) {
			if !yield(r.terminal.key, r.terminal.value) {
				return false
			}
		}
		for i := uint8(0); i < r.numChildren; i++ {
			childPath := concatByte(nodePath, r.keys[i])
			if subtreeBefore(childPath, start) {
				continue
			}
			if subtreeAtOrAfter(childPath, end) {
				return true
			}
			if !iterateRange(r.children[i], childPath, start, end, yield) {
				return false
			}
		}
		return true
	case *node16:
		nodePath := concatPrefix(path, r.prefix)
		if r.terminal != nil && keyInRange(nodePath, start, end) {
			if !yield(r.terminal.key, r.terminal.value) {
				return false
			}
		}
		for i := uint8(0); i < r.numChildren; i++ {
			childPath := concatByte(nodePath, r.keys[i])
			if subtreeBefore(childPath, start) {
				continue
			}
			if subtreeAtOrAfter(childPath, end) {
				return true
			}
			if !iterateRange(r.children[i], childPath, start, end, yield) {
				return false
			}
		}
		return true
	case *node48:
		nodePath := concatPrefix(path, r.prefix)
		if r.terminal != nil && keyInRange(nodePath, start, end) {
			if !yield(r.terminal.key, r.terminal.value) {
				return false
			}
		}
		for edge := 0; edge < 256; edge++ {
			slot := r.childIndex[byte(edge)]
			if slot == 0 {
				continue
			}
			childPath := concatByte(nodePath, byte(edge))
			if subtreeBefore(childPath, start) {
				continue
			}
			if subtreeAtOrAfter(childPath, end) {
				return true
			}
			if !iterateRange(r.children[slot-1], childPath, start, end, yield) {
				return false
			}
		}
		return true
	case *node256:
		nodePath := concatPrefix(path, r.prefix)
		if r.terminal != nil && keyInRange(nodePath, start, end) {
			if !yield(r.terminal.key, r.terminal.value) {
				return false
			}
		}
		for edge := 0; edge < 256; edge++ {
			child := r.children[edge]
			if child == nil {
				continue
			}
			childPath := concatByte(nodePath, byte(edge))
			if subtreeBefore(childPath, start) {
				continue
			}
			if subtreeAtOrAfter(childPath, end) {
				return true
			}
			if !iterateRange(child, childPath, start, end, yield) {
				return false
			}
		}
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

// subtreeBefore reports whether every key that has childPath as a
// prefix is strictly less than start. A nil start is never before.
func subtreeBefore(childPath, start []byte) bool {
	if start == nil {
		return false
	}
	k := len(childPath)
	if len(start) < k {
		k = len(start)
	}
	return bytes.Compare(childPath[:k], start[:k]) < 0
}

// subtreeAtOrAfter reports whether every key that has childPath as a
// prefix is greater than or equal to end. A nil end never bounds from
// above.
func subtreeAtOrAfter(childPath, end []byte) bool {
	if end == nil {
		return false
	}
	k := len(childPath)
	if len(end) < k {
		k = len(end)
	}
	c := bytes.Compare(childPath[:k], end[:k])
	return c > 0 || (c == 0 && len(childPath) >= len(end))
}

// concatPrefix returns a fresh slice containing path followed by
// prefix so recursive callers can extend it without aliasing.
func concatPrefix(path, prefix []byte) []byte {
	out := make([]byte, len(path)+len(prefix))
	copy(out, path)
	copy(out[len(path):], prefix)
	return out
}

// concatByte returns a fresh slice containing path with b appended.
func concatByte(path []byte, b byte) []byte {
	out := make([]byte, len(path)+1)
	copy(out, path)
	out[len(path)] = b
	return out
}
