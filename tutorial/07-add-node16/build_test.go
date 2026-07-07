package addnode16

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unsafe"

	chapter6 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/06-introduce-polymorphism"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

// benchOnce shares one measurement run between the time and space
// regions so -update-bench doesn't benchmark everything twice.
var benchOnce = sync.OnceValue(func() harness.BenchResults {
	return harness.BenchAll(contenders())
})

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", []buildcheck.Region{
		{Name: "nodesizes", Render: renderNodeSizes},
		{Name: "innernodemix", Render: renderInnerNodeMix},
		{Name: "heapfootprint", Render: renderHeapFootprint, Volatile: true},
		{Name: "optime", Render: func() string { return benchOnce().TimeTable() }, Volatile: true},
		{Name: "opspace", Render: func() string { return benchOnce().SpaceTable() }, Volatile: true},
		{Name: "capacity", Render: func() string { return harness.CapacityTable(contenders()) }, Volatile: true},
	})
}

func renderNodeSizes() string {
	n4 := int(unsafe.Sizeof(node4[int]{}))
	n16 := int(unsafe.Sizeof(node16[int]{}))
	n256 := int(unsafe.Sizeof(node256[int]{}))
	lf := int(unsafe.Sizeof(leaf[int]{}))
	return fmt.Sprintf("```\n"+
		"Type       Bytes   Slot\n"+
		"node4    %7d   sorted [4]keys + [4]children\n"+
		"node16   %7d   sorted [16]keys + [16]children\n"+
		"node256  %7d   indexed [256]children\n"+
		"leaf     %7d   key slice header + value (V == int)\n"+
		"```", n4, n16, n256, lf)
}

func renderInnerNodeMix() string {
	rows := []string{
		"Workload    Chapter 6 (n4 + n256)      Chapter 7 (n4 + n16 + n256)",
	}
	for _, w := range harness.Workloads1k() {
		c6 := chapter6.New[int]()
		c7 := New[int]()
		for i, k := range w.Keys {
			c6.Put(k, w.Vals[i])
			c7.Put(k, w.Vals[i])
		}
		c6n4, c6n256 := c6.CountByKind()
		c7n4, c7n16, c7n256 := c7.CountByKind()
		rows = append(rows, fmt.Sprintf("%-11s %5d + %-5d              %5d + %d + %d",
			workloadShortName(w), c6n4, c6n256, c7n4, c7n16, c7n256))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// renderHeapFootprint compares measured heap per key for the two
// chapters -- what the node16 rung of the ladder is worth.
func renderHeapFootprint() string {
	rows := []string{fmt.Sprintf("%-9s %15s %15s %8s",
		"Workload", "Chapter6 heap", "Chapter7 heap", "improvement")}
	for _, w := range harness.Workloads1k() {
		n := len(w.Keys)
		c6 := int(harness.HeapDelta(chapter6Factory(), w)) / n
		c7 := int(harness.HeapDelta(factory(), w)) / n
		rows = append(rows, fmt.Sprintf("%-9s %9d B/key %9d B/key %7.2f×",
			workloadShortName(w), c6, c7, float64(c6)/float64(c7)))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
