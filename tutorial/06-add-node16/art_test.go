package addnode16

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

// TestInnerNodeInterface pins that every inner-node concrete type
// satisfies the innerNode interface -- including the new node16.
// Chapter 7 will extend this list with *node48.
func TestInnerNodeInterface(t *testing.T) {
	var _ innerNode = (*node4[int])(nil)
	var _ innerNode = (*node16[int])(nil)
	var _ innerNode = (*node256[int])(nil)
}

func TestPutGetDelete(t *testing.T) {
	tree := New[int]()
	if got, ok := tree.Get([]byte("missing")); ok || got != 0 {
		t.Fatalf("Get on empty = (%d, %v)", got, ok)
	}
	tree.Put([]byte("hello"), 1)
	tree.Put([]byte("hello"), 99)
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

// Promotion ladder: 4 children -> node4, 5 -> node16, 17 -> node256.
func TestPromotesNode4ToNode16ToNode256(t *testing.T) {
	tree := New[int]()
	for i := 0; i < 4; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	n4, n16, n256 := tree.CountByKind()
	if n4 != 1 || n16 != 0 || n256 != 0 {
		t.Fatalf("after 4 keys: (n4=%d n16=%d n256=%d), want (1,0,0)", n4, n16, n256)
	}

	tree.Put([]byte{4}, 4) // 5th distinct first byte -> node4 promotes to node16
	n4, n16, n256 = tree.CountByKind()
	if n4 != 0 || n16 != 1 || n256 != 0 {
		t.Fatalf("after 5 keys: (n4=%d n16=%d n256=%d), want (0,1,0)", n4, n16, n256)
	}

	for i := 5; i < 16; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	n4, n16, n256 = tree.CountByKind()
	if n4 != 0 || n16 != 1 || n256 != 0 {
		t.Fatalf("after 16 keys: (n4=%d n16=%d n256=%d), want (0,1,0)", n4, n16, n256)
	}

	tree.Put([]byte{16}, 16) // 17th -> node16 promotes to node256
	n4, n16, n256 = tree.CountByKind()
	if n4 != 0 || n16 != 0 || n256 != 1 {
		t.Fatalf("after 17 keys: (n4=%d n16=%d n256=%d), want (0,0,1)", n4, n16, n256)
	}
}

// Demotion ladder: node256 with <=16 -> node16; node16 with <=4 -> node4.
func TestDemotesNode256ToNode16ToNode4(t *testing.T) {
	tree := New[int]()
	for i := 0; i < 17; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	if _, _, n256 := tree.CountByKind(); n256 != 1 {
		t.Fatalf("setup wanted 1 node256, got %d", n256)
	}
	tree.Delete([]byte{16}) // back to 16 children -> demote 256 -> 16
	n4, n16, n256 := tree.CountByKind()
	if n4 != 0 || n16 != 1 || n256 != 0 {
		t.Fatalf("after first demotion: (n4=%d n16=%d n256=%d), want (0,1,0)", n4, n16, n256)
	}

	for i := 5; i < 16; i++ {
		tree.Delete([]byte{byte(i)})
	}
	n4, n16, n256 = tree.CountByKind()
	if n4 != 0 || n16 != 1 || n256 != 0 {
		t.Fatalf("at 5 children: (n4=%d n16=%d n256=%d), want (0,1,0)", n4, n16, n256)
	}

	tree.Delete([]byte{4}) // back to 4 children -> demote 16 -> 4
	n4, n16, n256 = tree.CountByKind()
	if n4 != 1 || n16 != 0 || n256 != 0 {
		t.Fatalf("at 4 children: (n4=%d n16=%d n256=%d), want (1,0,0)", n4, n16, n256)
	}
}

// Demotion preserves the sorted-keys invariant on both stages of
// the ladder: node256 -> node16 walks ascending byte order, and
// node16 -> node4 already has sorted keys.
func TestDemotionPreservesSortOrder(t *testing.T) {
	tree := New[int]()
	bytes := []byte{0x80, 0x10, 0xff, 0x40, 0x20, 0x60, 0xa0, 0x90,
		0x30, 0x50, 0x70, 0xc0, 0xd0, 0xe0, 0x05, 0x15, 0x25}
	for _, b := range bytes {
		tree.Put([]byte{b}, int(b))
	}
	if _, _, n256 := tree.CountByKind(); n256 != 1 {
		t.Fatalf("setup wanted 1 node256")
	}
	tree.Delete([]byte{0xff})
	tree.Delete([]byte{0xe0})

	if _, _, n256 := tree.CountByKind(); n256 != 0 {
		t.Fatalf("expected demotion to node16")
	}
	var got []byte
	for k := range tree.All() {
		got = append(got, k[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("All order broken at %d: %v", i, got)
		}
	}
}

func TestNode16SortedInsertion(t *testing.T) {
	// Insert keys that fit in a node16 in scrambled order; All
	// must still yield sorted.
	tree := New[int]()
	for _, b := range []byte{0x40, 0x10, 0x80, 0x20, 0x60, 0x05, 0xa0, 0x70, 0xb0} {
		tree.Put([]byte{b}, int(b))
	}
	if _, n16, _ := tree.CountByKind(); n16 != 1 {
		t.Fatalf("expected node16 root")
	}
	var got []byte
	for k := range tree.All() {
		got = append(got, k[0])
	}
	want := []byte{0x05, 0x10, 0x20, 0x40, 0x60, 0x70, 0x80, 0xa0, 0xb0}
	if !bytes.Equal(got, want) {
		t.Fatalf("All order = %v, want %v", got, want)
	}
}

func TestPrefixSplitMergeCycle(t *testing.T) {
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
}
