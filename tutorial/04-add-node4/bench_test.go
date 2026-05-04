package addnode4

import (
	"runtime"
	"testing"
	"unsafe"

	stage3 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/03-path-compression"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// Stage 4 footprint constants. Derived from unsafe.Sizeof so the
// numbers can't drift when struct layout changes. The first version
// of this file got the math wrong by counting child slots as 8 B
// each; an interface slot is two words = 16 B. node256 is 37x larger
// than node4 (4136 B vs 112 B), which is the headline cost of having
// only a single inner-node type.
var (
	bytesPerNode4   = int(unsafe.Sizeof(node4[int]{}))
	bytesPerNode256 = int(unsafe.Sizeof(node256[int]{}))
	bytesPerLeaf    = int(unsafe.Sizeof(leaf[int]{}))
	// Stage 3 had only node256 (with prefix). Slightly different
	// layout from stage 4's node256 (no numChildren counter) so it
	// gets its own constant.
	stage3BytesPerInner = 24 + 256*16 + 8 + 8 // prefix + children + terminal + padding
)

// measureLiveHeap returns the live-heap delta after build() runs and
// returns its result. Two GCs around the measurement flush stale
// allocations so the delta reflects only the surviving tree.
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

func reportFootprint[V any](b *testing.B, t *Tree[V], w bench.Workload) {
	b.Helper()
	if len(w.Keys) == 0 {
		return
	}
	n4, n256 := t.CountByKind()
	innerBytes := n4*bytesPerNode4 + n256*bytesPerNode256
	leafFixed := t.CountLeaves() * bytesPerLeaf
	prefixBytes := t.PrefixBytes()
	keyBytes := 0
	for _, k := range w.Keys {
		keyBytes += len(k)
	}
	total := innerBytes + leafFixed + keyBytes + prefixBytes
	b.ReportMetric(float64(total)/float64(len(w.Keys)), "stage4-bytes/key")
	b.ReportMetric(float64(n4)/float64(len(w.Keys)), "stage4-n4/key")
	b.ReportMetric(float64(n256)/float64(len(w.Keys)), "stage4-n256/key")
}

func runPutBenches(b *testing.B, w bench.Workload) {
	b.Run("Stage3", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := stage3.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
		}
	})
	b.Run("Stage4", func(b *testing.B) {
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
	s3 := stage3.New[int]()
	s4 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s3.Put(k, w.Vals[j])
		s4.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage3", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s3.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		_ = sink
	})
	b.Run("Stage4", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s4.Get(w.Keys[i%len(w.Keys)])
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
	s3 := stage3.New[int]()
	s4 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s3.Put(k, w.Vals[j])
		s4.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage3", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s3.Range(nil, nil) {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage4", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s4.Range(nil, nil) {
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

// TestReportFootprint surfaces three numbers per workload:
//   - structural bytes/key (sum of unsafe.Sizeof contributions)
//   - actual live-heap bytes/key (runtime.ReadMemStats around build)
//   - the heap-bytes ratio of stage 3 -> stage 4.
//
// The headline this chapter chases is the per-node-type cost
// difference: a node256 is 37x heavier than a node4 (4136 B vs
// 112 B). The Sparse and URL workloads see most of their inner
// nodes shrink from node256 to node4 once the demotion rule fires.
func TestReportFootprint(t *testing.T) {
	t.Logf("per-node sizes (unsafe.Sizeof): node4=%d  node256=%d  leaf=%d  ratio=%dx",
		bytesPerNode4, bytesPerNode256, bytesPerLeaf,
		bytesPerNode256/bytesPerNode4)

	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		s3Heap, s3Tree := measureLiveHeap(func() any {
			t := stage3.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s3 := s3Tree.(*stage3.Tree[int])

		s4Heap, s4Tree := measureLiveHeap(func() any {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s4 := s4Tree.(*Tree[int])

		keyBytes := 0
		for _, k := range w.Keys {
			keyBytes += len(k)
		}
		s3Structural := s3.CountInner()*stage3BytesPerInner +
			s3.CountLeaves()*bytesPerLeaf + keyBytes + s3.PrefixBytes()
		n4, n256 := s4.CountByKind()
		s4Structural := n4*bytesPerNode4 + n256*bytesPerNode256 +
			s4.CountLeaves()*bytesPerLeaf + keyBytes + s4.PrefixBytes()
		nKeys := len(w.Keys)
		t.Logf("%-13s  S3 struct=%4d B/key heap=%4d B/key  |  S4 (n4=%-3d n256=%-3d) struct=%4d B/key heap=%4d B/key  (heap %.2fx tighter)",
			w.Name,
			s3Structural/nKeys, int(s3Heap)/nKeys,
			n4, n256,
			s4Structural/nKeys, int(s4Heap)/nKeys,
			float64(s3Heap)/float64(s4Heap))
	}
}

// runRangeWindowBenches measures iteration of the *middle* 1 % of
// the sorted keys: a window from the 49.5th to the 50.5th
// percentile of the keyspace. For btree this is a sub-range walk
// via AscendRange; for both stages it is Range(lo, hi). Range in
// stages 3 and 4 still walks every node and filters at the leaf,
// so the cost is dominated by the unavoidable full descent.
// Chapter 8 will make Range prune subtrees by prefix and close
// the gap to btree.
func runRangeWindowBenches(b *testing.B, w bench.Workload) {
	prev := stage3.New[int]()
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

	b.Run("Stage3", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range prev.Range(lo, hi) {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage4", func(b *testing.B) {
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
