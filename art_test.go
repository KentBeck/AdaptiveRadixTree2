package art

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
)

func TestPutThenGet(t *testing.T) {
	tree := New()

	if got, ok := tree.Get([]byte("missing")); ok || got != nil {
		t.Fatalf("Get on empty tree = (%v, %v), want (nil, false)", got, ok)
	}

	tree.Put([]byte("hello"), 42)

	got, ok := tree.Get([]byte("hello"))
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "hello")
	}
	if got != 42 {
		t.Fatalf("Get(%q) = %v, want 42", "hello", got)
	}
}

func TestPutTwoKeysNoCommonPrefix(t *testing.T) {
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("banana"), 2)

	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("banana")); !ok || v != 2 {
		t.Fatalf("Get(banana) = (%v, %v), want (2, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("cherry")); ok {
		t.Fatalf("Get(cherry) = (%v, %v), want (nil, false)", v, ok)
	}
}

func TestOverwriteExistingKey(t *testing.T) {
	tree := New()

	tree.Put([]byte("hello"), 1)
	tree.Put([]byte("hello"), 2)
	if v, ok := tree.Get([]byte("hello")); !ok || v != 2 {
		t.Fatalf("after leaf-root overwrite, Get(hello) = (%v, %v), want (2, true)", v, ok)
	}

	tree.Put([]byte("world"), 10)
	tree.Put([]byte("hello"), 3)
	tree.Put([]byte("world"), 20)
	if v, ok := tree.Get([]byte("hello")); !ok || v != 3 {
		t.Fatalf("after node4-child overwrite, Get(hello) = (%v, %v), want (3, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("world")); !ok || v != 20 {
		t.Fatalf("after node4-child overwrite, Get(world) = (%v, %v), want (20, true)", v, ok)
	}
}

func TestPutMoreThanFourKeys(t *testing.T) {
	tree := New()
	keys := []string{"a", "b", "c", "d", "e"}
	for i, k := range keys {
		tree.Put([]byte(k), i)
	}
	for i, k := range keys {
		if v, ok := tree.Get([]byte(k)); !ok || v != i {
			t.Fatalf("Get(%q) = (%v, %v), want (%d, true)", k, v, ok, i)
		}
	}
}

func TestPutMoreThanSixteenKeys(t *testing.T) {
	tree := New()
	// 17 distinct first bytes forces node16 -> node48 promotion.
	const n = 17
	for i := 0; i < n; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	for i := 0; i < n; i++ {
		if v, ok := tree.Get([]byte{byte(i)}); !ok || v != i {
			t.Fatalf("Get(%d) = (%v, %v), want (%d, true)", i, v, ok, i)
		}
	}
	// Overwrite at node48 level.
	tree.Put([]byte{byte(5)}, 999)
	if v, ok := tree.Get([]byte{byte(5)}); !ok || v != 999 {
		t.Fatalf("after overwrite, Get(5) = (%v, %v), want (999, true)", v, ok)
	}
	// Miss still returns (nil, false).
	if v, ok := tree.Get([]byte{byte(200)}); ok {
		t.Fatalf("Get(200) = (%v, %v), want (nil, false)", v, ok)
	}
}

func TestPutMoreThanFortyEightKeys(t *testing.T) {
	tree := New()
	const n = 49
	for i := 0; i < n; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	for i := 0; i < n; i++ {
		if v, ok := tree.Get([]byte{byte(i)}); !ok || v != i {
			t.Fatalf("Get(%d) = (%v, %v), want (%d, true)", i, v, ok, i)
		}
	}
	tree.Put([]byte{byte(10)}, 123)
	if v, ok := tree.Get([]byte{byte(10)}); !ok || v != 123 {
		t.Fatalf("after overwrite, Get(10) = (%v, %v), want (123, true)", v, ok)
	}
}

func TestDeleteKey(t *testing.T) {
	tree := New()

	// Delete on empty tree.
	if tree.Delete([]byte("nope")) {
		t.Fatal("Delete on empty tree returned true")
	}

	// Delete single leaf root.
	tree.Put([]byte("solo"), 1)
	if !tree.Delete([]byte("solo")) {
		t.Fatal("Delete(solo) returned false")
	}
	if v, ok := tree.Get([]byte("solo")); ok {
		t.Fatalf("Get after delete = (%v, %v), want miss", v, ok)
	}

	// Delete one of three children; the other two remain reachable.
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("banana"), 2)
	tree.Put([]byte("cherry"), 3)
	if !tree.Delete([]byte("banana")) {
		t.Fatal("Delete(banana) returned false")
	}
	if v, ok := tree.Get([]byte("banana")); ok {
		t.Fatalf("Get(banana) after delete = (%v, %v), want miss", v, ok)
	}
	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("cherry")); !ok || v != 3 {
		t.Fatalf("Get(cherry) = (%v, %v), want (3, true)", v, ok)
	}

	// Deleting a missing key returns false without disturbing the tree.
	if tree.Delete([]byte("zzz")) {
		t.Fatal("Delete(zzz) returned true")
	}
	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) post-miss-delete = (%v, %v), want (1, true)", v, ok)
	}
}

func TestDeleteShrinksThroughAllNodeTypes(t *testing.T) {
	tree := New()

	// Fill to node256 (49+ distinct first bytes).
	const n = 49
	for i := 0; i < n; i++ {
		tree.Put([]byte{byte(i)}, i)
	}
	// Delete down to force node256 -> node48 -> node16 -> node4 -> leaf.
	for i := n - 1; i >= 1; i-- {
		if !tree.Delete([]byte{byte(i)}) {
			t.Fatalf("Delete(%d) returned false", i)
		}
	}
	// Only key 0 remains.
	if v, ok := tree.Get([]byte{byte(0)}); !ok || v != 0 {
		t.Fatalf("Get(0) = (%v, %v), want (0, true)", v, ok)
	}
	for i := 1; i < n; i++ {
		if v, ok := tree.Get([]byte{byte(i)}); ok {
			t.Fatalf("Get(%d) = (%v, %v), want miss", i, v, ok)
		}
	}
	// Delete the last one; tree becomes empty.
	if !tree.Delete([]byte{byte(0)}) {
		t.Fatal("Delete(0) returned false")
	}
	if v, ok := tree.Get([]byte{byte(0)}); ok {
		t.Fatalf("Get(0) on empty = (%v, %v), want miss", v, ok)
	}
}

func TestTwoKeysSharingPrefix(t *testing.T) {
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)

	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("apricot")); !ok || v != 2 {
		t.Fatalf("Get(apricot) = (%v, %v), want (2, true)", v, ok)
	}

	// Key that doesn't match the shared prefix at all.
	if v, ok := tree.Get([]byte("banana")); ok {
		t.Fatalf("Get(banana) = (%v, %v), want miss", v, ok)
	}
	// Key that matches the prefix but has no corresponding child.
	if v, ok := tree.Get([]byte("apology")); ok {
		t.Fatalf("Get(apology) = (%v, %v), want miss", v, ok)
	}
	// Key shorter than the prefix.
	if v, ok := tree.Get([]byte("a")); ok {
		t.Fatalf("Get(a) = (%v, %v), want miss", v, ok)
	}

	// Put a third key that shares the prefix; both old and new visible.
	tree.Put([]byte("apology"), 3)
	if v, ok := tree.Get([]byte("apology")); !ok || v != 3 {
		t.Fatalf("Get(apology) after Put = (%v, %v), want (3, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) still visible = (%v, %v), want (1, true)", v, ok)
	}

	// Overwrite of an existing key inside a prefixed node4.
	tree.Put([]byte("apple"), 99)
	if v, ok := tree.Get([]byte("apple")); !ok || v != 99 {
		t.Fatalf("Get(apple) after overwrite = (%v, %v), want (99, true)", v, ok)
	}
	tree.Put([]byte("apricot"), 200)
	if v, ok := tree.Get([]byte("apricot")); !ok || v != 200 {
		t.Fatalf("Get(apricot) after overwrite = (%v, %v), want (200, true)", v, ok)
	}
}

func TestSplitPrefixAtRoot(t *testing.T) {
	// Slice 8 creates node4(prefix="ap") for "apple" + "apricot".
	// Inserting "banana" diverges at depth 0 → split at position 0.
	// New root: node4(prefix="") with children 'a' (the old node4, now prefix="p") and 'b' (leaf).
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)
	tree.Put([]byte("banana"), 3)

	for _, c := range []struct {
		key   string
		value any
	}{{"apple", 1}, {"apricot", 2}, {"banana", 3}} {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}
	if _, ok := tree.Get([]byte("ap")); ok {
		t.Fatalf("Get(ap) should miss")
	}
	if _, ok := tree.Get([]byte("bandana")); ok {
		t.Fatalf("Get(bandana) should miss")
	}
}

