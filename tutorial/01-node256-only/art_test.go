package nodeonly256

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

func TestPutGetDelete(t *testing.T) {
	tree := New[int]()

	// Empty.
	if got, ok := tree.Get([]byte("missing")); ok || got != 0 {
		t.Fatalf("Get on empty = (%d, %v), want (0, false)", got, ok)
	}
	if tree.Len() != 0 {
		t.Fatalf("Len on empty = %d, want 0", tree.Len())
	}

	// Single Put / Get.
	tree.Put([]byte("hello"), 1)
	if got, ok := tree.Get([]byte("hello")); !ok || got != 1 {
		t.Fatalf("Get(hello) = (%d, %v), want (1, true)", got, ok)
	}
	if tree.Len() != 1 {
		t.Fatalf("Len after one Put = %d, want 1", tree.Len())
	}

	// Overwrite.
	tree.Put([]byte("hello"), 99)
	if got, _ := tree.Get([]byte("hello")); got != 99 {
		t.Fatalf("Get after overwrite = %d, want 99", got)
	}
	if tree.Len() != 1 {
		t.Fatalf("Len after overwrite = %d, want 1", tree.Len())
	}

	// Sibling and prefix-of relationship.
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

	// Delete leaves the rest intact.
	if !tree.Delete([]byte("help")) {
		t.Fatalf("Delete(help) = false, want true")
	}
	if _, ok := tree.Get([]byte("help")); ok {
		t.Fatalf("Get(help) after Delete returned ok=true")
	}
	if got, ok := tree.Get([]byte("hello")); !ok || got != 99 {
		t.Fatalf("Get(hello) after Delete(help) = (%d, %v)", got, ok)
	}
	if tree.Len() != 3 {
		t.Fatalf("Len after one Delete = %d, want 3", tree.Len())
	}

	// Delete missing returns false and does not change Len.
	if tree.Delete([]byte("missing")) {
		t.Fatalf("Delete(missing) = true, want false")
	}
	if tree.Len() != 3 {
		t.Fatalf("Len after Delete-miss = %d, want 3", tree.Len())
	}
}

func TestEmptyKey(t *testing.T) {
	tree := New[int]()
	tree.Put(nil, 7)
	if got, ok := tree.Get(nil); !ok || got != 7 {
		t.Fatalf("Get(nil) = (%d, %v), want (7, true)", got, ok)
	}
	if got, ok := tree.Get([]byte{}); !ok || got != 7 {
		t.Fatalf("Get([]byte{}) = (%d, %v), want (7, true)", got, ok)
	}
	tree.Put([]byte("x"), 8)
	if got, _ := tree.Get(nil); got != 7 {
		t.Fatalf("Get(nil) after extra Put = %d, want 7", got)
	}
	if !tree.Delete(nil) {
		t.Fatalf("Delete(nil) = false, want true")
	}
	if _, ok := tree.Get(nil); ok {
		t.Fatalf("Get(nil) after Delete(nil) returned ok=true")
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

func TestAllAgainstManyKeys(t *testing.T) {
	tree := New[int]()
	keys := [][]byte{
		{0x10, 0x20}, {0x10, 0x21}, {0x10, 0x10},
		{0xff}, {0x00}, {0x10},
	}
	for i, k := range keys {
		tree.Put(k, i)
	}
	var got [][]byte
	for k := range tree.All() {
		got = append(got, append([]byte(nil), k...))
	}
	want := append([][]byte(nil), keys...)
	sort.Slice(want, func(i, j int) bool { return bytes.Compare(want[i], want[j]) < 0 })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted output mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestDeletePrunesEmptyChains(t *testing.T) {
	// "hello" with no other keys allocates exactly one chain of 5
	// node256s plus the root = 6 inner nodes. Deleting it must
	// prune all the way back to an empty tree.
	tree := New[int]()
	tree.Put([]byte("hello"), 1)
	if got, want := tree.CountNodes(), 6; got != want {
		t.Fatalf("CountNodes after single Put = %d, want %d", got, want)
	}
	tree.Delete([]byte("hello"))
	if got := tree.CountNodes(); got != 0 {
		t.Fatalf("CountNodes after Delete = %d, want 0 (empty trie)", got)
	}
	if tree.Len() != 0 {
		t.Fatalf("Len after Delete = %d, want 0", tree.Len())
	}
}
