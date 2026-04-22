package art

func newLeaf[V any](key []byte, value V) *leaf[V] {
	// Copy the key so callers may safely reuse their slice. Keys up to
	// inlineKeyMax bytes go into the leaf's inline buffer to avoid a
	// second allocation; longer keys are heap-copied.
	l := &leaf[V]{value: value}
	if len(key) <= inlineKeyMax {
		n := copy(l.inline[:], key)
		l.key = l.inline[:n]
	} else {
		l.key = append([]byte(nil), key...)
	}
	return l
}

// longestCommonPrefix returns the leading slice of a that also
// prefixes b.
func longestCommonPrefix(a, b []byte) []byte {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// newNode4With returns a node4 whose prefix is the longest common tail
// of existing.key and newKey starting at depth. If one key is
// exhausted at that point it becomes the new node's terminal value;
// the other is attached as a branching child. If neither is exhausted
// both are attached as branching children on their first divergent
// byte. Caller guarantees the two keys are not equal.
func newNode4With[V any](existing *leaf[V], newKey []byte, newValue V, depth int) *node4[V] {
	shared := longestCommonPrefix(existing.key[depth:], newKey[depth:])
	diverge := depth + len(shared)
	existingExhausted := diverge == len(existing.key)
	newExhausted := diverge == len(newKey)
	if existingExhausted && newExhausted {
		panic("art: newNode4With called with equal keys - invariant violation")
	}
	n := &node4[V]{prefix: append([]byte(nil), shared...)}
	switch {
	case existingExhausted:
		n.terminal = existing
		n.addChild(newKey[diverge], newLeaf(newKey, newValue))
	case newExhausted:
		n.terminal = newLeaf(newKey, newValue)
		n.addChild(existing.key[diverge], existing)
	default:
		n.addChild(existing.key[diverge], existing)
		n.addChild(newKey[diverge], newLeaf(newKey, newValue))
	}
	return n
}

// splitPrefixedInner handles the case where key[depth:] shares only a
// proper prefix of adoptee's prefix. It builds a new parent node4
// whose prefix is a copy of shared and which adopts adoptee under
// edge byte oldBranch. If key is exhausted at the split point the
// parent's terminal holds (key, value); otherwise a new leaf is
// attached as the second branching child. Caller guarantees adoptee's
// own prefix has already been shortened past oldBranch.
func splitPrefixedInner[V any](adoptee innerNode[V], oldBranch byte, shared, key []byte, value V, depth, splitPoint int) *node4[V] {
	parent := &node4[V]{prefix: append([]byte(nil), shared...)}
	parent.addChild(oldBranch, adoptee)
	if depth+splitPoint == len(key) {
		parent.terminal = newLeaf(key, value)
	} else {
		parent.addChild(key[depth+splitPoint], newLeaf(key, value))
	}
	return parent
}
