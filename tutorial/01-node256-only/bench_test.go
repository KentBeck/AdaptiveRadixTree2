package nodeonly256

import (
	"fmt"
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// approxBytesPerNode is the structural footprint of one node256 in
// this stage on a 64-bit machine: [256]*node[V] = 256 * 8 = 2 048
// bytes for the children array, plus 8 bytes for the *V terminal
// pointer = 2 056 bytes per node. Counting nodes and multiplying
// reports the disaster baseline that motivates chapter 2.
const approxBytesPerNode = 256*8 + 8

// reportFootprint prints the structural memory cost as a custom
// metric on the bench output. Returned bytes/key includes only the
// inner-node footprint, not the user's keys or values.
func reportFootprint[V any](b *testing.B, t *Tree[V], nKeys int) {
	b.Helper()
	if nKeys == 0 {
		return
	}
	bytes := t.CountNodes() * approxBytesPerNode
	b.ReportMetric(float64(bytes)/float64(nKeys), "stage1-bytes/key")
	b.ReportMetric(float64(t.CountNodes())/float64(nKeys), "stage1-nodes/key")
}

// runPutBenches runs Put on a fresh tree per iteration, reporting
// alloc and time per Put. The structural footprint is reported once
// after all Puts so the headline number is visible in the output.
func runPutBenches(b *testing.B, w bench.Workload) {
	b.Run("Stage1", func(b *testing.B) {
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
			reportFootprint(b, lastTree, len(w.Keys))
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
	stage1 := New[int]()
	for j, k := range w.Keys {
		stage1.Put(k, w.Vals[j])
	}
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}

	b.Run("Stage1", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			k := w.Keys[i%len(w.Keys)]
			v, _ := stage1.Get(k)
			sink ^= v
		}
		_ = sink
	})
	b.Run("BTree", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			k := w.Keys[i%len(w.Keys)]
			v, _ := bt.Get(bench.BtreeItem{Key: k})
			sink ^= v.Val
		}
		_ = sink
	})
}

func runRangeBenches(b *testing.B, w bench.Workload) {
	stage1 := New[int]()
	for j, k := range w.Keys {
		stage1.Put(k, w.Vals[j])
	}
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}

	b.Run("Stage1", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range stage1.Range(nil, nil) {
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

// Workload sizes are kept small enough that Put benchmarks finish in
// reasonable time even at b.N = 1 (the harness re-runs them many
// times). The stage-1 disaster is most visible at the Sparse
// workload, where it allocates ~16 nodes per key.

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
// via AscendRange; for chapter 1's ART it is Range(lo, hi). The
// trie's Range still walks every node and filters at the leaf, so
// the cost is dominated by the unavoidable full descent. Chapter 8
// will make Range prune subtrees by prefix and close the gap.
func runRangeWindowBenches(b *testing.B, w bench.Workload) {
	stage1 := New[int]()
	for j, k := range w.Keys {
		stage1.Put(k, w.Vals[j])
	}
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	// Materialize sorted keys once to compute window bounds.
	sorted := make([][]byte, 0, len(w.Keys))
	for k := range stage1.Range(nil, nil) {
		sorted = append(sorted, append([]byte(nil), k...))
	}
	lo := sorted[len(sorted)*495/1000]
	hi := sorted[len(sorted)*505/1000]

	b.Run("Stage1", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range stage1.Range(lo, hi) {
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

// TestReportFootprint exists outside Go's bench framework so the
// disaster headline numbers (bytes/key, nodes/key) are produced even
// if the reader runs `go test` rather than `go test -bench`.
func TestReportFootprint(t *testing.T) {
	for _, w := range []bench.Workload{
		bench.Dense(1_000),
		bench.Sparse(1_000),
		bench.URL(1_000),
	} {
		tree := New[int]()
		for j, k := range w.Keys {
			tree.Put(k, w.Vals[j])
		}
		nodes := tree.CountNodes()
		bytes := nodes * approxBytesPerNode
		t.Logf("%-14s nodes=%-7d bytes=%-12s nodes/key=%5.2f bytes/key=%-7s avg-key-len=%.1f",
			w.Name, nodes, fmt.Sprintf("%d", bytes),
			float64(nodes)/float64(len(w.Keys)),
			fmt.Sprintf("%d", bytes/len(w.Keys)),
			w.AvgKeyLen())
	}
}
