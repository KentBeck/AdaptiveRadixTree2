package addnode48

import (
	"testing"

	chapter7 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/07-add-node16"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// contenders lists what this chapter measures itself against: its
// own tree, the previous chapter's, and the btree reference.
func contenders() []harness.Contender {
	return []harness.Contender{
		{Name: "Chapter8", New: factory()},
		{Name: "Chapter7", New: chapter7Factory()},
		harness.BTreeContender(),
	}
}

func chapter7Factory() harness.Factory {
	return func() harness.SortedMap { return chapter7.New[int]() }
}

// BenchmarkOps reproduces every cell of the chapter's tables. The
// Sparse/5000 workload is where node48 actually populates, so it
// runs alongside the standard trio:
//
//	go test -bench=. -benchmem -benchtime=300ms ./08-add-node48/
func BenchmarkOps(b *testing.B) {
	harness.RunOpBenchmarks(b, contenders(),
		append(harness.Workloads1k(), bench.Sparse(5000))...)
}
