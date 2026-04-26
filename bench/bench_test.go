package bench

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"sync"
	"testing"

	art "github.com/KentBeck/AdaptiveRadixTree2"
	"github.com/google/btree"
	plart "github.com/plar/go-adaptive-radix-tree"
	tbtree "github.com/tidwall/btree"
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
	artBig  *art.Tree[int]

	btOnce sync.Once
	btBig  *btree.BTreeG[kv]

	tidwallOnce sync.Once
	tidwallBig  *tbtree.BTreeG[kv]

	plarOnce sync.Once
	plarBig  plart.Tree
)

// tidwallOpts configures tidwall/btree to match google/btree's degree (32) and
// disables tidwall's internal RWMutex via NoLocks=true. Without NoLocks, every
// op pays a sync.RWMutex Lock/Unlock pair, which is overhead google/btree never
// charges; turning locks off keeps the comparison apples-to-apples.
var tidwallOpts = tbtree.Options{NoLocks: true, Degree: 32}

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

func getArtBig() *art.Tree[int] {
	initKeys()
	artOnce.Do(func() {
		t := art.New[int]()
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

func getTidwallBig() *tbtree.BTreeG[kv] {
	initKeys()
	tidwallOnce.Do(func() {
		t := tbtree.NewBTreeGOptions(kvLess, tidwallOpts)
		for i := 0; i < benchN; i++ {
			t.Set(kv{k: benchKeys[i], v: i})
		}
		tidwallBig = t
	})
	return tidwallBig
}

func getPlarBig() plart.Tree {
	initKeys()
	plarOnce.Do(func() {
		t := plart.New()
		for i := 0; i < benchN; i++ {
			t.Insert(plart.Key(benchKeys[i]), plart.Value(i))
		}
		plarBig = t
	})
	return plarBig
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
		t := art.New[int]()
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
		t := art.New[int]()
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

// --- RangeDescending: reverse in-order scan of the same window ---

func BenchmarkRangeDescending_ART(b *testing.B) {
	t := getArtBig()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		for range t.RangeDescending(rangeLo, rangeHi) {
			count++
		}
		if count != rangeN {
			b.Fatalf("ranged %d, want %d", count, rangeN)
		}
	}
	perKey(b, rangeN)
}

func BenchmarkRangeDescending_BTree(b *testing.B) {
	t := getBtBig()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		t.DescendRange(kv{k: rangeHi}, kv{k: rangeLo}, func(_ kv) bool {
			count++
			return true
		})
		if count != rangeN {
			b.Fatalf("ranged %d, want %d", count, rangeN)
		}
	}
	perKey(b, rangeN)
}

// --- RangeFrom: open-ended scan from start to the end of the tree ---

func BenchmarkRangeFrom_ART(b *testing.B) {
	t := getArtBig()
	wantCount := benchN - rangeLoV
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		for range t.RangeFrom(rangeLo) {
			count++
		}
		if count != wantCount {
			b.Fatalf("ranged %d, want %d", count, wantCount)
		}
	}
	perKey(b, wantCount)
}

func BenchmarkRangeFrom_BTree(b *testing.B) {
	t := getBtBig()
	wantCount := benchN - rangeLoV
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		t.AscendGreaterOrEqual(kv{k: rangeLo}, func(_ kv) bool {
			count++
			return true
		})
		if count != wantCount {
			b.Fatalf("ranged %d, want %d", count, wantCount)
		}
	}
	perKey(b, wantCount)
}

// --- tidwall/btree comparator (Options{NoLocks: true, Degree: 32}) ---

func BenchmarkPut_Tidwall(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		t := tbtree.NewBTreeGOptions(kvLess, tidwallOpts)
		for i := 0; i < benchN; i++ {
			t.Set(kv{k: benchKeys[i], v: i})
		}
	}
	perKey(b, benchN)
}

func BenchmarkGet_Tidwall(b *testing.B) {
	t := getTidwallBig()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(kv{k: benchKeys[i%benchN]})
	}
}

func BenchmarkGetMiss_Tidwall(b *testing.B) {
	t := getTidwallBig()
	n := len(missKeys)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(kv{k: missKeys[i%n]})
	}
}

func BenchmarkDelete_Tidwall(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := tbtree.NewBTreeGOptions(kvLess, tidwallOpts)
		for i := 0; i < benchN; i++ {
			t.Set(kv{k: benchKeys[i], v: i})
		}
		b.StartTimer()
		for i := 0; i < benchN; i++ {
			t.Delete(kv{k: benchKeys[i]})
		}
	}
	perKey(b, benchN)
}

func BenchmarkRange_Tidwall(b *testing.B) {
	t := getTidwallBig()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		t.Ascend(kv{k: rangeLo}, func(it kv) bool {
			if bytes.Compare(it.k, rangeHi) >= 0 {
				return false
			}
			count++
			return true
		})
		if count != rangeN {
			b.Fatalf("ranged %d, want %d", count, rangeN)
		}
	}
	perKey(b, rangeN)
}

// --- plar/go-adaptive-radix-tree comparator (interface{} values; no seek) ---

func BenchmarkPut_Plar(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		t := plart.New()
		for i := 0; i < benchN; i++ {
			t.Insert(plart.Key(benchKeys[i]), plart.Value(i))
		}
	}
	perKey(b, benchN)
}

func BenchmarkGet_Plar(b *testing.B) {
	t := getPlarBig()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Search(plart.Key(benchKeys[i%benchN]))
	}
}

func BenchmarkGetMiss_Plar(b *testing.B) {
	t := getPlarBig()
	n := len(missKeys)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Search(plart.Key(missKeys[i%n]))
	}
}

func BenchmarkDelete_Plar(b *testing.B) {
	initKeys()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := plart.New()
		for i := 0; i < benchN; i++ {
			t.Insert(plart.Key(benchKeys[i]), plart.Value(i))
		}
		b.StartTimer()
		for i := 0; i < benchN; i++ {
			t.Delete(plart.Key(benchKeys[i]))
		}
	}
	perKey(b, benchN)
}

// BenchmarkRange_Plar: plar's Iterator() does not expose efficient seeking, so
// we walk leaves from the start of the tree, skipping until key >= rangeLo and
// stopping when key >= rangeHi. This is the cheapest available form on the
// public API and is structurally slower than seek-then-scan; benchmarks.md
// documents the limitation.
func BenchmarkRange_Plar(b *testing.B) {
	t := getPlarBig()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		for it := t.Iterator(); it.HasNext(); {
			node, err := it.Next()
			if err != nil {
				b.Fatalf("iterator error: %v", err)
			}
			k := []byte(node.Key())
			if bytes.Compare(k, rangeLo) < 0 {
				continue
			}
			if bytes.Compare(k, rangeHi) >= 0 {
				break
			}
			count++
		}
		if count != rangeN {
			b.Fatalf("ranged %d, want %d", count, rangeN)
		}
	}
	perKey(b, rangeN)
}
