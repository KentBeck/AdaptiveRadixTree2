# Chapter 1 — Test harness

A new tree implementation is a maze of off-by-one errors.Without a reference to compare against op-by-op, debugging isguessing where the bug is — at insert time? at delete time? inthe iterator? Build the lie-detector first; debug everythingelse through it.

## The shared shape: SortedMap

```go
type SortedMap interface {
	Put(key []byte, value int)
	Get(key []byte) (int, bool)
	Delete(key []byte) bool
	Len() int
	Range(from, to []byte) iter.Seq2[[]byte, int]
}
```

```go
type Factory func() SortedMap
```

`SortedMap` is the minimum surface every chapter must implement:`Put`, `Get`, `Delete`, `Len`, `Range`. `Factory` lets theharness build a fresh tree per scenario without knowing theconcrete type. Every chapter's tree and `google/btree` satisfythis same shape, so either can be diffed against the other.

## The btree oracle

```go
type BTreeAdapter struct {
	t *btree.BTreeG[bench.BtreeItem]
	n int
}

func NewBTree() SortedMap { return &BTreeAdapter{t: bench.NewBtree()} }

func BTreeFactory() Factory { return func() SortedMap { return NewBTree() } }
```

`google/btree` is the reference because it implements orderediteration — `Range` produces sorted output we can diff directly.It is the only oracle the harness ships; every chapter diffs itstree against it.

## Operations as data: `Op` and the diff loop

```go
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

```go
func RunDiff(t *testing.T, candidate, reference SortedMap, ops []Op) {
	t.Helper()
	runDiff(t, candidate, reference, ops)
}
```

Every test reduces to a list of `Op` values. `RunDiff` walksthem in lockstep against the candidate and the reference; onmismatch it stops at the offending op and prints a short tail ofthe trace so the failure is locatable. The single most importantproperty: `Len`** is checked after every op, not just at theend.** Off-by-one bugs are caught on the next operation, not20 000 ops later when the symptom has drifted miles from thecause.

## Random traces with a logged seed

```go
type RandomConfig struct {
	Seed                                            uint64
	NumOps                                          int
	KeyAlphabet                                     []byte
	MaxKeyLen                                       int
	PutWeight, GetWeight, DeleteWeight, RangeWeight int
}

func RandomTrace(cfg RandomConfig) (seed uint64, ops []Op) {
	if cfg.Seed == 0 {
		cfg.Seed = uint64(time.Now().UnixNano())
	}
	seed = cfg.Seed
	if cfg.NumOps == 0 {
		cfg.NumOps = 1000
	}
	if len(cfg.KeyAlphabet) == 0 {
		cfg.KeyAlphabet = []byte("abc")
	}
	if cfg.MaxKeyLen == 0 {
		cfg.MaxKeyLen = 8
	}
	if cfg.PutWeight == 0 && cfg.GetWeight == 0 && cfg.DeleteWeight == 0 && cfg.RangeWeight == 0 {
		cfg.PutWeight, cfg.GetWeight, cfg.DeleteWeight, cfg.RangeWeight = 4, 2, 2, 1
	}
	r := rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15))
	total := cfg.PutWeight + cfg.GetWeight + cfg.DeleteWeight + cfg.RangeWeight
	ops = make([]Op, 0, cfg.NumOps)
	randKey := func() []byte {
		n := 1 + r.IntN(cfg.MaxKeyLen)
		k := make([]byte, n)
		for i := range k {
			k[i] = cfg.KeyAlphabet[r.IntN(len(cfg.KeyAlphabet))]
		}
		return k
	}
	for i := 0; i < cfg.NumOps; i++ {
		pick := r.IntN(total)
		switch {
		case pick < cfg.PutWeight:
			ops = append(ops, Put(randKey(), r.IntN(1<<20)))
		case pick < cfg.PutWeight+cfg.GetWeight:
			ops = append(ops, Get(randKey()))
		case pick < cfg.PutWeight+cfg.GetWeight+cfg.DeleteWeight:
			ops = append(ops, Del(randKey()))
		default:
			a, b := randKey(), randKey()
			if bytes.Compare(a, b) > 0 {
				a, b = b, a
			}
			ops = append(ops, Rng(a, b))
		}
	}
	return seed, ops
}
```

`RandomTrace` generates an op sequence from a seed. Thedefaults: alphabet `[]byte("abc")` so collisions are likely,`NumOps=1000`, weighted op mix favouring `Put` (4:2:2:1 acrossPut/Get/Delete/Range). A zero `Seed` auto-generates a fresh onefrom the wall clock, so every run varies; the typical callergoes through the `RandomTraceForT` helper, which logs theeffective seed via `t.Logf`, so every failure is reproducible bypinning that seed back into `cfg.Seed`:

```go
ops := harness.RandomTraceForT(t, harness.RandomConfig{NumOps: 1000})
```

Random tests find bugs the named scenarios miss; named scenariosmake those bugs easy to debug.

## Named scenarios for fast debugging

Each scenario is its own top-level function returning a`Scenario` value, so a single one is easy to cite or run inisolation. For example, `prefix-of` checks that storing keysthat are prefixes of each other (`h`, `hi`, `hello`, `help`)keeps reads consistent through a delete:

```go
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

