package artmap_test

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/artmap"
)

func TestOrdered_Int64_NegativeRangeIsSignedOrder(t *testing.T) {
	m := artmap.New[int64, string]()
	input := []int64{math.MinInt64, -1000, -1, 0, 1, 1000, math.MaxInt64}
	for _, k := range input {
		m.Put(k, fmt.Sprint(k))
	}

	// A range that straddles zero must come back in signed ascending
	// order: the byte-wise order of naïvely big-endian-encoded ints
	// would sort negatives after non-negatives; flipping the sign bit
	// fixes that.
	var got []int64
	for k := range m.Range(-1000, 1001) {
		got = append(got, k)
	}
	want := []int64{-1000, -1, 0, 1, 1000}
	if !equalInt64(got, want) {
		t.Fatalf("Range(-1000, 1001): got %v, want %v", got, want)
	}

	var all []int64
	for k := range m.All() {
		all = append(all, k)
	}
	if !sort.SliceIsSorted(all, func(i, j int) bool { return all[i] < all[j] }) {
		t.Fatalf("All not in signed ascending order: %v", all)
	}
}

func TestOrdered_Float64_OrderAcrossZero(t *testing.T) {
	m := artmap.New[float64, int]()
	keys := []float64{math.Inf(-1), -1e9, -1.5, math.Copysign(0, -1), 0.0, 1.5, 1e9, math.Inf(1)}
	for i, k := range keys {
		m.Put(k, i)
	}
	var got []float64
	for k := range m.All() {
		got = append(got, k)
	}
	if !sort.Float64sAreSorted(got) {
		t.Fatalf("Float64 All not sorted: %v", got)
	}
}

func TestOrdered_Clone_IsIndependent(t *testing.T) {
	m := artmap.New[int32, int]()
	for i := int32(-5); i <= 5; i++ {
		m.Put(i, int(i))
	}
	cp := m.Clone()
	m.Delete(0)
	if _, ok := m.Get(0); ok {
		t.Fatal("0 should be deleted from original")
	}
	if v, ok := cp.Get(0); !ok || v != 0 {
		t.Fatalf("Clone lost entry for 0: v=%d ok=%v", v, ok)
	}
}

func TestOrdered_CeilingFloor_Int64(t *testing.T) {
	m := artmap.New[int64, string]()
	for _, k := range []int64{-10, -1, 0, 5, 100} {
		m.Put(k, fmt.Sprint(k))
	}
	cases := []struct {
		target  int64
		ceilK   int64
		ceilOK  bool
		floorK  int64
		floorOK bool
	}{
		{target: -20, ceilK: -10, ceilOK: true, floorOK: false},
		{target: -5, ceilK: -1, ceilOK: true, floorK: -10, floorOK: true},
		{target: 0, ceilK: 0, ceilOK: true, floorK: 0, floorOK: true},
		{target: 50, ceilK: 100, ceilOK: true, floorK: 5, floorOK: true},
		{target: 200, ceilOK: false, floorK: 100, floorOK: true},
	}
	for _, c := range cases {
		ck, _, ok := m.Ceiling(c.target)
		if ok != c.ceilOK || (ok && ck != c.ceilK) {
			t.Errorf("Ceiling(%d)=(%d,%v), want (%d,%v)", c.target, ck, ok, c.ceilK, c.ceilOK)
		}
		fk, _, ok := m.Floor(c.target)
		if ok != c.floorOK || (ok && fk != c.floorK) {
			t.Errorf("Floor(%d)=(%d,%v), want (%d,%v)", c.target, fk, ok, c.floorK, c.floorOK)
		}
	}
}

func TestOrdered_RangeDescending_Int64(t *testing.T) {
	m := artmap.New[int64, int]()
	for _, k := range []int64{-5, -1, 0, 2, 7, 12} {
		m.Put(k, 1)
	}
	var got []int64
	for k := range m.RangeDescending(-3, 10) {
		got = append(got, k)
	}
	want := []int64{7, 2, 0, -1}
	if !equalInt64(got, want) {
		t.Fatalf("RangeDescending(-3,10): got %v want %v", got, want)
	}
}

func TestOrdered_RangeFromTo_String(t *testing.T) {
	m := artmap.New[string, int]()
	for i, k := range []string{"apple", "apricot", "banana", "cherry"} {
		m.Put(k, i)
	}
	var from []string
	for k := range m.RangeFrom("b") {
		from = append(from, k)
	}
	if !equalString(from, []string{"banana", "cherry"}) {
		t.Fatalf("RangeFrom b: %v", from)
	}
	var to []string
	for k := range m.RangeTo("b") {
		to = append(to, k)
	}
	if !equalString(to, []string{"apple", "apricot"}) {
		t.Fatalf("RangeTo b: %v", to)
	}
}

