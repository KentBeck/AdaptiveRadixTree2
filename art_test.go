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
