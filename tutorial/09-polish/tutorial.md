# Chapter 9 — Polish + reading guide

The structural work is done. The four-type ladder from chapter
[8](../08-add-node48/tutorial.md) matches the production
`art.Tree`'s shape, and the bench numbers across Sparse, Dense,
and URL workloads are within a small multiple of `google/btree`
on every operation.

Chapter 9 closes the gap with three small polishes — each one a
focused diff against chapter 8 — and then *upgrades* `Range` to
prune entire subtrees instead of walking every leaf and
filtering, exercising the most interesting of those polishes.
The chapter ends with a reading guide pointing at the parent
`art.Tree` source: every file, what to look for in it, and how
it differs from this chapter.

## Polish #1 — Inline-key buffer

In every prior chapter, `Put` allocates twice per new key: once
for the leaf struct, once for the heap-copied key bytes. Half of
those allocations are wasted: short keys (UUIDs, fixed-width
ints, small enum strings) easily fit in a 24-byte buffer next to
the leaf's value field.

The leaf grows by 24 bytes:

```go {src=art.go decls=inlineKeyMax,leaf,newLeaf}
const inlineKeyMax = 24

type leaf[V any] struct {
	key    []byte
	value  V
	inline [inlineKeyMax]byte
}

func newLeaf[V any](key []byte, value V) *leaf[V] {
	l := &leaf[V]{value: value}
	if len(key) <= inlineKeyMax {
		n := copy(l.inline[:], key)
		l.key = l.inline[:n]
	} else {
		l.key = append([]byte(nil), key...)
	}
	return l
}
```

For short keys, `l.key` is a sub-slice of `l.inline`. For long
keys, the heap-copy path is unchanged. The `&leaf[V]{...}`
construction sites from chapters 3–8 become `newLeaf[V](key,
value)` calls.

The trade is bytes-per-leaf vs allocs-per-leaf:

<!-- bench:leafsizes:start -->
```
Chapter 8 leaf   32 B   key slice header + value
Chapter 9 leaf   56 B   + 24-byte inline key buffer
```
<!-- bench:leafsizes:end -->

Dense (8 B) and Sparse (16 B) keys fit inline, so each Put costs
one allocation instead of two — the allocations table at the end
of the chapter shows Put allocs dropping roughly 2× on those
workloads. URL keys are 25–80 bytes; most miss the inline path
and Put still allocates twice.

Live heap actually goes *up* on Sparse, because every leaf
carries the inline buffer whether it uses it or not:

<!-- bench:heapfootprint:start -->
```
Workload    Chapter8 heap   Chapter9 heap    ratio
Dense            59 B/key        83 B/key    1.41×
Sparse          100 B/key       116 B/key    1.16×
URL             143 B/key       175 B/key    1.22×
```
<!-- bench:heapfootprint:end -->

The wins come elsewhere: Put gets meaningfully faster on Dense,
where allocator work dominates a small tree (see the time table
below); on Sparse and URL it's roughly flat — half the
allocations, but more bytes zeroed per leaf. The capacity table
at the end shows the same trade at 100 MB scale: every workload
fits somewhat fewer keys. Reasonable trade where allocator
latency matters more than resident bytes — which is the
production package's bet.

## Polish #2 — Embedded `innerHeader`

Chapter 8 ended with four nearly identical accessor blocks, one
per inner-node type:

```go
func (n *node4[V]) getPrefix() []byte  { return n.prefix }
func (n *node4[V]) setPrefix(p []byte) { n.prefix = p }
func (n *node4[V]) getTerminal() node  { return n.terminal }
func (n *node4[V]) setTerminal(t node) { n.terminal = t }
// ... and 12 more for node16, node48, node256
```

Sixteen one-liner methods. They differ only in the receiver type.
That's the signature for "extract a base struct and let Go's
method-promotion rules satisfy the interface."

```go {src=art.go decls=innerHeader,innerHeader.getPrefix,innerHeader.setPrefix,innerHeader.getTerminal,innerHeader.setTerminal,node4}
type innerHeader struct {
	prefix   []byte
	terminal node
}

func (h *innerHeader) getPrefix() []byte  { return h.prefix }

func (h *innerHeader) setPrefix(p []byte) { h.prefix = p }

func (h *innerHeader) getTerminal() node  { return h.terminal }

func (h *innerHeader) setTerminal(t node) { h.terminal = t }

type node4[V any] struct {
	innerHeader
	keys        [4]byte
	children    [4]node
	numChildren uint8
}
```

