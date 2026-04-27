package bench

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"testing"

	art "github.com/KentBeck/AdaptiveRadixTree2"
	"github.com/google/btree"
	plart "github.com/plar/go-adaptive-radix-tree"
	tbtree "github.com/tidwall/btree"
)

// stringyN is the per-shape working set for the stringy-key benchmarks.
// Held to 1M (one order of magnitude smaller than the int-key benchN of
// 10M) because string keys are 4-10x larger per entry; a 10M working
// set across 4 implementations would push RSS past 4 GB.
const stringyN = 1_000_000

// stringyRangeN is the 1 % window size used by every Range bench.
const stringyRangeN = stringyN / 100

// stringySeed and stringyMissSeed are the deterministic PRNG seeds for
// hit and miss key generation. Same seed across all three shapes so the
// per-shape generators are independently reproducible.
const (
	stringySeed     = int64(0xBE3)
	stringyMissSeed = int64(0xBE3 ^ 0xDEADBEEF)
)

// skv is the b-tree value type for the stringy benches. It holds the
// string key directly (vs bench_test.go's []byte-keyed kv) so the
// comparator can use strings.Compare.
//
// strings.Compare avoids double-comparing in the < / > pattern that
// Less-then-Greater-fallback callers do.
type skv struct {
	k string
	v int
}

func skvLess(a, b skv) bool { return strings.Compare(a.k, b.k) < 0 }

// stringyTidwallOpts mirrors bench_test.go's tidwallOpts: degree 32 to
// match google/btree, and NoLocks: true so tidwall's internal RWMutex
// stays out of the measurement.
var stringyTidwallOpts = tbtree.Options{NoLocks: true, Degree: 32}

// stringyShape bundles the per-shape hit + miss key sets, the
// 1 %-window range bounds bracketing the median sorted key, and lazy
// caches for the four pre-built trees that Get / GetMiss / Range reuse.
type stringyShape struct {
	name     string
	keys     []string
	missKeys []string
	rangeLo  string
	rangeHi  string

	artOnce sync.Once
	art     *art.Tree[int]
	btOnce  sync.Once
	bt      *btree.BTreeG[skv]
	twOnce  sync.Once
	tw      *tbtree.BTreeG[skv]
	plOnce  sync.Once
	pl      plart.Tree
}

var (
	stringyOnce  sync.Once
	stringyURL   *stringyShape
	stringyUUID  *stringyShape
	stringyShort *stringyShape
)

func ensureStringyShapes() {
	stringyOnce.Do(func() {
		stringyURL = buildStringyShape("URL", urlGen)
		stringyUUID = buildStringyShape("UUID", uuidGen)
		stringyShort = buildStringyShape("Short", shortGen)
	})
}

// buildStringyShape generates stringyN hit keys, stringyN miss keys
// (different seed; hits and misses are statistically disjoint at these
// alphabet sizes), and the [rangeLo, rangeHi) window that bounds the
// 10K keys at the median of the sorted hit-key set.
func buildStringyShape(name string, gen func(r *rand.Rand) string) *stringyShape {
	rHit := rand.New(rand.NewSource(stringySeed))
	keys := make([]string, stringyN)
	for i := range keys {
		keys[i] = gen(rHit)
	}
	rMiss := rand.New(rand.NewSource(stringyMissSeed))
	miss := make([]string, stringyN)
	for i := range miss {
		miss[i] = gen(rMiss)
	}
	sorted := make([]string, len(keys))
	copy(sorted, keys)
	sort.Strings(sorted)
	return &stringyShape{
		name:     name,
		keys:     keys,
		missKeys: miss,
		rangeLo:  sorted[stringyN/2],
		rangeHi:  sorted[stringyN/2+stringyRangeN],
	}
}

// urlGen produces a ~46-byte key with a fixed 24-byte shared prefix
// and 16 bytes of variation (two random uint32s as %08x hex).
func urlGen(r *rand.Rand) string {
	return fmt.Sprintf("https://example.com/u/%08x/%08x", r.Uint32(), r.Uint32())
}

