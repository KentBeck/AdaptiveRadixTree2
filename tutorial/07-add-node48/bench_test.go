package addnode48

import (
	"runtime"
	"testing"
	"unsafe"

	stage6 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/06-add-node16"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// Stage 7 footprint constants. node48 has the unusual layout
// (childIndex[256] + children[48] + childEdge[48]) so it is
// roughly 256 + 48*16 + 48 + slice header + terminal + count B.
// Sizes from unsafe.Sizeof.
var (
	bytesPerNode4   = int(unsafe.Sizeof(node4[int]{}))
	bytesPerNode16  = int(unsafe.Sizeof(node16[int]{}))
	bytesPerNode48  = int(unsafe.Sizeof(node48[int]{}))
	bytesPerNode256 = int(unsafe.Sizeof(node256[int]{}))
	bytesPerLeaf    = int(unsafe.Sizeof(leaf[int]{}))
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
	b.Run("Stage6", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := stage6.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
		}
	})
	b.Run("Stage7", func(b *testing.B) {
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
	s6 := stage6.New[int]()
	s7 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s6.Put(k, w.Vals[j])
		s7.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	b.Run("Stage6", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s6.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		_ = sink
	})
	b.Run("Stage7", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := s7.Get(w.Keys[i%len(w.Keys)])
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
	s6 := stage6.New[int]()
	s7 := New[int]()
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		s6.Put(k, w.Vals[j])
		s7.Put(k, w.Vals[j])
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
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
	b.Run("Stage7", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range s7.Range(nil, nil) {
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

// Sparse at 5k actually populates the 17-48 fanout band -- with
// ~5000/256 = 19.5 keys per first-byte bucket on average, the
// depth-1 inner nodes now hold node48-shaped fanout. Earlier
// chapters at the same scale forced those into node256.
func BenchmarkPut_Sparse_5k(b *testing.B)   { runPutBenches(b, bench.Sparse(5_000)) }
func BenchmarkGet_Sparse_5k(b *testing.B)   { runGetBenches(b, bench.Sparse(5_000)) }
func BenchmarkRange_Sparse_5k(b *testing.B) { runRangeBenches(b, bench.Sparse(5_000)) }

// TestReportFootprint surfaces the inner-node mix shift across
// chapter 6 -> chapter 7. Watch for stage 6's lone Sparse node256
// settling into a node48; many of stage 6's medium-fanout nodes
// are unaffected because they fit in node16 already.
func TestReportFootprint(t *testing.T) {
	t.Logf("per-node sizes (unsafe.Sizeof): node4=%d  node16=%d  node48=%d  node256=%d  leaf=%d",
		bytesPerNode4, bytesPerNode16, bytesPerNode48, bytesPerNode256, bytesPerLeaf)
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
		bench.Sparse(5_000), // node48 actually populates here
	} {
		s6Heap, _ := measureLiveHeap(func() any {
			t := stage6.New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s7Heap, s7Tree := measureLiveHeap(func() any {
			t := New[int]()
			for j, k := range w.Keys {
				t.Put(k, w.Vals[j])
			}
			return t
		})
		s7 := s7Tree.(*Tree[int])
		n4, n16, n48, n256 := s7.CountByKind()
		nKeys := len(w.Keys)
		t.Logf("%-13s  S6 heap=%4d B/key  |  S7 (n4=%-3d n16=%-3d n48=%-2d n256=%-1d) heap=%4d B/key  (heap %.2fx tighter)",
			w.Name,
			int(s6Heap)/nKeys,
			n4, n16, n48, n256,
			int(s7Heap)/nKeys,
			float64(s6Heap)/float64(s7Heap))
	}
}

// runMid1pctBenches measures iteration of the *middle* 1 % of the
// sorted keys: a window from the 49.5th to the 50.5th percentile
// of the keyspace. For btree this is a sub-range walk via
// AscendRange; for ART it is Range(lo, hi). Range still walks
// every node and filters at the leaf, so the cost is dominated
// by the unavoidable full descent. Chapter 8 will make Range
// prune subtrees by prefix and close the gap to btree.
func runMid1pctBenches(b *testing.B, w bench.Workload) {
	prev := stage6.New[int]()
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

	b.Run("Stage6", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range prev.Range(lo, hi) {
				sink ^= v
			}
		}
		_ = sink
	})
	b.Run("Stage7", func(b *testing.B) {
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
