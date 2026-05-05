package addnode4

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	stage3 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/03-path-compression"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", []buildcheck.Region{
		{Name: "nodesizes", Render: renderNodeSizes},
		{Name: "innernodemix", Render: renderInnerNodeMix},
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
// Stage 3 carried one inner-node type; stage 4 splits the population.
func renderInnerNodeMix() string {
	rows := []string{
		"Workload    Stage 3 inner   Stage 4 (n4 + n256)",
	}
	for _, w := range []bench.Workload{bench.Dense(1000), bench.Sparse(1000), bench.URL(1000)} {
		s3 := stage3.New[int]()
		s4 := New[int]()
		for i, k := range w.Keys {
			s3.Put(k, w.Vals[i])
			s4.Put(k, w.Vals[i])
		}
		n4, n256 := s4.CountByKind()
		rows = append(rows, fmt.Sprintf("%-11s %7d         %5d + %d",
			workloadShortName(w), s3.CountInner(), n4, n256))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
