# Chapter 1 — Test harness

A new tree implementation is a maze of off-by-one errors. Without
a reference to compare against op-by-op, debugging is guessing
where the bug is — at insert time? at delete time? in the
iterator? Build the lie-detector first; debug everything else
through it.

## The shared shape: SortedMap

```go {src=sortedmap.go decls=SortedMap,Factory}
type SortedMap interface {
	Put(key []byte, value int)
	Get(key []byte) (int, bool)
	Delete(key []byte) bool
	Len() int
	Range(from, to []byte) iter.Seq2[[]byte, int]
}

type Factory func() SortedMap
```

`SortedMap` is the minimum surface every chapter must implement:
`Put`, `Get`, `Delete`, `Len`, `Range`. `Factory` lets the harness
build a fresh tree per scenario without knowing the concrete type.
Every chapter's tree and `google/btree` satisfy this same shape,
so either can be diffed against the other.

## The btree oracle

```go {src=sortedmap.go decls=BTreeAdapter,NewBTree,BTreeFactory}
type BTreeAdapter struct {
	t *btree.BTreeG[bench.BtreeItem]
	n int
}

func NewBTree() SortedMap { return &BTreeAdapter{t: bench.NewBtree()} }

func BTreeFactory() Factory { return func() SortedMap { return NewBTree() } }
```

`google/btree` is the reference because it implements ordered
iteration — `Range` produces sorted output we can diff directly.
It is the only oracle the harness ships; every chapter diffs its
tree against it. The adapter methods are mechanical: copy the key
on `Put`, count inserts and deletes for `Len`, map nil `Range`
bounds onto btree's `Ascend*` variants.

## Operations as data: `Op` and the diff loop

```go {src=diff.go decls=OpKind,OpPut,Op}
type OpKind int

const (
	OpPut OpKind = iota
	OpGet
	OpDelete
	OpRange
	OpLen
)

type Op struct {
	Kind  OpKind
	Key   []byte
	Value int
	From  []byte // for Range
	To    []byte
}
```

Every test reduces to a list of `Op` values. One-line builders —
`Put(k, v)`, `Get(k)`, `Del(k)`, `Rng(from, to)`, `Length()` —
keep traces readable.

The exported `RunDiff` is a thin wrapper; the loop lives in
`runDiff`, which takes only the `Errorf` corner of `testing.T` so
the meta-test below can watch it fail on purpose:

```go {src=diff.go decls=reporter,runDiff}
type reporter interface {
	Errorf(format string, args ...any)
}

func runDiff(r reporter, candidate, reference SortedMap, ops []Op) {
	for i, op := range ops {
		switch op.Kind {
		case OpPut:
			candidate.Put(op.Key, op.Value)
			reference.Put(op.Key, op.Value)
		case OpGet:
			gv, gok := candidate.Get(op.Key)
			rv, rok := reference.Get(op.Key)
			if gv != rv || gok != rok {
				r.Errorf("op %d Get(%q): candidate=(%d,%v) reference=(%d,%v)\n%s",
					i, op.Key, gv, gok, rv, rok, traceTail(ops, i))
			}
		case OpDelete:
			gok := candidate.Delete(op.Key)
			rok := reference.Delete(op.Key)
			if gok != rok {
				r.Errorf("op %d Delete(%q): candidate=%v reference=%v\n%s",
					i, op.Key, gok, rok, traceTail(ops, i))
			}
		case OpRange:
			gPairs := collect(candidate.Range(op.From, op.To))
			rPairs := collect(reference.Range(op.From, op.To))
			if !pairsEqual(gPairs, rPairs) {
				r.Errorf("op %d Range(%q,%q):\n  candidate=%s\n  reference=%s\n%s",
					i, op.From, op.To, formatPairs(gPairs), formatPairs(rPairs), traceTail(ops, i))
			}
		case OpLen:
			// Len parity is asserted by the post-step check below.
		}
		if g, ref := candidate.Len(), reference.Len(); g != ref {
			r.Errorf("op %d (%s): Len mismatch: candidate=%d reference=%d\n%s",
				i, formatOp(op), g, ref, traceTail(ops, i))
			return
		}
	}
	gPairs := collect(candidate.Range(nil, nil))
	rPairs := collect(reference.Range(nil, nil))
	if !pairsEqual(gPairs, rPairs) {
		r.Errorf("final Range(nil,nil):\n  candidate=%s\n  reference=%s",
			formatPairs(gPairs), formatPairs(rPairs))
	}
}
```

