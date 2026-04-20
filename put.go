// This file contains the Put entry point. Per-node-type logic lives in
// put methods on each node type (see types.go).
package art

import "bytes"

// Put associates value with key, replacing any previous value. An
// inner node's prefix is consumed from the key as traversal descends;
// keys that end at such a node's exact path are stored in its terminal
// slot.
func (t *Tree) Put(key []byte, value any) {
	if t.root == nil {
		t.root = newLeaf(key, value)
		return
	}
	switch r := t.root.(type) {
	case *leaf:
		if bytes.Equal(r.key, key) {
			r.value = value
		} else {
			t.root = newNode4With(r, key, value, 0)
		}
	case innerNode:
		t.root, _ = r.put(key, value, 0)
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