The four methods on `*innerHeader` promote to `*node4`,
`*node16`, `*node48`, `*node256`. **Sixteen method definitions
deleted; behavior identical.**

There is no measurable impact on bench numbers — this polish is
pure code reduction. It's worth doing because the saved
boilerplate is exactly the kind of thing that drifts when
maintainers (or AI agents) make local edits. One less place for
bugs to hide.

A small honesty: struct-literal construction sites have to
either copy the full embedded value (`innerHeader: n.innerHeader`)
or initialise it inline (`innerHeader: innerHeader{prefix: ...}`).
Slightly noisier at the construction site, much cleaner across
the type.

## Polish #3 — `Range` with a reused path buffer

The naive `Range` from chapters 2–8 walks every leaf in order
and filters at the leaf — every leaf carries its own key (since
chapter [3](../03-lazy-expansion/tutorial.md)), so no
path-tracking is needed. That works, but it
visits 100% of the leaves even when the caller asked for a narrow
window. Efficient `Range(start, end)` must *prune* whole subtrees
that lie entirely outside the range, and pruning needs the
byte-path consumed from the root to the current node so that a
subtree's high/low-bound can be compared against `start`/`end`
without visiting any of its leaves.

The naive pruning approach allocates the path slice fresh at
every recursion level, or copies it on every yield. The polish:
thread a single `*[]byte` buffer through the recursion, growing
it as the descent enters each node's prefix and shrinking it
back to the caller's length on the way out.

```go {src=art.go decl=iterateRange}
func iterateRange[V any](n node, path *[]byte, start, end []byte, yield func([]byte, V) bool) bool {
	if l, ok := n.(*leaf[V]); ok {
		if keyInRange(l.key, start, end) {
			return yield(l.key, l.value)
		}
		return true
	}
	r := n.(innerNode)
	before := len(*path)
	*path = append(*path, r.getPrefix()...)
	nodeLen := len(*path)
	if term, ok := r.getTerminal().(*leaf[V]); ok && keyInRange((*path)[:nodeLen], start, end) {
		if !yield(term.key, term.value) {
			*path = (*path)[:before]
			return false
		}
	}
	cont := r.eachAscending(func(b byte, child node) bool {
		// Skip subtrees whose entire byte range falls below start
		// or above end without ever materialising their full path.
		if subtreeBeforeWithByte((*path)[:nodeLen], b, start) {
			return true
		}
		if subtreeAtOrAfterWithByte((*path)[:nodeLen], b, end) {
			return false
		}
		*path = append((*path)[:nodeLen], b)
		return iterateRange[V](child, path, start, end, yield)
	})
	*path = (*path)[:before]
	return cont
}
```

`subtreeBeforeWithByte` and `subtreeAtOrAfterWithByte` are
allocation-free predicates that decide whether a child's whole
subtree falls outside the range based on the path-so-far + the
edge byte alone.

**Measured impact**: the `RangeWindow` rows of the time table
below iterate the middle 1% of each workload — this chapter's
pruning `Range` against chapter 8's walk-and-filter, with
`google/btree` for reference. The speedup is an order of
magnitude or more on Sparse and URL, whose full traversals do far
more inner-node work; the pruning predicate skips almost all of
it.

The flip side is visible in the `Range` rows: a *full* scan now
runs the pruning machinery (path threading, two predicates per
child) for no benefit and costs roughly 2× chapter 8's plain
walk. Narrow windows are the common case for `Range`; full scans
have `Range(nil, nil)`, which is why the production package
exposes it as its own `All()` path.

The path-buffer reuse worked too — there are zero per-yield
allocations, and the single shared buffer is amortised to ~zero
allocs per call. **The remaining allocations are closure escapes
on every inner-node visit**, the same interface-dispatch cost
(chapter [6](../06-introduce-polymorphism/tutorial.md)) that the
naive `Range` already paid. (One closure per
`eachAscending` call, captured because the call goes through the
`innerNode` interface and the compiler cannot prove the closure
stays on the stack.)

`btree` doesn't pay this cost because its iteration uses a
concrete-type recursion. ART's polymorphic dispatch is the price
of being adaptive. We made that trade in chapter 6; it lands at
~1 closure per inner-node here. That's a known floor, not a bug.

