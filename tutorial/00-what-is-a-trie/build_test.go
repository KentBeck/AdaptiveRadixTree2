// Chapter 0 is prose-only, but its cross-chapter links still get
// verified by the shared build check.
package whatisatrie

import (
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", nil)
}
