package art

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
	"unsafe"
)

// This file is the test counterpart of INVARIANTS.md. Each test is
// named TestInvariant_<Name> and exercises one structural rule
// directly, rather than through a Get/Range round-trip. Behavioural
// equivalence to the Go-map oracle lives in art_fuzz_test.go; the
// tests here catch structural drift (wrong shape, miscounted slot,
// stale index) that would not change a Get result.

// walkInnerNodes invokes visit on every inner node reachable from
// root, in pre-order, passing the byte path consumed from the root to
// (and including) that node's prefix. The supplied path slice must not
// be retained by visit beyond the call.
func walkInnerNodes(root node, visit func(path []byte, n innerNode)) {
	var rec func(n node, path []byte)
	rec = func(n node, path []byte) {
		if n == nil {
			return
		}
		r, ok := n.(innerNode)
		if !ok {
			return
		}
		nodePath := append(append([]byte(nil), path...), r.getPrefix()...)
		visit(nodePath, r)
		r.eachAscending(func(b byte, child node) bool {
			rec(child, append(nodePath, b))
			return true
		})
	}
	rec(root, nil)
}

// occupiedEdges returns the edge bytes of n's branching children in
// ascending order, decoded directly from the node's own representation
// (not via eachAscending, so the test catches drift between the
// representation and the iterator).
func occupiedEdges(n innerNode) []byte {
	switch r := n.(type) {
	case *node4:
		return append([]byte(nil), r.keys[:r.numChildren]...)
	case *node16:
		return append([]byte(nil), r.keys[:r.numChildren]...)
	case *node48:
		out := make([]byte, 0, r.numChildren)
		for b := 0; b < 256; b++ {
			if r.childIndex[byte(b)] != 0 {
				out = append(out, byte(b))
			}
		}
		return out
	case *node256:
		out := make([]byte, 0, r.numChildren)
		for b := 0; b < 256; b++ {
			if r.children[b] != nil {
				out = append(out, byte(b))
			}
		}
		return out
	}
	return nil
}

// occupiedChildCount returns the count of branching children stored
// in n's representation. Used to cross-check numChildren.
func occupiedChildCount(n innerNode) int {
	switch r := n.(type) {
	case *node4:
		count := 0
		for i := 0; i < node4Capacity; i++ {
			if r.children[i] != nil {
				count++
			}
		}
		return count
	case *node16:
		count := 0
		for i := 0; i < node16Capacity; i++ {
			if r.children[i] != nil {
				count++
			}
		}
		return count
	case *node48:
		count := 0
		for b := 0; b < 256; b++ {
			if r.childIndex[byte(b)] != 0 {
				count++
			}
		}
		return count
	case *node256:
		count := 0
		for b := 0; b < 256; b++ {
			if r.children[b] != nil {
				count++
			}
		}
		return count
	}
	return 0
}

// countLeaves returns the number of *leaf[int] reachable from n,
// counting both terminal slots and branching children.
func countLeaves(n node) int {
	switch n.(type) {
	case nil:
		return 0
	case *leaf[int]:
		return 1
	}
	r := n.(innerNode)
	count := 0
	if _, ok := r.getTerminal().(*leaf[int]); ok {
		count++
	}
	r.eachAscending(func(_ byte, c node) bool {
		count += countLeaves(c)
		return true
	})
	return count
}

// fillTreeAcrossNodeTypes builds a tree that contains at least one
// instance of each inner node type by carefully choosing key fan-out.
// Returns the tree and the keys inserted, in insertion order.
func fillTreeAcrossNodeTypes(t *testing.T) (*Tree[int], [][]byte) {
	t.Helper()
	tree := New[int]()
	var keys [][]byte
	// 60 distinct first bytes -> root will be a node256 (capacity boundary
	// crossings: 4 -> 16 -> 48 -> 256).
	for i := 0; i < 60; i++ {
		k := []byte{byte(i), 'x'}
		tree.Put(k, i)
		keys = append(keys, k)
	}
	// Add a shared-prefix family so we exercise prefix splits and a
	// nested node4 / node16 elsewhere in the tree.
	for _, suffix := range []string{"a", "b", "c", "d", "e", "f"} {
		k := append([]byte("common-"), suffix...)
		tree.Put(k, 1000+len(suffix))
		keys = append(keys, k)
	}
	return tree, keys
}

func TestInvariant_Node4ChildrenSortedAscending(t *testing.T) {
	tree := New[int]()
	// Insert four distinct first bytes in scrambled order so the sort
	// invariant cannot be satisfied by accident of insertion order.
	for _, b := range []byte{0x40, 0x10, 0x80, 0x20} {
		tree.Put([]byte{b}, int(b))
	}
	root, ok := tree.root.(*node4)
	if !ok {
		t.Fatalf("root kind = %T, want *node4", tree.root)
	}
	keys := root.keys[:root.numChildren]
	if !sort.SliceIsSorted(keys, func(i, j int) bool { return keys[i] < keys[j] }) {
		t.Fatalf("node4 keys not ascending: %v", keys)
	}
}

