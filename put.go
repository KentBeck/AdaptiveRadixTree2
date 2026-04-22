package art

import "bytes"

// Put associates value with key, replacing any previous value. An
// inner node's prefix is consumed from the key as traversal descends;
// keys that end at such a node's exact path are stored in its terminal
// slot.
func (t *Tree) Put(key []byte, value any) {
	newRoot, inserted := putInto(t.root, key, value, 0)
	t.root = newRoot
	if inserted {
		t.size++
	}
}

// putInto returns the (possibly replaced) subtree root and whether a
// brand-new key was inserted (as opposed to an existing key's value
// being overwritten).
func putInto(current node, key []byte, value any, depth int) (node, bool) {
	if current == nil {
		return newLeaf(key, value), true
	}
	switch r := current.(type) {
	case *leaf:
		if bytes.Equal(r.key, key) {
			r.value = value
			return r, false
		}
		return newNode4With(r, key, value, depth), true
	case *node4:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			shared := r.prefix[:splitPoint]
			oldBranch := r.prefix[splitPoint]
			r.prefix = r.prefix[splitPoint+1:]
			return splitPrefixedInner(r, oldBranch, shared, key, value, depth, splitPoint), true
		}
		return putIntoNode4(r, key, value, depth+len(r.prefix))
	case *node16:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			shared := r.prefix[:splitPoint]
			oldBranch := r.prefix[splitPoint]
			r.prefix = r.prefix[splitPoint+1:]
			return splitPrefixedInner(r, oldBranch, shared, key, value, depth, splitPoint), true
		}
		return putIntoNode16(r, key, value, depth+len(r.prefix))
	case *node48:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			shared := r.prefix[:splitPoint]
			oldBranch := r.prefix[splitPoint]
			r.prefix = r.prefix[splitPoint+1:]
			return splitPrefixedInner(r, oldBranch, shared, key, value, depth, splitPoint), true
		}
		return putIntoNode48(r, key, value, depth+len(r.prefix))
	case *node256:
		splitPoint := len(longestCommonPrefix(key[depth:], r.prefix))
		if splitPoint < len(r.prefix) {
			shared := r.prefix[:splitPoint]
			oldBranch := r.prefix[splitPoint]
			r.prefix = r.prefix[splitPoint+1:]
			return splitPrefixedInner(r, oldBranch, shared, key, value, depth, splitPoint), true
		}
		return putIntoNode256(r, key, value, depth+len(r.prefix))
	}
	return current, false
}

// putIntoNode4 writes (key, value) into r given that r.prefix has
// already been consumed from key (the caller passes the advanced
// depth). The decision reads as: key exhausted? → terminal. Else
// switch on the child at key[depth]: absent → add/grow; leaf same
// key → overwrite; leaf different key → nested node4; inner node →
// recurse.
func putIntoNode4(r *node4, key []byte, value any, depth int) (node, bool) {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.value = value
			return r, false
		}
		r.terminal = newLeaf(key, value)
		return r, true
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		return node4AddOrGrow(r, branch, newLeaf(key, value)), true
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return r, false
		}
		r.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return r, true
	default:
		newChild, inserted := putInto(c, key, value, depth+1)
		r.replaceChild(branch, newChild)
		return r, inserted
	}
}

// putIntoNode16 mirrors putIntoNode4 at node16 capacity. r.prefix has
// already been consumed from key by the caller.
func putIntoNode16(r *node16, key []byte, value any, depth int) (node, bool) {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.value = value
			return r, false
		}
		r.terminal = newLeaf(key, value)
		return r, true
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		return node16AddOrGrow(r, branch, newLeaf(key, value)), true
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return r, false
		}
		r.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return r, true
	default:
		newChild, inserted := putInto(c, key, value, depth+1)
		r.replaceChild(branch, newChild)
		return r, inserted
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
func putIntoNode48(r *node48, key []byte, value any, depth int) (node, bool) {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.value = value
			return r, false
		}
		r.terminal = newLeaf(key, value)
		return r, true
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		return node48AddOrGrow(r, branch, newLeaf(key, value)), true
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return r, false
		}
		r.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return r, true
	default:
		newChild, inserted := putInto(c, key, value, depth+1)
		r.replaceChild(branch, newChild)
		return r, inserted
	}
}

// putIntoNode256 mirrors putIntoNode4 at node256 capacity. r.prefix has
// already been consumed from key by the caller.
func putIntoNode256(r *node256, key []byte, value any, depth int) (node, bool) {
	if depth == len(key) {
		if r.terminal != nil {
			r.terminal.value = value
			return r, false
		}
		r.terminal = newLeaf(key, value)
		return r, true
	}
	branch := key[depth]
	switch c := r.findChild(branch).(type) {
	case nil:
		r.addChild(branch, newLeaf(key, value))
		return r, true
	case *leaf:
		if bytes.Equal(c.key, key) {
			c.value = value
			return r, false
		}
		r.replaceChild(branch, newNode4With(c, key, value, depth+1))
		return r, true
	default:
		newChild, inserted := putInto(c, key, value, depth+1)
		r.replaceChild(branch, newChild)
		return r, inserted
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
