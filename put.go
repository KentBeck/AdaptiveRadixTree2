package art

import "bytes"

// Put associates value with key, replacing any previous value. An
// inner node's prefix is consumed from the key as traversal descends;
// keys that end at such a node's exact path are stored in its terminal
// slot. A nil key and an empty-slice key are equivalent (both
// represent the empty key); the tree can hold at most one entry at
// the empty key.
func (t *Tree[V]) Put(key []byte, value V) {
	t.root = putInto(t, t.root, key, value, 0)
}

// putInto returns the (possibly replaced) subtree root. Size
// accounting lives in [Tree.insertLeaf]: every fresh-leaf allocation
// bumps t.size, so recursion only needs to return the new subtree.
// Inner-node dispatch is polymorphic via the innerNode interface; the
// node-specific work (capacity-aware add, sorted insertion) lives in
// each type's [innerNode.addOrGrow].
func putInto[V any](t *Tree[V], current node, key []byte, value V, depth int) node {
	if current == nil {
		return t.insertLeaf(key, value)
	}
	if l, ok := current.(*leaf[V]); ok {
		if bytes.Equal(l.key, key) {
			l.value = value
			return l
		}
		return newNode4With(t, l, key, value, depth)
	}
	r := current.(innerNode)
	prefix := r.getPrefix()
	splitPoint := len(longestCommonPrefix(key[depth:], prefix))
	if splitPoint < len(prefix) {
		shared := prefix[:splitPoint]
		oldBranch := prefix[splitPoint]
		r.setPrefix(prefix[splitPoint+1:])
		return splitPrefixedInner(t, r, oldBranch, shared, key, value, depth, splitPoint)
	}
	return putIntoInner(t, r, key, value, depth+len(prefix))
}

// putIntoInner writes (key, value) into r given that r.getPrefix() has
// already been consumed from key (the caller passes the advanced
// depth). The decision reads as: key exhausted? → terminal. Else
// switch on the child at key[depth]: absent → addOrGrow; leaf same
// key → overwrite; leaf different key → nested node4; inner node →
// recurse.
func putIntoInner[V any](t *Tree[V], r innerNode, key []byte, value V, depth int) node {
	if depth == len(key) {
		if term, ok := r.getTerminal().(*leaf[V]); ok {
			term.value = value
			return r
		}
		r.setTerminal(t.insertLeaf(key, value))
		return r
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		return r.addOrGrow(branch, t.insertLeaf(key, value))
	case *leaf[V]:
		if bytes.Equal(c.key, key) {
			c.value = value
			return r
		}
		r.replaceChild(branch, newNode4With(t, c, key, value, depth+1))
		return r
	default:
		r.replaceChild(branch, putInto(t, c, key, value, depth+1))
		return r
	}
}
