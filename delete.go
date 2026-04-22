package art

import "bytes"

// Delete removes key from the tree, returning whether it was present.
// Traversal consumes each inner node's prefix and, if the key is
// exhausted at a node, targets that node's terminal value. After a
// successful remove the affected node is demoted to a smaller node
// type (or collapsed to its only child) whenever its child count
// crosses the next-smaller capacity.
func (t *Tree) Delete(key []byte) bool {
	newRoot, deleted := deleteFrom(t.root, key, 0)
	if deleted {
		t.root = newRoot
		t.size--
	}
	return deleted
}

// deleteFrom removes key from the subtree rooted at current, returning
// the (possibly replaced) root and whether the key was present. A nil
// return means the caller should drop its reference to this subtree.
// The structure mirrors Get: consume the node's prefix, then either
// clear the terminal (key exhausted) or recurse through the branching
// child at key[depth].
func deleteFrom(current node, key []byte, depth int) (node, bool) {
	switch r := current.(type) {
	case nil:
		return nil, false
	case *leaf:
		if bytes.Equal(r.key, key) {
			return nil, true
		}
		return r, false
	case *node4:
		if pl := len(r.prefix); pl != 0 {
			end := depth + pl
			if end > len(key) || !bytes.Equal(r.prefix, key[depth:end]) {
				return r, false
			}
			depth = end
		}
		if depth == len(key) {
			if r.terminal == nil || !bytes.Equal(r.terminal.key, key) {
				return r, false
			}
			r.terminal = nil
			return postDeleteReshape(r), true
		}
		branch := key[depth]
		child := r.findChild(branch)
		if child == nil {
			return r, false
		}
		newChild, deleted := deleteFrom(child, key, depth+1)
		if !deleted {
			return r, false
		}
		if newChild == nil {
			r.removeChild(branch)
		} else {
			r.replaceChild(branch, newChild)
		}
		return postDeleteReshape(r), true
	case *node16:
		if pl := len(r.prefix); pl != 0 {
			end := depth + pl
			if end > len(key) || !bytes.Equal(r.prefix, key[depth:end]) {
				return r, false
			}
			depth = end
		}
		if depth == len(key) {
			if r.terminal == nil || !bytes.Equal(r.terminal.key, key) {
				return r, false
			}
			r.terminal = nil
			return postDeleteReshape(r), true
		}
		branch := key[depth]
		child := r.findChild(branch)
		if child == nil {
			return r, false
		}
		newChild, deleted := deleteFrom(child, key, depth+1)
		if !deleted {
			return r, false
		}
		if newChild == nil {
			r.removeChild(branch)
		} else {
			r.replaceChild(branch, newChild)
		}
		return postDeleteReshape(r), true
	case *node48:
		if pl := len(r.prefix); pl != 0 {
			end := depth + pl
			if end > len(key) || !bytes.Equal(r.prefix, key[depth:end]) {
				return r, false
			}
			depth = end
		}
		if depth == len(key) {
			if r.terminal == nil || !bytes.Equal(r.terminal.key, key) {
				return r, false
			}
			r.terminal = nil
			return postDeleteReshape(r), true
		}
		branch := key[depth]
		child := r.findChild(branch)
		if child == nil {
			return r, false
		}
		newChild, deleted := deleteFrom(child, key, depth+1)
		if !deleted {
			return r, false
		}
		if newChild == nil {
			r.removeChild(branch)
		} else {
			r.replaceChild(branch, newChild)
		}
		return postDeleteReshape(r), true
	case *node256:
		if pl := len(r.prefix); pl != 0 {
			end := depth + pl
			if end > len(key) || !bytes.Equal(r.prefix, key[depth:end]) {
				return r, false
			}
			depth = end
		}
		if depth == len(key) {
			if r.terminal == nil || !bytes.Equal(r.terminal.key, key) {
				return r, false
			}
			r.terminal = nil
			return postDeleteReshape(r), true
		}
		branch := key[depth]
		child := r.findChild(branch)
		if child == nil {
			return r, false
		}
		newChild, deleted := deleteFrom(child, key, depth+1)
		if !deleted {
			return r, false
		}
		if newChild == nil {
			r.removeChild(branch)
		} else {
			r.replaceChild(branch, newChild)
		}
		return postDeleteReshape(r), true
	}
	return current, false
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
			if leaf, isLeaf := only.(*leaf); isLeaf {
				return leaf
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
