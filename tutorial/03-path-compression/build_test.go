package pathcompression

import (
	"fmt"
	"strings"
	"testing"

	stage2 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/02-lazy-expansion"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", []buildcheck.Region{
		{Name: "innernodemix", Render: renderInnerNodeMix},
	})
}

// renderInnerNodeMix shows what path compression collapsed: how many
// inner nodes survived from stage 2, and how many bytes of prefix
// the surviving nodes hold.
func renderInnerNodeMix() string {
	rows := []string{
		"Workload    Stage 2 inner   Stage 3 inner   prefix bytes",
	}
	for _, w := range []bench.Workload{bench.Dense(1000), bench.Sparse(1000), bench.URL(1000)} {
		s2 := stage2.New[int]()
		s3 := New[int]()
		for i, k := range w.Keys {
			s2.Put(k, w.Vals[i])
			s3.Put(k, w.Vals[i])
		}
		rows = append(rows, fmt.Sprintf("%-11s %7d         %7d         %4d B",
			workloadShortName(w), s2.CountInner(), s3.CountInner(), s3.PrefixBytes()))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
