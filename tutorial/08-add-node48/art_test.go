package addnode48

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

// TestInnerNodeInterface pins that every inner-node type satisfies
// the innerNode interface, including the new node48.
func TestInnerNodeInterface(t *testing.T) {
	var _ innerNode = (*node4[int])(nil)
	var _ innerNode = (*node16[int])(nil)
	var _ innerNode = (*node48[int])(nil)
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

// Promotion ladder: 4 -> n4, 5 -> n16, 17 -> n48, 49 -> n256.
func TestPromotionLadder(t *testing.T) {
	tree := New[int]()
	for i := 0; i < 4; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	if n4, _, _, _ := tree.CountByKind(); n4 != 1 {
		t.Fatalf("after 4: want 1 n4, got %d", n4)
	}
	tree.Put([]byte{4}, 4)
	if _, n16, _, _ := tree.CountByKind(); n16 != 1 {
		t.Fatalf("after 5: want 1 n16")
	}
	for i := 5; i < 17; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	if _, _, n48, _ := tree.CountByKind(); n48 != 1 {
		t.Fatalf("after 17: want 1 n48")
	}
	for i := 17; i < 49; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	if _, _, _, n256 := tree.CountByKind(); n256 != 1 {
		t.Fatalf("after 49: want 1 n256")
	}
}

// Demotion ladder: n256 with <=48 -> n48; n48 with <=16 -> n16;
// n16 with <=4 -> n4.
func TestDemotionLadder(t *testing.T) {
	tree := New[int]()
	for i := 0; i < 49; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	if _, _, _, n256 := tree.CountByKind(); n256 != 1 {
		t.Fatalf("setup wanted 1 n256, got %d", n256)
	}
	tree.Delete([]byte{48}) // 48 children -> demote to n48
	if _, _, n48, n256 := tree.CountByKind(); n48 != 1 || n256 != 0 {
		t.Fatalf("expected n48 after first demotion, got n48=%d n256=%d", n48, n256)
	}

	for i := 16; i <= 47; i++ {
		tree.Delete([]byte{byte(i)})
	}
	// Tree now has bytes 0..15, but we just deleted byte 16 too,
	// so 16 live children. n48 demotes to n16 at 16.
	if _, n16, n48, _ := tree.CountByKind(); n16 != 1 || n48 != 0 {
		t.Fatalf("expected n16 at 16 children, got n16=%d n48=%d", n16, n48)
	}

	for i := 5; i <= 15; i++ {
		tree.Delete([]byte{byte(i)})
	}
	tree.Delete([]byte{4})
	if n4, n16, _, _ := tree.CountByKind(); n4 != 1 || n16 != 0 {
		t.Fatalf("expected n4 at 4 children, got n4=%d n16=%d", n4, n16)
	}
}

// Demotion preserves sort order at every step. node256 -> node48,
// node48 -> node16, node16 -> node4 each have to copy in
// ascending edge-byte order.
func TestDemotionPreservesSortOrder(t *testing.T) {
	tree := New[int]()
	for i := 0; i < 49; i++ {
		tree.Put([]byte{byte(i * 5)}, i*5) // step by 5 to scramble ordering
	}
	tree.Delete([]byte{0}) // demote n256 -> n48
	if _, _, n48, _ := tree.CountByKind(); n48 != 1 {
		t.Fatalf("expected n48 after demotion")
	}
	var got []byte
	for k := range tree.Range(nil, nil) {
		got = append(got, k[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("Range order broken at %d after n48 demotion: %v", i, got)
		}
	}
}

// node48 removeChild swaps the last live child into the freed slot
// to keep children[:numChildren] dense. Test that addChild after
// removeChild correctly reuses the slot and that lookups still
// work for the swapped-in child.
func TestNode48RemoveChildSwap(t *testing.T) {
	tree := New[int]()
	for i := 0; i < 17; i++ {
		tree.Put([]byte{byte(i * 10)}, i)
	}
	if _, _, n48, _ := tree.CountByKind(); n48 != 1 {
		t.Fatalf("expected node48")
	}
	// Remove a middle key; check that the remaining keys are all
	// still findable.
	tree.Delete([]byte{50})
	for i := 0; i < 17; i++ {
		want := i
		k := []byte{byte(i * 10)}
		got, ok := tree.Get(k)
		if i*10 == 50 {
			if ok {
				t.Fatalf("Get(50) ok after delete")
			}
			continue
		}
		if !ok || got != want {
			t.Fatalf("Get(%d) = (%d, %v), want (%d, true)", i*10, got, ok, want)
		}
	}
	// Add a new key to fill the slot back up.
	tree.Put([]byte{77}, 77)
	if got, ok := tree.Get([]byte{77}); !ok || got != 77 {
		t.Fatalf("Get(77) = (%d, %v) after re-add", got, ok)
	}
}

func TestNode48InsertionScrambled(t *testing.T) {
	// Insert 30 keys in scrambled order; Range must yield sorted.
	tree := New[int]()
	keys := []byte{
		0x40, 0x10, 0x80, 0x20, 0x60, 0x05, 0xa0, 0x70, 0xb0,
		0xc0, 0x35, 0x55, 0x75, 0xe0, 0x15, 0x25, 0x95, 0xa5,
		0x65, 0x85, 0xc5, 0xd5, 0xe5, 0xf5, 0x07, 0x17, 0x27,
		0x37, 0x47, 0x57,
	}
	for _, b := range keys {
		tree.Put([]byte{b}, int(b))
	}
	if _, _, n48, _ := tree.CountByKind(); n48 != 1 {
		t.Fatalf("expected node48 root")
	}
	want := append([]byte(nil), keys...)
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	var got []byte
	for k := range tree.Range(nil, nil) {
		got = append(got, k[0])
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Range order = %v, want %v", got, want)
	}
}
