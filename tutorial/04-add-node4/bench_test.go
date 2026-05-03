package addnode4

import (
	"fmt"
	"testing"

	stage3 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/03-path-compression"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

const (
	approxBytesPerNode4   = 24 + 4 + 4 + 32 + 8 + 1 + 3 // prefix hdr + keys + pad + child ptrs + terminal + numChildren + pad
	approxBytesPerNode256 = 24 + 256*8 + 8 + 2 + 6      // prefix hdr + children + terminal + numChildren + pad
	approxBytesPerLeaf    = 24 + 8                      // []byte header + V (int)
)

func reportFootprint[V any](b *testing.B, t *Tree[V], w bench.Workload) {
	b.Helper()
	if len(w.Keys) == 0 {
		return
	}
	n4, n256 := t.CountByKind()
	innerBytes := n4*approxBytesPerNode4 + n256*approxBytesPerNode256
	leafFixed := t.CountLeaves() * approxBytesPerLeaf
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

func runAllBenches(b *testing.B, w bench.Workload) {
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
			for _, v := range s3.All() {
				sink ^= v
			}
		}
		_ = sink
	})
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

// TestReportFootprint surfaces side-by-side stage 3 / stage 4
// counts in plain `go test` output.
func TestReportFootprint(t *testing.T) {
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		s3 := stage3.New[int]()
		s4 := New[int]()
		for j, k := range w.Keys {
			s3.Put(k, w.Vals[j])
			s4.Put(k, w.Vals[j])
		}
		keyBytes := 0
		for _, k := range w.Keys {
			keyBytes += len(k)
		}
		s3Bytes := s3.CountInner()*(24+256*8+8) +
			s3.CountLeaves()*approxBytesPerLeaf + keyBytes + s3.PrefixBytes()
		n4, n256 := s4.CountByKind()
		s4Bytes := n4*approxBytesPerNode4 + n256*approxBytesPerNode256 +
			s4.CountLeaves()*approxBytesPerLeaf + keyBytes + s4.PrefixBytes()
		t.Logf("%-13s  S3 inner=%-3d %s/key  |  S4 n4=%-3d n256=%-3d %s/key  (%.1fx tighter)",
			w.Name,
			s3.CountInner(), fmt.Sprintf("%d", s3Bytes/len(w.Keys)),
			n4, n256, fmt.Sprintf("%d", s4Bytes/len(w.Keys)),
			float64(s3Bytes)/float64(s4Bytes))
	}
}