func TestSplitPrefixInMiddle(t *testing.T) {
	// node4(prefix="ap") for "apple" + "apricot".
	// Insert "aardvark": LCP with prefix "ap" is "a" (length 1). Split at position 1.
	// New root: node4(prefix="a") with children 'p' (old node4, now prefix="") and 'a' (leaf).
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)
	tree.Put([]byte("aardvark"), 3)

	for _, c := range []struct {
		key   string
		value any
	}{{"apple", 1}, {"apricot", 2}, {"aardvark", 3}} {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}
	if _, ok := tree.Get([]byte("a")); ok {
		t.Fatalf("Get(a) should miss")
	}
	if _, ok := tree.Get([]byte("apply")); ok {
		t.Fatalf("Get(apply) should miss")
	}

	// And a fourth key that requires another split of the new root's prefix.
	// "cherry" diverges at position 0 with prefix "a" → split at 0. New root gets prefix "".
	tree.Put([]byte("cherry"), 4)
	for _, c := range []struct {
		key   string
		value any
	}{{"apple", 1}, {"apricot", 2}, {"aardvark", 3}, {"cherry", 4}} {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}
}

func TestPutShorterKeyAfterLonger(t *testing.T) {
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("applepie"), 2)
	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("applepie")); !ok || v != 2 {
		t.Fatalf("Get(applepie) = (%v, %v), want (2, true)", v, ok)
	}
	if _, ok := tree.Get([]byte("app")); ok {
		t.Fatalf("Get(app) should miss")
	}
	if _, ok := tree.Get([]byte("applesauce")); ok {
		t.Fatalf("Get(applesauce) should miss")
	}
}

func TestPutLongerKeyAfterShorter(t *testing.T) {
	tree := New()
	tree.Put([]byte("applepie"), 1)
	tree.Put([]byte("apple"), 2)
	if v, ok := tree.Get([]byte("applepie")); !ok || v != 1 {
		t.Fatalf("Get(applepie) = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("apple")); !ok || v != 2 {
		t.Fatalf("Get(apple) = (%v, %v), want (2, true)", v, ok)
	}
}

func TestOverwriteTerminalValue(t *testing.T) {
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("applepie"), 2)
	tree.Put([]byte("apple"), 99)
	if v, ok := tree.Get([]byte("apple")); !ok || v != 99 {
		t.Fatalf("Get(apple) after overwrite = (%v, %v), want (99, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("applepie")); !ok || v != 2 {
		t.Fatalf("Get(applepie) still visible = (%v, %v), want (2, true)", v, ok)
	}
}

func TestSplitWithExhaustedKey(t *testing.T) {
	// Slice 9 built node4(prefix="ap") for "apple"+"apricot".
	// Inserting "a" splits at position 1, and "a" is exhausted at the split point.
	// New root: node4(prefix="a") with terminal="a" and one branching child 'p' = old node4 (prefix="").
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)
	tree.Put([]byte("a"), 3)
	for _, c := range []struct {
		key   string
		value any
	}{{"apple", 1}, {"apricot", 2}, {"a", 3}} {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}
	if _, ok := tree.Get([]byte("")); ok {
		t.Fatalf(`Get("") should miss`)
	}
	if _, ok := tree.Get([]byte("ap")); ok {
		t.Fatalf("Get(ap) should miss")
	}
}

func TestGrowToNode16PreservesPrefixAndTerminal(t *testing.T) {
	// Build a node4(prefix="ap", terminal="ap", children 'p','r','o','e') with 4 children:
	tree := New()
	tree.Put([]byte("ap"), 0)
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)
	tree.Put([]byte("apology"), 3)
	tree.Put([]byte("apex"), 4)
	// Adding a fifth distinct branching byte forces grow to node16.
	tree.Put([]byte("apt"), 5)

	cases := []struct {
		key   string
		value any
	}{
		{"ap", 0}, {"apple", 1}, {"apricot", 2},
		{"apology", 3}, {"apex", 4}, {"apt", 5},
	}
	for _, c := range cases {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}

	// Misses that exercise prefix and terminal handling:
	if _, ok := tree.Get([]byte("a")); ok {
		t.Fatalf("Get(a) should miss")
	}
	if _, ok := tree.Get([]byte("apply")); ok {
		t.Fatalf("Get(apply) should miss")
	}
	if _, ok := tree.Get([]byte("apes")); ok {
		t.Fatalf("Get(apes) should miss")
	}
	if _, ok := tree.Get([]byte("banana")); ok {
		t.Fatalf("Get(banana) should miss")
	}

	// Add a sixth branching byte — still inside node16's capacity.
	tree.Put([]byte("apse"), 6)
	if v, ok := tree.Get([]byte("apse")); !ok || v != 6 {
		t.Fatalf("Get(apse) after second add = (%v, %v), want (6, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("ap")); !ok || v != 0 {
		t.Fatalf("Get(ap) still visible = (%v, %v), want (0, true)", v, ok)
	}

	// Overwrite the terminal value at the promoted node16.
	tree.Put([]byte("ap"), 99)
	if v, ok := tree.Get([]byte("ap")); !ok || v != 99 {
		t.Fatalf("Get(ap) after overwrite = (%v, %v), want (99, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) still visible = (%v, %v), want (1, true)", v, ok)
	}
}

func TestNestedNode4FromDivergentLeaves(t *testing.T) {
	// Root node4 has children at 'a' and 'b' (leaves "apple", "banana").
	// Inserting "apricot" makes the 'a' slot need a nested node4 with prefix "p"
	// (LCP of "pple" and "pricot" below depth 1 is "p", diverging at 'p'/'r').
	// Wait: keys from depth 1 are "pple" and "pricot". LCP is "p", diverging at position 2 ('p' vs 'r').
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("banana"), 2)
	tree.Put([]byte("apricot"), 3)
	for _, c := range []struct {
		key   string
		value any
	}{{"apple", 1}, {"banana", 2}, {"apricot", 3}} {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}
	if _, ok := tree.Get([]byte("ap")); ok {
		t.Fatalf("Get(ap) should miss")
	}
}

func TestSplitPrefixedNode16(t *testing.T) {
	// Build a node16 whose prefix is "comm" by inserting keys that share "comm".
	// Five keys with distinct bytes after "comm" promote node4 → node16.
	tree := New()
	tree.Put([]byte("commit"), 1)
	tree.Put([]byte("common"), 2)
	tree.Put([]byte("compare"), 3)
	tree.Put([]byte("compute"), 4)
	tree.Put([]byte("company"), 5)

	// All five read back.
	for _, c := range []struct {
		key   string
		value any
	}{{"commit", 1}, {"common", 2}, {"compare", 3}, {"compute", 4}, {"company", 5}} {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}

	// Insert a key that shares only "co" — forces a split of the prefixed node16.
	tree.Put([]byte("copper"), 6)
	for _, c := range []struct {
		key   string
		value any
	}{{"commit", 1}, {"common", 2}, {"compare", 3}, {"compute", 4}, {"company", 5}, {"copper", 6}} {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) after split = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}
	// Intermediate prefixes should miss.
	if _, ok := tree.Get([]byte("co")); ok {
		t.Fatalf("Get(co) should miss")
	}
	if _, ok := tree.Get([]byte("comm")); ok {
		t.Fatalf("Get(comm) should miss")
	}
}

func TestSplitPrefixedNode16WithExhaustedKey(t *testing.T) {
	// Same node16 with prefix "comm" as above, but the splitting key is
	// exhausted exactly at the split point → parent node4 gets a terminal.
	tree := New()
	tree.Put([]byte("commit"), 1)
	tree.Put([]byte("common"), 2)
	tree.Put([]byte("compare"), 3)
	tree.Put([]byte("compute"), 4)
	tree.Put([]byte("company"), 5)

	tree.Put([]byte("co"), 42) // exhausts at the split point

	if v, ok := tree.Get([]byte("co")); !ok || v != 42 {
		t.Fatalf("Get(co) = (%v, %v), want (42, true)", v, ok)
	}
	for _, c := range []struct {
		key   string
		value any
	}{{"commit", 1}, {"common", 2}, {"compare", 3}, {"compute", 4}, {"company", 5}} {
		if v, ok := tree.Get([]byte(c.key)); !ok || v != c.value {
			t.Fatalf("Get(%q) after exhausted split = (%v, %v), want (%v, true)", c.key, v, ok, c.value)
		}
	}
	// Overwrite the new terminal.
	tree.Put([]byte("co"), 43)
	if v, ok := tree.Get([]byte("co")); !ok || v != 43 {
		t.Fatalf("Get(co) after overwrite = (%v, %v), want (43, true)", v, ok)
	}
}

