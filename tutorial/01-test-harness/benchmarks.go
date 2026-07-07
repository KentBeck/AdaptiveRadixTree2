package harness

import (
	"fmt"
	"strings"
	"testing"

	"github.com/KentBeck/AdaptiveRadixTree2/tutorial/bench"
)

// Contender pairs a display name with a Factory for the shared
// time/space/capacity comparisons. By convention a chapter lists its
// own tree first, its predecessor next, and google/btree last.
type Contender struct {
	Name string
	New  Factory
}

// BTreeContender is the reference entry every chapter includes.
func BTreeContender() Contender { return Contender{Name: "btree", New: BTreeFactory()} }

// Workloads1k returns the three standard 1000-key workloads every
// chapter benchmarks against.
func Workloads1k() []bench.Workload {
	return []bench.Workload{bench.Dense(1000), bench.Sparse(1000), bench.URL(1000)}
}

func shortName(w bench.Workload) string {
	name, _, _ := strings.Cut(w.Name, "/")
	return name
}

func fill(m SortedMap, w bench.Workload) SortedMap {
	for i, k := range w.Keys {
		m.Put(k, w.Vals[i])
	}
	return m
}

// --- the four benchmarked operations ----------------------------

type opSpec struct {
	name  string
	bench func(Factory, bench.Workload) func(*testing.B)
}

func opSpecs() []opSpec {
	return []opSpec{
		{"Put", benchPut},
		{"Get", benchGet},
		{"Range", benchRange},
		{"RangeWindow", benchRangeWindow},
	}
}

// benchPut builds a fresh map per iteration and inserts the whole
// workload, so ns/op, B/op, and allocs/op describe a full
// 1000-key build.
func benchPut(f Factory, w bench.Workload) func(*testing.B) {
	return func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			fill(f(), w)
		}
	}
}

// sinkHole defeats dead-code elimination in the read benchmarks.
var sinkHole int

func benchGet(f Factory, w bench.Workload) func(*testing.B) {
	return func(b *testing.B) {
		m := fill(f(), w)
		b.ReportAllocs()
		b.ResetTimer()
		var sink int
		for i := 0; i < b.N; i++ {
			v, _ := m.Get(w.Keys[i%len(w.Keys)])
			sink ^= v
		}
		sinkHole = sink
	}
}

func benchRange(f Factory, w bench.Workload) func(*testing.B) {
	return func(b *testing.B) {
		m := fill(f(), w)
		b.ReportAllocs()
		b.ResetTimer()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range m.Range(nil, nil) {
				sink ^= v
			}
		}
		sinkHole = sink
	}
}

// benchRangeWindow iterates the middle 1% of the sorted keyspace —
// the 49.5th to 50.5th percentile — so implementations that can
// skip subtrees are distinguishable from ones that walk everything
// and filter.
func benchRangeWindow(f Factory, w bench.Workload) func(*testing.B) {
	return func(b *testing.B) {
		m := fill(f(), w)
		var sorted [][]byte
		for k := range m.Range(nil, nil) {
			sorted = append(sorted, append([]byte(nil), k...))
		}
		lo := sorted[len(sorted)*495/1000]
		hi := sorted[len(sorted)*505/1000]
		b.ReportAllocs()
		b.ResetTimer()
		var sink int
		for i := 0; i < b.N; i++ {
			for _, v := range m.Range(lo, hi) {
				sink ^= v
			}
		}
		sinkHole = sink
	}
}

// RunOpBenchmarks is the `go test -bench` entry point: one
// sub-benchmark per (operation, workload, contender), so every cell
// of the chapter's comparison tables can be reproduced with
// `go test -bench=. -benchmem`. Extra workloads beyond the standard
// 1000-key trio (e.g. a chapter-specific Sparse/5000) can be passed
// explicitly; omitting them runs the standard trio.
func RunOpBenchmarks(b *testing.B, contenders []Contender, workloads ...bench.Workload) {
	if len(workloads) == 0 {
		workloads = Workloads1k()
	}
	for _, op := range opSpecs() {
		for _, w := range workloads {
			for _, c := range contenders {
				b.Run(op.name+"/"+shortName(w)+"/"+c.Name, op.bench(c.New, w))
			}
		}
	}
}

