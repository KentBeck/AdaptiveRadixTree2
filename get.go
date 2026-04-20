// This file contains Get operation logic across all node types.
package art

import "bytes"

// Get returns the value previously stored under key, if any.
func (t *Tree) Get(key []byte) (value any, ok bool) {
	current := t.root
	depth := 0
	for current != nil {
		switch n := current.(type) {
		case *leaf:
			if bytes.Equal(n.key, key) {
				return n.value, true
			}
			return nil, false
		case *node4:
			end := depth + len(n.prefix)
			if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
				return nil, false
			}
			depth = end
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return nil, false
			}
			current = n.findChild(key[depth])
			depth++
		case *node16:
			end := depth + len(n.prefix)
			if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
				return nil, false
			}
			depth = end
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return nil, false
			}
			current = n.findChild(key[depth])
			depth++
		case *node48:
			end := depth + len(n.prefix)
			if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
				return nil, false
			}
			depth = end
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return nil, false
			}
			current = n.findChild(key[depth])
			depth++
		case *node256:
			end := depth + len(n.prefix)
			if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
				return nil, false
			}
			depth = end
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return nil, false
			}
			current = n.findChild(key[depth])
			depth++
		}
	}
	return nil, false
}
