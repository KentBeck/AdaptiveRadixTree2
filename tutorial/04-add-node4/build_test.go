package addnode4

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", []buildcheck.Region{
		{Name: "nodesizes", Render: renderNodeSizes},
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
