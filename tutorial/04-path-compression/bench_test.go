package pathcompression

import (
	"testing"

	chapter3 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/03-lazy-expansion"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// contenders lists what this chapter measures itself against: its
// own tree, the previous chapter's, and the btree reference.
func contenders() []harness.Contender {
	return []harness.Contender{
		{Name: "Chapter4", New: factory()},
		{Name: "Chapter3", New: chapter3Factory()},
		harness.BTreeContender(),
	}
}

func chapter3Factory() harness.Factory {
	return func() harness.SortedMap { return chapter3.New[int]() }
}

// BenchmarkOps reproduces every cell of the chapter's tables:
//
//	go test -bench=. -benchmem -benchtime=300ms ./04-path-compression/
func BenchmarkOps(b *testing.B) {
	harness.RunOpBenchmarks(b, contenders())
}
