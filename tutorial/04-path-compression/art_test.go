package pathcompression

import (
	"bytes"
	"reflect"
	"sort"
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
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
		t.Fatalf("Overwrite did not stick: got %d", got)
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
	if tree.Len() != 5 {
		t.Fatalf("Len = %d, want 5", tree.Len())
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
	if tree.Len() != 4 {
		t.Fatalf("Len = %d, want 4", tree.Len())
	}
	if tree.Delete([]byte("missing")) {
		t.Fatalf("Delete(missing) = true")
	}
}

func TestEmptyKey(t *testing.T) {
	tree := New[int]()
	tree.Put(nil, 7)
	if got, _ := tree.Get(nil); got != 7 {
		t.Fatalf("Get(nil) = %d, want 7", got)
	}
	if got, _ := tree.Get([]byte{}); got != 7 {
		t.Fatalf("Get([]byte{}) = %d, want 7", got)
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

func TestPrefixOfRelationship(t *testing.T) {
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
		t.Fatalf("Get(hello) after deleting prefix = %d", v)
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

func TestRangeByteOrderingNotASCII(t *testing.T) {
	// Keys with byte values outside printable ASCII -- the trie must
	// still iterate in raw byte-wise order, not collation order.
	tree := New[int]()
	keys := [][]byte{{0xff, 0x10}, {0x10, 0x20}, {0x10, 0x10}, {0x00}}
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
		t.Fatalf("ordering wrong:\n got %v\nwant %v", got, want)
	}
}

func TestSplitTwoLeavesUsesSinglePrefix(t *testing.T) {
	// Inserting two URL-shaped keys that share 33 bytes used to cost
	// 33 inner node256s in chapter 3; with path compression it is
	// exactly one parent carrying a 33-byte prefix.
	tree := New[int]()
	tree.Put([]byte("https://api.example.com/v1/users/a"), 1)
	tree.Put([]byte("https://api.example.com/v1/users/b"), 2)
	if got, want := tree.CountInner(), 1; got != want {
		t.Fatalf("CountInner = %d, want %d", got, want)
	}
	if got, want := tree.PrefixBytes(), 33; got != want {
		t.Fatalf("PrefixBytes = %d, want %d", got, want)
	}
}

func TestSplitPrefixedNode(t *testing.T) {
	// Build a tree whose root has prefix "common/". Then insert a
	// key sharing only "comm" -- the root must split: new parent
	// with prefix "comm", and the old root demoted to prefix "n/".
	tree := New[int]()
	tree.Put([]byte("common/a"), 1)
	tree.Put([]byte("common/b"), 2)
	// At this point the root has prefix "common/" and two leaf children.
	tree.Put([]byte("commercial"), 3)
	// New shape: root prefix "comm", children['o'] = old root with
	// prefix "n/", children['e'] = leaf "commercial".

	for k, v := range map[string]int{
		"common/a":   1,
		"common/b":   2,
		"commercial": 3,
	} {
		if got, ok := tree.Get([]byte(k)); !ok || got != v {
			t.Fatalf("Get(%q) = (%d, %v), want (%d, true)", k, got, ok, v)
		}
	}
}

func TestSplitWithKeyEndingMidPrefix(t *testing.T) {
	tree := New[int]()
	tree.Put([]byte("application"), 1)
	tree.Put([]byte("appliance"), 2)
	// Both share "appli" (5 bytes); the root holds that prefix.
	tree.Put([]byte("app"), 3)
	// "app" matches the first 3 bytes of root's prefix and runs out.
	// Root must split: new parent prefix "app", terminal=leaf{"app"},
	// children['l'] = old root with prefix "i".

	for k, v := range map[string]int{
		"application": 1,
		"appliance":   2,
		"app":         3,
	} {
		if got, _ := tree.Get([]byte(k)); got != v {
			t.Fatalf("Get(%q) = %d, want %d", k, got, v)
		}
	}
}

func TestDeleteMergesPrefixChain(t *testing.T) {
	// The new chapter-3 collapse: when an inner node has 1 inner
	// child and no terminal, merge the child's prefix in.
	tree := New[int]()
	tree.Put([]byte("common/aaa"), 1)
	tree.Put([]byte("common/bbb"), 2)
	tree.Put([]byte("commercial"), 3)
	// Tree shape after these puts:
	//   root prefix "comm"
	//     children['e'] = leaf "commercial"
	//     children['o'] = node prefix "n/" with two leaves
	tree.Delete([]byte("commercial"))
	// Now root has 1 child (children['o']) and no terminal: must
	// merge. New root has prefix "comm" + 'o' + "n/" = "common/" and
	// the same two leaves.
	if got, want := tree.CountInner(), 1; got != want {
		t.Fatalf("CountInner after merge = %d, want %d", got, want)
	}
	for k, v := range map[string]int{
		"common/aaa": 1,
		"common/bbb": 2,
	} {
		if got, _ := tree.Get([]byte(k)); got != v {
			t.Fatalf("Get(%q) = %d, want %d", k, got, v)
		}
	}
}

// Tree[int] satisfies harness.SortedMap as-is; no adapter needed.
func factory() harness.Factory {
	return func() harness.SortedMap { return New[int]() }
}

func TestAcceptance(t *testing.T) {
	harness.RunAcceptance(t, factory())
}
