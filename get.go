package art

import "bytes"

// Get returns the value previously stored under key, if any. If the
// key is absent the returned value is the zero value of V. A nil key
// and an empty-slice key are equivalent (both represent the empty
// key).
func (t *Tree[V]) Get(key []byte) (value V, ok bool) {
	var zero V
	current := t.root
	depth := 0
	for current != nil {
		switch n := current.(type) {
		case *leaf[V]:
			if bytes.Equal(n.key, key) {
				return n.value, true
			}
			return zero, false
		case *node4[V]:
			if pl := len(n.prefix); pl != 0 {
				end := depth + pl
				if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
					return zero, false
				}
				depth = end
			}
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return zero, false
			}
			current = n.findChild(key[depth])
			depth++
		case *node16[V]:
			if pl := len(n.prefix); pl != 0 {
				end := depth + pl
				if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
					return zero, false
				}
				depth = end
			}
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return zero, false
			}
			current = n.findChild(key[depth])
			depth++
		case *node48[V]:
			if pl := len(n.prefix); pl != 0 {
				end := depth + pl
				if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
					return zero, false
				}
				depth = end
			}
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return zero, false
			}
			current = n.findChild(key[depth])
			depth++
		case *node256[V]:
			if pl := len(n.prefix); pl != 0 {
				end := depth + pl
				if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
					return zero, false
				}
				depth = end
			}
			if depth == len(key) {
				if n.terminal != nil && bytes.Equal(n.terminal.key, key) {
					return n.terminal.value, true
				}
				return zero, false
			}
			current = n.findChild(key[depth])
			depth++
		}
	}
	return zero, false
}
