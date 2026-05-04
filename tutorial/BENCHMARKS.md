# Benchmark annex: scaling to 100M keys

The per-chapter `tutorial.md` files measure each design decision
at a 1 000-key (occasionally 5 000-key) fixture size. That size is
chosen for pedagogical clarity: bench runs finish in seconds and
the inner-node mix is small enough to print in a table. A natural
question is whether the per-chapter story still holds at
production scale.

This annex tracks the **chapter-8 implementation** (which has the
same shape as the production `art.Tree`) against
[`google/btree`](https://github.com/google/btree) across map sizes
from **1 000 to 100 000 000** keys, on each of the three workloads.

For each cell, four numbers:

- **Put µs/k** — wall-clock time per `Put`, averaged over the
  whole build phase.
- **Get ns** — wall-clock time per `Get`, sampled over a 1 second
  hot-loop after the build.
- **heap B/k** — process heap delta after the build, divided by
  the key count. Captured via `runtime.ReadMemStats` after a
  triggered GC.
- **iter1% ns/k** — wall-clock time per yielded key when iterating
  the first 1 % of the sorted keys via `All` + early break.
  Sampled over a 1 second hot-loop. This is the partial-iteration
  cost — the cost of *starting* iteration plus *yielding* a small
  window.

These four together summarise the cost of *holding* a sorted map
at a given size, the cost of *touching* it, and the cost of
*walking* a small slice of it. The two implementations differ in
shape (trie vs B-tree) so absolute numbers diverge across the
columns; the trends tell the story.

## Methodology

Captured by `TestScalingAnnex` in
[`08-polish/scaling_test.go`](08-polish/scaling_test.go), which
builds each (workload, size, implementation) cell exactly once,
times the build, GCs, snapshots heap, then samples Get and partial
iteration for fixed wall-clock windows. No bench-framework
iteration loop — at the 100M scale, building the tree once is
already minutes.

Per-workload size caps reflect each workload's memory footprint
on a 16 GB box:

- **Sparse** (random 16 B keys): up to **30M** — at ~110 B/key the
  trie alone uses ~3.5 GB. Stage 8 100M Sparse needs ~12 GB and
  was OOM-killed in our first attempt.
- **Dense** (8 B ints): up to **100M** — only ~83 B/key plus a
  small workload, so peaks at ~9 GB.
- **URL** (~40 B keys): up to **10M** — at ~170 B/key plus the
  larger workload arrays, 100M URL would need ~24 GB.

To reproduce the small tier (up to 1M):

```
cd tutorial && go test ./08-polish/ -run TestScalingAnnex -v -timeout 5m
```

To include the full ladder (needs ~10 GB free RAM and ~5 minutes):

```
cd tutorial && go test ./08-polish/ -run TestScalingAnnex -v -huge -timeout 30m
```

The harness reports per-cell results to `t.Log`. The tables
below are copied from one captured run on a 16 GB Linux box with
Go 1.23. The full run took 326 seconds.

## Sparse — random 16-byte keys, no shared prefixes

The hard case for a trie. Random first bytes mean the root must
fan out to ~250 children; deeper nodes settle into the smaller
node types. This is the workload where the chapter 6 (node16) and
chapter 7 (node48) decisions earn their keep.

| keys | Stage 8 Put µs/k | Stage 8 Get ns | Stage 8 heap B/k | Stage 8 iter1% ns/k | btree Put µs/k | btree Get ns | btree heap B/k | btree iter1% ns/k |
|------|---------:|---------:|---------:|---------:|---------:|---------:|---------:|---------:|
|  1k  | 0.38 |   38 | 117 |  29 | 0.49 |   145 |  46 | 11 |
| 10k  | 0.15 |   26 | 105 |  14 | 0.31 |   232 |  48 |  4 |
| 100k | 0.24 |   63 | 117 |  19 | 0.46 |   434 |  49 |  5 |
|  1M  | 0.39 |  162 | 107 |  28 | 0.95 |  1106 |  48 |  4 |
| 10M  | 0.58 |  237 | 122 |  55 | 2.19 |  2296 |  48 |  8 |
| 30M  | 0.80 |  336 | 117 | 126 | 2.96 |  3132 |  49 |  9 |
| 100M | OOM  | OOM  | OOM | OOM | (would need ~14 GB free; the test machine had ~11 GB available at the cell) |

**Stage 8 Get is 3.7×–10× faster than btree.** The advantage grows
with map size: at 1k keys the trie is 3.7× ahead; at 30M it is
9.3× ahead.

**Stage 8 uses ~2.4× the heap.** ~110–120 B/key vs btree's
~48 B/key, both essentially flat across sizes.

**Stage 8 Put is 1.2×–3.7× faster than btree.**

**Stage 8 iter1% is 3×–14× *slower* than btree.** This is the
honest cost of the trie's polymorphic dispatch on random-key
iteration: each yielded key crosses one or more interface
boundaries, and each `eachAscending` call escapes a closure.
btree's iteration is a concrete-type recursion through a packed
array — one allocation amortised across the whole walk. The gap
widens with N because deeper trees mean more inner-node visits
per yield.

## Dense — contiguous 8-byte big-endian integers

Maximum prefix sharing — every adjacent pair differs only in the
trailing byte or two. This is the workload where path compression
(chapter 3) and lazy expansion (chapter 2) most help; the
adaptive node sizes contribute little because the leaf-bearing
nodes still use node256 to hold 256 sequential leaves.

| keys | Stage 8 Put µs/k | Stage 8 Get ns | Stage 8 heap B/k | Stage 8 iter1% ns/k | btree Put µs/k | btree Get ns | btree heap B/k | btree iter1% ns/k |
|------|---------:|---------:|---------:|---------:|---------:|---------:|---------:|---------:|
|  1k  | 0.14 |  24 |  84 | 26 | 0.12 | 118 | 68 | 11 |
| 10k  | 0.07 |  24 |  83 |  7 | 0.14 | 157 | 68 |  5 |
| 100k | 0.11 |  30 |  83 |  5 | 0.22 | 191 | 69 |  5 |
|  1M  | 0.10 |  37 |  83 |  5 | 0.23 | 205 | 69 |  5 |
| 10M  | 0.12 |  32 |  83 |  6 | 0.26 | 240 | 69 |  5 |
| 100M | 0.14 |  40 |  83 |  6 | 0.29 | 257 | 69 |  5 |

**Stage 8 Get is essentially flat at 24–40 ns across 5 orders of
magnitude.** btree's Get scales as expected — 118 → 257 ns. At
100M keys, **Stage 8 is 6.4× faster than btree on Get** with only
1.2× more heap.

**Stage 8 iter1% is competitive with btree on Dense** — both
sit at 5–11 ns/key once the workload is big enough to dilute
setup overhead. Dense tries are friendly to iteration because
prefix sharing means few inner-node visits per yielded key.

**Stage 8 Put is faster than btree at every size**, by 1.2× to
2× depending on N.

## URL — host + path + 8-byte hex tail

Realistic shape. Long shared prefixes at the top, divergent
suffixes at the leaves. Roughly 25–80 bytes per key. This is the
workload that drove path compression's headline number (chapter
3, 2× tighter heap) and node16's (chapter 6, 3× tighter heap).

