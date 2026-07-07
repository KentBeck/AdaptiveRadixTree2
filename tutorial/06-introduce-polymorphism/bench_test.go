package introducepolymorphism

import (
	"testing"

	chapter5 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/05-add-node4"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// contenders lists what this chapter measures itself against: its
// own tree, the previous chapter's, and the btree reference.
func contenders() []harness.Contender {
	return []harness.Contender{
		{Name: "Chapter6", New: factory()},
		{Name: "Chapter5", New: chapter5Factory()},
		harness.BTreeContender(),
	}
}

func chapter5Factory() harness.Factory {
	return func() harness.SortedMap { return chapter5.New[int]() }
}

// BenchmarkOps reproduces every cell of the chapter's tables:
//
//	go test -bench=. -benchmem -benchtime=300ms ./06-introduce-polymorphism/
func BenchmarkOps(b *testing.B) {
	harness.RunOpBenchmarks(b, contenders())
}
