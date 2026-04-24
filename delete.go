package art

import "bytes"

// Delete removes key from the tree, returning whether it was present.
// Traversal consumes each inner node's prefix and, if the key is
// exhausted at a node, targets that node's terminal value. After a
// successful remove the affected node is demoted to a smaller node
// type (or collapsed to its only child) whenever its child count
// crosses the next-smaller capacity. A nil key and an empty-slice key
// are equivalent (both represent the empty key).
func (t *Tree[V]) Delete(key []byte) bool {
	before := t.size
	t.root = deleteFrom(t, t.root, key, 0)
	return t.size != before
}

// deleteFrom removes key from the subtree rooted at current, returning
// the (possibly replaced) root. A nil return means the caller should
// drop its reference to this subtree. Size accounting lives at the
// leaf-removal chokepoints ([clearTerminalIfMatches] for terminals and
// the *leaf case here for branching leaves); a "nothing changed"
// signal is communicated by returning the same pointer that was
// passed in.
func deleteFrom[V any](t *Tree[V], current node, key []byte, depth int) node {
	switch r := current.(type) {
	case nil:
		return nil
	case *leaf[V]:
		if bytes.Equal(r.key, key) {
			t.size--
			return nil
		}
		return r
	case *node4:
		d, ok := consumePrefix(r.prefix, key, depth)
		if !ok {
			return r
		}
		if d == len(key) {
			if !clearTerminalIfMatches[V](t, &r.terminal, key) {
				return r
			}
			return postDeleteReshape(r)
		}
		branch := key[d]
		child := r.findChild(branch)
		if child == nil {
			return r
		}
		newChild := deleteFrom(t, child, key, d+1)
		if newChild == child {
			return r
		}
		if newChild == nil {
			r.removeChild(branch)
		} else {
			r.replaceChild(branch, newChild)
		}
		return postDeleteReshape(r)
	case *node16:
		d, ok := consumePrefix(r.prefix, key, depth)
		if !ok {
			return r
		}
		if d == len(key) {
			if !clearTerminalIfMatches[V](t, &r.terminal, key) {
				return r
			}
			return postDeleteReshape(r)
		}
		branch := key[d]
		child := r.findChild(branch)
		if child == nil {
			return r
		}
		newChild := deleteFrom(t, child, key, d+1)
		if newChild == child {
			return r
		}
		if newChild == nil {
			r.removeChild(branch)
		} else {
			r.replaceChild(branch, newChild)
		}
		return postDeleteReshape(r)
	case *node48:
		d, ok := consumePrefix(r.prefix, key, depth)
		if !ok {
			return r
		}
		if d == len(key) {
			if !clearTerminalIfMatches[V](t, &r.terminal, key) {
				return r
			}
			return postDeleteReshape(r)
		}
		branch := key[d]
		child := r.findChild(branch)
		if child == nil {
			return r
		}
		newChild := deleteFrom(t, child, key, d+1)
		if newChild == child {
			return r
		}
		if newChild == nil {
			r.removeChild(branch)
		} else {
			r.replaceChild(branch, newChild)
		}
		return postDeleteReshape(r)
	case *node256:
		d, ok := consumePrefix(r.prefix, key, depth)
		if !ok {
			return r
		}
		if d == len(key) {
			if !clearTerminalIfMatches[V](t, &r.terminal, key) {
				return r
			}
			return postDeleteReshape(r)
		}
		branch := key[d]
		child := r.findChild(branch)
		if child == nil {
			return r
		}
		newChild := deleteFrom(t, child, key, d+1)
		if newChild == child {
			return r
		}
		if newChild == nil {
			r.removeChild(branch)
		} else {
			r.replaceChild(branch, newChild)
		}
		return postDeleteReshape(r)
	}
	return current
}

// postDeleteReshape inspects n after a child removal or terminal clear
// and either demotes, collapses, or returns it unchanged. The flat
// decision: 0 children with a terminal collapses to the terminal leaf;
// 0 children without a terminal drops the subtree (nil return); a
// node4 with exactly one child and no terminal collapses to that child
// (a leaf replaces the node directly; an inner child absorbs the
// parent's prefix and branch byte into its own prefix).
func postDeleteReshape(n innerNode) node {
	switch m := n.(type) {
	case *node256:
		if m.numChildren == 0 {
			if m.terminal != nil {
				return m.terminal
			}
			return nil
		}
		if m.numChildren == node48Capacity {
			return shrinkToNode48(m)
		}
	case *node48:
		if m.numChildren == 0 {
			if m.terminal != nil {
				return m.terminal
			}
			return nil
		}
		if m.numChildren == node16Capacity {
			return shrinkToNode16(m)
		}
	case *node16:
		if m.numChildren == 0 {
			if m.terminal != nil {
				return m.terminal
			}
			return nil
		}
		if m.numChildren == node4Capacity {
			return shrinkToNode4(m)
		}
	case *node4:
		if m.numChildren == 0 {
			if m.terminal != nil {
				return m.terminal
			}
			return nil
		}
		if m.numChildren == 1 && m.terminal == nil {
			only := m.children[0]
			if only.kind() == kindLeaf {
				return only
			}
			return mergePrefixIntoChild(m.prefix, m.keys[0], only.(innerNode))
		}
	}
	return n
}

// mergePrefixIntoChild rewrites child's prefix to parentPrefix ||
// branchByte || child's old prefix and returns child for use as the
// replacement of its collapsed parent. The merged slice is freshly
// allocated so the parent's prefix backing array is not aliased.
func mergePrefixIntoChild(parentPrefix []byte, branchByte byte, child innerNode) node {
	switch c := child.(type) {
	case *node4:
		c.prefix = mergedPrefix(parentPrefix, branchByte, c.prefix)
		return c
	case *node16:
		c.prefix = mergedPrefix(parentPrefix, branchByte, c.prefix)
		return c
	case *node48:
		c.prefix = mergedPrefix(parentPrefix, branchByte, c.prefix)
		return c
	case *node256:
		c.prefix = mergedPrefix(parentPrefix, branchByte, c.prefix)
		return c
	}
	return child
}

// mergedPrefix returns a new slice holding parent || branch || child.
func mergedPrefix(parent []byte, branch byte, child []byte) []byte {
	merged := make([]byte, len(parent)+1+len(child))
	copy(merged, parent)
	merged[len(parent)] = branch
	copy(merged[len(parent)+1:], child)
	return merged
}
