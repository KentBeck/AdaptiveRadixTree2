package lazyexpansion

import (
	"iter"
	"testing"

	harness "github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// chapterAdapter wraps Tree[int] so it satisfies harness.SortedMap.
type chapterAdapter struct{ t *Tree[int] }

func (a chapterAdapter) Put(k []byte, v int)      { a.t.Put(k, v) }
func (a chapterAdapter) Get(k []byte) (int, bool) { return a.t.Get(k) }
func (a chapterAdapter) Delete(k []byte) bool     { return a.t.Delete(k) }
func (a chapterAdapter) Len() int                 { return a.t.Len() }
func (a chapterAdapter) Range(from, to []byte) iter.Seq2[[]byte, int] {
	return a.t.Range(from, to)
}

func factory() harness.Factory {
	return func() harness.SortedMap { return chapterAdapter{t: New[int]()} }
}

// --- harness-driven ---

func TestRegression(t *testing.T) {
	harness.RunRegression(t, factory(), harness.BTreeFactory())
}

func TestRandomDiff(t *testing.T) {
	ops := harness.RandomTraceForT(t, harness.RandomConfig{})
	cand := factory()()
	ref := harness.NewBTree()
	harness.RunDiff(t, cand, ref, ops)
}

// --- chapter-specific structural ---

func TestSingleKeyIsLeafOnly(t *testing.T) {
	// Lazy expansion's headline: one key allocates exactly one leaf
	// and zero inner nodes. Chapter 2 allocated 6 inner node256s for
	// the same input.
	tr := New[int]()
	tr.Put([]byte("hello"), 1)
	if got, want := tr.CountInner(), 0; got != want {
		t.Fatalf("CountInner after single Put = %d, want %d", got, want)
	}
	if got, want := tr.CountLeaves(), 1; got != want {
		t.Fatalf("CountLeaves after single Put = %d, want %d", got, want)
	}
}

func TestDivergenceAtSharedPrefix(t *testing.T) {
	// "hello" and "help" share "hel" (3 bytes). Without path
	// compression the split builds inner nodes at depths 0..2 plus
	// a divergence node256 at depth 3 holding both leaves.
	tr := New[int]()
	tr.Put([]byte("hello"), 1)
	tr.Put([]byte("help"), 2)
	if got, want := tr.CountInner(), 4; got != want {
		t.Fatalf("CountInner = %d, want %d", got, want)
	}
	if got, want := tr.CountLeaves(), 2; got != want {
		t.Fatalf("CountLeaves = %d, want %d", got, want)
	}
}

func TestDeleteCollapseChain(t *testing.T) {
	// {hello, help} then delete help. Each ancestor up the chain
	// then has exactly one leaf child and no terminal, so the leaf
	// hoists all the way to the root.
	tr := New[int]()
	tr.Put([]byte("hello"), 1)
	tr.Put([]byte("help"), 2)
	tr.Delete([]byte("help"))
	if got, want := tr.CountInner(), 0; got != want {
		t.Fatalf("CountInner after collapse = %d, want %d", got, want)
	}
	if got, want := tr.CountLeaves(), 1; got != want {
		t.Fatalf("CountLeaves after collapse = %d, want %d", got, want)
	}
	if v, _ := tr.Get([]byte("hello")); v != 1 {
		t.Fatalf("Get(hello) after collapse = %d", v)
	}
}

func TestDeleteEmptiesTree(t *testing.T) {
	tr := New[int]()
	tr.Put([]byte("only"), 1)
	tr.Delete([]byte("only"))
	if tr.Len() != 0 {
		t.Fatalf("Len after empty = %d", tr.Len())
	}
	if tr.CountInner() != 0 || tr.CountLeaves() != 0 {
		t.Fatalf("nodes left after empty: inner=%d leaves=%d", tr.CountInner(), tr.CountLeaves())
	}
}
