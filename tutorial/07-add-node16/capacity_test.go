package addnode16

import (
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/01-test-harness"
)

// TestCapacity_100MB answers the chapter's capacity question on this
// machine; the committed table in tutorial.md is rendered from the
// same probe via -update-bench.
func TestCapacity_100MB(t *testing.T) {
	harness.RunCapacityProbe(t, contenders()...)
}
