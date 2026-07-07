package polish

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unsafe"

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
		{Name: "leafsizes", Render: renderLeafSizes},
		{Name: "heapfootprint", Render: renderHeapFootprint, Volatile: true},
		{Name: "optime", Render: func() string { return benchOnce().TimeTable() }, Volatile: true},
		{Name: "opspace", Render: func() string { return benchOnce().SpaceTable() }, Volatile: true},
		{Name: "capacity", Render: func() string { return harness.CapacityTable(contenders()) }, Volatile: true},
	})
}

// chapter8Leaf mirrors chapter 8's leaf[int] layout (no inline
// buffer) for sizeof arithmetic.
type chapter8Leaf struct {
	key   []byte
	value int
}

func renderLeafSizes() string {
	return fmt.Sprintf("```\n"+
		"Chapter 8 leaf   %2d B   key slice header + value\n"+
		"Chapter 9 leaf   %2d B   + %d-byte inline key buffer\n"+
		"```",
		int(unsafe.Sizeof(chapter8Leaf{})),
		int(unsafe.Sizeof(leaf[int]{})), inlineKeyMax)
}

// renderHeapFootprint shows the inline-key trade on live heap: every
// leaf carries the buffer whether it uses it or not.
func renderHeapFootprint() string {
	rows := []string{fmt.Sprintf("%-9s %15s %15s %8s",
		"Workload", "Chapter8 heap", "Chapter9 heap", "ratio")}
	for _, w := range harness.Workloads1k() {
		n := len(w.Keys)
		c8 := int(harness.HeapDelta(chapter8Factory(), w)) / n
		c9 := int(harness.HeapDelta(factory(), w)) / n
		rows = append(rows, fmt.Sprintf("%-9s %9d B/key %9d B/key %7.2f×",
			workloadShortName(w), c8, c9, float64(c9)/float64(c8)))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
