package introducepolymorphism

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
		{Name: "nodesizes", Render: renderNodeSizes},
		{Name: "heapfootprint", Render: renderHeapFootprint, Volatile: true},
		{Name: "optime", Render: func() string { return benchOnce().TimeTable() }, Volatile: true},
		{Name: "opspace", Render: func() string { return benchOnce().SpaceTable() }, Volatile: true},
		{Name: "capacity", Render: func() string { return harness.CapacityTable(contenders()) }, Volatile: true},
	})
}

// chapter 5's node types stored terminal as *leaf[V] (8 B pointer);
// this chapter's store it as node (16 B interface). Mirrors for
// sizeof arithmetic, since the fields are unexported across packages.
type (
	chapter5Node4 struct {
		prefix      []byte
		keys        [4]byte
		children    [4]any
		terminal    *int
		numChildren uint8
	}
	chapter5Node256 struct {
		prefix      []byte
		children    [256]any
		terminal    *int
		numChildren uint16
	}
)

func renderNodeSizes() string {
	return fmt.Sprintf("```\n"+
		"Type        Chapter 5    Chapter 6\n"+
		"node4       %5d B      %5d B\n"+
		"node256     %5d B      %5d B\n"+
		"leaf        %5d B      %5d B\n"+
		"```",
		int(unsafe.Sizeof(chapter5Node4{})), int(unsafe.Sizeof(node4[int]{})),
		int(unsafe.Sizeof(chapter5Node256{})), int(unsafe.Sizeof(node256[int]{})),
		int(unsafe.Sizeof(leaf[int]{})), int(unsafe.Sizeof(leaf[int]{})))
}

// renderHeapFootprint compares measured heap per key for the two
// chapters -- the refactor's space cost, which should be ~none.
func renderHeapFootprint() string {
	rows := []string{fmt.Sprintf("%-9s %15s %15s %8s",
		"Workload", "Chapter5 heap", "Chapter6 heap", "ratio")}
	for _, w := range harness.Workloads1k() {
		n := len(w.Keys)
		c5 := int(harness.HeapDelta(chapter5Factory(), w)) / n
		c6 := int(harness.HeapDelta(factory(), w)) / n
		rows = append(rows, fmt.Sprintf("%-9s %9d B/key %9d B/key %7.2f×",
			workloadShortName(w), c5, c6, float64(c6)/float64(c5)))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
