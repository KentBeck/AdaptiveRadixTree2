package harness

import (
	"flag"
	"testing"
)

var capacityFlag = flag.Bool("capacity", false, "run the 100MB capacity benchmarks")

// TestCapacity_100MB_BTree measures how many keys fit in google/btree
// before HeapAlloc crosses 100 MiB, for each of the three standard
// workloads. Output is logged in a fixed-width table for copy-paste
// into the eventual chapter.
//
//	go test ./harness/ -run TestCapacity -capacity -timeout 5m -v
func TestCapacity_100MB_BTree(t *testing.T) {
	if !*capacityFlag {
		t.Skip("set -capacity to run")
	}
	const budget = uint64(100) << 20
	const batch = 1000

	cases := []struct {
		name string
		gen  func(int) ([]byte, int)
	}{
		{"Dense", DenseGen()},
		{"Sparse", SparseGen()},
		{"URL", URLGen()},
	}

	t.Logf("%-8s  %-12s  %-12s  %-10s  %-12s",
		"workload", "keys-fit", "heap-bytes", "avg-key", "bytes/key")
	for _, c := range cases {
		res := MeasureCapacity(BTreeFactory(), c.name, c.gen, budget, batch)
		t.Logf("%-8s  %-12d  %-12d  %-10.2f  %-12.2f",
			res.Workload, res.KeysFit, res.HeapBytes, res.AvgKeyLen, res.BytesPerKey)
	}
}
