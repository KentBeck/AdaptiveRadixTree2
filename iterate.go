package art

import (
	"bytes"
	"iter"
)

// All returns an iterator over every (key, value) pair in the tree in
// ascending byte-wise key order. Breaking out of the range stops the
// traversal immediately.
//
// The yielded key slice aliases the tree's internal storage. It is
// safe to retain while the corresponding entry remains in the tree,
// and must be treated as read-only; mutating it corrupts the tree.
// If the entry may be deleted (including by the caller during
// iteration) while a retained reference is in use, copy the key.
//
// See [Tree.AllDescending] for the reverse walk.
func (t *Tree[V]) All() iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		iterate(t.root, yield)
	}
}

// AllDescending returns an iterator over every (key, value) pair in
// the tree in descending byte-wise key order. Breaking out of the
// range stops the traversal immediately.
//
// The yielded key slice aliases the tree's internal storage under the
// same contract as [Tree.All]. See [Tree.All] for the forward walk.
func (t *Tree[V]) AllDescending() iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		iterateDescending(t.root, yield)
	}
}

// iterate visits every (key, value) pair reachable from n in sorted
// key order, returning false as soon as yield does so the caller can
// short-circuit all the way up.
func iterate[V any](n node, yield func([]byte, V) bool) bool {
	switch r := n.(type) {
	case nil:
		return true
	case *leaf[V]:
		return yield(r.key, r.value)
	case *node4:
		if tl, ok := r.terminal.(*leaf[V]); ok && !yield(tl.key, tl.value) {
			return false
		}
		for i := uint8(0); i < r.numChildren; i++ {
			if !iterate[V](r.children[i], yield) {
				return false
			}
		}
		return true
	case *node16:
		if tl, ok := r.terminal.(*leaf[V]); ok && !yield(tl.key, tl.value) {
			return false
		}
		for i := uint8(0); i < r.numChildren; i++ {
			if !iterate[V](r.children[i], yield) {
				return false
			}
		}
		return true
	case *node48:
		if tl, ok := r.terminal.(*leaf[V]); ok && !yield(tl.key, tl.value) {
			return false
		}
		for edge := 0; edge < 256; edge++ {
			slot := r.childIndex[byte(edge)]
			if slot == 0 {
				continue
			}
			if !iterate[V](r.children[slot-1], yield) {
				return false
			}
		}
		return true
	case *node256:
		if tl, ok := r.terminal.(*leaf[V]); ok && !yield(tl.key, tl.value) {
			return false
		}
		for edge := 0; edge < 256; edge++ {
			child := r.children[edge]
			if child == nil {
				continue
			}
			if !iterate[V](child, yield) {
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
//
// The yielded key slice aliases the tree's internal storage under the
// same contract as [Tree.All]: safe to retain while the entry remains
// in the tree, must be treated as read-only, and must be copied if
// retained past a possible deletion of the entry.
//
// See [Tree.RangeFrom] and [Tree.RangeTo] for open-ended variants and
// [Tree.RangeDescending] for the reverse walk of the same interval.
func (t *Tree[V]) Range(start, end []byte) iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if start != nil && end != nil && bytes.Compare(start, end) >= 0 {
			return
		}
		path := make([]byte, 0, 32)
		iterateRange(t.root, &path, start, end, yield)
	}
}

// RangeFrom returns an iterator over every (key, value) pair whose key
// is byte-wise >= start, in ascending order. A nil start is equivalent
// to [Tree.All]. This is the shorthand for Range(start, nil) and
// matches google/btree's AscendGreaterOrEqual.
//
// The yielded key slice aliases the tree's internal storage under the
// same contract as [Tree.All].
func (t *Tree[V]) RangeFrom(start []byte) iter.Seq2[[]byte, V] {
	return t.Range(start, nil)
}

// RangeTo returns an iterator over every (key, value) pair whose key
// is byte-wise < end, in ascending order. A nil end is equivalent to
// [Tree.All]. This is the shorthand for Range(nil, end) and matches
// google/btree's AscendLessThan.
//
// The yielded key slice aliases the tree's internal storage under the
// same contract as [Tree.All].
func (t *Tree[V]) RangeTo(end []byte) iter.Seq2[[]byte, V] {
	return t.Range(nil, end)
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
func iterateRange[V any](n node, path *[]byte, start, end []byte, yield func([]byte, V) bool) bool {
	switch r := n.(type) {
	case nil:
		return true
	case *leaf[V]:
		if keyInRange(r.key, start, end) {
			return yield(r.key, r.value)
		}
		return true
	case *node4:
		before := len(*path)
		*path = append(*path, r.prefix...)
		nodeLen := len(*path)
		if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(tl.key, tl.value) {
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
			if !iterateRange[V](r.children[i], path, start, end, yield) {
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
		if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(tl.key, tl.value) {
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
			if !iterateRange[V](r.children[i], path, start, end, yield) {
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
		if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(tl.key, tl.value) {
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
			if !iterateRange[V](r.children[slot-1], path, start, end, yield) {
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
		if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(tl.key, tl.value) {
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
			if !iterateRange[V](child, path, start, end, yield) {
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

// iterateDescending visits every (key, value) pair reachable from n
// in descending key order. Within each inner node children are
// traversed from highest to lowest edge byte, and the terminal (when
// present) is yielded last: a node's terminal key is shorter than any
// child-extension and therefore sorts before every child subtree.
func iterateDescending[V any](n node, yield func([]byte, V) bool) bool {
	switch r := n.(type) {
	case nil:
		return true
	case *leaf[V]:
		return yield(r.key, r.value)
	case *node4:
		for i := int(r.numChildren) - 1; i >= 0; i-- {
			if !iterateDescending[V](r.children[i], yield) {
				return false
			}
		}
		if tl, ok := r.terminal.(*leaf[V]); ok && !yield(tl.key, tl.value) {
			return false
		}
		return true
	case *node16:
		for i := int(r.numChildren) - 1; i >= 0; i-- {
			if !iterateDescending[V](r.children[i], yield) {
				return false
			}
		}
		if tl, ok := r.terminal.(*leaf[V]); ok && !yield(tl.key, tl.value) {
			return false
		}
		return true
	case *node48:
		for edge := 255; edge >= 0; edge-- {
			slot := r.childIndex[byte(edge)]
			if slot == 0 {
				continue
			}
			if !iterateDescending[V](r.children[slot-1], yield) {
				return false
			}
		}
		if tl, ok := r.terminal.(*leaf[V]); ok && !yield(tl.key, tl.value) {
			return false
		}
		return true
	case *node256:
		for edge := 255; edge >= 0; edge-- {
			child := r.children[edge]
			if child == nil {
				continue
			}
			if !iterateDescending[V](child, yield) {
				return false
			}
		}
		if tl, ok := r.terminal.(*leaf[V]); ok && !yield(tl.key, tl.value) {
			return false
		}
		return true
	}
	return true
}

// RangeDescending returns an iterator over every (key, value) pair
// whose key lies in the half-open interval [start, end), in
// descending byte-wise key order. The bounds have the same semantics
// as [Tree.Range] — start is inclusive, end is exclusive, and a nil
// bound is unbounded on that side — so RangeDescending(nil, nil) is
// equivalent to [Tree.AllDescending] and start >= end yields nothing.
// Breaking out of the range stops the traversal immediately.
//
// The yielded key slice aliases the tree's internal storage under the
// same contract as [Tree.All]. See [Tree.Range] for the ascending walk
// of the same interval.
func (t *Tree[V]) RangeDescending(start, end []byte) iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if start != nil && end != nil && bytes.Compare(start, end) >= 0 {
			return
		}
		path := make([]byte, 0, 32)
		iterateRangeDescending(t.root, &path, start, end, yield)
	}
}

// iterateRangeDescending is the descending analogue of iterateRange.
// *path is the byte sequence consumed from the root to n (before n's
// own prefix) and is reused across the recursion with every exit path
// restoring its length on entry. At each inner node, child edges are
// visited from highest to lowest byte before the terminal; a subtree
// whose keys all fall outside [start, end) is skipped without
// materializing its path, and because edges are processed in
// descending order, the first subtree that is strictly before start
// ends the walk of this node.
func iterateRangeDescending[V any](n node, path *[]byte, start, end []byte, yield func([]byte, V) bool) bool {
	switch r := n.(type) {
	case nil:
		return true
	case *leaf[V]:
		if keyInRange(r.key, start, end) {
			return yield(r.key, r.value)
		}
		return true
	case *node4:
		before := len(*path)
		*path = append(*path, r.prefix...)
		nodeLen := len(*path)
		for i := int(r.numChildren) - 1; i >= 0; i-- {
			b := r.keys[i]
			if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
				continue
			}
			if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
				if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
					if !yield(tl.key, tl.value) {
						*path = (*path)[:before]
						return false
					}
				}
				*path = (*path)[:before]
				return true
			}
			*path = append((*path)[:nodeLen], b)
			if !iterateRangeDescending[V](r.children[i], path, start, end, yield) {
				*path = (*path)[:before]
				return false
			}
		}
		if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(tl.key, tl.value) {
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
		for i := int(r.numChildren) - 1; i >= 0; i-- {
			b := r.keys[i]
			if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
				continue
			}
			if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
				if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
					if !yield(tl.key, tl.value) {
						*path = (*path)[:before]
						return false
					}
				}
				*path = (*path)[:before]
				return true
			}
			*path = append((*path)[:nodeLen], b)
			if !iterateRangeDescending[V](r.children[i], path, start, end, yield) {
				*path = (*path)[:before]
				return false
			}
		}
		if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(tl.key, tl.value) {
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
		for edge := 255; edge >= 0; edge-- {
			slot := r.childIndex[byte(edge)]
			if slot == 0 {
				continue
			}
			b := byte(edge)
			if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
				continue
			}
			if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
				if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
					if !yield(tl.key, tl.value) {
						*path = (*path)[:before]
						return false
					}
				}
				*path = (*path)[:before]
				return true
			}
			*path = append((*path)[:nodeLen], b)
			if !iterateRangeDescending[V](r.children[slot-1], path, start, end, yield) {
				*path = (*path)[:before]
				return false
			}
		}
		if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(tl.key, tl.value) {
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
		for edge := 255; edge >= 0; edge-- {
			child := r.children[edge]
			if child == nil {
				continue
			}
			b := byte(edge)
			if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
				continue
			}
			if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
				if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
					if !yield(tl.key, tl.value) {
						*path = (*path)[:before]
						return false
					}
				}
				*path = (*path)[:before]
				return true
			}
			*path = append((*path)[:nodeLen], b)
			if !iterateRangeDescending[V](child, path, start, end, yield) {
				*path = (*path)[:before]
				return false
			}
		}
		if tl, ok := r.terminal.(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
			if !yield(tl.key, tl.value) {
				*path = (*path)[:before]
				return false
			}
		}
		*path = (*path)[:before]
		return true
	}
	return true
}
