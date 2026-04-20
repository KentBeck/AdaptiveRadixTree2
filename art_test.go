package art

import "testing"

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
