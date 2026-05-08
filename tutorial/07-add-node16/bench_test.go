package addnode16

import (
	"runtime"
	"testing"
	"unsafe"

	stage5 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/06-introduce-polymorphism"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// Stage 6 footprint constants. node16 fills the gap between
// node4's 120 B and node256's 4 144 B at roughly 408 B per node.
// Sizes from unsafe.Sizeof so they can't drift.
var (
	bytesPerNode4   = int(unsafe.Sizeof(node4[int]{}))
	bytesPerNode16  = int(unsafe.Sizeof(node16[int]{}))
	bytesPerNode256 = int(unsafe.Sizeof(node256[int]{}))
	bytesPerLeaf    = int(unsafe.Sizeof(leaf[int]{}))
)

// measureLiveHeap returns the live-heap delta after build() runs.
// Two GCs around the measurement flush stale allocations.
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
	b.Run("Stage5", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := stage5.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
		}
	})
	b.Run("Stage6", func(b *testing.B) {
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
	s5 := stage5.New[int]()
	s6 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s5.Put(k, w.Vals[j])
		s6.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage5", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s5.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		_ = sink
	})
	b.Run("Stage6", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s6.Get(w.Keys[i%len(w.Keys)])
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
	s5 := stage5.New[int]()
	s6 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s5.Put(k, w.Vals[j])
		s6.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage5", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s5.Range(nil, nil) {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage6", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s6.Range(nil, nil) {
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

// TestReportFootprint surfaces the inner-node mix shift across
// chapter 6 -> chapter 7. Watch the URL row: many of stage 5's
// node256s settle into node16s.
func TestReportFootprint(t *testing.T) {
	t.Logf("per-node sizes (unsafe.Sizeof): node4=%d  node16=%d  node256=%d  leaf=%d",
		bytesPerNode4, bytesPerNode16, bytesPerNode256, bytesPerLeaf)
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		s5Heap, _ := measureLiveHeap(func() any {
			t := stage5.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})

		s6Heap, s6Tree := measureLiveHeap(func() any {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s6 := s6Tree.(*Tree[int])
		n4, n16, n256 := s6.CountByKind()
		nKeys := len(w.Keys)
		t.Logf("%-13s  S5 heap=%4d B/key  |  S6 (n4=%-3d n16=%-3d n256=%-2d) heap=%4d B/key  (heap %.2fx tighter)",
			w.Name,
			int(s5Heap)/nKeys,
			n4, n16, n256,
			int(s6Heap)/nKeys,
			float64(s5Heap)/float64(s6Heap))
	}
}

// runRangeWindowBenches measures iteration of the *middle* 1 % of
// the sorted keys: a window from the 49.5th to the 50.5th
// percentile of the keyspace. For btree this is a sub-range walk
// via AscendRange; for chapter 7's ART it is Range(lo, hi). The
// trie's Range still walks every node and filters at the leaf,
// so the cost is dominated by the unavoidable full descent.
// Chapter 9 will make Range prune subtrees by prefix and close
// the gap to btree.
func runRangeWindowBenches(b *testing.B, w bench.Workload) {
	prev := stage5.New[int]()
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

	b.Run("Stage5", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range prev.Range(lo, hi) {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage6", func(b *testing.B) {
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

func BenchmarkRangeWindow_Dense_1k(b *testing.B)  { runRangeWindowBenches(b, bench.Dense(1_000)) }
func BenchmarkRangeWindow_Sparse_1k(b *testing.B) { runRangeWindowBenches(b, bench.Sparse(1_000)) }
func BenchmarkRangeWindow_URL_1k(b *testing.B)    { runRangeWindowBenches(b, bench.URL(1_000)) }
