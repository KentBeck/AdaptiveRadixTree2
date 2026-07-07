package addnode4

import (
	"testing"

	chapter4 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/04-path-compression"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// contenders lists what this chapter measures itself against: its
// own tree, the previous chapter's, and the btree reference.
func contenders() []harness.Contender {
	return []harness.Contender{
		{Name: "Chapter5", New: factory()},
		{Name: "Chapter4", New: chapter4Factory()},
		harness.BTreeContender(),
	}
}

func chapter4Factory() harness.Factory {
	return func() harness.SortedMap { return chapter4.New[int]() }
}

// BenchmarkOps reproduces every cell of the chapter's tables:
//
//	go test -bench=. -benchmem -benchtime=300ms ./05-add-node4/
func BenchmarkOps(b *testing.B) {
	harness.RunOpBenchmarks(b, contenders())
}
