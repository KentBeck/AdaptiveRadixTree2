// This file contains the Get entry point. Per-node-type logic lives in
// get methods on each node type (see types.go).
package art

import "bytes"

// Get returns the value previously stored under key, if any.
func (t *Tree) Get(key []byte) (value any, ok bool) {
	if t.root == nil {
		return nil, false
	}
	switch r := t.root.(type) {
	case *leaf:
		if bytes.Equal(r.key, key) {
			return r.value, true
		}
		return nil, false
	case innerNode:
		return r.get(key, 0)
	}
	return nil, false
}
