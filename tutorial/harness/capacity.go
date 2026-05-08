package harness

import (
	"fmt"
	"math/rand/v2"
	"runtime"
)

// CapacityResult reports how many keys fit before HeapAlloc post-GC
// exceeds the budget.
type CapacityResult struct {
	Workload    string
	BudgetBytes uint64
	KeysFit     int
	HeapBytes   uint64
	AvgKeyLen   float64
	BytesPerKey float64
}

// MeasureCapacity inserts keys from gen() in batches, taking a heap
// snapshot every batchSize keys (after two runtime.GC calls), and
// returns when HeapAlloc - baseline >= budget. The factory is called
// once; the same SortedMap accumulates throughout.
//
// gen is called repeatedly to produce keys; it returns a key slice
// (which the SortedMap MUST copy, by contract) and a value.
func MeasureCapacity(factory Factory, workload string, gen func(i int) (key []byte, value int), budget uint64, batchSize int) CapacityResult {
	if batchSize <= 0 {
		batchSize = 1000
	}
	m := factory()

	runtime.GC()
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)

	var (
		keysFit  int
		heapNow  uint64
		totalLen uint64
	)

	i := 0
	for {
		for j := 0; j < batchSize; j++ {
			k, v := gen(i)
			m.Put(k, v)
			totalLen += uint64(len(k))
			i++
		}
		runtime.GC()
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		heapNow = ms.HeapAlloc
		used := uint64(0)
		if heapNow > base.HeapAlloc {
			used = heapNow - base.HeapAlloc
		}
		keysFit = m.Len()
		if used >= budget {
			break
		}
	}

	used := uint64(0)
	if heapNow > base.HeapAlloc {
		used = heapNow - base.HeapAlloc
	}
	avg := 0.0
	bpk := 0.0
	if keysFit > 0 {
		avg = float64(totalLen) / float64(keysFit)
		bpk = float64(used) / float64(keysFit)
	}
	runtime.KeepAlive(m)
	return CapacityResult{
		Workload:    workload,
		BudgetBytes: budget,
		KeysFit:     keysFit,
		HeapBytes:   used,
		AvgKeyLen:   avg,
		BytesPerKey: bpk,
	}
}

// DenseGen returns a streaming generator for the Dense workload --
// 8-byte big-endian counter keys, value = i. Stateless and matches
// bench.Dense's deterministic shape.
func DenseGen() func(i int) ([]byte, int) {
	return func(i int) ([]byte, int) {
		k := make([]byte, 8)
		k[0] = byte(uint64(i) >> 56)
		k[1] = byte(uint64(i) >> 48)
		k[2] = byte(uint64(i) >> 40)
		k[3] = byte(uint64(i) >> 32)
		k[4] = byte(uint64(i) >> 24)
		k[5] = byte(uint64(i) >> 16)
		k[6] = byte(uint64(i) >> 8)
		k[7] = byte(uint64(i))
		return k, i
	}
}

// SparseGen returns a streaming generator that produces 16-byte
// pseudorandom keys. Seeds match bench.Sparse so capacity numbers
// agree with the rest of the tutorial's benchmark plumbing.
func SparseGen() func(i int) ([]byte, int) {
	r := rand.New(rand.NewPCG(0xa11ce, 0xb0b))
	return func(i int) ([]byte, int) {
		k := make([]byte, 16)
		for j := range k {
			k[j] = byte(r.UintN(256))
		}
		return k, i
	}
}

// URLGen returns a streaming generator that produces URL-shaped keys
// with the same hosts, paths, and PCG seeds as bench.URL.
func URLGen() func(i int) ([]byte, int) {
	hosts := []string{
		"https://api.example.com/",
		"https://www.example.com/",
		"https://docs.example.com/",
		"https://example.org/",
		"https://example.net/",
		"https://cdn.example.io/",
	}
	paths := []string{
		"v1/users/", "v1/orders/", "v2/users/", "v2/orders/",
		"static/img/", "static/css/", "static/js/",
		"blog/posts/", "blog/tags/", "search?q=",
	}
	r := rand.New(rand.NewPCG(0xc0ffee, 0xdeadbeef))
	return func(i int) ([]byte, int) {
		host := hosts[r.UintN(uint(len(hosts)))]
		path := paths[r.UintN(uint(len(paths)))]
		tail := fmt.Sprintf("%08x", r.Uint32())
		return []byte(host + path + tail), i
	}
}
