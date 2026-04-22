package art

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"sync"
	"testing"

	"github.com/google/btree"
)

// benchN is the working set size: 10 million entries.
const benchN = 10_000_000

type kv struct {
	k []byte
	v int
}

func kvLess(a, b kv) bool { return bytes.Compare(a.k, b.k) < 0 }

// Range window: 1 % of the working set. Keys are big-endian uint64 of a
// permutation of [0, benchN), so the half-open byte range [rangeLo, rangeHi)
// contains exactly rangeN distinct entries in sorted order.
const (
	rangeLoV = benchN / 2
	rangeHiV = rangeLoV + benchN/100
	rangeN   = rangeHiV - rangeLoV
)

var (
	benchKeys [][]byte
	missKeys  [][]byte
	rangeLo   []byte
	rangeHi   []byte
	keysOnce  sync.Once

	artOnce sync.Once
	artBig  *Tree[int]

	btOnce sync.Once
	btBig  *btree.BTreeG[kv]
)

func initKeys() {
	keysOnce.Do(func() {
		r := rand.New(rand.NewSource(42))
		perm := r.Perm(benchN)
		benchKeys = make([][]byte, benchN)
		for i, v := range perm {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b, uint64(v))
			benchKeys[i] = b
		}
		// Miss keys: values outside [0, benchN), guaranteed not present.
		const missCount = 1024
		missKeys = make([][]byte, missCount)
		for i := 0; i < missCount; i++ {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b, uint64(benchN+i))
			missKeys[i] = b
		}
		rangeLo = make([]byte, 8)
		rangeHi = make([]byte, 8)
		binary.BigEndian.PutUint64(rangeLo, uint64(rangeLoV))
		binary.BigEndian.PutUint64(rangeHi, uint64(rangeHiV))
	})
}

func getArtBig() *Tree[int] {
	initKeys()
	artOnce.Do(func() {
		t := New[int]()
		for i := 0; i < benchN; i++ {
			t.Put(benchKeys[i], i)
		}
		artBig = t
	})
	return artBig
}

func getBtBig() *btree.BTreeG[kv] {
	initKeys()
	btOnce.Do(func() {
		t := btree.NewG[kv](32, kvLess)
		for i := 0; i < benchN; i++ {
			t.ReplaceOrInsert(kv{k: benchKeys[i], v: i})
		}
		btBig = t
	})
	return btBig
}

func perKey(b *testing.B, ops int) {
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*ops), "ns/key")
}

// --- Put: build a 10M-element tree from empty ---

func BenchmarkPut_ART(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		t := New[int]()
		for i := 0; i < benchN; i++ {
			t.Put(benchKeys[i], i)
		}
	}
	perKey(b, benchN)
}

func BenchmarkPut_BTree(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		t := btree.NewG[kv](32, kvLess)
		for i := 0; i < benchN; i++ {
			t.ReplaceOrInsert(kv{k: benchKeys[i], v: i})
		}
	}
	perKey(b, benchN)
}

// --- Get: hit on populated 10M-element tree ---

func BenchmarkGet_ART(b *testing.B) {
	t := getArtBig()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(benchKeys[i%benchN])
	}
}

func BenchmarkGet_BTree(b *testing.B) {
	t := getBtBig()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(kv{k: benchKeys[i%benchN]})
	}
}

// --- GetMiss: lookup of keys not present in the tree ---

func BenchmarkGetMiss_ART(b *testing.B) {
	t := getArtBig()
	n := len(missKeys)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(missKeys[i%n])
	}
}

func BenchmarkGetMiss_BTree(b *testing.B) {
	t := getBtBig()
	n := len(missKeys)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(kv{k: missKeys[i%n]})
	}
}

// --- Delete: delete all 10M entries from a populated tree (setup excluded) ---

func BenchmarkDelete_ART(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := New[int]()
		for i := 0; i < benchN; i++ {
			t.Put(benchKeys[i], i)
		}
		b.StartTimer()
		for i := 0; i < benchN; i++ {
			t.Delete(benchKeys[i])
		}
	}
	perKey(b, benchN)
}

func BenchmarkDelete_BTree(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := btree.NewG[kv](32, kvLess)
		for i := 0; i < benchN; i++ {
			t.ReplaceOrInsert(kv{k: benchKeys[i], v: i})
		}
		b.StartTimer()
		for i := 0; i < benchN; i++ {
			t.Delete(kv{k: benchKeys[i]})
		}
	}
	perKey(b, benchN)
}

// --- Range: in-order scan of 1 % of the working set (~100K entries) ---

func BenchmarkRange_ART(b *testing.B) {
	t := getArtBig()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		for range t.Range(rangeLo, rangeHi) {
			count++
		}
		if count != rangeN {
			b.Fatalf("ranged %d, want %d", count, rangeN)
		}
	}
	perKey(b, rangeN)
}

func BenchmarkRange_BTree(b *testing.B) {
	t := getBtBig()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		t.AscendRange(kv{k: rangeLo}, kv{k: rangeHi}, func(_ kv) bool {
			count++
			return true
		})
		if count != rangeN {
			b.Fatalf("ranged %d, want %d", count, rangeN)
		}
	}
	perKey(b, rangeN)
}
