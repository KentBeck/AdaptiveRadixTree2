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
		if l, ok := current.(*leaf[V]); ok {
			return terminalValue[V](l, key)
		}
		n := current.(innerNode)
		d, matched := consumePrefix(n.getPrefix(), key, depth)
		if !matched {
			return zero, false
		}
		if d == len(key) {
			return terminalValue[V](n.getTerminal(), key)
		}
		current = n.findChild(key[d])
		depth = d + 1
	}
	return zero, false
}
