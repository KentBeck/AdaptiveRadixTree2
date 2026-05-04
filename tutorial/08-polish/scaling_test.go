package polish

import (
	"flag"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// includeHuge enables the 10M+ cases. They need ~1.5 GB (URL 10M)
// up to ~10 GB (Dense 100M). Off by default so `go test ./...`
// stays fast and fits on a laptop.
var includeHuge = flag.Bool("huge", false, "run the 10M+ scaling rows")

// Workload-specific size ladders. Caps are tuned to fit on a
// 16 GB box: Sparse stops at 30M (1.6 B/key workload + ~120 B/key
// tree = ~4 GB peak), Dense reaches 100M (smallest per-key
// footprint, ~9 GB peak), URL stops at 10M (largest per-key
// footprint, ~1.5 GB peak; 100M URL would need ~23 GB).
type workloadSpec struct {
	name  string
	make  func(int) bench.Workload
	sizes []int
}

var workloadSpecs = []workloadSpec{
	{"Sparse", bench.Sparse, []int{1_000, 10_000, 100_000, 1_000_000, 10_000_000, 30_000_000}},
	{"Dense", bench.Dense, []int{1_000, 10_000, 100_000, 1_000_000, 10_000_000, 100_000_000}},
	{"URL", bench.URL, []int{1_000, 10_000, 100_000, 1_000_000, 10_000_000}},
}

// TestScalingAnnex produces the rolled-up table for BENCHMARKS.md.
// For each (workload, size), it builds the tree once with stage 8
// and once with btree, measuring:
//   - heap bytes per key (post-build, after a triggered GC)
//   - put time per key (averaged over the build)
//   - get time per op (sampled over a fixed-time loop)
//
// One pass per cell. No bench-framework iteration loops, so this
// test is fast at small N and tractable at large N. Output goes
// to t.Log; copy into BENCHMARKS.md.
//
// Run with:
//
//	go test ./08-polish/ -run TestScalingAnnex -v -timeout 5m         # up to 1M
//	go test ./08-polish/ -run TestScalingAnnex -v -huge -timeout 30m  # full ladder
func TestScalingAnnex(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling annex skipped under -short")
	}

	cap := 1_000_000
	if *includeHuge {
		cap = 100_000_000 // any large value; per-workload sizes still apply
	}

	for _, spec := range workloadSpecs {
		t.Logf("\n== %s workload ==", spec.name)
		t.Logf("%-10s  %-32s  %-32s",
			"keys",
			"stage 8 (Put µs/k | Get ns | heap B/k | iter1% ns/k)",
			"btree   (Put µs/k | Get ns | heap B/k | iter1% ns/k)")

		for _, n := range spec.sizes {
			if n > cap {
				continue
			}
			w := spec.make(n)

			s8 := measureStage8(w)
			bt := measureBtree(w)

			t.Logf("%-10s  %-32s  %-32s",
				humanize(n),
				fmt.Sprintf("%6.2f | %6.0f | %6.0f | %5.0f", s8.putUsPerKey, s8.getNs, s8.heapBPerKey, s8.iter1pctNsPerKey),
				fmt.Sprintf("%6.2f | %6.0f | %6.0f | %5.0f", bt.putUsPerKey, bt.getNs, bt.heapBPerKey, bt.iter1pctNsPerKey))
		}
	}
}

type scalePoint struct {
	putUsPerKey      float64
	getNs            float64
	heapBPerKey      float64
	iter1pctNsPerKey float64 // ns per yielded key when iterating len(keys)/100 via All
}

const getSampleSeconds = 1.0

func measureStage8(w bench.Workload) scalePoint {
	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	t := New[int]()
	putStart := time.Now()
	for j, k := range w.Keys {
		t.Put(k, w.Vals[j])
	}
	putElapsed := time.Since(putStart)

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	heap := float64(0)
	if after.HeapAlloc > before.HeapAlloc {
		heap = float64(after.HeapAlloc-before.HeapAlloc) / float64(len(w.Keys))
	}

	getNs := timeGet(func(i int) {
		_, _ = t.Get(w.Keys[i%len(w.Keys)])
	})

	target := len(w.Keys) / 100
	if target < 1 {
		target = 1
	}
	iterNs := timeIter1pct(func() int {
		yielded := 0
		for k, v := range t.All() {
			_ = k
			_ = v
			yielded++
			if yielded >= target {
				break
			}
		}
		return yielded
	})

	runtime.KeepAlive(t)
	return scalePoint{
		putUsPerKey:      float64(putElapsed.Microseconds()) / float64(len(w.Keys)),
		getNs:            getNs,
		heapBPerKey:      heap,
		iter1pctNsPerKey: iterNs,
	}
}

func measureBtree(w bench.Workload) scalePoint {
	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	t := bench.NewBtree()
	putStart := time.Now()
	for j, k := range w.Keys {
		t.ReplaceOrInsert(bench.BtreeItem{Key: k, Val: w.Vals[j]})
	}
	putElapsed := time.Since(putStart)

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	heap := float64(0)
	if after.HeapAlloc > before.HeapAlloc {
		heap = float64(after.HeapAlloc-before.HeapAlloc) / float64(len(w.Keys))
	}

	getNs := timeGet(func(i int) {
		_, _ = t.Get(bench.BtreeItem{Key: w.Keys[i%len(w.Keys)]})
	})

	target := len(w.Keys) / 100
	if target < 1 {
		target = 1
	}
	iterNs := timeIter1pct(func() int {
		yielded := 0
		t.Ascend(func(it bench.BtreeItem) bool {
			yielded++
			return yielded < target
		})
		return yielded
	})

	runtime.KeepAlive(t)
	return scalePoint{
		putUsPerKey:      float64(putElapsed.Microseconds()) / float64(len(w.Keys)),
		getNs:            getNs,
		heapBPerKey:      heap,
		iter1pctNsPerKey: iterNs,
	}
}

// timeIter1pct runs the partial-iteration closure repeatedly for a
// fixed wall-clock window and returns ns per yielded key. Each call
// to op iterates len(keys)/100 yields and returns how many it
// yielded.
func timeIter1pct(op func() int) float64 {
	deadline := time.Now().Add(time.Duration(getSampleSeconds * float64(time.Second)))
	totalYielded := 0
	start := time.Now()
	for {
		totalYielded += op()
		if time.Now().After(deadline) {
			break
		}
	}
	elapsed := time.Since(start)
	if totalYielded == 0 {
		return 0
	}
	return float64(elapsed.Nanoseconds()) / float64(totalYielded)
}

// timeGet runs op in a loop for getSampleSeconds wall-clock and
// returns ns/op. Doing this manually (vs the bench framework)
// avoids the framework's setup/teardown skew at the per-cell
// scale of this annex.
func timeGet(op func(i int)) float64 {
	deadline := time.Now().Add(time.Duration(getSampleSeconds * float64(time.Second)))
	calls := 0
	start := time.Now()
	for {
		// Run a batch so the time check doesn't dominate.
		for i := 0; i < 1024; i++ {
			op(calls + i)
		}
		calls += 1024
		if time.Now().After(deadline) {
			break
		}
	}
	elapsed := time.Since(start)
	if calls == 0 {
		return 0
	}
	return float64(elapsed.Nanoseconds()) / float64(calls)
}

func humanize(n int) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%dB", n/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%dM", n/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1_000)
	}
	return fmt.Sprintf("%d", n)
}
