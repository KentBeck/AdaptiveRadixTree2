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
	keys := []float64{math.Inf(-1), -1e9, -1.5, -0.0, 0.0, 1.5, 1e9, math.Inf(1)}
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
