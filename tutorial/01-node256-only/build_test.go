package nodeonly256

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", []buildcheck.Region{
		{Name: "footprint", Render: renderFootprint},
	})
}

// renderFootprint regenerates the structural-footprint table for
// the chapter's three workloads. Per-node size is unsafe.Sizeof of
// the only node type; nodes/key and bytes/key come from a fresh
// build of each workload.
func renderFootprint() string {
	perNode := int(unsafe.Sizeof(node[int]{}))
	rows := []string{
		"Workload    nodes/key  bytes/key   per node",
		"─────────────────────────────────────────────",
	}
	for _, w := range []bench.Workload{bench.Dense(1000), bench.Sparse(1000), bench.URL(1000)} {
		t := New[int]()
		for i, k := range w.Keys {
			t.Put(k, w.Vals[i])
		}
		nodes := t.CountNodes()
		nodesPerKey := float64(nodes) / float64(len(w.Keys))
		bytesPerKey := nodesPerKey * float64(perNode) / 1024
		rows = append(rows, fmt.Sprintf("%-11s %5.2f      %5.1f KB     %5d B",
			workloadShortName(w), nodesPerKey, bytesPerKey, perNode))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

func workloadShortName(w bench.Workload) string {
	// Trim "/N" suffix; the table groups by shape, not by size.
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