// uuidGen produces a 36-char hex-with-dashes string. The version /
// variant nibbles are NOT set: we want maximum entropy across all 36
// chars (no shared prefix), not RFC-4122 conformance.
func uuidGen(r *rand.Rand) string {
	var b [16]byte
	_, _ = r.Read(b[:])
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

// stringyAlphabet is the 62-char alphanumeric alphabet for shortGen.
const stringyAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// shortGen produces an 8-byte alphanumeric string (no prefix
// structure; closest stringy analog to the int-key baseline shape).
func shortGen(r *rand.Rand) string {
	var b [8]byte
	for i := range b {
		b[i] = stringyAlphabet[r.Intn(len(stringyAlphabet))]
	}
	return string(b[:])
}

// --- per-shape tree builders (lazy, populated once per process) ---

func (s *stringyShape) getART() *art.Tree[int] {
	s.artOnce.Do(func() {
		t := art.New[int]()
		for i, k := range s.keys {
			t.Put([]byte(k), i)
		}
		s.art = t
	})
	return s.art
}

func (s *stringyShape) getBT() *btree.BTreeG[skv] {
	s.btOnce.Do(func() {
		t := btree.NewG[skv](32, skvLess)
		for i, k := range s.keys {
			t.ReplaceOrInsert(skv{k: k, v: i})
		}
		s.bt = t
	})
	return s.bt
}

func (s *stringyShape) getTidwall() *tbtree.BTreeG[skv] {
	s.twOnce.Do(func() {
		t := tbtree.NewBTreeGOptions(skvLess, stringyTidwallOpts)
		for i, k := range s.keys {
			t.Set(skv{k: k, v: i})
		}
		s.tw = t
	})
	return s.tw
}

func (s *stringyShape) getPlar() plart.Tree {
	s.plOnce.Do(func() {
		t := plart.New()
		for i, k := range s.keys {
			t.Insert(plart.Key([]byte(k)), plart.Value(i))
		}
		s.pl = t
	})
	return s.pl
}

// stringySinkAcc is a package-level sink for Range value yields so the
// compiler can't optimise the scan body away.
var stringySinkAcc int

// --- Put helpers (fresh tree per outer iteration) ---

func putART(b *testing.B, s *stringyShape) {
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		t := art.New[int]()
		for i, k := range s.keys {
			t.Put([]byte(k), i)
		}
	}
	perKey(b, stringyN)
}

func putBTree(b *testing.B, s *stringyShape) {
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		t := btree.NewG[skv](32, skvLess)
		for i, k := range s.keys {
			t.ReplaceOrInsert(skv{k: k, v: i})
		}
	}
	perKey(b, stringyN)
}

func putTidwall(b *testing.B, s *stringyShape) {
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		t := tbtree.NewBTreeGOptions(skvLess, stringyTidwallOpts)
		for i, k := range s.keys {
			t.Set(skv{k: k, v: i})
		}
	}
	perKey(b, stringyN)
}

// putPlar pays plar's interface{} boxing tax once per Put: each int
// value escapes to the heap because plart.Value is interface{}.
func putPlar(b *testing.B, s *stringyShape) {
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		t := plart.New()
		for i, k := range s.keys {
			t.Insert(plart.Key([]byte(k)), plart.Value(i))
		}
	}
	perKey(b, stringyN)
}

// --- Get helpers (pre-populated tree; one lookup per b.N iteration) ---

func getART(b *testing.B, s *stringyShape) {
	t := s.getART()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get([]byte(s.keys[i%stringyN]))
	}
}

func getBTree(b *testing.B, s *stringyShape) {
	t := s.getBT()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(skv{k: s.keys[i%stringyN]})
	}
}

func getTidwall(b *testing.B, s *stringyShape) {
	t := s.getTidwall()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(skv{k: s.keys[i%stringyN]})
	}
}

func getPlar(b *testing.B, s *stringyShape) {
	t := s.getPlar()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Search(plart.Key([]byte(s.keys[i%stringyN])))
	}
}

// --- GetMiss helpers (pre-populated tree; lookup of regenerated misses) ---

func getMissART(b *testing.B, s *stringyShape) {
	t := s.getART()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get([]byte(s.missKeys[i%stringyN]))
	}
}

func getMissBTree(b *testing.B, s *stringyShape) {
	t := s.getBT()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(skv{k: s.missKeys[i%stringyN]})
	}
}