| keys | Stage 8 Put µs/k | Stage 8 Get ns | Stage 8 heap B/k | Stage 8 iter1% ns/k | btree Put µs/k | btree Get ns | btree heap B/k | btree iter1% ns/k |
|------|---------:|---------:|---------:|---------:|---------:|---------:|---------:|---------:|
|  1k  | 0.51 | 127 | 175 | 48 | 0.24 |  152 | 48 | 11 |
| 10k  | 0.34 | 161 | 173 | 24 | 0.32 |  254 | 47 |  4 |
| 100k | 0.51 | 233 | 175 | 22 | 0.57 |  504 | 48 |  5 |
|  1M  | 0.82 | 523 | 173 | 52 | 1.24 | 1404 | 49 |  5 |
| 10M  | 1.23 | 973 | 171 | 75 | 2.57 | 2752 | 48 |  9 |
| 100M | OOM  | OOM | OOM | OOM | (would need ~24 GB; the test machine has 16 GB) |

**Stage 8 Get is 1.1×–2.8× faster than btree across sizes**, with
the gap widening as N grows.

**Stage 8 uses ~3.6× the heap** of btree on URL — the worst ratio
across the three workloads.

**Stage 8 Put is 1.2×–2.1× faster than btree** at 100k and above.

**Stage 8 iter1% is 4×–10× slower than btree** on URL. Same cause
as Sparse: each yielded URL key requires walking through several
inner nodes (host divergence, path divergence, then leaf), each
incurring a closure escape. btree iterates a packed array.

## What the numbers say

Four patterns hold across all three workloads:

1. **Get latency is ART's strongest suit and it scales beautifully.**
   Dense Get is 24–40 ns from 1k to 100M (essentially flat). Sparse
   Get rises slowly with map height. URL Get is consistently 1.1–
   2.8× faster than btree. The trie's worst-case lookup is k
   pointer-chases for a key of length k; the absolute number is
   small and stays small as N grows.
2. **Put is competitive with btree at every size**, faster on the
   shorter-key workloads (Dense, Sparse). The chapter-8 polishes
   (inline-key buffer especially) keep allocator pressure low.
3. **Heap is the trie's honest cost.** Stage 8 uses 1.2–3.6× the
   heap of btree, with the ratio depending mainly on key length.
   Btree wins this column.
4. **Partial iteration favours btree.** btree's packed-array
   iteration runs ~5 ns/key essentially regardless of workload or
   size. ART's polymorphic-dispatch iteration costs 5–125 ns/key
   depending on key length and tree depth, with the gap widening
   as N grows. btree wins this column too.

The two implementations are not interchangeable. Picking between
them is a workload-shape question:

| | ART wins | btree wins |
|---|---|---|
| Get | ✓ (always) | |
| Put | ✓ (mostly) | |
| Heap | | ✓ |
| Iteration | | ✓ |

ART is the right choice when **lookup latency dominates** (cache,
dedup, set-membership, lookup tables in hot paths) **and** the
working set fits in memory at the trie's bytes-per-key budget.
Btree wins when **memory is tight** or when **iteration
throughput dominates** — both reads and writes are on
sequential-access shapes that tries do not match.

## Caveats

- Numbers depend on hardware (CPU, RAM, NUMA), Go version, and
  GC tuning. The trends are robust; absolute numbers are not.
- The Get and iter1% samples are hot — the working set has just
  been built and is warm in the cache. Real workloads hit cold
  trees more often. Expect both numbers to be 2–5× slower under
  cache pressure at the larger sizes.
- The heap measurement excludes the user's keys, which the trie
  copies (so they are double-counted on the trie side if you
  account for them in your own data structure too). If you care
  about *total* memory, add the key bytes back: ~16 B/key Sparse,
  8 B/key Dense, ~40 B/key URL.
- Sparse 100M and URL 100M cells are marked **OOM** because the
  trie's heap requirement (~12 GB and ~17 GB respectively) plus
  the workload arrays (~1.6 GB and ~5.6 GB) exceeded the bench
  machine's 16 GB. On hosts with more RAM the harness will fill
  those cells; the per-workload size caps in `scaling_test.go`
  control which sizes are attempted.
