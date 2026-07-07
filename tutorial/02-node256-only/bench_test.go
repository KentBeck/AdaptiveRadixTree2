package nodeonly256

import (
	"fmt"
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// contenders lists what this chapter measures itself against: its
// own tree and the btree reference. Later chapters add their
// predecessor here.
func contenders() []harness.Contender {
	return []harness.Contender{
		{Name: "Chapter2", New: factory()},
		harness.BTreeContender(),
	}
}

// BenchmarkOps reproduces every cell of the chapter's tables:
//
//	go test -bench=. -benchmem -benchtime=300ms ./02-node256-only/
func BenchmarkOps(b *testing.B) {
	harness.RunOpBenchmarks(b, contenders())
}

// approxBytesPerNode is the structural footprint of one node256 in
// this chapter on a 64-bit machine: [256]*node[V] = 256 * 8 = 2 048
// bytes for the children array, plus 8 bytes for the *V terminal
// pointer = 2 056 bytes per node. Counting nodes and multiplying
// reports the disaster baseline that motivates chapter 3.
const approxBytesPerNode = 256*8 + 8

// TestReportFootprint logs the structural numbers (nodes/key,
// bytes/key from sizeof arithmetic) that the chapter prose quotes.
func TestReportFootprint(t *testing.T) {
	for _, w := range harness.Workloads1k() {
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