func getMissTidwall(b *testing.B, s *stringyShape) {
	t := s.getTidwall()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Get(skv{k: s.missKeys[i%stringyN]})
	}
}

func getMissPlar(b *testing.B, s *stringyShape) {
	t := s.getPlar()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = t.Search(plart.Key([]byte(s.missKeys[i%stringyN])))
	}
}

// --- Delete helpers (setup excluded via StopTimer/StartTimer) ---

func deleteART(b *testing.B, s *stringyShape) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := art.New[int]()
		for i, k := range s.keys {
			t.Put([]byte(k), i)
		}
		b.StartTimer()
		for _, k := range s.keys {
			t.Delete([]byte(k))
		}
	}
	perKey(b, stringyN)
}

func deleteBTree(b *testing.B, s *stringyShape) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := btree.NewG[skv](32, skvLess)
		for i, k := range s.keys {
			t.ReplaceOrInsert(skv{k: k, v: i})
		}
		b.StartTimer()
		for _, k := range s.keys {
			t.Delete(skv{k: k})
		}
	}
	perKey(b, stringyN)
}

func deleteTidwall(b *testing.B, s *stringyShape) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := tbtree.NewBTreeGOptions(skvLess, stringyTidwallOpts)
		for i, k := range s.keys {
			t.Set(skv{k: k, v: i})
		}
		b.StartTimer()
		for _, k := range s.keys {
			t.Delete(skv{k: k})
		}
	}
	perKey(b, stringyN)
}

func deletePlar(b *testing.B, s *stringyShape) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		t := plart.New()
		for i, k := range s.keys {
			t.Insert(plart.Key([]byte(k)), plart.Value(i))
		}
		b.StartTimer()
		for _, k := range s.keys {
			t.Delete(plart.Key([]byte(k)))
		}
	}
	perKey(b, stringyN)
}

// --- Range helpers (yield 1 % window of sorted keyspace) ---

func rangeART(b *testing.B, s *stringyShape) {
	t := s.getART()
	lo, hi := []byte(s.rangeLo), []byte(s.rangeHi)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		for _, v := range t.Range(lo, hi) {
			stringySinkAcc += v
			count++
		}
		if count != stringyRangeN {
			b.Fatalf("ranged %d, want %d", count, stringyRangeN)
		}
	}
	perKey(b, stringyRangeN)
}

func rangeBTree(b *testing.B, s *stringyShape) {
	t := s.getBT()
	lo, hi := s.rangeLo, s.rangeHi
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		t.AscendGreaterOrEqual(skv{k: lo}, func(it skv) bool {
			if it.k >= hi {
				return false
			}
			stringySinkAcc += it.v
			count++
			return true
		})
		if count != stringyRangeN {
			b.Fatalf("ranged %d, want %d", count, stringyRangeN)
		}
	}
	perKey(b, stringyRangeN)
}

func rangeTidwall(b *testing.B, s *stringyShape) {
	t := s.getTidwall()
	lo, hi := s.rangeLo, s.rangeHi
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0
		t.Ascend(skv{k: lo}, func(it skv) bool {
			if it.k >= hi {
				return false
			}
			stringySinkAcc += it.v
			count++
			return true
		})
		if count != stringyRangeN {
			b.Fatalf("ranged %d, want %d", count, stringyRangeN)
		}
	}
	perKey(b, stringyRangeN)
}

// rangePlar walks every leaf from the start of the tree, skipping until
// the key is >= rangeLo and stopping when the key is >= rangeHi. plar
// v1.0.7 exposes no seek primitive, so this is the cheapest correct
// form on the public API; the resulting numbers are harness-bound, not
// tree-shape — same caveat documented in benchmarks.md and in
// bench_test.go's BenchmarkRange_Plar.
func rangePlar(b *testing.B, s *stringyShape) {
	t := s.getPlar()
	loBytes, hiBytes := []byte(s.rangeLo), []byte(s.rangeHi)
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
			if bytes.Compare(k, loBytes) < 0 {
				continue
			}
			if bytes.Compare(k, hiBytes) >= 0 {
				break
			}
			stringySinkAcc += node.Value().(int)
			count++
		}
		if count != stringyRangeN {
			b.Fatalf("ranged %d, want %d", count, stringyRangeN)
		}
	}
	perKey(b, stringyRangeN)
}

