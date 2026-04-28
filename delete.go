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
// passed in. Inner-node post-delete demotion/collapse is dispatched
// polymorphically via [innerNode.reshape].
func deleteFrom[V any](t *Tree[V], current node, key []byte, depth int) node {
	if current == nil {
		return nil
	}
	if l, ok := current.(*leaf[V]); ok {
		if bytes.Equal(l.key, key) {
			t.size--
			return nil
		}
		return l
	}
	r := current.(innerNode)
	d, ok := consumePrefix(r.getPrefix(), key, depth)
	if !ok {
		return r
	}
	if d == len(key) {
		if !clearTerminalIfMatches[V](t, r, key) {
			return r
		}
		return r.reshape()
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
	return r.reshape()
}
