package lazyexpansion

import (
	"testing"

	chapter2 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/02-node256-only"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// contenders lists what this chapter measures itself against: its
// own tree, the previous chapter's, and the btree reference.
func contenders() []harness.Contender {
	return []harness.Contender{
		{Name: "Chapter3", New: factory()},
		{Name: "Chapter2", New: chapter2Factory()},
		harness.BTreeContender(),
	}
}

func chapter2Factory() harness.Factory {
	return func() harness.SortedMap { return chapter2.New[int]() }
}

// BenchmarkOps reproduces every cell of the chapter's tables:
//
//	go test -bench=. -benchmem -benchtime=300ms ./03-lazy-expansion/
func BenchmarkOps(b *testing.B) {
	harness.RunOpBenchmarks(b, contenders())
}
