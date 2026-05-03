package pathcompression

import (
	"fmt"
	"testing"

	stage2 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/02-lazy-expansion"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

const (
	approxBytesPerInner = 256*8 + 8 // children + terminal pointer
	approxBytesPerLeaf  = 24 + 8    // []byte header + V (int)
)

// reportFootprint adds prefix bytes to the inner-node and leaf
// overhead from chapter 2 -- prefixes are heap-allocated separately
// from the node256 struct, so they need their own term.
func reportFootprint[V any](b *testing.B, t *Tree[V], w bench.Workload) {
	b.Helper()
	if len(w.Keys) == 0 {
		return
	}
	innerBytes := t.CountInner() * approxBytesPerInner
	leafFixed := t.CountLeaves() * approxBytesPerLeaf
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

// TestReportFootprint surfaces the side-by-side bytes/key comparison
// even without -bench. Stage 2 is shown alongside stage 3 so the
// per-decision impact is visible in plain `go test` output.
func TestReportFootprint(t *testing.T) {
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		s2 := stage2.New[int]()
		s3 := New[int]()
		for j, k := range w.Keys {
			s2.Put(k, w.Vals[j])
			s3.Put(k, w.Vals[j])
		}
		keyBytes := 0
		for _, k := range w.Keys {
			keyBytes += len(k)
		}
		s2Bytes := s2.CountInner()*approxBytesPerInner +
			s2.CountLeaves()*approxBytesPerLeaf + keyBytes
		s3Bytes := s3.CountInner()*approxBytesPerInner +
			s3.CountLeaves()*approxBytesPerLeaf + keyBytes + s3.PrefixBytes()
		t.Logf("%-13s  S2 inner=%-4d %s/key   |  S3 inner=%-3d prefix=%-4dB %s/key  (%.1fx tighter)",
			w.Name,
			s2.CountInner(), fmt.Sprintf("%d", s2Bytes/len(w.Keys)),
			s3.CountInner(), s3.PrefixBytes(),
			fmt.Sprintf("%d", s3Bytes/len(w.Keys)),
			float64(s2Bytes)/float64(s3Bytes))
	}
}
