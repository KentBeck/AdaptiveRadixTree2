package lazyexpansion

import (
	"fmt"
	"testing"

	stage1 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-node256-only"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// Stage 2 footprint accounting:
//   - one inner node256 = [256]node + *leaf terminal = 257 * 8 = 2 056 B
//   - one leaf = []byte slice header (24) + V (8 for int) = 32 B,
//     plus the key bytes copied from the caller.
const (
	approxBytesPerInner = 256*8 + 8
	approxBytesPerLeaf  = 24 + 8 // V == int in the bench workloads
)

func reportFootprint[V any](b *testing.B, t *Tree[V], w bench.Workload) {
	b.Helper()
	if len(w.Keys) == 0 {
		return
	}
	innerBytes := t.CountInner() * approxBytesPerInner
	leafFixed := t.CountLeaves() * approxBytesPerLeaf
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

func runAllBenches(b *testing.B, w bench.Workload) {
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
			for _, v := range s1.All() {
				sink ^= v
			}
		}
		_ = sink
	})
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

// TestReportFootprint produces the bytes/key headline alongside the
// stage 1 count so the side-by-side comparison shows up in `go test`
// output even without -bench. Reports for inner-node count, leaf
// count, and a rough total-bytes/key estimate (inner footprint +
// leaf overhead + key bytes).
func TestReportFootprint(t *testing.T) {
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		s1 := stage1.New[int]()
		s2 := New[int]()
		for j, k := range w.Keys {
			s1.Put(k, w.Vals[j])
			s2.Put(k, w.Vals[j])
		}
		s1Bytes := s1.CountNodes() * approxBytesPerInner
		s2Inner := s2.CountInner()
		s2Leaves := s2.CountLeaves()
		keyBytes := 0
		for _, k := range w.Keys {
			keyBytes += len(k)
		}
		s2Bytes := s2Inner*approxBytesPerInner + s2Leaves*approxBytesPerLeaf + keyBytes

		t.Logf("%-13s  S1 inner=%-5d %s/key   |  S2 inner=%-4d leaves=%-4d %s/key  (%.1fx tighter)",
			w.Name,
			s1.CountNodes(), fmt.Sprintf("%d", s1Bytes/len(w.Keys)),
			s2Inner, s2Leaves, fmt.Sprintf("%d", s2Bytes/len(w.Keys)),
			float64(s1Bytes)/float64(s2Bytes))
	}
}