// --- measured tables for tutorial.md -----------------------------

// BenchResults holds one testing.BenchmarkResult per (op, workload,
// contender) cell, ready to be rendered as tutorial.md tables.
type BenchResults struct {
	contenders []string
	workloads  []string
	cells      map[string]testing.BenchmarkResult
}

func cellKey(op, workload, contender string) string {
	return op + "/" + workload + "/" + contender
}

// BenchAll measures every cell via testing.Benchmark. Slow — up to
// a second per cell at the default -benchtime — so chapters call it
// from Volatile buildcheck regions, which render only under
// -update-bench. Omitting workloads runs the standard 1000-key trio.
func BenchAll(contenders []Contender, workloads ...bench.Workload) BenchResults {
	if len(workloads) == 0 {
		workloads = Workloads1k()
	}
	res := BenchResults{cells: map[string]testing.BenchmarkResult{}}
	for _, c := range contenders {
		res.contenders = append(res.contenders, c.Name)
	}
	for _, w := range workloads {
		res.workloads = append(res.workloads, shortName(w))
	}
	for _, op := range opSpecs() {
		for _, w := range workloads {
			for _, c := range contenders {
				res.cells[cellKey(op.name, shortName(w), c.Name)] = testing.Benchmark(op.bench(c.New, w))
			}
		}
	}
	return res
}

// TimeTable renders time per op — for Put, per 1000-key build —
// one column per contender.
func (r BenchResults) TimeTable() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-12s %-9s", "Op", "Workload")
	for _, c := range r.contenders {
		fmt.Fprintf(&b, "  %11s", c)
	}
	b.WriteByte('\n')
	for _, op := range opSpecs() {
		for _, w := range r.workloads {
			fmt.Fprintf(&b, "%-12s %-9s", op.name, w)
			for _, c := range r.contenders {
				cell := r.cells[cellKey(op.name, w, c)]
				fmt.Fprintf(&b, "  %11s", fmtDuration(float64(cell.NsPerOp())))
			}
			b.WriteByte('\n')
		}
	}
	return "```\n" + b.String() + "```"
}

// SpaceTable renders B/op and allocs/op for the allocating ops (Get
// allocates nothing in any contender). Put rows describe a full
// 1000-key build; Range rows a full iteration.
func (r BenchResults) SpaceTable() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-6s %-9s", "Op", "Workload")
	for _, c := range r.contenders {
		fmt.Fprintf(&b, "  %11s %8s", c+" B", "allocs")
	}
	b.WriteByte('\n')
	for _, op := range []string{"Put", "Range"} {
		for _, w := range r.workloads {
			fmt.Fprintf(&b, "%-6s %-9s", op, w)
			for _, c := range r.contenders {
				cell := r.cells[cellKey(op, w, c)]
				fmt.Fprintf(&b, "  %11s %8s", fmtBytes(float64(cell.AllocedBytesPerOp())), group(int(cell.AllocsPerOp())))
			}
			b.WriteByte('\n')
		}
	}
	return "```\n" + b.String() + "```"
}

// --- formatting helpers ------------------------------------------

func fmtDuration(ns float64) string {
	switch {
	case ns < 1_000:
		return fmt.Sprintf("%.1f ns", ns)
	case ns < 1_000_000:
		return fmt.Sprintf("%.1f µs", ns/1_000)
	default:
		return fmt.Sprintf("%.2f ms", ns/1_000_000)
	}
}

func fmtBytes(n float64) string {
	switch {
	case n < 1_000:
		return fmt.Sprintf("%.0f B", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1f KB", n/1_000)
	default:
		return fmt.Sprintf("%.1f MB", n/1_000_000)
	}
}

// group formats n with thin spaces between thousands: 35134898 →
// "35 134 898".
func group(n int) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s
}
