package lazyexpansion

import (
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/buildcheck"
)

func TestTutorialMD(t *testing.T) {
	buildcheck.Check(t, "tutorial.md", nil)
}
