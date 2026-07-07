package pathcompression

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unsafe"

	chapter3 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/03-lazy-expansion"

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

// renderInnerNodeMix shows what path compression collapsed: how many
// inner nodes survived from chapter 3, and how many bytes of prefix
// the surviving nodes hold.
func renderInnerNodeMix() string {
	rows := []string{
		"Workload    Chapter 3 inner   Chapter 4 inner   prefix bytes",
	}
	for _, w := range harness.Workloads1k() {
		c3 := chapter3.New[int]()
		c4 := New[int]()
		for i, k := range w.Keys {
			c3.Put(k, w.Vals[i])
			c4.Put(k, w.Vals[i])
		}
		rows = append(rows, fmt.Sprintf("%-11s %9d         %9d         %4d B",
			workloadShortName(w), c3.CountInner(), c4.CountInner(), c4.PrefixBytes()))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// renderFootprint compares, per key, the structural cost (sizeof
// arithmetic over live nodes, key bytes, and prefix bytes) with the
// measured heap cost for both chapters.
func renderFootprint() string {
	const c3InnerSize = int(unsafe.Sizeof(chapter3Node{}))
	c4InnerSize := int(unsafe.Sizeof(node256[int]{}))
	leafSize := int(unsafe.Sizeof(leaf[int]{}))

	rows := []string{fmt.Sprintf("%-9s %16s %10s %16s %10s %12s",
		"Workload", "Chapter3 struct", "heap", "Chapter4 struct", "heap", "improvement")}
	for _, w := range harness.Workloads1k() {
		c3 := chapter3.New[int]()
		c4 := New[int]()
		keyBytes := 0
		for i, k := range w.Keys {
			c3.Put(k, w.Vals[i])
			c4.Put(k, w.Vals[i])
			keyBytes += len(k)
		}
		n := len(w.Keys)
		c3Struct := (c3.CountInner()*c3InnerSize + c3.CountLeaves()*leafSize + keyBytes) / n
		c4Struct := (c4.CountInner()*c4InnerSize + c4.CountLeaves()*leafSize + keyBytes + c4.PrefixBytes()) / n
		c3Heap := int(harness.HeapDelta(chapter3Factory(), w)) / n
		c4Heap := int(harness.HeapDelta(factory(), w)) / n
		rows = append(rows, fmt.Sprintf("%-9s %14s B %8s B %14s B %8s B %11.2f×",
			workloadShortName(w), group(c3Struct), group(c3Heap), group(c4Struct), group(c4Heap),
			float64(c3Heap)/float64(c4Heap)))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// chapter3Node mirrors chapter 3's node256[int] layout for sizeof
// arithmetic (its fields are unexported across packages): 256
// interface slots plus a terminal pointer.
type chapter3Node struct {
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
