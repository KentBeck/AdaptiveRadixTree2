package nodeonly256

import (
	"flag"
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/harness"
)

var capacityFlag = flag.Bool("capacity", false, "run the 100MB capacity benchmarks")

// TestCapacity_100MB measures how many keys node256-only fits before
// HeapAlloc crosses 100 MiB, for each of the three standard workloads.
//
//	go test ./02-node256-only/ -run TestCapacity -capacity -timeout 10m -v
func TestCapacity_100MB(t *testing.T) {
	if !*capacityFlag {
		t.Skip("set -capacity to run")
	}
	const budget = uint64(100) << 20
	const batch = 1000

	cases := []struct {
		name string
		gen  func(int) ([]byte, int)
	}{
		{"Dense", harness.DenseGen()},
		{"Sparse", harness.SparseGen()},
		{"URL", harness.URLGen()},
	}

	t.Logf("%-8s  %-12s  %-12s  %-10s  %-12s",
		"workload", "keys-fit", "heap-bytes", "avg-key", "bytes/key")
	for _, c := range cases {
		res := harness.MeasureCapacity(factory(), c.name, c.gen, budget, batch)
		t.Logf("%-8s  %-12d  %-12d  %-10.2f  %-12.2f",
			res.Workload, res.KeysFit, res.HeapBytes, res.AvgKeyLen, res.BytesPerKey)
	}
}
