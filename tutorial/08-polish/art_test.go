package polish

import (
	"bytes"
	"reflect"
	"sort"
	"testing"
	"unsafe"
)

func TestPutGetDelete(t *testing.T) {
	tree := New[int]()
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

// Polish #1: short keys live in the leaf's inline buffer; long
// keys go to a separately-allocated slice.
func TestInlineKeyBoundary(t *testing.T) {
	short := bytes.Repeat([]byte{'k'}, inlineKeyMax)
	l := newLeaf[int](short, 1)
	if !slicePointsInto(l.key, l.inline[:]) {
		t.Fatalf("len=%d key (== inlineKeyMax) does not alias inline buffer", len(short))
	}
	long := bytes.Repeat([]byte{'k'}, inlineKeyMax+1)
	l = newLeaf[int](long, 1)
	if slicePointsInto(l.key, l.inline[:]) {
		t.Fatalf("len=%d key (> inlineKeyMax) aliases inline buffer", len(long))
	}
}

func slicePointsInto(sub, arr []byte) bool {
	if len(arr) == 0 || len(sub) == 0 {
		return false
	}
	subBase := uintptr(unsafe.Pointer(unsafe.SliceData(sub)))
	arrBase := uintptr(unsafe.Pointer(unsafe.SliceData(arr)))
	arrEnd := arrBase + uintptr(len(arr))
	return subBase >= arrBase && subBase < arrEnd
}

// Polish #2: every inner-node type's prefix/terminal accessors
// come from the embedded innerHeader by promotion, not from
// per-type duplication.
func TestInnerHeaderEmbeddingSatisfiesInterface(t *testing.T) {
	var _ innerNode = (*node4[int])(nil)
	var _ innerNode = (*node16[int])(nil)
	var _ innerNode = (*node48[int])(nil)
	var _ innerNode = (*node256[int])(nil)
}

// Polish #3: Range with reused path buffer.
func TestRangeBasics(t *testing.T) {
	tree := New[int]()
	keys := []string{"alpha", "beta", "gamma", "zeta"}
	for i, k := range keys {
		tree.Put([]byte(k), i)
	}

	collect := func(start, end []byte) []string {
		var got []string
		for k := range tree.Range(start, end) {
			got = append(got, string(k))
		}
		return got
	}

	if got := collect([]byte("beta"), []byte("h")); !reflect.DeepEqual(got, []string{"beta", "gamma"}) {
		t.Fatalf("Range(beta, h) = %v", got)
	}
	if got := collect(nil, nil); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma", "zeta"}) {
		t.Fatalf("Range(nil, nil) = %v", got)
	}
	if got := collect([]byte("beta"), nil); !reflect.DeepEqual(got, []string{"beta", "gamma", "zeta"}) {
		t.Fatalf("Range(beta, nil) = %v", got)
	}
	if got := collect(nil, []byte("gamma")); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("Range(nil, gamma) = %v", got)
	}
	if got := collect([]byte("beta"), []byte("beta")); len(got) != 0 {
		t.Fatalf("Range(beta, beta) = %v, want empty", got)
	}
}

func TestRangeAcrossLargeTree(t *testing.T) {
	tree := New[int]()
	want := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		k := []byte{byte(i / 256), byte(i % 256)}
		tree.Put(k, i)
		want = append(want, string(k))
	}
	sort.Strings(want)
	// Range over the middle third.
	lo, hi := want[300], want[700]
	expected := want[300:700]

	var got []string
	for k := range tree.Range([]byte(lo), []byte(hi)) {
		got = append(got, string(k))
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Range mismatch (got %d, want %d)", len(got), len(expected))
	}
}