For range workloads where this matters, the production code's
`Range` ships with the same shape and the same residual cost; we
have not invented a faster version.

## The whole ladder, measured

Same acceptance criteria, same yardsticks, one last time. The
tables are rendered by `go test -update-bench` from the shared
harness benchmarks — this chapter's tree alongside chapter
[8](../08-add-node48/tutorial.md)'s, with `google/btree` for
context. Reproduce any cell with
`go test -bench=. -benchmem -benchtime=300ms ./09-polish/`.

<!-- bench:optime:start -->
```
Op           Workload      Chapter9     Chapter8        btree
Put          Dense          95.6 µs     122.3 µs     188.9 µs
Put          Sparse        183.2 µs     180.8 µs     258.3 µs
Put          URL           365.4 µs     349.2 µs     300.7 µs
Get          Dense          38.0 ns      36.0 ns     138.0 ns
Get          Sparse         48.0 ns      48.0 ns     168.0 ns
Get          URL           138.0 ns     134.0 ns     200.0 ns
Range        Dense          19.6 µs       9.5 µs       6.5 µs
Range        Sparse         43.9 µs      23.9 µs       6.2 µs
Range        URL            62.2 µs      33.6 µs       6.3 µs
RangeWindow  Dense           3.8 µs      14.6 µs     368.0 ns
RangeWindow  Sparse          2.5 µs      29.0 µs     358.0 ns
RangeWindow  URL             2.0 µs      40.6 µs     459.0 ns
```
<!-- bench:optime:end -->

<!-- bench:opspace:start -->
```
Op     Workload    Chapter9 B   allocs   Chapter8 B   allocs      btree B   allocs
Put    Dense          90.0 KB    1 020      66.0 KB    2 020     109.6 KB    1 115
Put    Sparse        129.8 KB    1 330     113.8 KB    2 330      86.3 KB    1 085
Put    URL           183.8 KB    2 603     151.8 KB    2 603     121.4 KB    1 088
Range  Dense            648 B       10        312 B        9         96 B        3
Range  Sparse         22.6 KB      239       5.8 KB      238         96 B        3
Range  URL            38.0 KB      399       9.6 KB      397         96 B        3
```
<!-- bench:opspace:end -->

<!-- bench:capacity:start -->
```
Workload    Chapter9 keys     B/key   Chapter8 keys     B/key      btree keys     B/key
Dense           1 263 082      83.1       1 564 099      67.1       1 239 811      84.6
Sparse          1 025 383     108.0       1 215 570      98.6       1 634 042      64.6
URL               614 009     170.8         755 036     139.6       1 091 008      96.4
```
<!-- bench:capacity:end -->

## What's left out

- **Min, Max, Ceiling, Floor.** The production `Tree` exposes
  these but the user explicitly excluded them from the tutorial
  ("they don't add anything to the learning"). They are sorted-map
  niceties, not new ART techniques. See `sorted.go` in the parent
  package.
- **`LockedTree[V]`.** A thin `sync.RWMutex` wrapper. Concurrency
  is a real topic but orthogonal to the trie itself. See
  `locked.go`.
- **`artmap.Ordered[K, V]`.** The typed facade with
  byte-order-preserving encoders for numeric / string `K`. Worth
  reading if you ever need a `map[int64]V` with sorted iteration.
  See the `artmap/` subpackage.
- **`RangeDescending`** (and the production `AllDescending`
  shorthand). Mirror of `Range` walking children in reverse.
  Mechanical — the sort invariant guarantees correctness in
  either direction.
- **`Clone`, `Clear`.** Standard sorted-map methods. `Clone` is a
  shallow walk that builds a fresh structure pointing at the same
  leaves; `Clear` drops the root pointer and resets `size`.

## Reading guide

> Production `art.Tree` exposes `All()` as a shorthand for
> `Range(nil, nil)`; the tutorial uses only `Range` to keep the
> iteration story unified across chapters.

The production `art.Tree` lives at the root of the repo. Each
file is small (the largest is ~16 KB). Here is what to look for:

