package lazyexpansion

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unsafe"

	chapter2 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/02-node256-only"

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
		{Name: "innernodemix", Render: renderInnerNodeMix},
		{Name: "footprint", Render: renderFootprint, Volatile: true},
		{Name: "optime", Render: func() string { return benchOnce().TimeTable() }, Volatile: true},
		{Name: "opspace", Render: func() string { return benchOnce().SpaceTable() }, Volatile: true},
		{Name: "capacity", Render: func() string { return harness.CapacityTable(contenders()) }, Volatile: true},
	})
}

// renderInnerNodeMix counts inner nodes and leaves for each workload
// in both chapters so the prose can show what lazy expansion changed
// structurally. Chapter 2 has no leaf type so its node count is the
// only column; chapter 3 splits into (inner + leaves).
func renderInnerNodeMix() string {
	rows := []string{
		"Workload    Chapter 2 inner    Chapter 3 inner + leaves",
	}
	for _, w := range harness.Workloads1k() {
		c2 := chapter2.New[int]()
		c3 := New[int]()
		for i, k := range w.Keys {
			c2.Put(k, w.Vals[i])
			c3.Put(k, w.Vals[i])
		}
		rows = append(rows, fmt.Sprintf("%-11s %9d        %7d + %d",
			workloadShortName(w), c2.CountNodes(), c3.CountInner(), c3.CountLeaves()))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// renderFootprint compares, per key, the structural cost (sizeof
// arithmetic over live nodes) with the measured heap cost (post-GC
// HeapAlloc delta) for both chapters. Heap ≥ structural: the
// difference is malloc size-class rounding and allocator metadata.
func renderFootprint() string {
	const c2NodeSize = int(unsafe.Sizeof(chapter2Node{}))
	c3InnerSize := int(unsafe.Sizeof(node256[int]{}))
	c3LeafSize := int(unsafe.Sizeof(leaf[int]{}))

	rows := []string{fmt.Sprintf("%-9s %16s %10s %16s %10s %12s",
		"Workload", "Chapter2 struct", "heap", "Chapter3 struct", "heap", "improvement")}
	for _, w := range harness.Workloads1k() {
		c2 := chapter2.New[int]()
		c3 := New[int]()
		keyBytes := 0
		for i, k := range w.Keys {
			c2.Put(k, w.Vals[i])
			c3.Put(k, w.Vals[i])
			keyBytes += len(k)
		}
		n := len(w.Keys)
		c2Struct := c2.CountNodes() * c2NodeSize / n
		c3Struct := (c3.CountInner()*c3InnerSize + c3.CountLeaves()*c3LeafSize + keyBytes) / n
		c2Heap := int(harness.HeapDelta(chapter2Factory(), w)) / n
		c3Heap := int(harness.HeapDelta(factory(), w)) / n
		rows = append(rows, fmt.Sprintf("%-9s %14s B %8s B %14s B %8s B %11.1f×",
			workloadShortName(w), group(c2Struct), group(c2Heap), group(c3Struct), group(c3Heap),
			float64(c2Heap)/float64(c3Heap)))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// chapter2Node mirrors chapter 2's node[int] layout for sizeof
// arithmetic (its fields are unexported across packages).
type chapter2Node struct {
	children [256]*chapter2Node
	terminal *int
}

func group(n int) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