The harness ships 14 such scenarios: `empty/get-missing`,`single-put-get`, `overwrite`, `delete-missing`, `empty-key`,`prefix-of`, `boundary-bytes`, `long-key`, `range-half-open`,`range-unbounded`, `range-empty-window`, `delete-then-reinsert`,`large-fanout`, `mass-insert-then-delete-all`. Each is onespecific shape that either bit us once or is obvious from theAPI surface — boundary bytes (0x00, 0x7f, 0x80, 0xff), a1024-byte key, the full 256-fanout, mass insert followed by massdelete in reverse. When one fails, the test name says what thebug is. A random trace fails with "op 137" plus a logged seed —informative once you replay it, slow to triage cold.

`RunRegression` runs them all:

```go
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

Each scenario gets its own `t.Run` sub-test so a failureattributes to the named scenario, not to the suite as a whole.

## How we know the harness works

```go
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

A deliberately broken adapter (`brokenMap`: every `Get` returnsnothing, `Len` is always zero) must make the harness fail. Thetest passes when `RunDiff` records at least one error. Withoutthis meta-test, a green build proves nothing about the harnessitself — only about the trees. A vacuous diff suite (one thatnever disagrees no matter what) would look identical to aworking one until the day a real bug slipped through.

## A different question: capacity

```go
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
```

Not "is it correct" but "how many keys before 100 MB". The probeinserts keys from `gen` in batches; after each batch it calls`runtime.GC()` twice and reads `HeapAlloc`. When `HeapAlloc`exceeds the baseline by `budget`, it returns the keys that fitplus the bytes-per-key average. Chapter 2's headline number — 4000 keys on Sparse, 34 653 B/key — is measured by this function.Every chapter from 2 onward reports the same triple (Dense,Sparse, URL) so the bytes/key column tells a story across theladder.

## How chapters consume the harness

A typical chapter's test file wires its `Tree[int]` into`SortedMap` via a small adapter plus a `factory()`, then dropstwo short tests in:

```go
func TestRegression(t *testing.T) {
	harness.RunRegression(t, factory(), harness.BTreeFactory())
}

func TestRandomDiff(t *testing.T) {
	ops := harness.RandomTraceForT(t, harness.RandomConfig{})
	cand := factory()()
	ref := harness.NewBTree()
	harness.RunDiff(t, cand, ref, ops)
}
```

`BTreeFactory()` is the reference. The first test runs all 14named scenarios. The second runs a 1000-op random trace underthe default config. A few lines of adapter glue per chapter; therest of the correctness suite comes for free.

## What's deliberately not here yet

No streaming key generators driving capacity from disk —`MeasureCapacity` calls `gen(i)` per key but the probe holds thewhole `SortedMap` (and therefore the whole key set) in memory byconstruction; a TODO in `capacity.go` flags this for the day afuture implementation outlasts the pre-allocated workloads. Nofuzzing beyond random traces. No sharded benches.`range-empty-window` is treated as a consistency check, not acontract — an empty range may or may not yield in a givenimplementation, but candidate and reference must agree.