func TestInvariant_Node16ChildrenSortedAscending(t *testing.T) {
	tree := New[int]()
	// 10 distinct first bytes -> still inside a node16 after promotion.
	for _, b := range []byte{0x55, 0x05, 0x95, 0x35, 0x75, 0x15, 0x85, 0x25, 0x65, 0x45} {
		tree.Put([]byte{b}, int(b))
	}
	root, ok := tree.root.(*node16)
	if !ok {
		t.Fatalf("root kind = %T, want *node16", tree.root)
	}
	keys := root.keys[:root.numChildren]
	if !sort.SliceIsSorted(keys, func(i, j int) bool { return keys[i] < keys[j] }) {
		t.Fatalf("node16 keys not ascending: %v", keys)
	}
}

func TestInvariant_NumChildrenMatchesOccupiedSlots(t *testing.T) {
	tree, _ := fillTreeAcrossNodeTypes(t)
	walkInnerNodes(tree.root, func(path []byte, n innerNode) {
		want := occupiedChildCount(n)
		var got int
		switch r := n.(type) {
		case *node4:
			got = int(r.numChildren)
		case *node16:
			got = int(r.numChildren)
		case *node48:
			got = int(r.numChildren)
		case *node256:
			got = int(r.numChildren)
		}
		if got != want {
			t.Fatalf("at path %q: numChildren=%d, occupied=%d (%T)", path, got, want, n)
		}
	})
}

func TestInvariant_TerminalKeyEqualsNodePath(t *testing.T) {
	tree := New[int]()
	// "common-" is held as a terminal at the inner node whose path is
	// "common-"; the longer keys hang as children below it.
	tree.Put([]byte("common-"), 0)
	for _, suffix := range []string{"a", "b", "c", "d", "e"} {
		tree.Put(append([]byte("common-"), suffix...), int(suffix[0]))
	}
	// Also a deeper terminal: empty key as the root terminal.
	tree.Put(nil, -1)
	saw := 0
	walkInnerNodes(tree.root, func(path []byte, n innerNode) {
		l, ok := n.getTerminal().(*leaf[int])
		if !ok {
			return
		}
		saw++
		if !bytes.Equal(l.key, path) {
			t.Fatalf("terminal key %q != node path %q", l.key, path)
		}
	})
	if saw < 2 {
		t.Fatalf("expected at least 2 terminals to be checked, saw %d", saw)
	}
}

func TestInvariant_NoEmptyInnerNodesAfterDelete(t *testing.T) {
	tree, keys := fillTreeAcrossNodeTypes(t)
	// Delete every other key, then half of what remains, then a final
	// pass: triggers reshape across all node types.
	for _, step := range []int{2, 3, 1} {
		for i := 0; i < len(keys); i += step {
			tree.Delete(keys[i])
		}
	}
	walkInnerNodes(tree.root, func(path []byte, n innerNode) {
		if occupiedChildCount(n) == 0 && n.getTerminal() == nil {
			t.Fatalf("inner node %T at path %q has 0 children and nil terminal", n, path)
		}
	})
}

func TestInvariant_SingleChildNode4Collapses(t *testing.T) {
	tree := New[int]()
	// Build a node4 with two children sharing prefix "ap": "apple", "april".
	// Deleting one must collapse the parent into the survivor with the
	// "ap" prefix merged.
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("april"), 2)
	tree.Delete([]byte("apple"))
	// Survivor should be a single leaf at the root carrying "april".
	l, ok := tree.root.(*leaf[int])
	if !ok {
		t.Fatalf("root kind = %T, want *leaf", tree.root)
	}
	if !bytes.Equal(l.key, []byte("april")) {
		t.Fatalf("survivor key = %q, want %q", l.key, "april")
	}
}

func TestInvariant_PromotesAtCapacityBoundaries(t *testing.T) {
	t.Run("Node4ToNode16", func(t *testing.T) {
		tree := New[int]()
		for i := 0; i < 5; i++ { // 5th distinct first byte forces 4->16.
			tree.Put([]byte{byte(i)}, i)
		}
		if _, ok := tree.root.(*node16); !ok {
			t.Fatalf("after 5 distinct first bytes: root = %T, want *node16", tree.root)
		}
	})
	t.Run("Node16ToNode48", func(t *testing.T) {
		tree := New[int]()
		for i := 0; i < 17; i++ { // 17th distinct first byte forces 16->48.
			tree.Put([]byte{byte(i)}, i)
		}
		if _, ok := tree.root.(*node48); !ok {
			t.Fatalf("after 17 distinct first bytes: root = %T, want *node48", tree.root)
		}
	})
	t.Run("Node48ToNode256", func(t *testing.T) {
		tree := New[int]()
		for i := 0; i < 49; i++ { // 49th distinct first byte forces 48->256.
			tree.Put([]byte{byte(i)}, i)
		}
		if _, ok := tree.root.(*node256); !ok {
			t.Fatalf("after 49 distinct first bytes: root = %T, want *node256", tree.root)
		}
	})
}

