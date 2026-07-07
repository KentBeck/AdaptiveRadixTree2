package harness

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"runtime"
	"strings"
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
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

// heapAlloc returns HeapAlloc after two GC passes, so transient
// garbage doesn't count against the budget.
func heapAlloc() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// MeasureCapacity inserts keys from gen() in batches, taking a heap
// snapshot after each batch, and returns when the heap has grown by
// budget. Batches start at 1000 keys and then aim for the remaining
// budget using the bytes/key measured so far (growing at most 4× per
// batch), so a 100 MB probe needs only a handful of GC pauses whether
// the map fits 4 000 keys or 1.6 million.
//
// gen is called repeatedly to produce keys; it returns a key slice
// (which the SortedMap MUST copy, by contract) and a value.
func MeasureCapacity(factory Factory, workload string, gen func(i int) (key []byte, value int), budget uint64) CapacityResult {
	m := factory()
	base := heapAlloc()
	var used, totalLen uint64
	for i := 0; used < budget; {
		batch := 1000
		if used > 0 {
			batch = int(float64(budget-used) / float64(used) * float64(i))
			if batch > 4*i {
				batch = 4 * i
			}
			if batch < 1000 {
				batch = 1000
			}
		}
		for j := 0; j < batch; j++ {
			k, v := gen(i)
			m.Put(k, v)
			totalLen += uint64(len(k))
			i++
		}
		if h := heapAlloc(); h > base {
			used = h - base
		}
	}
	keysFit := m.Len()
	runtime.KeepAlive(m)
	return CapacityResult{
		Workload:    workload,
		BudgetBytes: budget,
		KeysFit:     keysFit,
		HeapBytes:   used,
		AvgKeyLen:   float64(totalLen) / float64(keysFit),
		BytesPerKey: float64(used) / float64(keysFit),
	}
}

// HeapDelta reports the post-GC heap growth from building w in a
// fresh map — the real cost of the structure, malloc rounding and
// allocator bookkeeping included.
func HeapDelta(f Factory, w bench.Workload) uint64 {
	base := heapAlloc()
	m := f()
	for i, k := range w.Keys {
		m.Put(k, w.Vals[i])
	}
	h := heapAlloc()
	runtime.KeepAlive(m)
	if h < base {
		return 0
	}
	return h - base
}

// CapacityBudget is the tutorial's standard capacity question: how
// many keys fit before the heap grows by 100 MB?
const CapacityBudget = uint64(100) << 20

var capacityFlag = flag.Bool("capacity", false, "run the 100MB capacity probes")

// capacityWorkloads pairs each standard workload with its streaming
// generator constructor. Constructors, not generators: the RNG-backed
// gens are stateful, so every measurement needs a fresh one.
var capacityWorkloads = []struct {
	name string
	gen  func() func(int) ([]byte, int)
}{
	{"Dense", DenseGen},
	{"Sparse", SparseGen},
	{"URL", URLGen},
}

// RunCapacityProbe measures keys-fit at the 100 MB budget for every
// contender × workload and logs a table. Gated behind -capacity —
// the probe takes seconds per cell:
//
//	go test ./<chapter>/ -run TestCapacity -capacity -timeout 10m -v
func RunCapacityProbe(t *testing.T, contenders ...Contender) {
	if !*capacityFlag {
		t.Skip("set -capacity to run")
	}
	t.Logf("%-9s %-10s %12s %14s %10s %10s",
		"Workload", "contender", "keys-fit", "heap-bytes", "avg-key", "B/key")
	for _, cw := range capacityWorkloads {
		for _, c := range contenders {
			res := MeasureCapacity(c.New, cw.name, cw.gen(), CapacityBudget)
			t.Logf("%-9s %-10s %12s %14s %10.2f %10.1f",
				res.Workload, c.Name, group(res.KeysFit), group(int(res.HeapBytes)), res.AvgKeyLen, res.BytesPerKey)
		}
	}
}

// CapacityTable measures keys-fit at the 100 MB budget for every
// contender × workload and renders the chapter's capacity table.
// Slow — wire it as a Volatile buildcheck region.
func CapacityTable(contenders []Contender) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-9s", "Workload")
	for _, c := range contenders {
		fmt.Fprintf(&b, "  %14s %9s", c.Name+" keys", "B/key")
	}
	b.WriteByte('\n')
	for _, cw := range capacityWorkloads {
		fmt.Fprintf(&b, "%-9s", cw.name)
		for _, c := range contenders {
			res := MeasureCapacity(c.New, cw.name, cw.gen(), CapacityBudget)
			fmt.Fprintf(&b, "  %14s %9s", group(res.KeysFit), fmtPerKey(res.BytesPerKey))
		}
		b.WriteByte('\n')
	}
	return "```\n" + b.String() + "```"
}

func fmtPerKey(x float64) string {
	if x < 1000 {
		return fmt.Sprintf("%.1f", x)
	}
	return group(int(x + 0.5))
}

// TODO: streaming generators if a future implementation outlasts `bench.X(N)`'s pre-allocated key set.

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
