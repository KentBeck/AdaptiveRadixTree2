package addnode16

import (
	"testing"

	chapter6 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/06-introduce-polymorphism"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// contenders lists what this chapter measures itself against: its
// own tree, the previous chapter's, and the btree reference.
func contenders() []harness.Contender {
	return []harness.Contender{
		{Name: "Chapter7", New: factory()},
		{Name: "Chapter6", New: chapter6Factory()},
		harness.BTreeContender(),
	}
}

func chapter6Factory() harness.Factory {
	return func() harness.SortedMap { return chapter6.New[int]() }
}

// BenchmarkOps reproduces every cell of the chapter's tables:
//
//	go test -bench=. -benchmem -benchtime=300ms ./07-add-node16/
func BenchmarkOps(b *testing.B) {
	harness.RunOpBenchmarks(b, contenders())
}
