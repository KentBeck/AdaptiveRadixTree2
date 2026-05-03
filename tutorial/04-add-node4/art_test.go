package addnode4

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

func TestPutGetDelete(t *testing.T) {
	tree := New[int]()

	if got, ok := tree.Get([]byte("missing")); ok || got != 0 {
		t.Fatalf("Get on empty = (%d, %v)", got, ok)
	}

	tree.Put([]byte("hello"), 1)
	if got, _ := tree.Get([]byte("hello")); got != 1 {
		t.Fatalf("Get(hello) = %d, want 1", got)
	}
	tree.Put([]byte("hello"), 99)
	if got, _ := tree.Get([]byte("hello")); got != 99 {
		t.Fatalf("Overwrite did not stick")
	}

	tree.Put([]byte("help"), 2)
	tree.Put([]byte("hi"), 3)
	tree.Put([]byte("h"), 4)
	tree.Put([]byte(""), 5)

	for _, c := range []struct {
		k    string
		want int
	}{{"hello", 99}, {"help", 2}, {"hi", 3}, {"h", 4}, {"", 5}} {
		if got, ok := tree.Get([]byte(c.k)); !ok || got != c.want {
			t.Fatalf("Get(%q) = (%d, %v), want (%d, true)", c.k, got, ok, c.want)
		}
	}

	if !tree.Delete([]byte("help")) {
		t.Fatalf("Delete(help) = false")
	}
	if _, ok := tree.Get([]byte("help")); ok {
		t.Fatalf("help still present after Delete")
	}
	if tree.Len() != 4 {
		t.Fatalf("Len = %d, want 4", tree.Len())
	}
}

func TestEmptyKey(t *testing.T) {
	tree := New[int]()
	tree.Put(nil, 7)
	if got, _ := tree.Get(nil); got != 7 {
		t.Fatalf("Get(nil) = %d, want 7", got)
	}
	if got, _ := tree.Get([]byte{}); got != 7 {
		t.Fatalf("Get([]byte{}) = %d", got)
	}
	tree.Put([]byte("x"), 8)
	if got, _ := tree.Get(nil); got != 7 {
		t.Fatalf("Get(nil) after extra Put = %d", got)
	}
	if !tree.Delete(nil) {
		t.Fatalf("Delete(nil) = false")
	}
	if _, ok := tree.Get(nil); ok {
		t.Fatalf("Get(nil) after Delete(nil) = ok")
	}
}

func TestAllSorted(t *testing.T) {
	tree := New[int]()
	keys := []string{"hello", "help", "hi", "h", "apple", "april", ""}
	for i, k := range keys {
		tree.Put([]byte(k), i)
	}
	want := append([]string(nil), keys...)
	sort.Strings(want)
	var got []string
	for k := range tree.All() {
		got = append(got, string(k))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("All order = %v, want %v", got, want)
	}
}

func TestSmallTreeIsNode4(t *testing.T) {
	// Two keys that share a prefix should sit under a single node4
	// (not a node256), since the parent has only two children.
	tree := New[int]()
	tree.Put([]byte("hello"), 1)
	tree.Put([]byte("help"), 2)
	n4, n256 := tree.CountByKind()
	if n4 != 1 || n256 != 0 {
		t.Fatalf("CountByKind = (%d, %d), want (1, 0)", n4, n256)
	}
}

func TestPromotesAtFifthChild(t *testing.T) {
	// 4 distinct first bytes fit in a node4. Adding a 5th must
	// promote the root to node256.
	tree := New[int]()
	for _, b := range []byte{'a', 'b', 'c', 'd'} {
		tree.Put([]byte{b}, int(b))
	}
	n4, n256 := tree.CountByKind()
	if n4 != 1 || n256 != 0 {
		t.Fatalf("after 4 keys: (%d, %d), want (1, 0)", n4, n256)
	}
	tree.Put([]byte{'e'}, int('e'))
	n4, n256 = tree.CountByKind()
	if n4 != 0 || n256 != 1 {
		t.Fatalf("after 5 keys: (%d, %d), want (0, 1)", n4, n256)
	}
	for _, b := range []byte{'a', 'b', 'c', 'd', 'e'} {
		if v, ok := tree.Get([]byte{b}); !ok || v != int(b) {
			t.Fatalf("Get(%c) = (%d, %v), want (%d, true)", b, v, ok, b)
		}
	}
}

func TestDemotesAtFourthChild(t *testing.T) {
	// Build a node256, then delete down to <=4 children. The
	// remaining root should demote back to node4.
	tree := New[int]()
	for _, b := range []byte{'a', 'b', 'c', 'd', 'e'} {
		tree.Put([]byte{b}, int(b))
	}
	if _, n256 := tree.CountByKind(); n256 != 1 {
		t.Fatalf("expected node256 root, got node256 count = %d", n256)
	}
	tree.Delete([]byte{'e'})
	n4, n256 := tree.CountByKind()
	if n4 != 1 || n256 != 0 {
		t.Fatalf("after demotion: (%d, %d), want (1, 0)", n4, n256)
	}
	for _, b := range []byte{'a', 'b', 'c', 'd'} {
		if _, ok := tree.Get([]byte{b}); !ok {
			t.Fatalf("lost key %c after demotion", b)
		}
	}
}

func TestDemotionPreservesSortOrder(t *testing.T) {
	// node256 is byte-indexed so its iteration is naturally sorted.
	// node4 stores keys in an array that *must* also be sorted to
	// preserve All's invariant. Demotion has to keep the sort.
	tree := New[int]()
	for _, b := range []byte{0x80, 0x10, 0xff, 0x40, 0x20} {
		tree.Put([]byte{b}, int(b))
	}
	tree.Delete([]byte{0xff}) // drop one to fall to 4
	if _, n256 := tree.CountByKind(); n256 != 0 {
		t.Fatalf("demotion did not happen")
	}
	var got []byte
	for k := range tree.All() {
		got = append(got, k[0])
	}
	want := []byte{0x10, 0x20, 0x40, 0x80}
	if !bytes.Equal(got, want) {
		t.Fatalf("All order after demotion = %v, want %v", got, want)
	}
}

func TestNode4SortedInsertion(t *testing.T) {
	// Insert 4 keys in scrambled order; All must yield them sorted.
	tree := New[int]()
	for _, b := range []byte{0x40, 0x10, 0x80, 0x20} {
		tree.Put([]byte{b}, int(b))
	}
	var got []byte
	for k := range tree.All() {
		got = append(got, k[0])
	}
	want := []byte{0x10, 0x20, 0x40, 0x80}
	if !bytes.Equal(got, want) {
		t.Fatalf("All on scrambled inserts = %v, want %v", got, want)
	}
}

func TestPrefixSplitMergeCycle(t *testing.T) {
	// Same scenario as chapter 3's prefix-merge test, on the new
	// node4-rooted tree.
	tree := New[int]()
	tree.Put([]byte("common/aaa"), 1)
	tree.Put([]byte("common/bbb"), 2)
	tree.Put([]byte("commercial"), 3)
	tree.Delete([]byte("commercial"))
	for k, v := range map[string]int{
		"common/aaa": 1,
		"common/bbb": 2,
	} {
		if got, _ := tree.Get([]byte(k)); got != v {
			t.Fatalf("Get(%q) = %d, want %d", k, got, v)
		}
	}
	// After the merge, everything should fit in a single node4.
	if got, want := tree.CountInner(), 1; got != want {
		t.Fatalf("CountInner after merge = %d, want %d", got, want)
	}
}