func TestInvariant_DemotesAtCapacityBoundaries(t *testing.T) {
	t.Run("Node256ToNode48", func(t *testing.T) {
		tree := New[int]()
		for i := 0; i < 60; i++ {
			tree.Put([]byte{byte(i)}, i)
		}
		// Drop down to 48 children: 60 -> 48 means 12 deletes.
		for i := 0; i < 12; i++ {
			tree.Delete([]byte{byte(i)})
		}
		if _, ok := tree.root.(*node48); !ok {
			t.Fatalf("after demotion to 48: root = %T, want *node48", tree.root)
		}
	})
	t.Run("Node48ToNode16", func(t *testing.T) {
		tree := New[int]()
		for i := 0; i < 30; i++ {
			tree.Put([]byte{byte(i)}, i)
		}
		// Drop down to 16 children: 30 -> 16 means 14 deletes.
		for i := 0; i < 14; i++ {
			tree.Delete([]byte{byte(i)})
		}
		if _, ok := tree.root.(*node16); !ok {
			t.Fatalf("after demotion to 16: root = %T, want *node16", tree.root)
		}
	})
	t.Run("Node16ToNode4", func(t *testing.T) {
		tree := New[int]()
		for i := 0; i < 10; i++ {
			tree.Put([]byte{byte(i)}, i)
		}
		// Drop down to 4 children: 10 -> 4 means 6 deletes.
		for i := 0; i < 6; i++ {
			tree.Delete([]byte{byte(i)})
		}
		if _, ok := tree.root.(*node4); !ok {
			t.Fatalf("after demotion to 4: root = %T, want *node4", tree.root)
		}
	})
}

func TestInvariant_Node48IndexAndEdgeConsistency(t *testing.T) {
	// Build a tree whose root is a node48: 25 distinct first bytes
	// crosses the 16->48 promotion threshold but stays well below 49
	// (the 48->256 threshold).
	tree := New[int]()
	for i := 0; i < 25; i++ {
		tree.Put([]byte{byte(i * 7)}, i)
	}
	if _, ok := tree.root.(*node48); !ok {
		t.Fatalf("fixture root = %T, want *node48", tree.root)
	}
	// Delete a few to exercise the swap-with-last-slot path of removeChild.
	for i := 0; i < 5; i++ {
		tree.Delete([]byte{byte(i * 7)})
	}
	saw := 0
	walkInnerNodes(tree.root, func(path []byte, n innerNode) {
		r, ok := n.(*node48)
		if !ok {
			return
		}
		saw++
		// childIndex non-zero => corresponding slot occupied and edge matches.
		for b := 0; b < 256; b++ {
			slot := r.childIndex[byte(b)]
			if slot == 0 {
				continue
			}
			if slot > r.numChildren {
				t.Fatalf("childIndex[%d]=%d exceeds numChildren=%d", b, slot, r.numChildren)
			}
			if r.children[slot-1] == nil {
				t.Fatalf("childIndex[%d] points to empty slot %d", b, slot-1)
			}
			if r.childEdge[slot-1] != byte(b) {
				t.Fatalf("childIndex[%d]=%d but childEdge[%d]=%d", b, slot, slot-1, r.childEdge[slot-1])
			}
		}
		// Reverse direction: occupied slots have a back-pointer.
		for i := uint8(0); i < r.numChildren; i++ {
			if r.children[i] == nil {
				t.Fatalf("slot %d below numChildren=%d is nil", i, r.numChildren)
			}
			edge := r.childEdge[i]
			if r.childIndex[edge] != i+1 {
				t.Fatalf("slot %d (edge %d) has childIndex=%d, want %d", i, edge, r.childIndex[edge], i+1)
			}
		}
	})
	if saw == 0 {
		t.Fatalf("fixture produced no node48; cannot verify invariant")
	}
}

