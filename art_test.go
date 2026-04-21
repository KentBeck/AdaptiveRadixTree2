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