func TestGrowToNode48PreservesPrefixAndTerminal(t *testing.T) {
	// Build a node16 with prefix "k" and terminal set to ("k", 0).
	// Then push it to 17 branching bytes to force node16 → node48.
	tree := New()
	tree.Put([]byte("k"), 0)
	for i := 0; i < 17; i++ {
		key := []byte{'k', byte('a' + i)}
		tree.Put(key, i+1)
	}

	if v, ok := tree.Get([]byte("k")); !ok || v != 0 {
		t.Fatalf("Get(k) after grow = (%v, %v), want (0, true)", v, ok)
	}
	for i := 0; i < 17; i++ {
		key := []byte{'k', byte('a' + i)}
		if v, ok := tree.Get(key); !ok || v != i+1 {
			t.Fatalf("Get(%q) after grow = (%v, %v), want (%d, true)", key, v, ok, i+1)
		}
	}

	// Overwrite terminal survives at node48.
	tree.Put([]byte("k"), 99)
	if v, ok := tree.Get([]byte("k")); !ok || v != 99 {
		t.Fatalf("Get(k) after overwrite = (%v, %v), want (99, true)", v, ok)
	}
}

func TestGrowToNode256PreservesPrefixAndTerminal(t *testing.T) {
	tree := New()
	tree.Put([]byte("k"), 0)
	for i := 0; i < 49; i++ {
		key := []byte{'k', byte('a' + i)}
		tree.Put(key, i+1)
	}

	if v, ok := tree.Get([]byte("k")); !ok || v != 0 {
		t.Fatalf("Get(k) after grow = (%v, %v), want (0, true)", v, ok)
	}
	for i := 0; i < 49; i++ {
		key := []byte{'k', byte('a' + i)}
		if v, ok := tree.Get(key); !ok || v != i+1 {
			t.Fatalf("Get(%q) after grow = (%v, %v), want (%d, true)", key, v, ok, i+1)
		}
	}

	tree.Put([]byte("k"), 99)
	if v, ok := tree.Get([]byte("k")); !ok || v != 99 {
		t.Fatalf("Get(k) after overwrite = (%v, %v), want (99, true)", v, ok)
	}
}

func TestSplitPrefixedNode48(t *testing.T) {
	// Promote to a node48 with prefix "comm" by using 17 distinct bytes after "comm".
	tree := New()
	for i := 0; i < 17; i++ {
		key := append([]byte("comm"), byte('a'+i))
		tree.Put(key, i)
	}
	// Split it with a key sharing only "co".
	tree.Put([]byte("copper"), 999)

	for i := 0; i < 17; i++ {
		key := append([]byte("comm"), byte('a'+i))
		if v, ok := tree.Get(key); !ok || v != i {
			t.Fatalf("Get(%q) after split = (%v, %v), want (%d, true)", key, v, ok, i)
		}
	}
	if v, ok := tree.Get([]byte("copper")); !ok || v != 999 {
		t.Fatalf("Get(copper) = (%v, %v), want (999, true)", v, ok)
	}
	if _, ok := tree.Get([]byte("comm")); ok {
		t.Fatalf("Get(comm) should miss")
	}
}

func TestSplitPrefixedNode256(t *testing.T) {
	tree := New()
	for i := 0; i < 49; i++ {
		key := append([]byte("comm"), byte('a'+i))
		tree.Put(key, i)
	}
	tree.Put([]byte("copper"), 999)

	for i := 0; i < 49; i++ {
		key := append([]byte("comm"), byte('a'+i))
		if v, ok := tree.Get(key); !ok || v != i {
			t.Fatalf("Get(%q) after split = (%v, %v), want (%d, true)", key, v, ok, i)
		}
	}
	if v, ok := tree.Get([]byte("copper")); !ok || v != 999 {
		t.Fatalf("Get(copper) = (%v, %v), want (999, true)", v, ok)
	}
}

func TestDeleteTerminalLeavesChildren(t *testing.T) {
	tree := New()
	tree.Put([]byte("ap"), 0)
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)

	if !tree.Delete([]byte("ap")) {
		t.Fatal("Delete(ap) returned false")
	}
	if _, ok := tree.Get([]byte("ap")); ok {
		t.Fatal("Get(ap) after delete should miss")
	}
	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("apricot")); !ok || v != 2 {
		t.Fatalf("Get(apricot) = (%v, %v), want (2, true)", v, ok)
	}
}

func TestDeleteMissOnPrefixMismatch(t *testing.T) {
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)

	if tree.Delete([]byte("banana")) {
		t.Fatal("Delete(banana) on unrelated key should return false")
	}
	if tree.Delete([]byte("ap")) {
		t.Fatal("Delete(ap) when no terminal exists should return false")
	}
	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) = (%v, %v), want (1, true)", v, ok)
	}
}

func TestDeleteDemotesPrefixedNode256(t *testing.T) {
	tree := New()
	tree.Put([]byte("k"), 0)
	for i := 0; i < 49; i++ {
		key := []byte{'k', byte(i)}
		tree.Put(key, i+1)
	}

	if !tree.Delete([]byte{'k', byte(0)}) {
		t.Fatal("Delete returned false")
	}
	if v, ok := tree.Get([]byte("k")); !ok || v != 0 {
		t.Fatalf("Get(k) after demote = (%v, %v), want (0, true)", v, ok)
	}
	for i := 1; i < 49; i++ {
		key := []byte{'k', byte(i)}
		if v, ok := tree.Get(key); !ok || v != i+1 {
			t.Fatalf("Get(%q) after demote = (%v, %v), want (%d, true)", key, v, ok, i+1)
		}
	}
}

func TestDeleteCollapsesToTerminalAtRoot(t *testing.T) {
	// Root is a node4 with prefix "ap", terminal ("ap", 1), one leaf child at 'p' ("apple", 2).
	// Wait — that's 1 child + terminal. Insert three keys so root ends up with terminal + multiple children.
	tree := New()
	tree.Put([]byte("ap"), 1)
	tree.Put([]byte("apple"), 2)
	tree.Put([]byte("apricot"), 3)

	// Delete both branching children → root should collapse to the terminal leaf ("ap", 1).
	if !tree.Delete([]byte("apple")) {
		t.Fatal("Delete(apple) returned false")
	}
	if !tree.Delete([]byte("apricot")) {
		t.Fatal("Delete(apricot) returned false")
	}

	if v, ok := tree.Get([]byte("ap")); !ok || v != 1 {
		t.Fatalf("Get(ap) after collapse = (%v, %v), want (1, true)", v, ok)
	}
	if _, ok := tree.Get([]byte("apple")); ok {
		t.Fatal("Get(apple) after delete should miss")
	}
	if _, ok := tree.Get([]byte("apricot")); ok {
		t.Fatal("Get(apricot) after delete should miss")
	}
	// Deleting the terminal itself empties the tree.
	if !tree.Delete([]byte("ap")) {
		t.Fatal("Delete(ap) returned false")
	}
	if _, ok := tree.Get([]byte("ap")); ok {
		t.Fatal("Get(ap) after final delete should miss")
	}
}

func TestDeleteCollapsesToTerminalAtInnerNode(t *testing.T) {
	// Root node4 has no prefix, branches 'a' (subtree) and 'b' (leaf "banana").
	// 'a' subtree has prefix "p", terminal ("ap", 1), and child 'p' leaf "apple".
	tree := New()
	tree.Put([]byte("ap"), 1)
	tree.Put([]byte("apple"), 2)
	tree.Put([]byte("banana"), 3)

	// Delete "apple" → 'a' subtree becomes (0 children, terminal "ap") → collapse.
	// Root's 'a' child should become the leaf ("ap", 1) directly.
	if !tree.Delete([]byte("apple")) {
		t.Fatal("Delete(apple) returned false")
	}
	if v, ok := tree.Get([]byte("ap")); !ok || v != 1 {
		t.Fatalf("Get(ap) = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("banana")); !ok || v != 3 {
		t.Fatalf("Get(banana) = (%v, %v), want (3, true)", v, ok)
	}
	if _, ok := tree.Get([]byte("apple")); ok {
		t.Fatal("Get(apple) should miss")
	}
}

func TestDeletePrefixMergeCollapse(t *testing.T) {
	// Root node4 has prefix "", branches 'a' (subtree) and 'b' (leaf "banana").
	// 'a' subtree has prefix "p", branches 'p' (leaf "apple") and 'r' (leaf "apricot").
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)
	tree.Put([]byte("banana"), 3)

	// Delete "banana" → root has 1 child (the 'a' subtree, an inner node) and no terminal.
	// Root and child must merge: new root has prefix "" || 'a' || "p" = "ap".
	if !tree.Delete([]byte("banana")) {
		t.Fatal("Delete(banana) returned false")
	}

	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) after merge = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("apricot")); !ok || v != 2 {
		t.Fatalf("Get(apricot) after merge = (%v, %v), want (2, true)", v, ok)
	}
	if _, ok := tree.Get([]byte("banana")); ok {
		t.Fatal("Get(banana) should miss")
	}
	// Further inserts/deletes on the merged structure still work.
	tree.Put([]byte("aprons"), 4)
	if v, ok := tree.Get([]byte("aprons")); !ok || v != 4 {
		t.Fatalf("Get(aprons) = (%v, %v), want (4, true)", v, ok)
	}
}

