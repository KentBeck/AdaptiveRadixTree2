package lazyexpansion

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

func TestPutGetDelete(t *testing.T) {
	tree := New[int]()

	if got, ok := tree.Get([]byte("missing")); ok || got != 0 {
		t.Fatalf("Get on empty = (%d, %v), want (0, false)", got, ok)
	}

	tree.Put([]byte("hello"), 1)
	if got, ok := tree.Get([]byte("hello")); !ok || got != 1 {
		t.Fatalf("Get(hello) = (%d, %v)", got, ok)
	}
	if tree.Len() != 1 {
		t.Fatalf("Len = %d, want 1", tree.Len())
	}

	tree.Put([]byte("hello"), 99)
	if got, _ := tree.Get([]byte("hello")); got != 99 {
		t.Fatalf("Overwrite did not stick: got %d", got)
	}
	if tree.Len() != 1 {
		t.Fatalf("Len after overwrite = %d, want 1", tree.Len())
	}

	tree.Put([]byte("help"), 2)
	tree.Put([]byte("hi"), 3)
	tree.Put([]byte("h"), 4)

	for _, c := range []struct {
		k    string
		want int
	}{{"hello", 99}, {"help", 2}, {"hi", 3}, {"h", 4}} {
		if got, ok := tree.Get([]byte(c.k)); !ok || got != c.want {
			t.Fatalf("Get(%q) = (%d, %v), want (%d, true)", c.k, got, ok, c.want)
		}
	}
	if tree.Len() != 4 {
		t.Fatalf("Len = %d, want 4", tree.Len())
	}

	if !tree.Delete([]byte("help")) {
		t.Fatalf("Delete(help) = false")
	}
	if _, ok := tree.Get([]byte("help")); ok {
		t.Fatalf("help still present after Delete")
	}
	if got, ok := tree.Get([]byte("hello")); !ok || got != 99 {
		t.Fatalf("Get(hello) after Delete(help) = (%d, %v)", got, ok)
	}
	if tree.Len() != 3 {
		t.Fatalf("Len after Delete = %d, want 3", tree.Len())
	}

	if tree.Delete([]byte("missing")) {
		t.Fatalf("Delete(missing) = true")
	}
	if tree.Len() != 3 {
		t.Fatalf("Len after Delete-miss = %d", tree.Len())
	}
}

func TestEmptyKey(t *testing.T) {
	tree := New[int]()
	tree.Put(nil, 7)
	if got, ok := tree.Get(nil); !ok || got != 7 {
		t.Fatalf("Get(nil) = (%d, %v)", got, ok)
	}
	if got, ok := tree.Get([]byte{}); !ok || got != 7 {
		t.Fatalf("Get([]byte{}) = (%d, %v)", got, ok)
	}
	tree.Put([]byte("x"), 8)
	if got, _ := tree.Get(nil); got != 7 {
		t.Fatalf("Get(nil) after extra Put = %d", got)
	}
	if !tree.Delete(nil) {
		t.Fatalf("Delete(nil) = false")
	}
	if _, ok := tree.Get(nil); ok {
		t.Fatalf("Get(nil) after Delete(nil) ok=true")
	}
}

func TestPrefixOfRelationship(t *testing.T) {
	// "hell" is a prefix of "hello": after inserting both, the inner
	// node at path "hell" carries terminal=leaf{"hell"} and
	// children['o']=leaf{"hello"}. Delete then exercises both
	// reshape branches (terminal-clear and child-removal).
	tree := New[int]()
	tree.Put([]byte("hell"), 1)
	tree.Put([]byte("hello"), 2)
	if v, _ := tree.Get([]byte("hell")); v != 1 {
		t.Fatalf("Get(hell) = %d, want 1", v)
	}
	if v, _ := tree.Get([]byte("hello")); v != 2 {
		t.Fatalf("Get(hello) = %d, want 2", v)
	}

	tree.Delete([]byte("hell"))
	if _, ok := tree.Get([]byte("hell")); ok {
		t.Fatalf("hell still present after Delete")
	}
	if v, _ := tree.Get([]byte("hello")); v != 2 {
		t.Fatalf("Get(hello) after deleting prefix = %d, want 2", v)
	}
	if tree.Len() != 1 {
		t.Fatalf("Len = %d, want 1", tree.Len())
	}
}

