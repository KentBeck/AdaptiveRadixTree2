package addnode48

import (
	"fmt"
	"strings"
	"testing"

	stage6 "github.com/KentBeck/AdaptiveRadixTree2/tutorial/07-add-node16"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", []buildcheck.Region{
		{Name: "innernodemix1k", Render: renderInnerNodeMix1k},
		{Name: "innernodemix5k", Render: renderInnerNodeMix5kSparse},
	})
}

// renderInnerNodeMix1k shows that the 17-48 fanout band is empty at
// the 1k fixture size for all three workloads -- so node48 catches
// nothing here. The 5k Sparse table below is where it earns its
// keep.
func renderInnerNodeMix1k() string {
	rows := []string{
		"Workload    Stage 6 mix              Stage 7 mix",
	}
	for _, w := range []bench.Workload{bench.Dense(1000), bench.Sparse(1000), bench.URL(1000)} {
		s6, s7 := buildPair(w)
		s6n4, s6n16, s6n256 := s6.CountByKind()
		s7n4, s7n16, s7n48, s7n256 := s7.CountByKind()
		rows = append(rows, fmt.Sprintf("%-11s %dn4 + %dn16 + %dn256        %dn4 + %dn16 + %dn48 + %dn256",
			workloadShortName(w),
			s6n4, s6n16, s6n256,
			s7n4, s7n16, s7n48, s7n256))
	}
	return "```\n" + strings.Join(rows, "\n") + "\n```"
}

// renderInnerNodeMix5kSparse is the headline cell where node48
// actually settles into the inner-node mix.
func renderInnerNodeMix5kSparse() string {
	w := bench.Sparse(5000)
	s6, s7 := buildPair(w)
	s6n4, s6n16, s6n256 := s6.CountByKind()
	s7n4, s7n16, s7n48, s7n256 := s7.CountByKind()
	return fmt.Sprintf("```\n"+
		"Sparse/5000  S6 mix:  %d n4 + %d n16 + %d n256\n"+
		"             S7 mix:  %d n4 + %d n16 + %d n48 + %d n256\n"+
		"```",
		s6n4, s6n16, s6n256,
		s7n4, s7n16, s7n48, s7n256)
}

func buildPair(w bench.Workload) (*stage6.Tree[int], *Tree[int]) {
	s6 := stage6.New[int]()
	s7 := New[int]()
	for i, k := range w.Keys {
		s6.Put(k, w.Vals[i])
		s7.Put(k, w.Vals[i])
	}
	return s6, s7
}

func workloadShortName(w bench.Workload) string {
	if i := strings.IndexByte(w.Name, '/'); i >= 0 {
		return w.Name[:i]
	}
	return w.Name
}
