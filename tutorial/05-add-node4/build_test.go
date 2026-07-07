package addnode4

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unsafe"

	chapter4 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/04-path-compression"

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
		{Name: "footprint", Render: renderFootprint, Volatile: true},
		{Name: "optime", Render: func() string { return benchOnce().TimeTable() }, Volatile: true},
		{Name: "opspace", Render: func() string { return benchOnce().SpaceTable() }, Volatile: true},
		{Name: "capacity", Render: func() string { return harness.CapacityTable(contenders()) }, Volatile: true},
	})
}

func renderNodeSizes() string {
	n4 := int(unsafe.Sizeof(node4[int]{}))
	n256 := int(unsafe.Sizeof(node256[int]{}))
	lf := int(unsafe.Sizeof(leaf[int]{}))
	return fmt.Sprintf("```\n"+
		"Type      Bytes    What it holds\n"+
		"node4   %7d    prefix slice, 4 sorted keys, 4 child slots, terminal, count\n"+
		"node256 %7d    prefix slice, 256 child slots, terminal, count\n"+
		"leaf    %7d    key slice header, value (V == int)\n"+
		"ratio   %6dx    node256 / node4\n"+
		"```", n4, n256, lf, n256/n4)
}

// renderInnerNodeMix shows where node4 actually replaces node256.
// Chapter 4 carried one inner-node type; this chapter splits the
// population.
func renderInnerNodeMix() string {
	rows := []string{
		"Workload    Chapter 4 inner   Chapter 5 (n4 + n256)",
	}
	for _, w := range harness.Workloads1k() {
		c4 := chapter4.New[int]()
		c5 := New[int]()
		for i, k := range w.Keys {
			c4.Put(k, w.Vals[i])
			c5.Put(k, w.Vals[i])
		}
		n4, n256 := c5.CountByKind()
		rows = append(rows, fmt.Sprintf("%-11s %9d           %5d + %d",
			workloadShortName(w), c4.CountInner(), n4, n256))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// renderFootprint compares, per key, the structural cost (sizeof
// arithmetic over live nodes, key bytes, and prefix bytes) with the
// measured heap cost for both chapters.
func renderFootprint() string {
	const c4InnerSize = int(unsafe.Sizeof(chapter4Node{}))
	n4Size := int(unsafe.Sizeof(node4[int]{}))
	n256Size := int(unsafe.Sizeof(node256[int]{}))
	leafSize := int(unsafe.Sizeof(leaf[int]{}))

	rows := []string{fmt.Sprintf("%-9s %16s %10s %16s %10s %12s",
		"Workload", "Chapter4 struct", "heap", "Chapter5 struct", "heap", "improvement")}
	for _, w := range harness.Workloads1k() {
		c4 := chapter4.New[int]()
		c5 := New[int]()
		keyBytes := 0
		for i, k := range w.Keys {
			c4.Put(k, w.Vals[i])
			c5.Put(k, w.Vals[i])
			keyBytes += len(k)
		}
		n := len(w.Keys)
		nn4, nn256 := c5.CountByKind()
		c4Struct := (c4.CountInner()*c4InnerSize + c4.CountLeaves()*leafSize + keyBytes + c4.PrefixBytes()) / n
		c5Struct := (nn4*n4Size + nn256*n256Size + c5.CountLeaves()*leafSize + keyBytes + c5.PrefixBytes()) / n
		c4Heap := int(harness.HeapDelta(chapter4Factory(), w)) / n
		c5Heap := int(harness.HeapDelta(factory(), w)) / n
		rows = append(rows, fmt.Sprintf("%-9s %14s B %8s B %14s B %8s B %11.2f×",
			workloadShortName(w), group(c4Struct), group(c4Heap), group(c5Struct), group(c5Heap),
			float64(c4Heap)/float64(c5Heap)))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// chapter4Node mirrors chapter 4's node256[int] layout for sizeof
// arithmetic (its fields are unexported across packages).
type chapter4Node struct {
	prefix   []byte
	children [256]any
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