func TestRangeSorted(t *testing.T) {
	tree := New[int]()
	keys := []string{"hello", "help", "hi", "h", "apple", "april", ""}
	for i, k := range keys {
		tree.Put([]byte(k), i)
	}
	want := append([]string(nil), keys...)
	sort.Strings(want)

	var got []string
	for k := range tree.Range(nil, nil) {
		got = append(got, string(k))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Range order = %v, want %v", got, want)
	}
}

func TestRangeAgainstManyKeys(t *testing.T) {
	tree := New[int]()
	keys := [][]byte{
		{0x10, 0x20}, {0x10, 0x21}, {0x10, 0x10},
		{0xff}, {0x00}, {0x10},
	}
	for i, k := range keys {
		tree.Put(k, i)
	}
	var got [][]byte
	for k := range tree.Range(nil, nil) {
		got = append(got, append([]byte(nil), k...))
	}
	want := append([][]byte(nil), keys...)
	sort.Slice(want, func(i, j int) bool { return bytes.Compare(want[i], want[j]) < 0 })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted output mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestSingleKeyIsLeafOnly(t *testing.T) {
	// Lazy expansion's headline: one key should allocate exactly one
	// leaf and zero inner nodes -- no chain. In chapter 1 the same
	// tree allocated 6 inner node256s.
	tree := New[int]()
	tree.Put([]byte("hello"), 1)
	if got, want := tree.CountInner(), 0; got != want {
		t.Fatalf("CountInner after single Put = %d, want %d", got, want)
	}
	if got, want := tree.CountLeaves(), 1; got != want {
		t.Fatalf("CountLeaves after single Put = %d, want %d", got, want)
	}
}

func TestDivergenceAtSharedPrefix(t *testing.T) {
	// "hello" and "help" share "hel" (3 bytes). The split builds
	// inner nodes at depths 0..2 and a divergence node256 at depth
	// 3 holding both leaves.
	tree := New[int]()
	tree.Put([]byte("hello"), 1)
	tree.Put([]byte("help"), 2)

	if got, want := tree.CountInner(), 4; got != want {
		t.Fatalf("CountInner = %d, want %d", got, want)
	}
	if got, want := tree.CountLeaves(), 2; got != want {
		t.Fatalf("CountLeaves = %d, want %d", got, want)
	}
}

func TestDeleteCollapseChain(t *testing.T) {
	// {hello, help} → delete help. The depth-3 node has 1 leaf
	// child + nil terminal → leaf hoisted. Each parent up the chain
	// then has 1 child + 0 terminal: if that child is a leaf, hoist
	// again. Eventually root is a single leaf{"hello"}.
	tree := New[int]()
	tree.Put([]byte("hello"), 1)
	tree.Put([]byte("help"), 2)
	tree.Delete([]byte("help"))

	if got, want := tree.CountInner(), 0; got != want {
		t.Fatalf("CountInner after collapse = %d, want %d", got, want)
	}
	if got, want := tree.CountLeaves(), 1; got != want {
		t.Fatalf("CountLeaves after collapse = %d, want %d", got, want)
	}
	if v, _ := tree.Get([]byte("hello")); v != 1 {
		t.Fatalf("Get(hello) after collapse = %d", v)
	}
}

func TestDeleteEmptiesTree(t *testing.T) {
	tree := New[int]()
	tree.Put([]byte("only"), 1)
	tree.Delete([]byte("only"))
	if tree.Len() != 0 {
		t.Fatalf("Len after empty = %d", tree.Len())
	}
	if tree.CountInner() != 0 || tree.CountLeaves() != 0 {
		t.Fatalf("nodes left after empty: inner=%d leaves=%d", tree.CountInner(), tree.CountLeaves())
	}
}
