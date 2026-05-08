package addnode16

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	stage5 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/06-introduce-polymorphism"
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
		"Workload    Stage 5 (n4 + n256)        Stage 6 (n4 + n16 + n256)",
	}
	for _, w := range []bench.Workload{bench.Dense(1000), bench.Sparse(1000), bench.URL(1000)} {
		s5 := stage5.New[int]()
		s6 := New[int]()
		for i, k := range w.Keys {
			s5.Put(k, w.Vals[i])
			s6.Put(k, w.Vals[i])
		}
		s5n4, s5n256 := s5.CountByKind()
		s6n4, s6n16, s6n256 := s6.CountByKind()
		rows = append(rows, fmt.Sprintf("%-11s %5d + %-5d              %5d + %d + %d",
			workloadShortName(w), s5n4, s5n256, s6n4, s6n16, s6n256))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
