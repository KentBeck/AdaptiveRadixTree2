package lazyexpansion

import (
	"runtime"
	"testing"
	"unsafe"

	stage1 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-node256-only"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// measureLiveHeap returns the number of bytes the Go runtime reports
// as live on the heap after build() finishes. Two GCs flush stale
// allocations so the delta reflects only what build()'s output keeps
// alive (the tree it returns, captured by the closure). This is the
// honest counterpart to the structural-bytes calculation: it's what
// the OS would actually see, including malloc rounding, slice header
// overhead, and small per-allocation bookkeeping.
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

// Stage 2 footprint constants are derived from unsafe.Sizeof rather
// than hand-counted: a `node` slot in stage 2 is an interface (2 words
// = 16 B on 64-bit), not a pointer, so a [256]node array is 4 KB, not
// 2 KB. (The first version of this file got the math wrong by counting
// the child slots as 8 B each.) Stage 1's node is a concrete struct
// with [256]*node[V] -- pointer slots, 8 B each -- so it has its own
// constant.
var (
	bytesPerInner       = int(unsafe.Sizeof(node256[int]{}))
	bytesPerLeaf        = int(unsafe.Sizeof(leaf[int]{}))
	stage1BytesPerInner = 256*8 + 8 // [256]*node[V] + *V
)

func reportFootprint[V any](b *testing.B, t *Tree[V], w bench.Workload) {
	b.Helper()
	if len(w.Keys) == 0 {
		return
	}
	innerBytes := t.CountInner() * bytesPerInner
	leafFixed := t.CountLeaves() * bytesPerLeaf
	keyBytes := 0
	for _, k := range w.Keys {
		keyBytes += len(k)
	}
	total := innerBytes + leafFixed + keyBytes
	b.ReportMetric(float64(total)/float64(len(w.Keys)), "stage2-bytes/key")
	b.ReportMetric(float64(t.CountInner())/float64(len(w.Keys)), "stage2-inner/key")
}

func runPutBenches(b *testing.B, w bench.Workload) {
	b.Run("Stage1", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := stage1.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
		}
	})
	b.Run("Stage2", func(b *testing.B) {
		var lastTree *Tree[int]
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			lastTree = t
		}
		b.StopTimer()
		if lastTree != nil {
			reportFootprint(b, lastTree, w)
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
	s1 := stage1.New[int]()
	s2 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s1.Put(k, w.Vals[j])
		s2.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}

	b.Run("Stage1", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s1.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		_ = sink
	})
	b.Run("Stage2", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s2.Get(w.Keys[i%len(w.Keys)])
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

func runRangeBenches(b *testing.B, w bench.Workload) {
	s1 := stage1.New[int]()
	s2 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s1.Put(k, w.Vals[j])
		s2.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}

	b.Run("Stage1", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s1.Range(nil, nil) {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage2", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s2.Range(nil, nil) {
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

func BenchmarkRange_Dense_1k(b *testing.B)  { runRangeBenches(b, bench.Dense(1_000)) }
func BenchmarkRange_Sparse_1k(b *testing.B) { runRangeBenches(b, bench.Sparse(1_000)) }
func BenchmarkRange_URL_1k(b *testing.B)    { runRangeBenches(b, bench.URL(1_000)) }

// runRangeWindowBenches measures iteration of the *middle* 1 % of
// the sorted keys: a window from the 49.5th to the 50.5th
// percentile of the keyspace. For btree this is a sub-range walk
// via AscendRange; for the chapter's ART it is Range(lo, hi). The
// trie's Range still walks every node and filters at the leaf, so
// the cost is dominated by the unavoidable full descent. Chapter 8
// will make Range prune subtrees by prefix and close the gap.
func runRangeWindowBenches(b *testing.B, w bench.Workload) {
	s1 := stage1.New[int]()
	s2 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s1.Put(k, w.Vals[j])
		s2.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	// Materialize sorted keys once to compute window bounds.
	sorted := make([][]byte, 0, len(w.Keys))
	for k := range s2.Range(nil, nil) {
		sorted = append(sorted, append([]byte(nil), k...))
	}
	lo := sorted[len(sorted)*495/1000]
	hi := sorted[len(sorted)*505/1000]

	b.Run("Stage1", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s1.Range(lo, hi) {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage2", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s2.Range(lo, hi) {
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

func BenchmarkRangeWindow_Dense_1k(b *testing.B)  { runRangeWindowBenches(b, bench.Dense(1_000)) }
func BenchmarkRangeWindow_Sparse_1k(b *testing.B) { runRangeWindowBenches(b, bench.Sparse(1_000)) }
func BenchmarkRangeWindow_URL_1k(b *testing.B)    { runRangeWindowBenches(b, bench.URL(1_000)) }

// TestReportFootprint surfaces three numbers for each workload:
//   - structural bytes/key (inner footprint + leaf overhead + key
//     bytes), computed from unsafe.Sizeof.
//   - actual live-heap bytes/key, captured via runtime.ReadMemStats
//     around tree construction and a triggered GC. This is what the
//     Go runtime actually reserves; it differs from the structural
//     number by malloc rounding, slice header overhead, and small
//     bookkeeping.
//   - the ratio of stage 1 -> stage 2.
func TestReportFootprint(t *testing.T) {
	t.Logf("per-node sizes (unsafe.Sizeof): stage1.node=%d  stage2.node256=%d  stage2.leaf=%d",
		stage1BytesPerInner, bytesPerInner, bytesPerLeaf)

	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		// Stage 1
		s1Heap, s1Tree := measureLiveHeap(func() any {
			t := stage1.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s1 := s1Tree.(*stage1.Tree[int])
		s1Structural := s1.CountNodes() * stage1BytesPerInner

		// Stage 2
		s2Heap, s2Tree := measureLiveHeap(func() any {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s2 := s2Tree.(*Tree[int])
		s2Inner := s2.CountInner()
		s2Leaves := s2.CountLeaves()
		keyBytes := 0
		for _, k := range w.Keys {
			keyBytes += len(k)
		}
		s2Structural := s2Inner*bytesPerInner + s2Leaves*bytesPerLeaf + keyBytes

		nKeys := len(w.Keys)
		t.Logf("%-13s  S1 struct=%5d B/key heap=%5d B/key  |  S2 struct=%4d B/key heap=%4d B/key  (heap %.1fx tighter)",
			w.Name,
			s1Structural/nKeys, int(s1Heap)/nKeys,
			s2Structural/nKeys, int(s2Heap)/nKeys,
			float64(s1Heap)/float64(s2Heap))
	}
}