func TestDeletePrefixMergeAtInnerNode(t *testing.T) {
	// Build a deeper tree so the merge happens below the root.
	// Root branches 'a' (subtree) and 'z' (leaf "zoo").
	// 'a' subtree has prefix "p", branches 'p' (subtree "pl"/apple/application) and 'r' (leaf apricot).
	// The 'p' subtree has prefix "pl", branches 'e' (leaf apple) and 'i' (leaf application).
	tree := New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("application"), 2)
	tree.Put([]byte("apricot"), 3)
	tree.Put([]byte("zoo"), 4)

	// Delete "apricot" → 'a' subtree is left with one child (the 'p' subtree, an inner node) and no terminal.
	// Must merge: 'a' subtree's prefix "p" + branch 'p' + child prefix "pl" = "ppl"
	// so the new 'a' child of the root is a node4 with prefix "ppl" branching on 'e' and 'i'.
	if !tree.Delete([]byte("apricot")) {
		t.Fatal("Delete(apricot) returned false")
	}
	if v, ok := tree.Get([]byte("apple")); !ok || v != 1 {
		t.Fatalf("Get(apple) = (%v, %v), want (1, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("application")); !ok || v != 2 {
		t.Fatalf("Get(application) = (%v, %v), want (2, true)", v, ok)
	}
	if v, ok := tree.Get([]byte("zoo")); !ok || v != 4 {
		t.Fatalf("Get(zoo) = (%v, %v), want (4, true)", v, ok)
	}
	if _, ok := tree.Get([]byte("apricot")); ok {
		t.Fatal("Get(apricot) should miss")
	}
}

func TestAllEmptyTreeYieldsNothing(t *testing.T) {
	tree := New()
	count := 0
	for range tree.All() {
		count++
	}
	if count != 0 {
		t.Fatalf("empty tree yielded %d pairs, want 0", count)
	}
}

func TestAllYieldsSingleKey(t *testing.T) {
	tree := New()
	tree.Put([]byte("apple"), 1)
	var gotKeys [][]byte
	var gotVals []any
	for k, v := range tree.All() {
		gotKeys = append(gotKeys, append([]byte(nil), k...))
		gotVals = append(gotVals, v)
	}
	if len(gotKeys) != 1 || string(gotKeys[0]) != "apple" || gotVals[0] != 1 {
		t.Fatalf("got %v / %v, want [apple] / [1]", gotKeys, gotVals)
	}
}

