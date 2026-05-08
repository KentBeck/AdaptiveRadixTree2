# Chapter 9 — Polish + reading guide

The structural work is done. The four-type ladder from chapter 8
matches the production `art.Tree`'s shape, and the bench numbers
across Sparse, Dense, and URL workloads are within a small
multiple of `google/btree` on every operation.

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
keys, the heap-copy path is unchanged. The five `&leaf[V]{...}`
construction sites in chapters 1–7 become `newLeaf[V](key, value)`
calls.

**Measured impact** (1 000 keys per workload, chapter 8 → chapter 9):

| Op | Workload | Stage 7 allocs/op | Stage 8 allocs/op | Drop |
|---|---|---|---|---|
| Put | Dense (8 B keys) | 2 019 | 1 019 | **1.98×** |
| Put | Sparse (16 B keys) | 2 329 | 1 329 | **1.75×** |
| Put | URL (~40 B keys) | 2 602 | 2 602 | **1.00×** |

Dense and Sparse keys are below `inlineKeyMax`, so each Put now
costs one alloc instead of two. URL keys are 25–80 bytes; most
miss the inline path and Put still allocates twice.

The trade is bytes-per-leaf vs allocs-per-leaf:

| Stage | leaf size | Heap (Sparse) |
|---|---|---|
| Stage 7 | 32 B | 100 B/key |
| Stage 8 | 56 B | 116 B/key |

Live heap actually goes *up* by 16% on Sparse because every leaf
carries the inline buffer whether it uses it or not. The wins
come elsewhere: Put time on Sparse drops from 148 µs to 113 µs
(~24% faster) because allocator work is the dominant cost. Put
time on Dense drops from 100 µs to 74 µs (~26% faster).

Reasonable trade. Heap is rarely the bottleneck; allocator latency
often is.

## Polish #2 — Embedded `innerHeader`

Chapters 5–7 have four nearly identical method blocks, one per
inner-node type:

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

The naive `Range` from chapters 1–7 walks every leaf in order and
filters at the leaf — every leaf carries its own key (since
chapter 3), so no path-tracking is needed. That works, but it
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

**Measured impact** (`Range(lo, hi)` over the middle 1% of each
workload, 1 000 keys, naive walk-and-filter from chapter 8 vs
the pruning Range above, with `google/btree` for reference):

| Workload | Stage 7 naive Range | Stage 8 pruning Range | Speedup | btree Range |
|---|---|---|---|---|
| Dense  |  8.76 µs /   8 allocs | 1.72 µs /  6 allocs |  5.1× | 0.13 µs / 0 allocs |
| Sparse | 16.36 µs / 237 allocs | 1.24 µs /  8 allocs | 13.2× | 0.13 µs / 0 allocs |
| URL    | 23.43 µs / 396 allocs | 0.97 µs / 15 allocs | 24.2× | 0.18 µs / 0 allocs |

That is the speedup Polish #3 buys: 5–24× wall-clock against the
naive walk-and-filter, depending on how much of the keyspace the
window actually overlaps. Sparse and URL win biggest because
their full traversals do far more inner-node work, and the
pruning predicate skips almost all of it.

The path-buffer reuse worked too — there are zero per-yield
allocations, and the single shared buffer is amortised to ~zero
allocs per call. **The remaining allocations are closure escapes
on every inner-node visit**, the same chapter-5 interface-dispatch
cost that the naive `Range` already paid. (One closure per
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
inline-key buffers, and a zero-yield-alloc `Range`. The Sparse
heap is within 1.6× of `google/btree`; `Get` is 4× *faster*.

That ladder cost roughly 2 600 lines of code distributed across
eight self-contained Go packages, and roughly the same number of
words of prose. Each chapter's diff against the previous was a
small, measurable, reversible decision. The decisions were made
in the chapter where they earned their keep, and only there.

That is what engineering — as distinct from coding — looks like
when written down.
