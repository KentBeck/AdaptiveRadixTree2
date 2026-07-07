package polish

import (
	"testing"

	chapter8 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/08-add-node48"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// contenders lists what this chapter measures itself against: its
// own tree, the previous chapter's, and the btree reference.
func contenders() []harness.Contender {
	return []harness.Contender{
		{Name: "Chapter9", New: factory()},
		{Name: "Chapter8", New: chapter8Factory()},
		harness.BTreeContender(),
	}
}

func chapter8Factory() harness.Factory {
	return func() harness.SortedMap { return chapter8.New[int]() }
}

// BenchmarkOps reproduces every cell of the chapter's tables:
//
//	go test -bench=. -benchmem -benchtime=300ms ./09-polish/
func BenchmarkOps(b *testing.B) {
	harness.RunOpBenchmarks(b, contenders())
}
