package lazyexpansion

import (
	"fmt"
	"strings"
	"testing"

	stage1 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/02-node256-only"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", []buildcheck.Region{
		{Name: "innernodemix", Render: renderInnerNodeMix},
	})
}

// renderInnerNodeMix counts inner nodes and leaves for each workload
// in both stages so the chapter can show what lazy expansion changed
// structurally. Stage 1 has no leaf type so its CountNodes is the
// only column; stage 2 splits into (inner + leaves).
func renderInnerNodeMix() string {
	rows := []string{
		"Workload    Stage 1 inner    Stage 2 inner + leaves",
	}
	for _, w := range []bench.Workload{bench.Dense(1000), bench.Sparse(1000), bench.URL(1000)} {
		s1 := stage1.New[int]()
		s2 := New[int]()
		for i, k := range w.Keys {
			s1.Put(k, w.Vals[i])
			s2.Put(k, w.Vals[i])
		}
		rows = append(rows, fmt.Sprintf("%-11s %7d        %5d + %d",
			workloadShortName(w), s1.CountNodes(), s2.CountInner(), s2.CountLeaves()))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
