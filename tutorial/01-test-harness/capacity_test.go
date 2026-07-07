package harness

import (
	"testing"
)

// TestCapacity_100MB_BTree measures how many keys fit in google/btree
// before the heap grows by 100 MB — the same probe every chapter runs
// against its own tree.
func TestCapacity_100MB_BTree(t *testing.T) {
	RunCapacityProbe(t, BTreeContender())
}