// TestOrdered_NamedTypes exercises the ~T side of the cmp.Ordered
// constraint: a user-defined type whose underlying type is int64 must
// encode identically to int64.
func TestOrdered_NamedTypes(t *testing.T) {
	type userID int64
	m := artmap.New[userID, string]()
	m.Put(-42, "neg")
	m.Put(0, "zero")
	m.Put(42, "pos")
	if v, ok := m.Get(-42); !ok || v != "neg" {
		t.Fatalf("Get(-42)=%q,%v", v, ok)
	}
	var got []userID
	for k := range m.All() {
		got = append(got, k)
	}
	if got[0] != -42 || got[1] != 0 || got[2] != 42 {
		t.Fatalf("named-type ordering wrong: %v", got)
	}
}

// TestOrdered_String_EmptyEqualBoundsYieldNothing pins the fix for
// the asymmetry where Range("", "") on a string-keyed Ordered used
// to fall through to All(): both empty-string bounds encoded to nil
// because append([]byte(nil), []byte("")...) returns nil, so the
// underlying tree saw two nil bounds and treated the call as
// unbounded. Switching the bounds-clone to bytes.Clone preserves the
// non-nil zero-length slice so the equal-bounds half-open interval
// yields nothing, matching numeric K behavior.
func TestOrdered_String_EmptyEqualBoundsYieldNothing(t *testing.T) {
	m := artmap.New[string, int]()
	m.Put("", 0) // yes, even with the empty key in the map
	m.Put("a", 1)
	m.Put("b", 2)

	count := 0
	for range m.Range("", "") {
		count++
	}
	if count != 0 {
		t.Fatalf("Range(\"\", \"\") yielded %d, want 0", count)
	}

	count = 0
	for range m.RangeDescending("", "") {
		count++
	}
	if count != 0 {
		t.Fatalf("RangeDescending(\"\", \"\") yielded %d, want 0", count)
	}
}

// TestOrdered_String_EmptyStartIsUnboundedLow pins that "" as the
// start (the smallest possible string) still passes through to the
// tree as an inclusive lower bound and yields every key, matching
// the natural reading "all keys >= empty string".
func TestOrdered_String_EmptyStartIsUnboundedLow(t *testing.T) {
	m := artmap.New[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	count := 0
	for range m.RangeFrom("") {
		count++
	}
	if count != 3 {
		t.Fatalf("RangeFrom(\"\") yielded %d, want 3", count)
	}
}

// TestOrdered_ReversedBoundsPanic pins that the underlying tree's
// reversed-bounds panic surfaces through the typed Ordered facade
// (numeric K, where the encoded bytes are unambiguously ordered).
func TestOrdered_ReversedBoundsPanic(t *testing.T) {
	m := artmap.New[int64, int]()
	m.Put(1, 1)
	m.Put(2, 2)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("Range(10, 1) did not panic")
		}
		msg, _ := r.(string)
		want := "art: Range called with reversed bounds (start > end)"
		if msg != want {
			t.Fatalf("panic = %q, want %q", msg, want)
		}
	}()
	_ = m.Range(10, 1)
}

// TestOrdered_ZeroValuePanics pins the typed-panic guard on every
// public Ordered method. A zero-value Ordered[K, V]{} has nil tree
// and nil decoder; without the requireConstructed guard each method
// would surface a generic Go runtime nil-deref, which Goal #1
// forbids.
func TestOrdered_ZeroValuePanics(t *testing.T) {
	const want = "artmap: Ordered must be constructed with New"
	cases := []struct {
		name string
		call func(*artmap.Ordered[int64, int])
	}{
		{"Len", func(o *artmap.Ordered[int64, int]) { _ = o.Len() }},
		{"Put", func(o *artmap.Ordered[int64, int]) { o.Put(1, 1) }},
		{"Get", func(o *artmap.Ordered[int64, int]) { _, _ = o.Get(1) }},
		{"Delete", func(o *artmap.Ordered[int64, int]) { _ = o.Delete(1) }},
		{"Min", func(o *artmap.Ordered[int64, int]) { _, _, _ = o.Min() }},
		{"Max", func(o *artmap.Ordered[int64, int]) { _, _, _ = o.Max() }},
		{"Ceiling", func(o *artmap.Ordered[int64, int]) { _, _, _ = o.Ceiling(1) }},
		{"Floor", func(o *artmap.Ordered[int64, int]) { _, _, _ = o.Floor(1) }},
		{"Clone", func(o *artmap.Ordered[int64, int]) { _ = o.Clone() }},
		{"All", func(o *artmap.Ordered[int64, int]) { _ = o.All() }},
		{"AllDescending", func(o *artmap.Ordered[int64, int]) { _ = o.AllDescending() }},
		{"Range", func(o *artmap.Ordered[int64, int]) { _ = o.Range(1, 2) }},
		{"RangeFrom", func(o *artmap.Ordered[int64, int]) { _ = o.RangeFrom(1) }},
		{"RangeTo", func(o *artmap.Ordered[int64, int]) { _ = o.RangeTo(1) }},
		{"RangeDescending", func(o *artmap.Ordered[int64, int]) { _ = o.RangeDescending(1, 2) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("%s on zero-value Ordered did not panic", tc.name)
				}
				msg, _ := r.(string)
				if msg != want {
					t.Fatalf("%s panic = %q, want %q", tc.name, msg, want)
				}
			}()
			var o artmap.Ordered[int64, int]
			tc.call(&o)
		})
	}
}

func equalInt64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalString(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
