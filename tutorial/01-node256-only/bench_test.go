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

func runAllBenches(b *testing.B, w bench.Workload) {
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
			for _, v := range stage1.All() {
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

func BenchmarkAll_Dense_1k(b *testing.B)  { runAllBenches(b, bench.Dense(1_000)) }
func BenchmarkAll_Sparse_1k(b *testing.B) { runAllBenches(b, bench.Sparse(1_000)) }
func BenchmarkAll_URL_1k(b *testing.B)    { runAllBenches(b, bench.URL(1_000)) }

// runAll1pctBenches measures partial iteration: walk the sorted
// keys via All and stop after len(keys)/100 yields. This isolates
// the cost of *starting* iteration plus the cost of *yielding* the
// first 1% of keys -- a different shape from full-tree All, and
// the closest analogue to "iterate a small window" before chapter
// 8 introduces Range. Numbers per outer iteration; divide by
// target yields if you want per-key.
func runAll1pctBenches(b *testing.B, w bench.Workload) {
	stage1 := New[int]()
	for j, k := range w.Keys {
		stage1.Put(k, w.Vals[j])
	}
	bt := bench.NewBtree()
	for j, k := range w.Keys {
		bt.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	target := len(w.Keys) / 100
	if target < 1 {
		target = 1
	}

	b.Run("Stage1", func(b *testing.B) {
		b.ReportAllocs()
		var sink int
		for i := 0; i < b.N; i++ {
			yielded := 0
			for _, v := range stage1.All() {
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
