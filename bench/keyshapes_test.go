package bench

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"

	art "github.com/KentBeck/AdaptiveRadixTree2"
)

// shapeN is the per-shape working set for key-shape sensitivity
// benchmarks. Kept an order of magnitude smaller than benchN so the
// suite finishes quickly at -benchtime=1s -count=1.
const shapeN = 100_000

// keyShape bundles a named key distribution with its precomputed byte
// keys and a half-open [lo, hi) slice that covers ~1 % of the sorted
// keyspace for the Range bench.
type keyShape struct {
	name          string
	keys          [][]byte
	rangeLo       []byte
	rangeHi       []byte
	rangeExpected int
}

var (
	shapesOnce sync.Once
	shapes     []keyShape
)

func buildShapes() {
	shapesOnce.Do(func() {
		shapes = []keyShape{
			newShape("seqInt64", seqInt64Keys()),
			newShape("randInt64", randInt64Keys()),
			newShape("uuid", uuidKeys()),
			newShape("urlPath", urlPathKeys()),
		}
	})
}

// newShape wraps keys with a derived 1 % range window picked from the
// sorted-key order so the window size is identical across shapes.
func newShape(name string, keys [][]byte) keyShape {
	sorted := make([][]byte, len(keys))
	copy(sorted, keys)
	sort.Slice(sorted, func(i, j int) bool { return bytes.Compare(sorted[i], sorted[j]) < 0 })
	loIdx := len(sorted) / 2
	hiIdx := loIdx + len(sorted)/100
	if hiIdx > len(sorted) {
		hiIdx = len(sorted)
	}
	return keyShape{
		name:          name,
		keys:          keys,
		rangeLo:       sorted[loIdx],
		rangeHi:       sorted[hiIdx],
		rangeExpected: hiIdx - loIdx,
	}
}

func seqInt64Keys() [][]byte {
	out := make([][]byte, shapeN)
	for i := 0; i < shapeN; i++ {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(i))
		out[i] = b
	}
	return out
}

func randInt64Keys() [][]byte {
	r := rand.New(rand.NewSource(42))
	perm := r.Perm(shapeN)
	out := make([][]byte, shapeN)
	for i, v := range perm {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(v))
		out[i] = b
	}
	return out
}

func uuidKeys() [][]byte {
	r := rand.New(rand.NewSource(7))
	out := make([][]byte, shapeN)
	for i := range out {
		b := make([]byte, 16)
		_, _ = r.Read(b)
		out[i] = b
	}
	return out
}

// urlPathKeys builds deep "/a/b/c/…" hierarchy keys that share long
// prefixes across siblings, so ART's path compression has something to
// bite on.
func urlPathKeys() [][]byte {
	out := make([][]byte, shapeN)
	for i := 0; i < shapeN; i++ {
		s := fmt.Sprintf("/api/v1/org/%03d/user/%03d/session/%06d", i%1000, (i/1000)%1000, i)
		out[i] = []byte(s)
	}
	return out
}

// buildShapeTree inserts every key from s into a fresh ART in the
// original (unsorted) order. Returned tree is reusable across a Get /
// Range benchmark because both are read-only.
func buildShapeTree(s keyShape) *art.Tree[int] {
	t := art.New[int]()
	for i, k := range s.keys {
		t.Put(k, i)
	}
	return t
}

// --- Put: fresh tree per iteration, per shape ---

func BenchmarkKeyShape_Put_ART(b *testing.B) {
	buildShapes()
	for _, s := range shapes {
		s := s
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				t := art.New[int]()
				for i, k := range s.keys {
					t.Put(k, i)
				}
			}
			perKey(b, shapeN)
		})
	}
}

// --- Get (hit): lookup every key once per b.N on a pre-built tree ---

func BenchmarkKeyShape_Get_ART(b *testing.B) {
	buildShapes()
	for _, s := range shapes {
		s := s
		t := buildShapeTree(s)
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = t.Get(s.keys[i%shapeN])
			}
		})
	}
}

// --- Delete: setup excluded via StopTimer/StartTimer ---

func BenchmarkKeyShape_Delete_ART(b *testing.B) {
	buildShapes()
	for _, s := range shapes {
		s := s
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				t := art.New[int]()
				for i, k := range s.keys {
					t.Put(k, i)
				}
				b.StartTimer()
				for _, k := range s.keys {
					t.Delete(k)
				}
			}
			perKey(b, shapeN)
		})
	}
}

// --- Range: yield ~1 % of the sorted keyspace ---

func BenchmarkKeyShape_Range_ART(b *testing.B) {
	buildShapes()
	for _, s := range shapes {
		s := s
		t := buildShapeTree(s)
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				count := 0
				for range t.Range(s.rangeLo, s.rangeHi) {
					count++
				}
				if count != s.rangeExpected {
					b.Fatalf("ranged %d, want %d", count, s.rangeExpected)
				}
			}
			perKey(b, s.rangeExpected)
		})
	}
}