func TestAllYieldsSortedOrderAcrossNodeTypes(t *testing.T) {
	tree := New()
	want := [][]byte{}
	for _, s := range []string{"", "a", "ap", "apple", "application", "apricot", "banana", "z", "zoo"} {
		tree.Put([]byte(s), s)
		want = append(want, []byte(s))
	}
	for i := 0; i < 260; i++ {
		key := []byte{'k', byte(i % 256), byte(i / 256)}
		tree.Put(key, string(key))
		want = append(want, append([]byte(nil), key...))
	}
	sort.Slice(want, func(i, j int) bool { return bytes.Compare(want[i], want[j]) < 0 })

	got := [][]byte{}
	for k := range tree.All() {
		got = append(got, append([]byte(nil), k...))
	}
	if len(got) != len(want) {
		t.Fatalf("got %d pairs, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("pair %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAllYieldsTerminalBeforeChildren(t *testing.T) {
	tree := New()
	tree.Put([]byte("ap"), 1)
	tree.Put([]byte("apple"), 2)
	tree.Put([]byte("apricot"), 3)
	var keys []string
	for k := range tree.All() {
		keys = append(keys, string(k))
	}
	want := []string{"ap", "apple", "apricot"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("got %v, want %v", keys, want)
	}
}

func TestAllEarlyTermination(t *testing.T) {
	tree := New()
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		tree.Put([]byte(s), s)
	}
	var keys []string
	for k := range tree.All() {
		keys = append(keys, string(k))
		if string(k) == "c" {
			break
		}
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("got %v, want %v", keys, want)
	}
}

func TestRangeInclusiveStartExclusiveEnd(t *testing.T) {
	tree := New()
	for _, s := range []string{"apple", "apricot", "banana", "blueberry", "cherry"} {
		tree.Put([]byte(s), s)
	}
	var got []string
	for k := range tree.Range([]byte("apricot"), []byte("cherry")) {
		got = append(got, string(k))
	}
	want := []string{"apricot", "banana", "blueberry"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRangeNilStartMeansUnbounded(t *testing.T) {
	tree := New()
	for _, s := range []string{"a", "b", "c", "d"} {
		tree.Put([]byte(s), s)
	}
	var got []string
	for k := range tree.Range(nil, []byte("c")) {
		got = append(got, string(k))
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRangeNilEndMeansUnbounded(t *testing.T) {
	tree := New()
	for _, s := range []string{"a", "b", "c", "d"} {
		tree.Put([]byte(s), s)
	}
	var got []string
	for k := range tree.Range([]byte("b"), nil) {
		got = append(got, string(k))
	}
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRangeBothNilEqualsAll(t *testing.T) {
	tree := New()
	inserted := []string{"", "a", "ap", "apple", "application", "apricot", "banana", "z"}
	for _, s := range inserted {
		tree.Put([]byte(s), s)
	}
	var all, ranged []string
	for k := range tree.All() {
		all = append(all, string(k))
	}
	for k := range tree.Range(nil, nil) {
		ranged = append(ranged, string(k))
	}
	if !reflect.DeepEqual(all, ranged) {
		t.Fatalf("Range(nil,nil) = %v, All = %v", ranged, all)
	}
}

func TestRangeStartEqualsEndIsEmpty(t *testing.T) {
	tree := New()
	for _, s := range []string{"a", "b", "c"} {
		tree.Put([]byte(s), s)
	}
	count := 0
	for range tree.Range([]byte("b"), []byte("b")) {
		count++
	}
	if count != 0 {
		t.Fatalf("Range with start==end yielded %d, want 0", count)
	}
}

func TestRangeStartAfterEndIsEmpty(t *testing.T) {
	tree := New()
	for _, s := range []string{"a", "b", "c"} {
		tree.Put([]byte(s), s)
	}
	count := 0
	for range tree.Range([]byte("c"), []byte("a")) {
		count++
	}
	if count != 0 {
		t.Fatalf("Range with start>end yielded %d, want 0", count)
	}
}

func TestRangeHandlesPrefixAndTerminal(t *testing.T) {
	tree := New()
	for _, s := range []string{"ap", "apple", "application", "apricot", "banana"} {
		tree.Put([]byte(s), s)
	}
	var got []string
	for k := range tree.Range([]byte("ap"), []byte("apricot")) {
		got = append(got, string(k))
	}
	want := []string{"ap", "apple", "application"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRangeAcrossLargeTree(t *testing.T) {
	tree := New()
	var keys [][]byte
	for i := 0; i < 300; i++ {
		k := []byte{byte(i % 256), byte(i / 256)}
		tree.Put(k, i)
		keys = append(keys, append([]byte(nil), k...))
	}
	sort.Slice(keys, func(i, j int) bool { return bytes.Compare(keys[i], keys[j]) < 0 })

	start := []byte{0x40, 0x00}
	end := []byte{0xC0, 0x00}
	var want [][]byte
	for _, k := range keys {
		if bytes.Compare(k, start) >= 0 && bytes.Compare(k, end) < 0 {
			want = append(want, k)
		}
	}

	var got [][]byte
	for k := range tree.Range(start, end) {
		got = append(got, append([]byte(nil), k...))
	}
	if len(got) != len(want) {
		t.Fatalf("got %d pairs, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("pair %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRangeEarlyTermination(t *testing.T) {
	tree := New()
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		tree.Put([]byte(s), s)
	}
	var got []string
	for k := range tree.Range([]byte("a"), []byte("e")) {
		got = append(got, string(k))
		if string(k) == "c" {
			break
		}
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// buildInnerNode constructs a Tree whose root — after descending through its
// path-compressed prefix — is an inner node of exactly the requested child
// count, with an optional terminal. Returns the tree plus the list of keys
// that were inserted (in insertion order). If withTerminal is true, the
// returned key slice's first element is the terminal key (equal to prefix);
// the remaining keys are the inner-node children in insertion order.
//
// childCount:
//
//	1..4    → node4
//	5..16   → node16
//	17..48  → node48
//	49..256 → node256
func buildInnerNode(t *testing.T, prefix []byte, childCount int, withTerminal bool) (*Tree, [][]byte) {
	t.Helper()
	if childCount < 1 || childCount > 256 {
		t.Fatalf("buildInnerNode: childCount must be 1..256, got %d", childCount)
	}
	tree := New()
	var keys [][]byte
	if withTerminal {
		tp := bytes.Clone(prefix)
		tree.Put(tp, "terminal")
		keys = append(keys, tp)
	}
	for i := 0; i < childCount; i++ {
		edge := byte((i * 256) / childCount)
		key := append(bytes.Clone(prefix), edge)
		tree.Put(key, int(edge))
		keys = append(keys, key)
	}
	return tree, keys
}

// rootKindOf returns a short name for the root node's type.
// Permitted because this file is in package art.
func rootKindOf(tree *Tree) string {
	if tree.root == nil {
		return "nil"
	}
	switch tree.root.(type) {
	case *leaf:
		return "leaf"
	case *node4:
		return "node4"
	case *node16:
		return "node16"
	case *node48:
		return "node48"
	case *node256:
		return "node256"
	default:
		return "unknown"
	}
}

func TestDeleteAcrossInnerNodeTypes(t *testing.T) {
	prefix := []byte("common-prefix/")
	cases := []struct {
		name       string
		childCount int
	}{
		{"node16", 5},
		{"node48", 17},
		{"node256", 49},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("delete_terminal", func(t *testing.T) {
				tree, keys := buildInnerNode(t, prefix, tc.childCount, true)
				terminal := keys[0]
				if ok := tree.Delete(terminal); !ok {
					t.Fatalf("Delete(terminal) = false, want true")
				}
				if got, ok := tree.Get(terminal); ok || got != nil {
					t.Fatalf("Get(terminal) after delete = (%v, %v), want (nil, false)", got, ok)
				}
				for _, k := range keys[1:] {
					if _, ok := tree.Get(k); !ok {
						t.Fatalf("Get(%q) after terminal delete: ok=false, want true", k)
					}
				}
			})
			t.Run("delete_first_child", func(t *testing.T) {
				tree, keys := buildInnerNode(t, prefix, tc.childCount, true)
				victim := keys[1]
				if ok := tree.Delete(victim); !ok {
					t.Fatalf("Delete(%q) = false, want true", victim)
				}
				if got, ok := tree.Get(victim); ok || got != nil {
					t.Fatalf("Get(%q) after delete = (%v, %v), want (nil, false)", victim, got, ok)
				}
				if _, ok := tree.Get(keys[0]); !ok {
					t.Fatalf("terminal lost after deleting sibling")
				}
				for _, k := range keys[2:] {
					if _, ok := tree.Get(k); !ok {
						t.Fatalf("sibling %q lost after deleting first child", k)
					}
				}
			})
			t.Run("delete_nonexistent", func(t *testing.T) {
				tree, keys := buildInnerNode(t, prefix, tc.childCount, true)
				before := 0
				for range tree.All() {
					before++
				}
				missing := append(bytes.Clone(prefix), byte(255))
				if ok := tree.Delete(missing); ok {
					t.Fatalf("Delete(missing) = true, want false")
				}
				after := 0
				for range tree.All() {
					after++
				}
				if after != before {
					t.Fatalf("count changed after no-op delete: before=%d after=%d", before, after)
				}
				for _, k := range keys {
					if _, ok := tree.Get(k); !ok {
						t.Fatalf("key %q lost after no-op delete", k)
					}
				}
			})
			t.Run("delete_down_to_empty", func(t *testing.T) {
				tree, keys := buildInnerNode(t, prefix, tc.childCount, true)
				for i, k := range keys {
					if ok := tree.Delete(k); !ok {
						t.Fatalf("Delete(keys[%d]=%q) = false, want true", i, k)
					}
				}
				if tree.root != nil {
					t.Fatalf("root = %s, want nil after deleting every key", rootKindOf(tree))
				}
				count := 0
				for range tree.All() {
					count++
				}
				if count != 0 {
					t.Fatalf("All() yielded %d pairs on empty tree, want 0", count)
				}
			})
		})
	}
}

func TestGetTerminalAndMissAcrossInnerNodeTypes(t *testing.T) {
	prefix := []byte("common-prefix/")
	cases := []struct {
		name       string
		childCount int
	}{
		{"node16", 5},
		{"node48", 17},
		{"node256", 49},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, _ := buildInnerNode(t, prefix, tc.childCount, true)

			if got, ok := tree.Get(prefix); !ok || got != "terminal" {
				t.Fatalf("Get(terminal) = (%v, %v), want (\"terminal\", true)", got, ok)
			}

			missingEdge := append(bytes.Clone(prefix), byte(255))
			if got, ok := tree.Get(missingEdge); ok || got != nil {
				t.Fatalf("Get(prefix+missingEdge) = (%v, %v), want (nil, false)", got, ok)
			}

			wrongPrefix := bytes.Clone(prefix)
			wrongPrefix[0] ^= 0xFF
			if got, ok := tree.Get(wrongPrefix); ok || got != nil {
				t.Fatalf("Get(wrongPrefix) = (%v, %v), want (nil, false)", got, ok)
			}

			shortPrefix := prefix[:3]
			if got, ok := tree.Get(shortPrefix); ok || got != nil {
				t.Fatalf("Get(shortPrefix) = (%v, %v), want (nil, false)", got, ok)
			}
		})
	}
}

func TestPutOverwriteTerminalAcrossInnerNodeTypes(t *testing.T) {
	prefix := []byte("common-prefix/")
	cases := []struct {
		name       string
		childCount int
	}{
		{"node16", 5},
		{"node48", 17},
		{"node256", 49},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, keys := buildInnerNode(t, prefix, tc.childCount, true)

			before := 0
			for range tree.All() {
				before++
			}

			tree.Put(bytes.Clone(prefix), "replacement")

			if got, ok := tree.Get(prefix); !ok || got != "replacement" {
				t.Fatalf("Get(terminal) after overwrite = (%v, %v), want (\"replacement\", true)", got, ok)
			}

			after := 0
			for range tree.All() {
				after++
			}
			if after != before {
				t.Fatalf("count changed after overwrite: before=%d after=%d", before, after)
			}

			for _, k := range keys[1:] {
				want := int(k[len(prefix)])
				got, ok := tree.Get(k)
				if !ok {
					t.Fatalf("child %q lost after terminal overwrite", k)
				}
				if gi, _ := got.(int); gi != want {
					t.Fatalf("child %q value = %v, want %d", k, got, want)
				}
			}
		})
	}
}

func TestDeleteNonLastChildFromNode16AndNode48(t *testing.T) {
	prefix := []byte("p/")
	shapes := []struct {
		name       string
		childCount int
	}{
		{"node16", 16},
		{"node48", 48},
	}
	assertSorted := func(t *testing.T, tree *Tree, want [][]byte) {
		t.Helper()
		var got [][]byte
		for k := range tree.All() {
			got = append(got, bytes.Clone(k))
		}
		if len(got) != len(want) {
			t.Fatalf("All() yielded %d keys, want %d", len(got), len(want))
		}
		for i := range got {
			if !bytes.Equal(got[i], want[i]) {
				t.Fatalf("All()[%d] = %v, want %v", i, got[i], want[i])
			}
			if i > 0 && bytes.Compare(got[i-1], got[i]) >= 0 {
				t.Fatalf("All() not sorted at %d: %v then %v", i, got[i-1], got[i])
			}
		}
	}
	remove := func(keys [][]byte, idx int) [][]byte {
		out := make([][]byte, 0, len(keys)-1)
		out = append(out, keys[:idx]...)
		out = append(out, keys[idx+1:]...)
		return out
	}
	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			t.Run("delete_middle", func(t *testing.T) {
				tree, keys := buildInnerNode(t, prefix, sh.childCount, false)
				mid := len(keys) / 2
				if ok := tree.Delete(keys[mid]); !ok {
					t.Fatalf("Delete(middle) = false, want true")
				}
				assertSorted(t, tree, remove(keys, mid))
			})
			t.Run("delete_first", func(t *testing.T) {
				tree, keys := buildInnerNode(t, prefix, sh.childCount, false)
				if ok := tree.Delete(keys[0]); !ok {
					t.Fatalf("Delete(first) = false, want true")
				}
				assertSorted(t, tree, remove(keys, 0))
			})
			t.Run("delete_last", func(t *testing.T) {
				tree, keys := buildInnerNode(t, prefix, sh.childCount, false)
				last := len(keys) - 1
				if ok := tree.Delete(keys[last]); !ok {
					t.Fatalf("Delete(last) = false, want true")
				}
				assertSorted(t, tree, remove(keys, last))
			})
		})
	}
}

func TestPutInsertMiddleKeyIntoNode16(t *testing.T) {
	prefix := []byte("p/")
	tree := New()
	edges := []byte{0x10, 0x30, 0x50, 0x70}
	var keys [][]byte
	for _, e := range edges {
		k := append(bytes.Clone(prefix), e)
		tree.Put(k, int(e))
		keys = append(keys, k)
	}
	if got := rootKindOf(tree); got != "node4" {
		t.Fatalf("rootKindOf after 4 puts = %q, want %q", got, "node4")
	}

	middle := append(bytes.Clone(prefix), byte(0x40))
	tree.Put(middle, int(0x40))
	keys = append(keys, middle)
	if got := rootKindOf(tree); got != "node16" {
		t.Fatalf("rootKindOf after 5th put = %q, want %q", got, "node16")
	}

	for _, k := range keys {
		want := int(k[len(prefix)])
		got, ok := tree.Get(k)
		if !ok {
			t.Fatalf("Get(%v) ok=false, want true", k)
		}
		if gi, _ := got.(int); gi != want {
			t.Fatalf("Get(%v) = %v, want %d", k, got, want)
		}
	}

	wantSorted := make([][]byte, len(keys))
	copy(wantSorted, keys)
	sort.Slice(wantSorted, func(i, j int) bool {
		return bytes.Compare(wantSorted[i], wantSorted[j]) < 0
	})
	var iterated [][]byte
	for k := range tree.All() {
		iterated = append(iterated, bytes.Clone(k))
	}
	if len(iterated) != len(wantSorted) {
		t.Fatalf("All() yielded %d keys, want %d", len(iterated), len(wantSorted))
	}
	for i := range iterated {
		if !bytes.Equal(iterated[i], wantSorted[i]) {
			t.Fatalf("All()[%d] = %v, want %v", i, iterated[i], wantSorted[i])
		}
	}
}

func TestGrowAndShrinkBoundaryStructure(t *testing.T) {
	prefix := []byte("p/")
	tree := New()

	putEdge := func(e int) {
		k := append(bytes.Clone(prefix), byte(e))
		tree.Put(k, e)
	}
	deleteEdge := func(e int) {
		k := append(bytes.Clone(prefix), byte(e))
		if ok := tree.Delete(k); !ok {
			t.Fatalf("Delete(edge=%d) = false, want true", e)
		}
	}
	checkKindAfterCount := func(count int, want string) {
		t.Helper()
		if got := rootKindOf(tree); got != want {
			t.Fatalf("rootKindOf at count=%d = %q, want %q", count, got, want)
		}
	}

	growBoundaries := []struct {
		afterPut int
		want     string
	}{
		{1, "leaf"},
		{2, "node4"},
		{4, "node4"},
		{5, "node16"},
		{16, "node16"},
		{17, "node48"},
		{48, "node48"},
		{49, "node256"},
	}
	next := 0
	for _, b := range growBoundaries {
		for next < b.afterPut {
			putEdge(next)
			next++
		}
		checkKindAfterCount(b.afterPut, b.want)
	}

	deleteEdge(48)
	checkKindAfterCount(48, "node48")
	for e := 47; e >= 16; e-- {
		deleteEdge(e)
	}
	checkKindAfterCount(16, "node16")
	for e := 15; e >= 4; e-- {
		deleteEdge(e)
	}
	checkKindAfterCount(4, "node4")
}

func TestRangeBoundaryHelpers(t *testing.T) {
	cases := []struct {
		name          string
		nodePath      []byte
		extra         byte
		bound         []byte
		wantBefore    bool
		wantAtOrAfter bool
	}{
		{"empty_nodepath_empty_bound", nil, 0, []byte{}, false, true},
		{"empty_nodepath_nil_bound", nil, 0, nil, false, false},
		{"empty_nodepath_extra_equals_single_bound", nil, 5, []byte{5}, false, true},
		{"exact_boundary", []byte{1, 2, 3}, 4, []byte{1, 2, 3, 4}, false, true},
		{"nodepath_equals_bound", []byte{1, 2, 3, 4}, 0, []byte{1, 2, 3, 4}, false, true},
		{"extra_greater_at_same_depth", []byte{1, 2, 3}, 5, []byte{1, 2, 3, 4}, false, true},
		{"extra_less_at_same_depth", []byte{1, 2, 3}, 3, []byte{1, 2, 3, 4}, true, false},
		{"nodepath_longer_with_earlier_divergence_greater", []byte{1, 5, 0, 0}, 0, []byte{1, 3}, false, true},
		{"nodepath_longer_with_earlier_divergence_less", []byte{1, 2, 9, 0, 0}, 0, []byte{1, 3, 5}, true, false},
		{"nodepath_shorter_by_more_than_one", []byte{1, 2}, 3, []byte{1, 2, 3, 4, 5}, false, false},
		{"nodepath_equals_bound_minus_one_extra_less", []byte{10, 20}, 0, []byte{10, 20, 30}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := subtreeBeforeWithByte(tc.nodePath, tc.extra, tc.bound); got != tc.wantBefore {
				t.Fatalf("subtreeBeforeWithByte(%v, %d, %v) = %v, want %v",
					tc.nodePath, tc.extra, tc.bound, got, tc.wantBefore)
			}
			if got := subtreeAtOrAfterWithByte(tc.nodePath, tc.extra, tc.bound); got != tc.wantAtOrAfter {
				t.Fatalf("subtreeAtOrAfterWithByte(%v, %d, %v) = %v, want %v",
					tc.nodePath, tc.extra, tc.bound, got, tc.wantAtOrAfter)
			}
		})
	}
}

func TestRangeAcrossNode256WithTerminal(t *testing.T) {
	prefix := []byte("p/")
	tree, keys := buildInnerNode(t, prefix, 50, true)
	if got := rootKindOf(tree); got != "node256" {
		t.Fatalf("rootKindOf = %q, want %q", got, "node256")
	}
	terminalKey := keys[0]

	collect := func(start, end []byte) [][]byte {
		var out [][]byte
		for k := range tree.Range(start, end) {
			out = append(out, bytes.Clone(k))
		}
		return out
	}
	assertSorted := func(t *testing.T, ks [][]byte) {
		t.Helper()
		for i := 1; i < len(ks); i++ {
			if bytes.Compare(ks[i-1], ks[i]) >= 0 {
				t.Fatalf("not sorted at %d: %v then %v", i, ks[i-1], ks[i])
			}
		}
	}

	t.Run("full_range", func(t *testing.T) {
		got := collect(nil, nil)
		if len(got) != 51 {
			t.Fatalf("full Range len = %d, want 51", len(got))
		}
		assertSorted(t, got)
	})

	t.Run("start_at_terminal", func(t *testing.T) {
		got := collect(terminalKey, nil)
		if len(got) != 51 {
			t.Fatalf("Range(terminal, nil) len = %d, want 51", len(got))
		}
		if !bytes.Equal(got[0], terminalKey) {
			t.Fatalf("first key = %v, want terminal %v", got[0], terminalKey)
		}
		assertSorted(t, got)
	})

	t.Run("end_just_after_zero_edge_child", func(t *testing.T) {
		firstChildKey := append(bytes.Clone(prefix), 0x00)
		end := append(bytes.Clone(firstChildKey), 0x01)
		got := collect(nil, end)
		if len(got) != 2 {
			t.Fatalf("Range(nil, %v) len = %d, want 2 (got %v)", end, len(got), got)
		}
		if !bytes.Equal(got[0], terminalKey) {
			t.Fatalf("got[0] = %v, want terminal %v", got[0], terminalKey)
		}
		if !bytes.Equal(got[1], firstChildKey) {
			t.Fatalf("got[1] = %v, want firstChild %v", got[1], firstChildKey)
		}
		assertSorted(t, got)
	})

	t.Run("strictly_after_terminal", func(t *testing.T) {
		start := append(bytes.Clone(terminalKey), 0x00)
		got := collect(start, nil)
		if len(got) != 50 {
			t.Fatalf("Range(%v, nil) len = %d, want 50", start, len(got))
		}
		for _, k := range got {
			if bytes.Equal(k, terminalKey) {
				t.Fatalf("result unexpectedly contains terminal %v", terminalKey)
			}
		}
		assertSorted(t, got)
	})
}

func TestNode256RemoveUpdatesNumChildren(t *testing.T) {
	prefix := []byte("p/")
	tree, keys := buildInnerNode(t, prefix, 49, false)
	if got := rootKindOf(tree); got != "node256" {
		t.Fatalf("rootKindOf at 49 children = %q, want %q", got, "node256")
	}

	victim := keys[0]
	if ok := tree.Delete(victim); !ok {
		t.Fatalf("Delete(%v) = false, want true", victim)
	}
	if got := rootKindOf(tree); got != "node48" {
		t.Fatalf("rootKindOf after delete to 48 children = %q, want %q", got, "node48")
	}

	for _, k := range keys[1:] {
		want := int(k[len(prefix)])
		got, ok := tree.Get(k)
		if !ok {
			t.Fatalf("Get(%v) ok=false, want true", k)
		}
		if gi, _ := got.(int); gi != want {
			t.Fatalf("Get(%v) = %v, want %d", k, got, want)
		}
	}
}

// buildRootWithInnerChild builds a tree whose root is an inner node of
// the requested kind and whose edge byte 'A' leads to another inner
// node with a non-empty prefix and two branching leaves. Returns the
// tree and the deep key that traverses root -> inner-node -> leaf.
func buildRootWithInnerChild(t *testing.T, rootKind string) (*Tree, []byte) {
	t.Helper()
	var fillers [][]byte
	switch rootKind {
	case "node16":
		for c := byte('B'); c <= byte('P'); c++ {
			fillers = append(fillers, []byte{c})
		}
	case "node48":
		for c := byte('B'); c <= byte('T'); c++ {
			fillers = append(fillers, []byte{c})
		}
	case "node256":
		for b := 1; b <= 49; b++ {
			fillers = append(fillers, []byte{byte(b)})
		}
	default:
		t.Fatalf("buildRootWithInnerChild: unsupported rootKind %q", rootKind)
	}
	tree := New()
	for i, k := range fillers {
		tree.Put(k, i)
	}
	var edge byte
	if rootKind == "node256" {
		edge = 0
	} else {
		edge = 'A'
	}
	deepA := []byte{edge, 'X', 'Y', 'Z', 'a'}
	deepB := []byte{edge, 'X', 'Y', 'Z', 'b'}
	tree.Put(deepA, "deepA")
	tree.Put(deepB, "deepB")
	if got := rootKindOf(tree); got != rootKind {
		t.Fatalf("root kind = %q, want %q", got, rootKind)
	}
	return tree, deepA
}

// TestGetThroughNode16ThenInnerNode exercises Get where the root is a
// node16 and the traversed child is another inner node with a
// non-empty prefix. A depth-- mutation on the node16 depth++ would
// misalign the child's prefix compare and the Get must fail.
func TestGetThroughNode16ThenInnerNode(t *testing.T) {
	tree, deep := buildRootWithInnerChild(t, "node16")
	got, ok := tree.Get(deep)
	if !ok || got != "deepA" {
		t.Fatalf("Get(%q) = (%v, %v), want (%q, true)", deep, got, ok, "deepA")
	}
}

// TestGetThroughNode48ThenInnerNode exercises Get where the root is a
// node48 and the traversed child is an inner node with a non-empty
// prefix. A depth-- mutation on the node48 depth++ would misalign the
// child's prefix compare and the Get must fail.
func TestGetThroughNode48ThenInnerNode(t *testing.T) {
	tree, deep := buildRootWithInnerChild(t, "node48")
	got, ok := tree.Get(deep)
	if !ok || got != "deepA" {
		t.Fatalf("Get(%q) = (%v, %v), want (%q, true)", deep, got, ok, "deepA")
	}
}

// TestGetThroughNode256ThenInnerNode exercises Get where the root is a
// node256 and the traversed child is an inner node with a non-empty
// prefix. A depth-- mutation on the node256 depth++ would misalign the
// child's prefix compare and the Get must fail.
func TestGetThroughNode256ThenInnerNode(t *testing.T) {
	tree, deep := buildRootWithInnerChild(t, "node256")
	got, ok := tree.Get(deep)
	if !ok || got != "deepA" {
		t.Fatalf("Get(%q) = (%v, %v), want (%q, true)", deep, got, ok, "deepA")
	}
}

// TestInlineLeafBoundaryAndLongKey pins the inlineKeyMax boundary and
// the heap-allocated long-key branch in newLeaf. The 24-byte subtest
// kills the CONDITIONALS_BOUNDARY mutant (<= -> <); the 48-byte
// subtest kills the CONDITIONALS_NEGATION mutant (<= -> >), under
// which the long key would be silently truncated into the inline
// buffer and Get/Delete/iteration would fail.
func TestInlineLeafBoundaryAndLongKey(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"exactlyInlineKeyMax", 24},
		{"longerThanInlineKeyMax", 48},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := make([]byte, tc.size)
			for i := range key {
				key[i] = byte('a' + (i % 26))
			}
			tree := New()
			tree.Put(key, tc.size)

			l, isLeaf := tree.root.(*leaf)
			if !isLeaf {
				t.Fatalf("root kind = %q, want leaf", rootKindOf(tree))
			}
			inlineAliased := len(l.key) > 0 && &l.inline[0] == &l.key[0]
			if tc.size <= inlineKeyMax && !inlineAliased {
				t.Fatalf("len=%d key not stored inline: &l.key[0]=%p, &l.inline[0]=%p",
					tc.size, &l.key[0], &l.inline[0])
			}
			if tc.size > inlineKeyMax && inlineAliased {
				t.Fatalf("len=%d key stored inline (truncation risk)", tc.size)
			}

			got, ok := tree.Get(key)
			if !ok || got != tc.size {
				t.Fatalf("Get(len=%d) = (%v, %v), want (%d, true)", tc.size, got, ok, tc.size)
			}

			var seenKeys [][]byte
			var seenVals []any
			for k, v := range tree.All() {
				seenKeys = append(seenKeys, bytes.Clone(k))
				seenVals = append(seenVals, v)
			}
			if len(seenKeys) != 1 || !bytes.Equal(seenKeys[0], key) || seenVals[0] != tc.size {
				t.Fatalf("All() = (%v, %v), want single (%v, %d)", seenKeys, seenVals, key, tc.size)
			}

			if !tree.Delete(key) {
				t.Fatalf("Delete(len=%d) = false, want true", tc.size)
			}
			if _, ok := tree.Get(key); ok {
				t.Fatalf("Get after Delete(len=%d) ok=true, want false", tc.size)
			}
		})
	}
}

