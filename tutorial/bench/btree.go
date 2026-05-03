package bench

import (
	"bytes"

	"github.com/google/btree"
)

// BtreeItem is the entry stored in google/btree's generic tree.
type BtreeItem struct {
	Key []byte
	Val int
}

// LessBtreeItem is the comparator used to order BtreeItem values
// byte-wise on Key. Exported so each stage's bench_test.go can build
// its own *btree.BTreeG[BtreeItem] for comparison.
func LessBtreeItem(a, b BtreeItem) bool { return bytes.Compare(a.Key, b.Key) < 0 }

// NewBtree returns an empty btree with degree 32 (matching the
// project's main bench harness setup), ordered by byte-wise key.
func NewBtree() *btree.BTreeG[BtreeItem] {
	return btree.NewG[BtreeItem](32, LessBtreeItem)
}
