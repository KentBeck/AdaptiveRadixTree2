package addnode48

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	chapter7 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/07-add-node16"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

// benchOnce / benchOnce5k share one measurement run per key-set so
// -update-bench doesn't benchmark anything twice.
var (
	benchOnce = sync.OnceValue(func() harness.BenchResults {
		return harness.BenchAll(contenders())
	})
	benchOnce5k = sync.OnceValue(func() harness.BenchResults {
		return harness.BenchAll(contenders(), bench.Sparse(5000))
	})
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", []buildcheck.Region{
		{Name: "innernodemix1k", Render: renderInnerNodeMix1k},
		{Name: "innernodemix5k", Render: renderInnerNodeMix5kSparse},
		{Name: "heapfootprint5k", Render: renderHeapFootprint5k, Volatile: true},
		{Name: "optime5k", Render: func() string { return benchOnce5k().TimeTable() }, Volatile: true},
		{Name: "optime", Render: func() string { return benchOnce().TimeTable() }, Volatile: true},
		{Name: "opspace", Render: func() string { return benchOnce().SpaceTable() }, Volatile: true},
		{Name: "capacity", Render: func() string { return harness.CapacityTable(contenders()) }, Volatile: true},
	})
}

// renderInnerNodeMix1k shows that the 17-48 fanout band is empty at
// the 1k fixture size for all three workloads -- so node48 catches
// nothing here. The 5k Sparse table below is where it earns its
// keep.
func renderInnerNodeMix1k() string {
	rows := []string{
		"Workload    Chapter 7 mix              Chapter 8 mix",
	}
	for _, w := range harness.Workloads1k() {
		c7, c8 := buildPair(w)
		c7n4, c7n16, c7n256 := c7.CountByKind()
		c8n4, c8n16, c8n48, c8n256 := c8.CountByKind()
		rows = append(rows, fmt.Sprintf("%-11s %dn4 + %dn16 + %dn256        %dn4 + %dn16 + %dn48 + %dn256",
			workloadShortName(w),
			c7n4, c7n16, c7n256,
			c8n4, c8n16, c8n48, c8n256))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// renderInnerNodeMix5kSparse is the headline cell where node48
// actually settles into the inner-node mix.
func renderInnerNodeMix5kSparse() string {
	w := bench.Sparse(5000)
	c7, c8 := buildPair(w)
	c7n4, c7n16, c7n256 := c7.CountByKind()
	c8n4, c8n16, c8n48, c8n256 := c8.CountByKind()
	return fmt.Sprintf("```\n"+
		"Sparse/5000  Chapter 7 mix:  %d n4 + %d n16 + %d n256\n"+
		"             Chapter 8 mix:  %d n4 + %d n16 + %d n48 + %d n256\n"+
		"```",
		c7n4, c7n16, c7n256,
		c8n4, c8n16, c8n48, c8n256)
}

// renderHeapFootprint5k measures heap per key at the 5k-Sparse scale
// where the node48 band is populated.
func renderHeapFootprint5k() string {
	w := bench.Sparse(5000)
	n := len(w.Keys)
	c7 := int(harness.HeapDelta(chapter7Factory(), w)) / n
	c8 := int(harness.HeapDelta(factory(), w)) / n
	bt := int(harness.HeapDelta(harness.BTreeFactory(), w)) / n
	return fmt.Sprintf("```\n"+
		"Workload      Chapter7      Chapter8      improvement     btree\n"+
		"Sparse/5000   %3d B/key     %3d B/key       %.2f×         %3d B/key\n"+
		"```", c7, c8, float64(c7)/float64(c8), bt)
}

func buildPair(w bench.Workload) (*chapter7.Tree[int], *Tree[int]) {
	c7 := chapter7.New[int]()
	c8 := New[int]()
	for i, k := range w.Keys {
		c7.Put(k, w.Vals[i])
		c8.Put(k, w.Vals[i])
	}
	return c7, c8
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
