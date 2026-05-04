package pathcompression

import (
	"runtime"
	"testing"
	"unsafe"

	stage2 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/02-lazy-expansion"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// Stage 3 footprint constants (derived from unsafe.Sizeof so they
// can't drift). Stage 2's node256 differs from stage 3's only by the
// added prefix slice header, so each adds 24 B per inner node.
var (
	bytesPerInner       = int(unsafe.Sizeof(node256[int]{}))
	bytesPerLeaf        = int(unsafe.Sizeof(leaf[int]{}))
	stage2BytesPerInner = 256*16 + 8 // [256]node (interface) + *leaf
)

// measureLiveHeap returns the live-heap delta after build() runs and
// returns its result (which the closure keeps alive). Two GCs around
// the measurement flush stale allocations so the delta reflects only
// the surviving tree.
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

// reportFootprint adds prefix bytes to the inner-node and leaf
// overhead from chapter 2 -- prefixes are heap-allocated separately
// from the node256 struct, so they need their own term.
func reportFootprint[V any](b *testing.B, t *Tree[V], w bench.Workload) {
	b.Helper()
	if len(w.Keys) == 0 {
		return
	}
	innerBytes := t.CountInner() * bytesPerInner
	leafFixed := t.CountLeaves() * bytesPerLeaf
	prefixBytes := t.PrefixBytes()
	keyBytes := 0
	for _, k := range w.Keys {
		keyBytes += len(k)
	}
	total := innerBytes + leafFixed + keyBytes + prefixBytes
	b.ReportMetric(float64(total)/float64(len(w.Keys)), "stage3-bytes/key")
	b.ReportMetric(float64(t.CountInner())/float64(len(w.Keys)), "stage3-inner/key")
}

func runPutBenches(b *testing.B, w bench.Workload) {
	b.Run("Stage2", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := stage2.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
		}
	})
	b.Run("Stage3", func(b *testing.B) {
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
	s2 := stage2.New[int]()
	s3 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s2.Put(k, w.Vals[j])
		s3.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage2", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s2.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		_ = sink
	})
	b.Run("Stage3", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s3.Get(w.Keys[i%len(w.Keys)])
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
	s2 := stage2.New[int]()
	s3 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s2.Put(k, w.Vals[j])
		s3.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage2", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s2.All() {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage3", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s3.All() {
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

// TestReportFootprint surfaces three numbers per workload:
//   - structural bytes/key (CountInner * unsafe.Sizeof, etc.)
//   - actual live-heap bytes/key (runtime.ReadMemStats around build)
//   - the heap-bytes ratio of stage 2 -> stage 3.
func TestReportFootprint(t *testing.T) {
	t.Logf("per-node sizes (unsafe.Sizeof): stage2.node256=%d  stage3.node256=%d  stage3.leaf=%d",
		stage2BytesPerInner, bytesPerInner, bytesPerLeaf)
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		s2Heap, s2Tree := measureLiveHeap(func() any {
			t := stage2.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s2 := s2Tree.(*stage2.Tree[int])

		s3Heap, s3Tree := measureLiveHeap(func() any {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s3 := s3Tree.(*Tree[int])

		keyBytes := 0
		for _, k := range w.Keys {
			keyBytes += len(k)
		}
		s2Structural := s2.CountInner()*stage2BytesPerInner +
			s2.CountLeaves()*bytesPerLeaf + keyBytes
		s3Structural := s3.CountInner()*bytesPerInner +
			s3.CountLeaves()*bytesPerLeaf + keyBytes + s3.PrefixBytes()
		nKeys := len(w.Keys)
		t.Logf("%-13s  S2 struct=%4d B/key heap=%4d B/key  |  S3 struct=%4d B/key heap=%4d B/key  (heap %.2fx tighter)",
			w.Name,
			s2Structural/nKeys, int(s2Heap)/nKeys,
			s3Structural/nKeys, int(s3Heap)/nKeys,
			float64(s2Heap)/float64(s3Heap))
	}
}

// runAll1pctBenches measures partial iteration: walk the sorted
// keys via All and stop after len(keys)/100 yields.
func runAll1pctBenches(b *testing.B, w bench.Workload) {
	prev := stage2.New[int]()
	curr := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		prev.Put(k, w.Vals[j])
		curr.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	target := len(w.Keys) / 100
	if target < 1 {
		target = 1
	}

	b.Run("Stage2", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			yielded := 0
			for _, v := range prev.All() {
				sink ^= v
				yielded++
				if yielded >= target {
					break
				}
			}
		}
		_ = sink
	})
	b.Run("Stage3", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			yielded := 0
			for _, v := range curr.All() {
				sink ^= v
				yielded++
				if yielded >= target {
					break
				}
			}
		}
		_ = sink
	})
	b.Run("BTree", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			yielded := 0
			bt.Ascend(func(it bench.BtreeItem) bool {
				sink ^= it.Val
				yielded++
				return yielded < target
			})
		}
		_ = sink
	})
}

func BenchmarkAll1pct_Dense_1k(b *testing.B)  { runAll1pctBenches(b, bench.Dense(1_000)) }
func BenchmarkAll1pct_Sparse_1k(b *testing.B) { runAll1pctBenches(b, bench.Sparse(1_000)) }
func BenchmarkAll1pct_URL_1k(b *testing.B)    { runAll1pctBenches(b, bench.URL(1_000)) }