// newSentinelLeaf returns a leaf carrying a unique tag used for pointer
// identity checks in structural tests of node child-list operations.
func newSentinelLeaf(tag string) *leaf {
	l := &leaf{value: tag}
	l.key = []byte(tag)
	return l
}

// TestNode4ChildListOps exercises node4.replaceChild and node4.removeChild
// under conditions that distinguish several surviving mutants: the loop
// bound (< vs <=), the key-match comparison (== vs !=), the loop
// increment (i++ vs i--), and the loop-start negation (< vs >=). Only
// direct structural observation of keys/children/numChildren can detect
// the bad slot writes produced by these mutations.
func TestNode4ChildListOps(t *testing.T) {
	t.Run("replaceChildAtMiddleIndex", func(t *testing.T) {
		c0 := newSentinelLeaf("c0")
		c1 := newSentinelLeaf("c1")
		c2 := newSentinelLeaf("c2")
		n := &node4{
			keys:        [4]byte{10, 20, 30, 0},
			children:    [4]node{c0, c1, c2, nil},
			numChildren: 3,
		}
		nc := newSentinelLeaf("new")
		n.replaceChild(20, nc)
		if n.children[0] != c0 {
			t.Fatalf("children[0] = %v, want c0", n.children[0])
		}
		if n.children[1] != nc {
			t.Fatalf("children[1] = %v, want new", n.children[1])
		}
		if n.children[2] != c2 {
			t.Fatalf("children[2] = %v, want c2", n.children[2])
		}
		if n.numChildren != 3 {
			t.Fatalf("numChildren = %d, want 3", n.numChildren)
		}
	})
	t.Run("replaceChildOnAbsentZeroEdge", func(t *testing.T) {
		c1 := newSentinelLeaf("c1")
		c2 := newSentinelLeaf("c2")
		nc := newSentinelLeaf("new")
		n := &node4{
			keys:        [4]byte{1, 2, 0, 0},
			children:    [4]node{c1, c2, nil, nil},
			numChildren: 2,
		}
		n.replaceChild(0, nc)
		if n.children[0] != c1 || n.children[1] != c2 {
			t.Fatalf("live slots modified: %v %v", n.children[0], n.children[1])
		}
		if n.children[2] != nil || n.children[3] != nil {
			t.Fatalf("padding slots modified: %v %v", n.children[2], n.children[3])
		}
		if n.numChildren != 2 {
			t.Fatalf("numChildren = %d, want 2", n.numChildren)
		}
	})
	t.Run("removeChildOnAbsentZeroEdge", func(t *testing.T) {
		c1 := newSentinelLeaf("c1")
		c2 := newSentinelLeaf("c2")
		n := &node4{
			keys:        [4]byte{1, 2, 0, 0},
			children:    [4]node{c1, c2, nil, nil},
			numChildren: 2,
		}
		n.removeChild(0)
		if n.numChildren != 2 {
			t.Fatalf("numChildren = %d, want 2", n.numChildren)
		}
		if n.keys[0] != 1 || n.keys[1] != 2 {
			t.Fatalf("keys corrupted: %v", n.keys)
		}
		if n.children[0] != c1 || n.children[1] != c2 {
			t.Fatalf("children corrupted: %v %v", n.children[0], n.children[1])
		}
	})
}

