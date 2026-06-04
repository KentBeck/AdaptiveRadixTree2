package nodeonly256

import (
	"iter"
	"reflect"
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

// --- sorted-map behavior ---

func TestSortedMapPutGetOverwrite(t *testing.T) {
	tr := New[int]()
	tr.Put([]byte("hello"), 1)
	tr.Put([]byte("help"), 2)
	tr.Put([]byte("hello"), 3)

	assertLen(t, tr, 2)
	assertGet(t, tr, "hello", 3)
	assertMissing(t, tr, "missing")
}

func TestSortedMapDelete(t *testing.T) {
	tr := New[int]()
	tr.Put([]byte("hello"), 1)
	tr.Put([]byte("help"), 2)

	assertDelete(t, tr, "hello", true)
	assertDelete(t, tr, "hello", false)
	assertMissing(t, tr, "hello")
	assertGet(t, tr, "help", 2)
	assertLen(t, tr, 1)
}

func TestSortedMapRangeAllYieldsByteSortedKeys(t *testing.T) {
	tr := New[int]()
	for i, key := range []string{"hi", "hello", "help", "apple", "app"} {
		tr.Put([]byte(key), i)
	}

	assertRangeKeys(t, tr, nil, nil, "app", "apple", "hello", "help", "hi")
}

func TestSortedMapRangeFromToIsHalfOpen(t *testing.T) {
	tr := New[int]()
	for i, key := range []string{"app", "apple", "hello", "help", "hi", "z"} {
		tr.Put([]byte(key), i)
	}

	assertRangeKeys(t, tr, []byte("hello"), []byte("z"), "hello", "help", "hi")
}

// --- chapter-specific structural ---

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

func rangeKeys(tr *Tree[int], from, to []byte) []string {
	var keys []string
	for key := range tr.Range(from, to) {
		keys = append(keys, string(key))
	}
	return keys
}

func assertLen(t *testing.T, tr *Tree[int], want int) {
	t.Helper()
	if got := tr.Len(); got != want {
		t.Fatalf("Len = %d, want %d", got, want)
	}
}

func assertGet(t *testing.T, tr *Tree[int], key string, want int) {
	t.Helper()
	if got, ok := tr.Get([]byte(key)); !ok || got != want {
		t.Fatalf("Get(%q) = (%d, %v), want (%d, true)", key, got, ok, want)
	}
}

func assertMissing(t *testing.T, tr *Tree[int], key string) {
	t.Helper()
	if got, ok := tr.Get([]byte(key)); ok || got != 0 {
		t.Fatalf("Get(%q) = (%d, %v), want (0, false)", key, got, ok)
	}
}

func assertDelete(t *testing.T, tr *Tree[int], key string, want bool) {
	t.Helper()
	if got := tr.Delete([]byte(key)); got != want {
		t.Fatalf("Delete(%q) = %v, want %v", key, got, want)
	}
}

func assertRangeKeys(t *testing.T, tr *Tree[int], from, to []byte, want ...string) {
	t.Helper()
	if got := rangeKeys(tr, from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range(%q, %q) keys = %v, want %v", from, to, got, want)
	}
}