| Parent file | What it has | What's the same as chapter 9 | What's different |
|---|---|---|---|
| `doc.go` | Package overview | Empty — same shape | (nothing in source) |
| `types.go` | Every node type + `innerNode` interface + `Tree[V]` + `New` + `Len` | Same shape; node4/16/48/256 all embed `innerHeader`; `node` interface; `*leaf[V]` with inline buffer | Adds `kind() nodeKind` method to the `node` interface for cheap leaf-vs-inner branching; `eachDescending` exists alongside `eachAscending`; `shallow()` is on `innerNode` for `Clone` |
| `helpers.go` | `newLeaf`, `splitTwoLeaves` (`newNode4With`), `splitPrefixedNode` (`splitPrefixedInner`), `consumePrefix`, `longestCommonPrefix`, `clearTerminalIfMatches`, `terminalValue` | Every helper present; same shape | Slightly cleaner naming; some helpers split between size accounting and the recursive logic |
| `put.go` | `Tree.Put` and `putInto`, `putIntoInner` | Recursive shape identical | Splits `putInto` (handles leaf and prefix split) from `putIntoInner` (handles in-node insert/recurse). Same logic, smaller functions |
| `get.go` | `Tree.Get` | Loop body identical | — |
| `delete.go` | `Tree.Delete` and `deleteFrom` | Recursive shape identical | Reshape lives on each inner-node type via the interface, same as chapter 6 onward |
| `iterate.go` | `Tree.Range`, `Tree.RangeDescending`, `Tree.RangeFrom`, `Tree.RangeTo` (plus the `Range(nil, nil)` shorthands noted in the callout above) + `iterate`, `iterateDescending`, `iterateRange`, `iterateRangeDescending`, `keyInRange`, `subtreeBeforeWithByte`, `subtreeAtOrAfterWithByte` | `Range` is *exactly* chapter 9's `iterateRange` | Adds the descending variants and the `Range(start, nil)` / `Range(nil, end)` shorthands; `Range` panics on reversed bounds (a documented choice — see `CONTRACT.md`) |
| `sorted.go` | `Min`, `Max`, `Ceiling`, `Floor`, `Clone`, `Clear` + their helpers | Not in tutorial | Worth reading. `Ceiling`/`Floor` are the most subtle: they walk down toward the target and cut to a sibling subtree's `Min`/`Max` when the path diverges |
| `locked.go` | `LockedTree[V]` and `NewLocked` | Not in tutorial | A `sync.RWMutex`-guarded wrapper. Constructor enforces non-nil tree; methods panic with a typed message on a zero-value `LockedTree[V]{}` |
| `artmap/codec.go` | Byte-order-preserving encoders for every supported `K` | Not in tutorial | The clever code is in the encoder for signed integers (XOR sign bit) and floats (XOR sign bit and, for negatives, all bits) |
| `artmap/ordered.go` | The typed facade | Not in tutorial | Wraps `art.Tree` with encode/decode at every boundary |

Beyond the source, three documents at the repo root tell you
*why* things are shaped the way they are:

- **`INVARIANTS.md`** — every structural rule the tree
  maintains, with file:line citations to the code that enforces
  it and a `TestInvariant_*` test that guards each one. Read this
  before changing anything in `types.go`.
- **`CONTRACT.md`** — the public-API contract. Each method's
  preconditions, postconditions, panics, and edge cases. Lawyer
  for the API.
- **`polymorphism-failed.md`** — the project's record of an
  earlier polymorphism attempt that didn't pan out. Worth a read
  before refactoring the dispatch in `types.go` or
  `iterate.go`. (Chapter 6 of this tutorial cites it.)

The fuzz harness lives at `art_fuzz_test.go` and the seed corpus
at `testdata/fuzz/FuzzSortedMap`. ~45 M cumulative executions
across campaigns with zero divergences against a Go `map`-backed
oracle. If you change anything load-bearing, run the fuzz
campaign too.

## What you've built

You started chapter 2 with one node type that allocated 31 KB
per Sparse key. Eight chapters later you have the same data
structure as the production package: a four-type adaptive ladder
with lazy expansion, path compression, polymorphic dispatch,
inline-key buffers, and a zero-yield-alloc pruning `Range`. The
Sparse heap is within ~2× of `google/btree`; `Get` is ~3× faster;
windowed `Range` beats the chapter-8 walk by an order of
magnitude.

That ladder cost roughly 2 600 lines of code distributed across
eight self-contained Go packages, and roughly the same number of
words of prose. Each chapter's diff against the previous was a
small, measurable, reversible decision. The decisions were made
in the chapter where they earned their keep, and only there.

That is what engineering — as distinct from coding — looks like
when written down.