// TestNode16ChildListOps mirrors TestNode4ChildListOps for node16,
// targeting the same mutation classes on the larger inner node.
func TestNode16ChildListOps(t *testing.T) {
	t.Run("replaceChildAtMiddleIndex", func(t *testing.T) {
		c0 := newSentinelLeaf("c0")
		c1 := newSentinelLeaf("c1")
		c2 := newSentinelLeaf("c2")
		n := &node16{numChildren: 3}
		n.keys[0], n.keys[1], n.keys[2] = 10, 20, 30
		n.children[0], n.children[1], n.children[2] = c0, c1, c2
		nc := newSentinelLeaf("new")
		n.replaceChild(20, nc)
		if n.children[0] != c0 {
			t.Fatalf("children[0] = %v, want c0", n.children[0])
		}
		if n.children[1] != nc {
			t.Fatalf("children[1] = %v, want new", n.children[1])
		}
		if n.children[2] != c2 {
			t.Fatalf("children[2] = %v, want c2", n.children[2])
		}
		if n.numChildren != 3 {
			t.Fatalf("numChildren = %d, want 3", n.numChildren)
		}
	})
	t.Run("replaceChildOnAbsentZeroEdge", func(t *testing.T) {
		c1 := newSentinelLeaf("c1")
		c2 := newSentinelLeaf("c2")
		nc := newSentinelLeaf("new")
		n := &node16{numChildren: 2}
		n.keys[0], n.keys[1] = 1, 2
		n.children[0], n.children[1] = c1, c2
		n.replaceChild(0, nc)
		if n.children[0] != c1 || n.children[1] != c2 {
			t.Fatalf("live slots modified: %v %v", n.children[0], n.children[1])
		}
		for i := uint8(2); i < node16Capacity; i++ {
			if n.children[i] != nil {
				t.Fatalf("children[%d] = %v, want nil", i, n.children[i])
			}
		}
		if n.numChildren != 2 {
			t.Fatalf("numChildren = %d, want 2", n.numChildren)
		}
	})
	t.Run("removeChildOnAbsentZeroEdge", func(t *testing.T) {
		c1 := newSentinelLeaf("c1")
		c2 := newSentinelLeaf("c2")
		n := &node16{numChildren: 2}
		n.keys[0], n.keys[1] = 1, 2
		n.children[0], n.children[1] = c1, c2
		n.removeChild(0)
		if n.numChildren != 2 {
			t.Fatalf("numChildren = %d, want 2", n.numChildren)
		}
		if n.keys[0] != 1 || n.keys[1] != 2 {
			t.Fatalf("keys corrupted: [%d %d]", n.keys[0], n.keys[1])
		}
		if n.children[0] != c1 || n.children[1] != c2 {
			t.Fatalf("children corrupted: %v %v", n.children[0], n.children[1])
		}
	})
}