`runDiff` walks the ops in lockstep against the candidate and the
reference and reports any disagreement. The single most important
property: **`Len` is checked after every op, not just at the
end.** Off-by-one bugs are caught on the next operation, not
20 000 ops later when the symptom has drifted miles from the
cause. Every mismatch prints a short tail of the recent trace so
the failure is locatable, and after the last op a full
`Range(nil, nil)` sweep confirms the surviving contents match
key-for-key.

## Random traces with a logged seed

```go {src=diff.go decls=RandomTrace,RandomTraceForT}
func RandomTrace(seed uint64, numOps int) (uint64, []Op) {
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	randKey := func() []byte {
		k := make([]byte, 1+r.IntN(8))
		for i := range k {
			k[i] = "abc"[r.IntN(3)]
		}
		return k
	}
	ops := make([]Op, numOps)
	for i := range ops {
		switch pick := r.IntN(9); {
		case pick < 4:
			ops[i] = Put(randKey(), r.IntN(1<<20))
		case pick < 6:
			ops[i] = Get(randKey())
		case pick < 8:
			ops[i] = Del(randKey())
		default:
			a, b := randKey(), randKey()
			if bytes.Compare(a, b) > 0 {
				a, b = b, a
			}
			ops[i] = Rng(a, b)
		}
	}
	return seed, ops
}

func RandomTraceForT(t *testing.T, numOps int) []Op {
	t.Helper()
	seed, ops := RandomTrace(0, numOps)
	t.Logf("RandomTrace seed=%d numOps=%d", seed, len(ops))
	return ops
}
```

The constants are tuned for collision, not realism: a
three-letter alphabet and keys of at most 8 bytes mean the same
keys recur constantly, so overwrites, deletes of present keys,
and prefix pile-ups all actually happen within a 1000-op trace.
A zero seed draws a fresh one from the clock, so every run
explores new ground; `RandomTraceForT` logs the effective seed,
so any failure can be replayed by passing that seed to
`RandomTrace` directly.

Random tests find bugs the named scenarios miss; named scenarios
make those bugs easy to debug.

## Named scenarios for fast debugging

Each scenario is its own top-level function returning a
`Scenario` value, so a single one is easy to cite or run in
isolation. For example, `prefix-of` checks that storing keys that
are prefixes of each other (`h`, `hi`, `hello`, `help`) keeps
reads consistent through a delete:

```go {src=regression.go decl=prefixOf}
func prefixOf() Scenario {
	return Scenario{
		Name: "prefix-of",
		Ops: []Op{
			Put([]byte("h"), 1), Put([]byte("hi"), 2),
			Put([]byte("hello"), 3), Put([]byte("help"), 4),
			Get([]byte("h")), Get([]byte("hi")),
			Get([]byte("hello")), Get([]byte("help")),
			Del([]byte("hi")),
			Get([]byte("h")), Get([]byte("hi")),
			Get([]byte("hello")), Get([]byte("help")),
			Length(),
		},
	}
}
```

The harness ships 14 such scenarios: `empty/get-missing`,
`single-put-get`, `overwrite`, `delete-missing`, `empty-key`,
`prefix-of`, `boundary-bytes`, `long-key`, `range-half-open`,
`range-unbounded`, `range-empty-window`, `delete-then-reinsert`,
`large-fanout`, `mass-insert-then-delete-all`. Each is one
specific shape that either bit us once or is obvious from the API
surface — boundary bytes (0x00, 0x7f, 0x80, 0xff), a 1024-byte
key, the full 256-fanout, mass insert followed by mass delete in
reverse. When one fails, the test name says what the bug is. A
random trace fails with "op 137" plus a logged seed — informative
once you replay it, slow to triage cold.

`RunRegression` runs them all:

```go {src=regression.go decl=RunRegression}
func RunRegression(t *testing.T, candidate, reference Factory) {
	t.Helper()
	for _, sc := range Scenarios() {
		sc := sc
		t.Run(sanitize(sc.Name), func(t *testing.T) {
			RunDiff(t, candidate(), reference(), sc.Ops)
		})
	}
}
```

Each scenario gets its own `t.Run` sub-test so a failure
attributes to the named scenario, not to the suite as a whole.

## How we know the harness works

