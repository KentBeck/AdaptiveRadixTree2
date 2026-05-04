package introducepolymorphism

import (
	"bytes"
	"runtime"
	"testing"
	"unsafe"

	stage4 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/04-add-node4"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// Stage 5 footprint constants. The terminal field changed from
// *leaf[V] (8 B pointer) to node (16 B interface), so node4 grew
// from 112 B to 120 B (rounded to 128 by Go's allocator) and
// node256 grew from 4 136 B to 4 144 B. That ~7% per-node growth
// is the price of the refactor; chapter 5's prose discusses it
// as a deliberate tradeoff.
var (
	bytesPerNode4   = int(unsafe.Sizeof(node4[int]{}))
	bytesPerNode256 = int(unsafe.Sizeof(node256[int]{}))
	bytesPerLeaf    = int(unsafe.Sizeof(leaf[int]{}))
	// Stage 4's node types stored terminal as *leaf[V] (a pointer),
	// not as the node interface. Same layout otherwise.
	stage4BytesPerNode4   = 24 + 4 + 4 + 64 + 8 + 1 + 7 // prefix + keys + pad + 4 children + *leaf + count + pad
	stage4BytesPerNode256 = 24 + 256*16 + 8 + 2 + 6     // prefix + 256 children + *leaf + count + pad
)

func measureLiveHeap(build func() any) (heapBytes uint64, keepAlive any) {
	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	keepAlive = build()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > before.HeapAlloc {
		heapBytes = after.HeapAlloc - before.HeapAlloc
	}
	return
}

func runPutBenches(b *testing.B, w bench.Workload) {
	b.Run("Stage4", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := stage4.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
		}
	})
	b.Run("Stage5", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
		}
	})
	b.Run("BTree", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := bench.NewBtree()
			for j, k := range w.Keys {
				t.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
			}
		}
	})
}

func runGetBenches(b *testing.B, w bench.Workload) {
	s4 := stage4.New[int]()
	s5 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s4.Put(k, w.Vals[j])
		s5.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage4", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s4.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		_ = sink
	})
	b.Run("Stage5", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s5.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		_ = sink
	})
	b.Run("BTree", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := bt.Get(bench.BtreeItem{Key: w.Keys[i%len(w.Keys)]})
			sink ^= v.Val
		}
		_ = sink
	})
}

func runAllBenches(b *testing.B, w bench.Workload) {
	s4 := stage4.New[int]()
	s5 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s4.Put(k, w.Vals[j])
		s5.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage4", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s4.All() {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage5", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s5.All() {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("BTree", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			bt.Ascend(func(it bench.BtreeItem) bool {
				sink ^= it.Val
				return true
			})
		}
		_ = sink
	})
}

func BenchmarkPut_Dense_1k(b *testing.B)  { runPutBenches(b, bench.Dense(1_000)) }
func BenchmarkPut_Sparse_1k(b *testing.B) { runPutBenches(b, bench.Sparse(1_000)) }
func BenchmarkPut_URL_1k(b *testing.B)    { runPutBenches(b, bench.URL(1_000)) }

func BenchmarkGet_Dense_1k(b *testing.B)  { runGetBenches(b, bench.Dense(1_000)) }
func BenchmarkGet_Sparse_1k(b *testing.B) { runGetBenches(b, bench.Sparse(1_000)) }
func BenchmarkGet_URL_1k(b *testing.B)    { runGetBenches(b, bench.URL(1_000)) }

func BenchmarkAll_Dense_1k(b *testing.B)  { runAllBenches(b, bench.Dense(1_000)) }
func BenchmarkAll_Sparse_1k(b *testing.B) { runAllBenches(b, bench.Sparse(1_000)) }
func BenchmarkAll_URL_1k(b *testing.B)    { runAllBenches(b, bench.URL(1_000)) }

// TestReportFootprint is the chapter-5 honesty check. The structural
// number should match chapter 4's exactly times the small per-node
// growth (terminal slot 8 B -> 16 B). The heap number should be
// essentially unchanged. The "essentially" is doing work: the
// chapter prose owns the explicit comparison.
func TestReportFootprint(t *testing.T) {
	t.Logf("per-node sizes (unsafe.Sizeof): node4=%d  node256=%d  leaf=%d",
		bytesPerNode4, bytesPerNode256, bytesPerLeaf)
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		s4Heap, s4Tree := measureLiveHeap(func() any {
			t := stage4.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s4 := s4Tree.(*stage4.Tree[int])

		s5Heap, s5Tree := measureLiveHeap(func() any {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s5 := s5Tree.(*Tree[int])

		keyBytes := 0
		for _, k := range w.Keys {
			keyBytes += len(k)
		}
		s4n4, s4n256 := s4.CountByKind()
		s4Structural := s4n4*stage4BytesPerNode4 + s4n256*stage4BytesPerNode256 +
			s4.CountLeaves()*bytesPerLeaf + keyBytes + s4.PrefixBytes()
		s5n4, s5n256 := s5.CountByKind()
		s5Structural := s5n4*bytesPerNode4 + s5n256*bytesPerNode256 +
			s5.CountLeaves()*bytesPerLeaf + keyBytes + s5.PrefixBytes()
		nKeys := len(w.Keys)
		t.Logf("%-13s  S4 struct=%4d B/key heap=%4d B/key  |  S5 struct=%4d B/key heap=%4d B/key  (heap %.2fx)",
			w.Name,
			s4Structural/nKeys, int(s4Heap)/nKeys,
			s5Structural/nKeys, int(s5Heap)/nKeys,
			float64(s5Heap)/float64(s4Heap))
	}
}

// runMid1pctBenches measures iteration of the *middle* 1 % of the
// sorted keys: a window from the 49.5th to the 50.5th percentile
// of the keyspace. For btree this is a true sub-range walk via
// AscendRange. For ART chapters without Range, the window is
// hit by walking All and skipping until k >= lo, yielding while
// k < hi, breaking when k >= hi -- so the cost is dominated by
// the unavoidable pre-window descent. Chapter 8 introduces Range
// and uses it directly here, which is the closest comparable
// shape to btree's AscendRange.
func runMid1pctBenches(b *testing.B, w bench.Workload) {
	prev := stage4.New[int]()
	curr := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		prev.Put(k, w.Vals[j])
		curr.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	// Materialize sorted keys once to compute window bounds.
	sorted := make([][]byte, 0, len(w.Keys))
	for k := range curr.All() {
		sorted = append(sorted, append([]byte(nil), k...))
	}
	lo := sorted[len(sorted)*495/1000]
	hi := sorted[len(sorted)*505/1000]

	b.Run("Stage4", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for k, v := range prev.All() {
				if bytes.Compare(k, hi) >= 0 {
					break
				}
				if bytes.Compare(k, lo) < 0 {
					continue
				}
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage5", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for k, v := range curr.All() {
				if bytes.Compare(k, hi) >= 0 {
					break
				}
				if bytes.Compare(k, lo) < 0 {
					continue
				}
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("BTree", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			bt.AscendRange(bench.BtreeItem{Key: lo}, bench.BtreeItem{Key: hi}, func(it bench.BtreeItem) bool {
				sink ^= it.Val
				return true
			})
		}
		_ = sink
	})
}

func BenchmarkMid1pct_Dense_1k(b *testing.B)  { runMid1pctBenches(b, bench.Dense(1_000)) }
func BenchmarkMid1pct_Sparse_1k(b *testing.B) { runMid1pctBenches(b, bench.Sparse(1_000)) }
func BenchmarkMid1pct_URL_1k(b *testing.B)    { runMid1pctBenches(b, bench.URL(1_000)) }
