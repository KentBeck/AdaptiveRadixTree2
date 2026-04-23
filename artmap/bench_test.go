package artmap_test

import (
	"encoding/binary"
	"math/rand/v2"
	"testing"

	art "github.com/KentBeck/AdaptiveRadixTree2"
	"github.com/KentBeck/AdaptiveRadixTree2/artmap"
)

const benchN = 10_000

func encodeInt64BE(buf []byte, v int64) {
	binary.BigEndian.PutUint64(buf, uint64(v)^0x8000000000000000)
}

func BenchmarkOrderedPut_int64(b *testing.B) {
	keys := make([]int64, benchN)
	r := rand.New(rand.NewPCG(1, 2))
	for i := range keys {
		keys[i] = int64(r.Uint64())
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := artmap.New[int64, int]()
		for _, k := range keys {
			m.Put(k, 0)
		}
	}
}

func BenchmarkTreePut_int64(b *testing.B) {
	keys := make([]int64, benchN)
	r := rand.New(rand.NewPCG(1, 2))
	for i := range keys {
		keys[i] = int64(r.Uint64())
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t := art.New[int]()
		var buf [8]byte
		for _, k := range keys {
			encodeInt64BE(buf[:], k)
			t.Put(buf[:], 0)
		}
	}
}

func BenchmarkOrderedGet_int64(b *testing.B) {
	keys := make([]int64, benchN)
	r := rand.New(rand.NewPCG(3, 4))
	for i := range keys {
		keys[i] = int64(r.Uint64())
	}
	m := artmap.New[int64, int]()
	for _, k := range keys {
		m.Put(k, 1)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Get(keys[i%benchN])
	}
}

func BenchmarkTreeGet_int64(b *testing.B) {
	keys := make([]int64, benchN)
	r := rand.New(rand.NewPCG(3, 4))
	for i := range keys {
		keys[i] = int64(r.Uint64())
	}
	t := art.New[int]()
	var buf [8]byte
	for _, k := range keys {
		encodeInt64BE(buf[:], k)
		t.Put(buf[:], 1)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encodeInt64BE(buf[:], keys[i%benchN])
		_, _ = t.Get(buf[:])
	}
}

func BenchmarkOrderedRange_int64(b *testing.B) {
	keys := make([]int64, benchN)
	r := rand.New(rand.NewPCG(5, 6))
	for i := range keys {
		keys[i] = int64(r.Uint64())
	}
	m := artmap.New[int64, int]()
	for _, k := range keys {
		m.Put(k, 1)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum int
		for _, v := range m.Range(-1<<62, 1<<62) {
			sum += v
		}
		_ = sum
	}
}

func BenchmarkTreeRange_int64(b *testing.B) {
	keys := make([]int64, benchN)
	r := rand.New(rand.NewPCG(5, 6))
	for i := range keys {
		keys[i] = int64(r.Uint64())
	}
	t := art.New[int]()
	var buf [8]byte
	for _, k := range keys {
		encodeInt64BE(buf[:], k)
		t.Put(buf[:], 1)
	}

	var startBuf, endBuf [8]byte
	encodeInt64BE(startBuf[:], -1<<62)
	encodeInt64BE(endBuf[:], 1<<62)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum int
		for _, v := range t.Range(startBuf[:], endBuf[:]) {
			sum += v
		}
		_ = sum
	}
}

