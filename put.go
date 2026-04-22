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
func putInto[V any](t *Tree[V], current node, key []byte, value V, depth int) node {
	if current == nil {
		return t.insertLeaf(key, value)
	}
	switch r := current.(type) {
	case *leaf[V]:
		if bytes.Equal(r.key, key) {
			r.value = value
			return r
		}
		return newNode4With(t, r, key, value, depth)
	case *node4:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			shared := r.prefix[:splitPoint]
			oldBranch := r.prefix[splitPoint]
			r.prefix = r.prefix[splitPoint+1:]
			return splitPrefixedInner(t, r, oldBranch, shared, key, value, depth, splitPoint)
		}
		return putIntoNode4(t, r, key, value, depth+len(r.prefix))
	case *node16:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			shared := r.prefix[:splitPoint]
			oldBranch := r.prefix[splitPoint]
			r.prefix = r.prefix[splitPoint+1:]
			return splitPrefixedInner(t, r, oldBranch, shared, key, value, depth, splitPoint)
		}
		return putIntoNode16(t, r, key, value, depth+len(r.prefix))
	case *node48:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			shared := r.prefix[:splitPoint]
			oldBranch := r.prefix[splitPoint]
			r.prefix = r.prefix[splitPoint+1:]
			return splitPrefixedInner(t, r, oldBranch, shared, key, value, depth, splitPoint)
		}
		return putIntoNode48(t, r, key, value, depth+len(r.prefix))
	case *node256:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			shared := r.prefix[:splitPoint]
			oldBranch := r.prefix[splitPoint]
			r.prefix = r.prefix[splitPoint+1:]
			return splitPrefixedInner(t, r, oldBranch, shared, key, value, depth, splitPoint)
		}
		return putIntoNode256(t, r, key, value, depth+len(r.prefix))
	}
	return current
}

// putIntoNode4 writes (key, value) into r given that r.prefix has
// already been consumed from key (the caller passes the advanced
// depth). The decision reads as: key exhausted? → terminal. Else
// switch on the child at key[depth]: absent → add/grow; leaf same
// key → overwrite; leaf different key → nested node4; inner node →
// recurse.
func putIntoNode4[V any](t *Tree[V], r *node4, key []byte, value V, depth int) node {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.(*leaf[V]).value = value
			return r
		}
		r.terminal = t.insertLeaf(key, value)
		return r
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		return node4AddOrGrow(r, branch, t.insertLeaf(key, value))
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

// putIntoNode16 mirrors putIntoNode4 at node16 capacity. r.prefix has
// already been consumed from key by the caller.
func putIntoNode16[V any](t *Tree[V], r *node16, key []byte, value V, depth int) node {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.(*leaf[V]).value = value
			return r
		}
		r.terminal = t.insertLeaf(key, value)
		return r
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		return node16AddOrGrow(r, branch, t.insertLeaf(key, value))
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

// node4AddOrGrow adds child under edge byte b, growing to a node16
// when r is already full. The grown node16 inherits r's prefix and
// terminal.
func node4AddOrGrow(r *node4, b byte, child node) node {
	if r.numChildren < node4Capacity {
		r.addChild(b, child)
		return r
	}
	grown := growToNode16(r)
	grown.insertChild(b, child)
	return grown
}

// node16AddOrGrow adds child under edge byte b, growing to a node48
// when r is already full. The grown node48 inherits r's prefix and
// terminal.
func node16AddOrGrow(r *node16, b byte, child node) node {
	if r.numChildren < node16Capacity {
		r.insertChild(b, child)
		return r
	}
	grown := growToNode48(r)
	grown.addChild(b, child)
	return grown
}

// putIntoNode48 mirrors putIntoNode4 at node48 capacity. r.prefix has
// already been consumed from key by the caller.
func putIntoNode48[V any](t *Tree[V], r *node48, key []byte, value V, depth int) node {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.(*leaf[V]).value = value
			return r
		}
		r.terminal = t.insertLeaf(key, value)
		return r
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		return node48AddOrGrow(r, branch, t.insertLeaf(key, value))
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

// putIntoNode256 mirrors putIntoNode4 at node256 capacity. r.prefix has
// already been consumed from key by the caller.
func putIntoNode256[V any](t *Tree[V], r *node256, key []byte, value V, depth int) node {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.(*leaf[V]).value = value
			return r
		}
		r.terminal = t.insertLeaf(key, value)
		return r
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		r.addChild(branch, t.insertLeaf(key, value))
		return r
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

// node48AddOrGrow adds child under edge byte b, growing to a node256
// when r is already full. The grown node256 inherits r's prefix and
// terminal.
func node48AddOrGrow(r *node48, b byte, child node) node {
	if r.numChildren < node48Capacity {
		r.addChild(b, child)
		return r
	}
	grown := growToNode256(r)
	grown.addChild(b, child)
	return grown
}
