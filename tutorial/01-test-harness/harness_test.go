package harness

import (
	"iter"
	"testing"
)

// TestRegression_BTreeVsMap runs the canonical scenario set with
// google/btree as the candidate and the sorted-map adapter as the
// reference. Both adapters are part of the harness, so this is the
// harness's own correctness check.
func TestRegression_BTreeVsMap(t *testing.T) {
	RunRegression(t, BTreeFactory(), MapFactory())
}

// TestDiff_Random_BTreeVsMap exercises a 1000-op random trace with
// the default config (small alphabet, lots of collisions).
func TestDiff_Random_BTreeVsMap(t *testing.T) {
	ops := RandomTraceForT(t, RandomConfig{})
	RunDiff(t, NewBTree(), NewMap(), ops)
}

// recorder satisfies the harness's reporter interface for the
// meta-test below. It captures Errorf calls instead of failing.
type recorder struct{ errors int }

func (r *recorder) Errorf(format string, args ...any) { r.errors++ }

// brokenMap implements SortedMap but always reports the map is empty.
// runDiff against a real reference must therefore fire on Get and
// Len -- if it doesn't, the harness isn't actually testing anything.
type brokenMap struct{}

func (brokenMap) Put(key []byte, value int)  {}
func (brokenMap) Get(key []byte) (int, bool) { return 0, false }
func (brokenMap) Delete(key []byte) bool     { return false }
func (brokenMap) Len() int                   { return 0 }
func (brokenMap) Range(from, to []byte) iter.Seq2[[]byte, int] {
	return func(yield func([]byte, int) bool) {}
}

// TestDiff_DetectsDivergence is the meta-test: it pits a deliberately
// broken SortedMap against the reference and asserts that runDiff
// records errors. Without this, a "passing" diff suite could be vacuous.
func TestDiff_DetectsDivergence(t *testing.T) {
	rec := &recorder{}
	ops := []Op{
		Put([]byte("a"), 1),
		Get([]byte("a")),
		Length(),
		Rng(nil, nil),
	}
	runDiff(rec, brokenMap{}, NewMap(), ops)
	if rec.errors == 0 {
		t.Fatalf("runDiff failed to detect divergence; harness is vacuous")
	}
}