// --- 60 top-level Benchmark wrappers (3 shapes x 5 ops x 4 impls) ---

func BenchmarkPut_URL_ART(b *testing.B)     { ensureStringyShapes(); putART(b, stringyURL) }
func BenchmarkPut_URL_BTree(b *testing.B)   { ensureStringyShapes(); putBTree(b, stringyURL) }
func BenchmarkPut_URL_Tidwall(b *testing.B) { ensureStringyShapes(); putTidwall(b, stringyURL) }
func BenchmarkPut_URL_Plar(b *testing.B)    { ensureStringyShapes(); putPlar(b, stringyURL) }

func BenchmarkGet_URL_ART(b *testing.B)     { ensureStringyShapes(); getART(b, stringyURL) }
func BenchmarkGet_URL_BTree(b *testing.B)   { ensureStringyShapes(); getBTree(b, stringyURL) }
func BenchmarkGet_URL_Tidwall(b *testing.B) { ensureStringyShapes(); getTidwall(b, stringyURL) }
func BenchmarkGet_URL_Plar(b *testing.B)    { ensureStringyShapes(); getPlar(b, stringyURL) }

func BenchmarkGetMiss_URL_ART(b *testing.B)     { ensureStringyShapes(); getMissART(b, stringyURL) }
func BenchmarkGetMiss_URL_BTree(b *testing.B)   { ensureStringyShapes(); getMissBTree(b, stringyURL) }
func BenchmarkGetMiss_URL_Tidwall(b *testing.B) { ensureStringyShapes(); getMissTidwall(b, stringyURL) }
func BenchmarkGetMiss_URL_Plar(b *testing.B)    { ensureStringyShapes(); getMissPlar(b, stringyURL) }

func BenchmarkDelete_URL_ART(b *testing.B)     { ensureStringyShapes(); deleteART(b, stringyURL) }
func BenchmarkDelete_URL_BTree(b *testing.B)   { ensureStringyShapes(); deleteBTree(b, stringyURL) }
func BenchmarkDelete_URL_Tidwall(b *testing.B) { ensureStringyShapes(); deleteTidwall(b, stringyURL) }
func BenchmarkDelete_URL_Plar(b *testing.B)    { ensureStringyShapes(); deletePlar(b, stringyURL) }

func BenchmarkRange_URL_ART(b *testing.B)     { ensureStringyShapes(); rangeART(b, stringyURL) }
func BenchmarkRange_URL_BTree(b *testing.B)   { ensureStringyShapes(); rangeBTree(b, stringyURL) }
func BenchmarkRange_URL_Tidwall(b *testing.B) { ensureStringyShapes(); rangeTidwall(b, stringyURL) }
func BenchmarkRange_URL_Plar(b *testing.B)    { ensureStringyShapes(); rangePlar(b, stringyURL) }

func BenchmarkPut_UUID_ART(b *testing.B)     { ensureStringyShapes(); putART(b, stringyUUID) }
func BenchmarkPut_UUID_BTree(b *testing.B)   { ensureStringyShapes(); putBTree(b, stringyUUID) }
func BenchmarkPut_UUID_Tidwall(b *testing.B) { ensureStringyShapes(); putTidwall(b, stringyUUID) }
func BenchmarkPut_UUID_Plar(b *testing.B)    { ensureStringyShapes(); putPlar(b, stringyUUID) }

func BenchmarkGet_UUID_ART(b *testing.B)     { ensureStringyShapes(); getART(b, stringyUUID) }
func BenchmarkGet_UUID_BTree(b *testing.B)   { ensureStringyShapes(); getBTree(b, stringyUUID) }
func BenchmarkGet_UUID_Tidwall(b *testing.B) { ensureStringyShapes(); getTidwall(b, stringyUUID) }
func BenchmarkGet_UUID_Plar(b *testing.B)    { ensureStringyShapes(); getPlar(b, stringyUUID) }