func TestInvariant_SizeEqualsLeafCount(t *testing.T) {
	tree, keys := fillTreeAcrossNodeTypes(t)
	if got, want := countLeaves(tree.root), tree.Len(); got != want {
		t.Fatalf("after fills: leaves=%d, Len=%d", got, want)
	}
	// Delete a third of the keys and re-check.
	for i := 0; i < len(keys); i += 3 {
		tree.Delete(keys[i])
	}
	if got, want := countLeaves(tree.root), tree.Len(); got != want {
		t.Fatalf("after deletes: leaves=%d, Len=%d", got, want)
	}
	// Replace-value path must not change size.
	before := tree.Len()
	for i := 1; i < len(keys); i += 3 {
		tree.Put(keys[i], -i) // keys[i] still present (we deleted i%3==0).
	}
	if got := tree.Len(); got != before {
		t.Fatalf("after value replacements: Len=%d, want unchanged %d", got, before)
	}
}

func TestInvariant_LeafKeyIsStableCopy(t *testing.T) {
	tree := New[int]()
	mutable := []byte("hello")
	tree.Put(mutable, 7)
	// Mutate the caller's slice. The tree must not see the change.
	mutable[0] = 'X'
	if v, ok := tree.Get([]byte("hello")); !ok || v != 7 {
		t.Fatalf("Get(hello) = (%v, %v), want (7, true) after caller mutation", v, ok)
	}
	if _, ok := tree.Get([]byte("Xello")); ok {
		t.Fatalf("Get(Xello) ok=true; tree key was aliased to caller's buffer")
	}
}

func TestInvariant_CloneIsStructurallyIndependent(t *testing.T) {
	orig, keys := fillTreeAcrossNodeTypes(t)
	clone := orig.Clone()
	// Mutate orig: delete every key. Clone must remain intact.
	for _, k := range keys {
		orig.Delete(k)
	}
	if got, want := clone.Len(), len(keys); got != want {
		t.Fatalf("clone.Len=%d after orig wipe, want %d", got, want)
	}
	for _, k := range keys {
		if _, ok := clone.Get(k); !ok {
			t.Fatalf("clone lost key %q after orig wipe", k)
		}
	}
	// And the reverse direction: a fresh clone, mutate clone, orig must
	// remain intact.
	orig2, keys2 := fillTreeAcrossNodeTypes(t)
	clone2 := orig2.Clone()
	for _, k := range keys2 {
		clone2.Delete(k)
	}
	if got, want := orig2.Len(), len(keys2); got != want {
		t.Fatalf("orig.Len=%d after clone wipe, want %d", got, want)
	}
}

func TestInvariant_EachAscendingIsSorted(t *testing.T) {
	tree, _ := fillTreeAcrossNodeTypes(t)
	walkInnerNodes(tree.root, func(path []byte, n innerNode) {
		var got []byte
		n.eachAscending(func(b byte, _ node) bool {
			got = append(got, b)
			return true
		})
		want := occupiedEdges(n)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("at path %q (%T): eachAscending=%v, want %v", path, n, got, want)
		}
	})
}

func TestInvariant_EachDescendingIsSorted(t *testing.T) {
	tree, _ := fillTreeAcrossNodeTypes(t)
	walkInnerNodes(tree.root, func(path []byte, n innerNode) {
		var got []byte
		n.eachDescending(func(b byte, _ node) bool {
			got = append(got, b)
			return true
		})
		// Expected = reversed occupiedEdges (which is ascending).
		asc := occupiedEdges(n)
		want := make([]byte, len(asc))
		for i, b := range asc {
			want[len(asc)-1-i] = b
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("at path %q (%T): eachDescending=%v, want %v", path, n, got, want)
		}
	})
}

func TestInvariant_InlineKeyBoundary(t *testing.T) {
	t.Run("ShortKeyAliasesInline", func(t *testing.T) {
		short := bytes.Repeat([]byte{'k'}, inlineKeyMax)
		l := newLeaf[int](short, 1)
		if !slicePointsInto(l.key, l.inline[:]) {
			t.Fatalf("len=%d key (== inlineKeyMax) does not alias inline buffer", len(short))
		}
	})
	t.Run("LongKeyDoesNotAliasInline", func(t *testing.T) {
		long := bytes.Repeat([]byte{'k'}, inlineKeyMax+1)
		l := newLeaf[int](long, 1)
		if slicePointsInto(l.key, l.inline[:]) {
			t.Fatalf("len=%d key (> inlineKeyMax) aliases inline buffer", len(long))
		}
	})
}

// slicePointsInto reports whether the backing array of sub overlaps
// the backing array of arr. Used to verify the inline-key boundary.
func slicePointsInto(sub, arr []byte) bool {
	if len(arr) == 0 {
		return false
	}
	subBase := uintptr(unsafe.Pointer(unsafe.SliceData(sub)))
	arrBase := uintptr(unsafe.Pointer(unsafe.SliceData(arr)))
	arrEnd := arrBase + uintptr(len(arr))
	return subBase >= arrBase && subBase < arrEnd
}
