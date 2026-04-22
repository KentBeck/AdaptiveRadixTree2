// Package art implements an Adaptive Radix Tree: a variable-fanout
// trie that stores []byte keys in byte-wise sorted order.
//
// A Tree[V] is a sorted map from []byte to V. Point operations (Put,
// Get, Delete) are O(k) in the key length, and in-order traversal is
// exposed via Go 1.23 range-over-func iterators.
//
// Keys are ordered lexicographically by their raw bytes; shorter keys
// sort before longer keys that share the shorter key as a prefix. Keys
// passed to Put are copied, so callers may reuse their slices freely.
// A nil key and an empty-slice key are equivalent (both represent the
// empty key); the tree can hold at most one entry at the empty key.
//
// Iteration:
//   - All yields every (key, value) pair in ascending key order.
//   - Range yields every (key, value) pair whose key lies in the
//     half-open interval [start, end). A nil bound is unbounded on
//     that side.
//
// Sorted-map surface:
//   - Min and Max return the smallest and largest entry.
//   - Ceiling and Floor return the successor and predecessor of a
//     target key (inclusive at equality).
//   - Clone returns an independent structural copy of the tree.
//   - Clear removes every entry in O(1).
//
// Goroutine safety: a Tree is not safe for concurrent use by multiple
// goroutines when any goroutine is writing. Callers that need
// concurrent access should guard a Tree with their own sync.RWMutex.
// See the Tree type for the authoritative statement.
//
// Minimal usage:
//
//	t := art.New[int]()
//	t.Put([]byte("apple"), 1)
//	t.Put([]byte("apricot"), 2)
//	t.Put([]byte("banana"), 3)
//
//	if v, ok := t.Get([]byte("apple")); ok {
//		_ = v // 1
//	}
//
//	for k, v := range t.All() {
//		_, _ = k, v // "apple"/1, "apricot"/2, "banana"/3
//	}
//
//	for k, v := range t.Range([]byte("ap"), []byte("b")) {
//		_, _ = k, v // "apple"/1, "apricot"/2
//	}
//
//	t.Delete([]byte("apple"))
//	_ = t.Len() // 2
package art
