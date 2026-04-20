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

var (
	benchKeys [][]byte
	keysOnce  sync.Once

	artOnce sync.Once
	artBig  *Tree

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
	})
}

func getArtBig() *Tree {
	initKeys()
	artOnce.Do(func() {
		t := New()
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
		t := New()
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

// --- Delete: delete all 10M entries from a populated tree (setup excluded) ---

func BenchmarkDelete_ART(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := New()
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

// --- Iterate: full in-order scan of 10M entries ---

func BenchmarkIterate_ART(b *testing.B) {
	t := getArtBig()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		for range t.All() {
			count++
		}
		if count != benchN {
			b.Fatalf("iterated %d, want %d", count, benchN)
		}
	}
	perKey(b, benchN)
}

func BenchmarkIterate_BTree(b *testing.B) {
	t := getBtBig()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		t.Ascend(func(_ kv) bool { count++; return true })
		if count != benchN {
			b.Fatalf("iterated %d, want %d", count, benchN)
		}
	}
	perKey(b, benchN)
}