// TestNode48ReplaceChildSlot pins node48.replaceChild against three
// mutants: the slot-guard negation (slot == 0 vs slot != 0), the
// slot-1 index sign inversion, and the slot-1 -> slot+1 arithmetic
// mutation. All require direct observation of which slot is written.
func TestNode48ReplaceChildSlot(t *testing.T) {
	c5 := newSentinelLeaf("c5")
	c50 := newSentinelLeaf("c50")
	c200 := newSentinelLeaf("c200")
	n := &node48{}
	n.addChild(5, c5)
	n.addChild(50, c50)
	n.addChild(200, c200)

	slot5 := n.childIndex[5]
	slot50 := n.childIndex[50]
	slot200 := n.childIndex[200]
	if slot5 == 0 || slot50 == 0 || slot200 == 0 {
		t.Fatalf("setup: childIndex not populated: %d %d %d", slot5, slot50, slot200)
	}

	new50 := newSentinelLeaf("new50")
	n.replaceChild(50, new50)

	if n.children[slot50-1] != new50 {
		t.Fatalf("children[slot50-1] = %v, want new50", n.children[slot50-1])
	}
	if n.children[slot5-1] != c5 {
		t.Fatalf("children[slot5-1] = %v, want c5", n.children[slot5-1])
	}
	if n.children[slot200-1] != c200 {
		t.Fatalf("children[slot200-1] = %v, want c200", n.children[slot200-1])
	}
	if n.childIndex[5] != slot5 || n.childIndex[50] != slot50 || n.childIndex[200] != slot200 {
		t.Fatalf("childIndex modified: [5]=%d [50]=%d [200]=%d",
			n.childIndex[5], n.childIndex[50], n.childIndex[200])
	}
	if n.numChildren != 3 {
		t.Fatalf("numChildren = %d, want 3", n.numChildren)
	}

	var snapshot [node48Capacity]node
	for i := 0; i < node48Capacity; i++ {
		snapshot[i] = n.children[i]
	}
	nc := newSentinelLeaf("absent")
	n.replaceChild(99, nc)
	for i := 0; i < node48Capacity; i++ {
		if n.children[i] != snapshot[i] {
			t.Fatalf("replaceChild on absent edge modified children[%d]: %v -> %v",
				i, snapshot[i], n.children[i])
		}
	}
	if n.numChildren != 3 {
		t.Fatalf("numChildren = %d after absent replace, want 3", n.numChildren)
	}
}

// TestRangeEmptyAndReversedBounds covers the Range top-level guard for
// equal, reversed, and mixed-nil bounds. Equal and reversed bounds
// must yield nothing; fully-nil bounds must yield every key; half-nil
// bounds must still enumerate the open side.
func TestRangeEmptyAndReversedBounds(t *testing.T) {
	tree := New()
	keys := [][]byte{[]byte("alpha"), []byte("beta"), []byte("gamma"), []byte("zeta")}
	for i, k := range keys {
		tree.Put(k, i)
	}

	collect := func(start, end []byte) [][]byte {
		var got [][]byte
		for k := range tree.Range(start, end) {
			got = append(got, bytes.Clone(k))
		}
		return got
	}

	if got := collect([]byte("beta"), []byte("beta")); len(got) != 0 {
		t.Fatalf("Range(beta,beta) = %v, want empty", got)
	}
	if got := collect([]byte("z"), []byte("a")); len(got) != 0 {
		t.Fatalf("Range(z,a) = %v, want empty (reversed bounds)", got)
	}
	if got := collect(nil, nil); len(got) != len(keys) {
		t.Fatalf("Range(nil,nil) yielded %d keys, want %d", len(got), len(keys))
	}
	if got := collect([]byte("beta"), nil); len(got) != 3 {
		t.Fatalf("Range(beta,nil) = %v, want 3 keys", got)
	}
	if got := collect(nil, []byte("gamma")); len(got) != 2 {
		t.Fatalf("Range(nil,gamma) = %v, want 2 keys", got)
	}
}

// TestRangeThroughNode48WithEdgeZeroChild builds a tree whose root is a
// node48 with a child under edge byte 0, then asserts Range(nil, nil)
// yields every key exactly once. The iterateRange :176 boundary mutant
// (edge < 256 -> <=) would, at edge == 256, wrap byte(edge) back to 0
// and re-yield the edge-0 subtree.
func TestRangeThroughNode48WithEdgeZeroChild(t *testing.T) {
	tree := New()
	keys := [][]byte{{0, 'x'}}
	for i := 0; i < 16; i++ {
		keys = append(keys, []byte{byte(1 + i), 'y'})
	}
	for i, k := range keys {
		tree.Put(k, i)
	}
	if got := rootKindOf(tree); got != "node48" {
		t.Fatalf("root = %q, want node48 (setup failed)", got)
	}

	seen := make(map[string]int)
	total := 0
	for k := range tree.Range(nil, nil) {
		seen[string(k)]++
		total++
	}
	if total != len(keys) {
		t.Fatalf("total yields = %d, want %d", total, len(keys))
	}
	for _, k := range keys {
		if seen[string(k)] != 1 {
			t.Fatalf("key %v yielded %d times, want exactly 1", k, seen[string(k)])
		}
	}
}