```go {src=harness_test.go decls=brokenMap,TestDiff_DetectsDivergence}
type brokenMap struct{}

func TestDiff_DetectsDivergence(t *testing.T) {
	rec := &recorder{}
	ops := []Op{
		Put([]byte("a"), 1),
		Get([]byte("a")),
		Length(),
		Rng(nil, nil),
	}
	runDiff(rec, brokenMap{}, NewBTree(), ops)
	if rec.errors == 0 {
		t.Fatalf("runDiff failed to detect divergence; harness is vacuous")
	}
}
```

`brokenMap` insists the map is always empty (`Get` finds nothing,
`Len` is zero); `recorder` counts `Errorf` calls instead of
failing. Pitted against the real reference, the broken map must
make `runDiff` record at least one error. Without this meta-test,
a green build proves nothing about the harness itself — only
about the trees. A vacuous diff suite (one that never disagrees
no matter what) would look identical to a working one until the
day a real bug slipped through.

## A different question: capacity

```go {src=capacity.go decls=heapAlloc,MeasureCapacity}
func heapAlloc() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

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
```

Not "is it correct" but "how many keys before 100 MB". The probe
inserts keys from `gen` in batches; after each batch it GCs twice
and reads `HeapAlloc`. When the heap has grown by `budget` over
the baseline, it reports the keys that fit and the bytes-per-key
average. Batches are sized adaptively — first 1000 keys, then
aim at the remaining budget using the bytes/key seen so far — so
the probe pays a handful of GC pauses whether the answer is
4 000 keys or 1.6 million. Chapter
[2](../02-node256-only/tutorial.md)'s headline number on the
Sparse workload is measured by this function. Every chapter from
2 onward reports the same triple (Dense, Sparse, URL) so the
bytes/key column tells a story across the ladder.

## One acceptance bar for every version

The point of the harness is that the acceptance criteria never
change as the tree changes shape. `RunAcceptance` is the whole
bar in one call:

```go {src=regression.go decl=RunAcceptance}
func RunAcceptance(t *testing.T, candidate Factory) {
	t.Helper()
	RunRegression(t, candidate, BTreeFactory())
	t.Run("random-trace", func(t *testing.T) {
		RunDiff(t, candidate(), NewBTree(), RandomTraceForT(t, 1000))
	})
}
```

A chapter's test file wires its `Tree[int]` into `SortedMap` via
a small adapter plus a `factory()`, then drops in one test:

```go {src=../02-node256-only/art_test.go}
func TestAcceptance(t *testing.T) {
	harness.RunAcceptance(t, factory())
}
```

A few lines of adapter glue per chapter; the whole correctness
suite comes for free, identical for every version.

## The same yardsticks: time and capacity

Correctness is only half the acceptance story. Every chapter also
answers the same two questions — how long do the operations take,
and how many keys fit in 100 MB — against the same contenders, so
the numbers are comparable across the whole ladder.

```go {src=benchmarks.go decls=Contender,RunOpBenchmarks}
type Contender struct {
	Name string
	New  Factory
}

func RunOpBenchmarks(b *testing.B, contenders []Contender) {
	for _, op := range opSpecs() {
		for _, w := range Workloads1k() {
			for _, c := range contenders {
				b.Run(op.name+"/"+shortName(w)+"/"+c.Name, op.bench(c.New, w))
			}
		}
	}
}
```

A chapter lists its contenders — its own tree, the previous
chapter's, `google/btree` — and gets one sub-benchmark per
(operation, workload, contender): `Put`, `Get`, `Range`, and
`RangeWindow` (the middle 1% of the keyspace) across Dense,
Sparse, and URL. `RunCapacityProbe` asks the capacity question
for the same contenders.

The tables printed in each chapter's `tutorial.md` are rendered
from these same functions (`BenchAll`, `CapacityTable`) through
the build-check machinery (see the tutorial README), so a
committed number is never hand-typed:
`go test -update-bench ./<chapter>/` re-measures and rewrites
them in place.

## What's deliberately not here yet

`MeasureCapacity` holds the whole map (and therefore the whole
key set) in memory by construction; a TODO in `capacity.go` flags
streaming generators for the day a future implementation outlasts
the pre-allocated workloads. No fuzzing beyond random traces. No
sharded benches. `range-empty-window` is treated as a consistency
check, not a contract — an empty range may or may not yield in a
given implementation, but candidate and reference must agree.
