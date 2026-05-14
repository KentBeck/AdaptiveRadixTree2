package nodeonly256

import (
	"iter"
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
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

func TestSinglePutAllocatesKplusOneNodes(t *testing.T) {
	// "hello" with no other keys allocates exactly 6 inner nodes
	// (root + one per byte of "hello"). The disaster baseline.
	tr := New[int]()
	tr.Put([]byte("hello"), 1)
	if got, want := tr.CountNodes(), 6; got != want {
		t.Fatalf("CountNodes = %d, want %d", got, want)
	}
}

func TestDeletePrunesAllTheWay(t *testing.T) {
	tr := New[int]()
	tr.Put([]byte("hello"), 1)
	tr.Delete([]byte("hello"))
	if got := tr.CountNodes(); got != 0 {
		t.Fatalf("CountNodes after Delete = %d, want 0 (full prune)", got)
	}
	if tr.Len() != 0 {
		t.Fatalf("Len after Delete = %d, want 0", tr.Len())
	}
	tr2 := New[int]()
	tr2.Put([]byte("hello"), 1)
	tr2.Delete([]byte("hello"))
	if tr2.root != nil {
		t.Fatalf("root not nil after full prune")
	}
}

func TestSingleByteFanout(t *testing.T) {
	// 256 single-byte keys: root plus one terminus per key = 257 nodes.
	tr := New[int]()
	for b := 0; b < 256; b++ {
		tr.Put([]byte{byte(b)}, b)
	}
	if got, want := tr.CountNodes(), 257; got != want {
		t.Fatalf("CountNodes = %d, want %d", got, want)
	}
}
