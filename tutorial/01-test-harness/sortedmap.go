// Package harness defines a SortedMap interface plus diff/regression
// runners and a 100 MB capacity probe. Each tutorial chapter plugs
// its Tree[int] into the harness via a small adapter and inherits
// the same correctness suite. The harness ships with two reference
// adapters of its own -- google/btree and a sorted map[string]int --
// so its self-tests exercise the runners end-to-end.
package harness

import (
	"bytes"
	"iter"
	"sort"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
	"github.com/google/btree"
)

// SortedMap is the surface every implementation under test (the
// chapter's Tree[int], google/btree, map[string]int wrapper, ...)
// satisfies. Values are int to keep diff-testing trivial.
type SortedMap interface {
	Put(key []byte, value int)
	Get(key []byte) (int, bool)
	Delete(key []byte) bool
	Len() int
	Range(from, to []byte) iter.Seq2[[]byte, int]
}

// Factory builds a fresh empty SortedMap. Used by the diff/regression
// runners so each scenario starts from a clean state.
type Factory func() SortedMap

// NamedFactory attaches a human label to a Factory for test output.
type NamedFactory struct {
	Name string
	New  Factory
}

// BTreeAdapter wraps google/btree behind the SortedMap interface.
// Range translates nil bounds into the appropriate Ascend variant
// so callers can use nil/nil for "everything".
type BTreeAdapter struct {
	t *btree.BTreeG[bench.BtreeItem]
	n int
}

// NewBTree returns an empty SortedMap backed by google/btree.
func NewBTree() SortedMap { return &BTreeAdapter{t: bench.NewBtree()} }

// BTreeFactory returns a Factory that produces fresh BTreeAdapters.
func BTreeFactory() Factory { return func() SortedMap { return NewBTree() } }

func (b *BTreeAdapter) Put(key []byte, value int) {
	k := append([]byte(nil), key...)
	if _, replaced := b.t.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: value}); !replaced {
		b.n++
	}
}

func (b *BTreeAdapter) Get(key []byte) (int, bool) {
	it, ok := b.t.Get(bench.BtreeItem{Key: key})
	if !ok {
		return 0, false
	}
	return it.Val, true
}

func (b *BTreeAdapter) Delete(key []byte) bool {
	if _, ok := b.t.Delete(bench.BtreeItem{Key: key}); ok {
		b.n--
		return true
	}
	return false
}

func (b *BTreeAdapter) Len() int { return b.n }

func (b *BTreeAdapter) Range(from, to []byte) iter.Seq2[[]byte, int] {
	return func(yield func([]byte, int) bool) {
		iter := func(it bench.BtreeItem) bool { return yield(it.Key, it.Val) }
		switch {
		case from == nil && to == nil:
			b.t.Ascend(iter)
		case from == nil:
			b.t.AscendLessThan(bench.BtreeItem{Key: to}, iter)
		case to == nil:
			b.t.AscendGreaterOrEqual(bench.BtreeItem{Key: from}, iter)
		default:
			if bytes.Compare(from, to) >= 0 {
				return
			}
			b.t.AscendRange(bench.BtreeItem{Key: from}, bench.BtreeItem{Key: to}, iter)
		}
	}
}

// MapAdapter wraps a map[string]int. Range materializes a sorted
// key slice on every call -- slow, but obviously correct, which is
// exactly what we want from a reference implementation.
type MapAdapter struct {
	m map[string]int
}

// NewMap returns an empty SortedMap backed by map[string]int.
func NewMap() SortedMap { return &MapAdapter{m: make(map[string]int)} }

// MapFactory returns a Factory that produces fresh MapAdapters.
func MapFactory() Factory { return func() SortedMap { return NewMap() } }

func (a *MapAdapter) Put(key []byte, value int) { a.m[string(key)] = value }

func (a *MapAdapter) Get(key []byte) (int, bool) {
	v, ok := a.m[string(key)]
	return v, ok
}

func (a *MapAdapter) Delete(key []byte) bool {
	k := string(key)
	if _, ok := a.m[k]; !ok {
		return false
	}
	delete(a.m, k)
	return true
}

func (a *MapAdapter) Len() int { return len(a.m) }

func (a *MapAdapter) Range(from, to []byte) iter.Seq2[[]byte, int] {
	return func(yield func([]byte, int) bool) {
		keys := make([]string, 0, len(a.m))
		for k := range a.m {
			kb := []byte(k)
			if from != nil && bytes.Compare(kb, from) < 0 {
				continue
			}
			if to != nil && bytes.Compare(kb, to) >= 0 {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !yield([]byte(k), a.m[k]) {
				return
			}
		}
	}
}
