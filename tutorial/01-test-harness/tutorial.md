# Chapter 1 — Test harness

A new tree implementation is a maze of off-by-one errors.
Without a reference to compare against op-by-op, debugging is
guessing where the bug is — at insert time? at delete time? in
the iterator? Build the lie-detector first; debug everything
else through it.

## The shared shape: SortedMap

```go {src=sortedmap.go decl=SortedMap}
type SortedMap interface {
	Put(key []byte, value int)
	Get(key []byte) (int, bool)
	Delete(key []byte) bool
	Len() int
	Range(from, to []byte) iter.Seq2[[]byte, int]
}
```

```go {src=sortedmap.go decl=Factory}
type Factory func() SortedMap
```

`SortedMap` is the minimum surface every chapter must implement:
`Put`, `Get`, `Delete`, `Len`, `Range`. `Factory` lets the
harness build a fresh tree per scenario without knowing the
concrete type. Every chapter's tree, `google/btree`, and
`map[string]int` all satisfy this same shape, so any pair of
them can be diffed against each other.

## Adapters: btree as oracle, map as backstop

```go {src=sortedmap.go decls=BTreeAdapter,NewBTree,BTreeFactory}
type BTreeAdapter struct {
	t *btree.BTreeG[bench.BtreeItem]
	n int
}

func NewBTree() SortedMap { return &BTreeAdapter{t: bench.NewBtree()} }

func BTreeFactory() Factory { return func() SortedMap { return NewBTree() } }
```

`google/btree` is the canonical reference because it implements
ordered iteration — `Range` produces sorted output we can diff
directly. `MapAdapter` (a `map[string]int` that sorts on the way
out of `Range`) is a one-sentence aside: it is a backstop for
everything except `Range`, useful when btree itself is what we
suspect of misbehaving. Rare, but cheap insurance.

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

```go {src=diff.go decl=RunDiff}
func RunDiff(t *testing.T, candidate, reference SortedMap, ops []Op) {
	t.Helper()
	runDiff(t, candidate, reference, ops)
}
```

Every test reduces to a list of `Op` values. `RunDiff` walks
them in lockstep against the candidate and the reference; on
mismatch it stops at the offending op and prints a short tail of
the trace so the failure is locatable. The single most important
property: **`Len` is checked after every op, not just at the
end.** Off-by-one bugs are caught on the next operation, not
20 000 ops later when the symptom has drifted miles from the
cause.

## Random traces with a logged seed

```go {src=diff.go decls=RandomConfig,RandomTrace}
type RandomConfig struct {
	Seed                                            uint64
	NumOps                                          int
	KeyAlphabet                                     []byte
	MaxKeyLen                                       int
	PutWeight, GetWeight, DeleteWeight, RangeWeight int
}

func RandomTrace(cfg RandomConfig) []Op {
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}
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
	ops := make([]Op, 0, cfg.NumOps)
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
	return ops
}
```

`RandomTrace` generates an op sequence from a seed. The
defaults: alphabet `[]byte("abc")` so collisions are likely,
`NumOps=1000`, weighted op mix favouring `Put` (4:2:2:1 across
Put/Get/Delete/Range). The seed is part of the input, so a
failing trace replays deterministically — copy the seed, paste
it into a one-shot test, debug. Random tests find bugs the named
scenarios miss; named scenarios make those bugs easy to debug.

## Named scenarios for fast debugging

The harness ships 14 hand-written scenarios:
`empty/get-missing`, `single-put-get`, `overwrite`,
`delete-missing`, `empty-key`, `prefix-of`, `boundary-bytes`,
`long-key`, `range-half-open`, `range-unbounded`,
`range-empty-window`, `delete-then-reinsert`, `large-fanout`,
`mass-insert-then-delete-all`. Each is one specific shape that
either bit us once or is obvious from the API surface — boundary
bytes (0x00, 0x7f, 0x80, 0xff), a 1024-byte key, the full
256-fanout, mass insert followed by mass delete in reverse. When
one fails, the test name says what the bug is. A random trace
fails with "seed 1 op 137" — informative once you replay it,
slow to triage cold.

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

```go {src=harness_test.go decl=TestDiff_DetectsDivergence}
func TestDiff_DetectsDivergence(t *testing.T) {
	rec := &recorder{}
	ops := []Op{
		Put([]byte("a"), 1),
		Get([]byte("a")),
		Length(),
		Rng(nil, nil),
	}
	runDiff(rec, brokenMap{}, NewMap(), ops)
	if rec.errors == 0 {
		t.Fatalf("runDiff failed to detect divergence; harness is vacuous")
	}
}
```

A deliberately broken adapter (`brokenMap`: every `Get` returns
nothing, `Len` is always zero) must make the harness fail. The
test passes when `RunDiff` records at least one error. Without
this meta-test, a green build proves nothing about the harness
itself — only about the trees. A vacuous diff suite (one that
never disagrees no matter what) would look identical to a
working one until the day a real bug slipped through.

## A different question: capacity

```go {src=capacity.go decl=MeasureCapacity}
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

Not "is it correct" but "how many keys before 100 MB". The probe
inserts keys from `gen` in batches; after each batch it calls
`runtime.GC()` twice and reads `HeapAlloc`. When `HeapAlloc`
exceeds the baseline by `budget`, it returns the keys that fit
plus the bytes-per-key average. Chapter 2's headline number — 4
000 keys on Sparse, 34 653 B/key — is measured by this function.
Every chapter from 2 onward reports the same triple (Dense,
Sparse, URL) so the bytes/key column tells a story across the
ladder.

## How chapters consume the harness

A typical chapter's test file wires its `Tree[int]` into
`SortedMap` via a small adapter plus a `factory()`, then drops
two short tests in:

```go {src=../02-node256-only/art_test.go decls=TestRegression,TestRandomDiff}
func TestRegression(t *testing.T) {
	harness.RunRegression(t, factory(), harness.MapFactory())
}

func TestRandomDiff(t *testing.T) {
	ops := harness.RandomTrace(harness.RandomConfig{})
	cand := factory()()
	ref := harness.MapFactory()()
	harness.RunDiff(t, cand, ref, ops)
}
```

`MapFactory()` is the reference here, not `BTreeFactory()` —
either works; the chapters happen to use the map adapter so a
btree bug doesn't cascade into every chapter at once. The first
test runs all 14 named scenarios. The second runs a 1000-op
random trace under the default config. A few lines of adapter
glue per chapter; the rest of the correctness suite comes for
free.

## What's deliberately not here yet

No streaming key generators driving capacity from disk —
`MeasureCapacity` calls `gen(i)` per key but the probe holds the
whole `SortedMap` (and therefore the whole key set) in memory by
construction; a TODO in `capacity.go` flags this for the day a
future implementation outlasts the pre-allocated workloads. No
fuzzing beyond random traces. No sharded benches.
`range-empty-window` is treated as a consistency check, not a
contract — an empty range may or may not yield in a given
implementation, but candidate and reference must agree.
