package art

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
			return terminalValue[V](n, key)
		case *node4:
			d, ok := consumePrefix(n.prefix, key, depth)
			if !ok {
				return zero, false
			}
			if d == len(key) {
				return terminalValue[V](n.terminal, key)
			}
			current = n.findChild(key[d])
			depth = d + 1
		case *node16:
			d, ok := consumePrefix(n.prefix, key, depth)
			if !ok {
				return zero, false
			}
			if d == len(key) {
				return terminalValue[V](n.terminal, key)
			}
			current = n.findChild(key[d])
			depth = d + 1
		case *node48:
			d, ok := consumePrefix(n.prefix, key, depth)
			if !ok {
				return zero, false
			}
			if d == len(key) {
				return terminalValue[V](n.terminal, key)
			}
			current = n.findChild(key[d])
			depth = d + 1
		case *node256:
			d, ok := consumePrefix(n.prefix, key, depth)
			if !ok {
				return zero, false
			}
			if d == len(key) {
				return terminalValue[V](n.terminal, key)
			}
			current = n.findChild(key[d])
			depth = d + 1
		}
	}
	return zero, false
}