func BenchmarkGetMiss_UUID_ART(b *testing.B)   { ensureStringyShapes(); getMissART(b, stringyUUID) }
func BenchmarkGetMiss_UUID_BTree(b *testing.B) { ensureStringyShapes(); getMissBTree(b, stringyUUID) }
func BenchmarkGetMiss_UUID_Tidwall(b *testing.B) {
	ensureStringyShapes()
	getMissTidwall(b, stringyUUID)
}
func BenchmarkGetMiss_UUID_Plar(b *testing.B) { ensureStringyShapes(); getMissPlar(b, stringyUUID) }

func BenchmarkDelete_UUID_ART(b *testing.B)     { ensureStringyShapes(); deleteART(b, stringyUUID) }
func BenchmarkDelete_UUID_BTree(b *testing.B)   { ensureStringyShapes(); deleteBTree(b, stringyUUID) }
func BenchmarkDelete_UUID_Tidwall(b *testing.B) { ensureStringyShapes(); deleteTidwall(b, stringyUUID) }
func BenchmarkDelete_UUID_Plar(b *testing.B)    { ensureStringyShapes(); deletePlar(b, stringyUUID) }

func BenchmarkRange_UUID_ART(b *testing.B)     { ensureStringyShapes(); rangeART(b, stringyUUID) }
func BenchmarkRange_UUID_BTree(b *testing.B)   { ensureStringyShapes(); rangeBTree(b, stringyUUID) }
func BenchmarkRange_UUID_Tidwall(b *testing.B) { ensureStringyShapes(); rangeTidwall(b, stringyUUID) }
func BenchmarkRange_UUID_Plar(b *testing.B)    { ensureStringyShapes(); rangePlar(b, stringyUUID) }

func BenchmarkPut_Short_ART(b *testing.B)     { ensureStringyShapes(); putART(b, stringyShort) }
func BenchmarkPut_Short_BTree(b *testing.B)   { ensureStringyShapes(); putBTree(b, stringyShort) }
func BenchmarkPut_Short_Tidwall(b *testing.B) { ensureStringyShapes(); putTidwall(b, stringyShort) }
func BenchmarkPut_Short_Plar(b *testing.B)    { ensureStringyShapes(); putPlar(b, stringyShort) }

func BenchmarkGet_Short_ART(b *testing.B)     { ensureStringyShapes(); getART(b, stringyShort) }
func BenchmarkGet_Short_BTree(b *testing.B)   { ensureStringyShapes(); getBTree(b, stringyShort) }
func BenchmarkGet_Short_Tidwall(b *testing.B) { ensureStringyShapes(); getTidwall(b, stringyShort) }
func BenchmarkGet_Short_Plar(b *testing.B)    { ensureStringyShapes(); getPlar(b, stringyShort) }

func BenchmarkGetMiss_Short_ART(b *testing.B)   { ensureStringyShapes(); getMissART(b, stringyShort) }
func BenchmarkGetMiss_Short_BTree(b *testing.B) { ensureStringyShapes(); getMissBTree(b, stringyShort) }
func BenchmarkGetMiss_Short_Tidwall(b *testing.B) {
	ensureStringyShapes()
	getMissTidwall(b, stringyShort)
}
func BenchmarkGetMiss_Short_Plar(b *testing.B) { ensureStringyShapes(); getMissPlar(b, stringyShort) }

func BenchmarkDelete_Short_ART(b *testing.B)   { ensureStringyShapes(); deleteART(b, stringyShort) }
func BenchmarkDelete_Short_BTree(b *testing.B) { ensureStringyShapes(); deleteBTree(b, stringyShort) }
func BenchmarkDelete_Short_Tidwall(b *testing.B) {
	ensureStringyShapes()
	deleteTidwall(b, stringyShort)
}
func BenchmarkDelete_Short_Plar(b *testing.B) { ensureStringyShapes(); deletePlar(b, stringyShort) }

func BenchmarkRange_Short_ART(b *testing.B)     { ensureStringyShapes(); rangeART(b, stringyShort) }
func BenchmarkRange_Short_BTree(b *testing.B)   { ensureStringyShapes(); rangeBTree(b, stringyShort) }
func BenchmarkRange_Short_Tidwall(b *testing.B) { ensureStringyShapes(); rangeTidwall(b, stringyShort) }
func BenchmarkRange_Short_Plar(b *testing.B)    { ensureStringyShapes(); rangePlar(b, stringyShort) }
