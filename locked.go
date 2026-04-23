package art

import "sync"

// LockedTree is a thin sync.RWMutex-guarded wrapper around [Tree] for
// callers who want a drop-in concurrent sorted map with the obvious
// semantics: writes are serialized and readers run in parallel with
// each other.
//
// The wrapper covers only the point operations (Put, Get, Delete,
// Len, Clear, Clone). Iteration is deliberately not wrapped because
// the caller controls when each yield returns, which makes a held
// RLock easy to hold for too long; callers that need an ordered scan
// should take a [LockedTree.Clone] and iterate the unlocked snapshot.
//
// See the Concurrency section of the project README for the full
// discussion and trade-offs.
type LockedTree[V any] struct {
	mu   sync.RWMutex
	tree *Tree[V]
}

// NewLocked returns an empty LockedTree.
func NewLocked[V any]() *LockedTree[V] {
	return &LockedTree[V]{tree: New[V]()}
}

// Put associates value with key, replacing any previous value.
func (t *LockedTree[V]) Put(key []byte, value V) {
	t.mu.Lock()
	t.tree.Put(key, value)
	t.mu.Unlock()
}

// Get returns the value stored under key, if any.
func (t *LockedTree[V]) Get(key []byte) (V, bool) {
	t.mu.RLock()
	v, ok := t.tree.Get(key)
	t.mu.RUnlock()
	return v, ok
}

// Delete removes key, returning whether it was present.
func (t *LockedTree[V]) Delete(key []byte) bool {
	t.mu.Lock()
	removed := t.tree.Delete(key)
	t.mu.Unlock()
	return removed
}

// Len returns the current number of entries.
func (t *LockedTree[V]) Len() int {
	t.mu.RLock()
	n := t.tree.Len()
	t.mu.RUnlock()
	return n
}

// Clear removes every entry.
func (t *LockedTree[V]) Clear() {
	t.mu.Lock()
	t.tree.Clear()
	t.mu.Unlock()
}

// Clone returns an unlocked snapshot [Tree]. The snapshot does not
// share its mutex with the original, so callers can iterate the
// returned tree without holding any LockedTree lock.
func (t *LockedTree[V]) Clone() *Tree[V] {
	t.mu.RLock()
	c := t.tree.Clone()
	t.mu.RUnlock()
	return c
}
