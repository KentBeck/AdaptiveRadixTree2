package polish

import (
	"runtime"
	"testing"
	"unsafe"

	stage7 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/08-add-node48"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// Stage 8 footprint constants. The leaf grew by inlineKeyMax bytes
// to accommodate the inline buffer; the inner-node sizes shrank
// slightly because innerHeader's two fields no longer pay
// padding inside each per-type struct.
var (
	bytesPerLeaf    = int(unsafe.Sizeof(leaf[int]{}))
	bytesPerNode4   = int(unsafe.Sizeof(node4[int]{}))
	bytesPerNode16  = int(unsafe.Sizeof(node16[int]{}))
	bytesPerNode48  = int(unsafe.Sizeof(node48[int]{}))
	bytesPerNode256 = int(unsafe.Sizeof(node256[int]{}))
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
	b.Run("Stage7", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := stage7.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
		}
	})
	b.Run("Stage8", func(b *testing.B) {
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
	s7 := stage7.New[int]()
	s8 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s7.Put(k, w.Vals[j])
		s8.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage7", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s7.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		_ = sink
	})
	b.Run("Stage8", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s8.Get(w.Keys[i%len(w.Keys)])
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

// runRangeBenches measures Range over the middle 10% of the
// workload's key space, which is the shape that exposes the
// reused-path-buffer polish: a true sub-range walk that prunes
// most of the tree.
func runRangeBenches(b *testing.B, w bench.Workload) {
	s8 := New[int]()
	for j, k := range w.Keys {
		s8.Put(k, w.Vals[j])
	}
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	// Pick range bounds that cover roughly the middle 10% of sorted keys.
	sorted := make([][]byte, 0, len(w.Keys))
	for k := range s8.Range(nil, nil) {
		sorted = append(sorted, append([]byte(nil), k...))
	}
	lo := sorted[len(sorted)*45/100]
	hi := sorted[len(sorted)*55/100]

	b.Run("Stage8", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s8.Range(lo, hi) {
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

func BenchmarkPut_Dense_1k(b *testing.B)  { runPutBenches(b, bench.Dense(1_000)) }
func BenchmarkPut_Sparse_1k(b *testing.B) { runPutBenches(b, bench.Sparse(1_000)) }
func BenchmarkPut_URL_1k(b *testing.B)    { runPutBenches(b, bench.URL(1_000)) }

func BenchmarkGet_Dense_1k(b *testing.B)  { runGetBenches(b, bench.Dense(1_000)) }
func BenchmarkGet_Sparse_1k(b *testing.B) { runGetBenches(b, bench.Sparse(1_000)) }
func BenchmarkGet_URL_1k(b *testing.B)    { runGetBenches(b, bench.URL(1_000)) }

// Range benches over the middle 10% window. Stage 7 is omitted
// here because its Range is the same shape as chapter 9's
// (chapter 8 has the naive walk-and-filter); the Polish #3
// comparison lives in BenchmarkMid1pct below, which exercises
// pruning at a tighter window where the speedup is sharpest.
func BenchmarkRange_Dense_1k(b *testing.B)  { runRangeBenches(b, bench.Dense(1_000)) }
func BenchmarkRange_Sparse_1k(b *testing.B) { runRangeBenches(b, bench.Sparse(1_000)) }
func BenchmarkRange_URL_1k(b *testing.B)    { runRangeBenches(b, bench.URL(1_000)) }

// TestReportFootprint surfaces both struct sizes and live heap.
// The headline this chapter chases is the leaf size shift (32 ->
// 32+24 = 56 -> rounded to 64 B by the allocator), the modest
// inner-node size shrink from embedded innerHeader, and the
// per-Put alloc reduction from the inline-key buffer.
func TestReportFootprint(t *testing.T) {
	t.Logf("per-node sizes (unsafe.Sizeof): leaf=%d  node4=%d  node16=%d  node48=%d  node256=%d",
		bytesPerLeaf, bytesPerNode4, bytesPerNode16, bytesPerNode48, bytesPerNode256)
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		s7Heap, _ := measureLiveHeap(func() any {
			t := stage7.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s8Heap, _ := measureLiveHeap(func() any {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		nKeys := len(w.Keys)
		t.Logf("%-13s  S7 heap=%4d B/key  |  S8 heap=%4d B/key  (heap %.2fx)",
			w.Name,
			int(s7Heap)/nKeys,
			int(s8Heap)/nKeys,
			float64(s7Heap)/float64(s8Heap))
	}
}

// runMid1pctBenches measures iteration of the *middle* 1 % of the
// sorted keys: a window from the 49.5th to the 50.5th percentile
// of the keyspace. For btree this is a true sub-range walk via
// AscendRange. Chapter 8's Range walks every leaf and filters at
// the leaf, so the cost is dominated by the unavoidable full
// descent. Chapter 9's Range prunes whole subtrees outside
// [lo, hi) using the path-buffer machinery in iterateRange, which
// is the closest comparable shape to btree's AscendRange.
func runMid1pctBenches(b *testing.B, w bench.Workload) {
	prev := stage7.New[int]()
	curr := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		prev.Put(k, w.Vals[j])
		curr.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	// Materialize sorted keys once to compute window bounds.
	sorted := make([][]byte, 0, len(w.Keys))
	for k := range curr.Range(nil, nil) {
		sorted = append(sorted, append([]byte(nil), k...))
	}
	lo := sorted[len(sorted)*495/1000]
	hi := sorted[len(sorted)*505/1000]

	b.Run("Stage7", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range prev.Range(lo, hi) {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage8", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range curr.Range(lo, hi) {
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
