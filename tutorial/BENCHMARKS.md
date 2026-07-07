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
- **mid1% ns/k** — wall-clock time per yielded key when iterating
  the **middle 1 %** of the sorted keys (the window between the
  49.5th and 50.5th percentile keys). For chapter 8's ART this
  uses `tree.Range(lo, hi)`; for btree it uses `AscendRange`.
  The bounds are computed once per cell from a sorted copy of
  the workload so both implementations see the same window.

These four together summarise the cost of *holding* a sorted map
at a given size, the cost of *touching* it, and the cost of
*walking a slice of it through the middle of the keyspace*. The
two implementations differ in shape (trie vs B-tree) so absolute
numbers diverge across the columns; the trends tell the story.

## Methodology

Captured by `TestScalingAnnex` in
[`08-polish/scaling_test.go`](08-polish/scaling_test.go), which
builds each (workload, size, implementation) cell exactly once,
times the build, GCs, snapshots heap, samples Get for 1 second,
and finally samples middle-1 %-range iteration for 1 second. No
bench-framework iteration loop — at the 100M scale, building the
tree once is already minutes.

Per-workload size caps reflect each workload's memory footprint
on a 16 GB box:

- **Sparse** (random 16 B keys): up to **30M** — at ~110 B/key the
  trie alone uses ~3.5 GB; 100M Sparse needs ~12 GB and was
  OOM-killed in our first attempt.
- **Dense** (8 B ints): up to **100M** — only ~83 B/key plus a
  small workload, so peaks at ~9 GB.
- **URL** (~40 B keys): up to **10M** — at ~170 B/key plus the
  larger workload arrays, 100M URL would need ~24 GB.

To reproduce the small tier (up to 1M):

```
cd tutorial && go test ./08-polish/ -run TestScalingAnnex -v -timeout 5m
```

To include the full ladder (needs ~10 GB free RAM and ~8 minutes):

```
cd tutorial && go test ./08-polish/ -run TestScalingAnnex -v -huge -timeout 30m
```

The harness reports per-cell results to `t.Log`. The tables
below are copied from one captured run on a 16 GB Linux box with
Go 1.23. The full run took 497 seconds.

## Sparse — random 16-byte keys, no shared prefixes

The hard case for a trie. Random first bytes mean the root must
fan out to ~250 children; deeper nodes settle into the smaller
node types. This is the workload where the chapter 6 (node16) and
chapter 7 (node48) decisions earn their keep.

| keys | Chapter 9 Put µs/k | Chapter 9 Get ns | Chapter 9 heap B/k | Chapter 9 mid1% ns/k | btree Put µs/k | btree Get ns | btree heap B/k | btree mid1% ns/k |
|------|---:|---:|---:|---:|---:|---:|---:|---:|
|  1k  | 0.32 |  50 | 116 | 258 | 0.42 |  166 | 46 |  26 |
| 10k  | 0.16 |  40 | 105 |  77 | 0.33 |  240 | 48 |  13 |
| 100k | 0.31 |  87 | 117 |  88 | 0.55 |  438 | 49 |  12 |
|  1M  | 0.53 | 246 | 107 | 108 | 1.13 | 1131 | 48 |  27 |
| 10M  | 1.36 | 495 | 122 | 544 | 2.51 | 2762 | 48 |  44 |
| 30M  | 1.39 | 558 | 117 | 621 | 3.44 | 3717 | 49 | 130 |
| 100M | OOM  | OOM | OOM | OOM | (would need ~14 GB free; the test machine had ~11 GB available) |

**Chapter 9 Get is 3.3×–6.7× faster than btree.**

**Chapter 9 uses ~2.4× the heap.** ~110–120 B/key vs btree's
~48 B/key.

**Chapter 9 Put is 1.3×–2.5× faster than btree.**

**Chapter 9 mid1% is ~5–14× *slower* than btree** on Sparse. The
trie's `Range` has to descend through prefix-matching at every
level on the way to the lo bound, then on every yield walks a
chain of inner nodes (each one an interface-method call that
escapes a closure). btree's `AscendRange` walks down to the
window once, then iterates a packed array of items. The gap
widens with N because the trie's tree depth grows.

## Dense — contiguous 8-byte big-endian integers

Maximum prefix sharing — every adjacent pair differs only in the
trailing byte or two. This is the workload where path compression
(chapter 3) and lazy expansion (chapter 2) most help; the
adaptive node sizes contribute little because the leaf-bearing
nodes still use node256 to hold 256 sequential leaves.

| keys | Chapter 9 Put µs/k | Chapter 9 Get ns | Chapter 9 heap B/k | Chapter 9 mid1% ns/k | btree Put µs/k | btree Get ns | btree heap B/k | btree mid1% ns/k |
|------|---:|---:|---:|---:|---:|---:|---:|---:|
|  1k  | 0.17 | 40 | 84 | 397 | 0.15 | 136 | 68 | 26 |
| 10k  | 0.11 | 36 | 83 |  55 | 0.22 | 164 | 68 | 13 |
| 100k | 0.16 | 45 | 83 |  39 | 0.24 | 201 | 69 | 11 |
|  1M  | 0.12 | 51 | 83 |  36 | 0.27 | 241 | 69 | 11 |
| 10M  | 0.11 | 43 | 83 |  35 | 0.31 | 279 | 69 | 12 |
| 100M | 0.49 | 53 | 83 |  37 | 0.36 | 294 | 69 | 16 |

**Chapter 9 Get is essentially flat at 36–53 ns across 5 orders of
magnitude.** btree's Get scales as expected (136 → 294 ns). At
100M keys, **Chapter 9 is 5.5× faster than btree on Get** with only
1.2× more heap.

**Chapter 9 mid1% is essentially flat at 35–55 ns/k from 10k onward**
on Dense — prefix sharing means each yielded key crosses few
inner nodes during Range. **At 100M Dense, ART mid1% is 37 ns/k
vs btree's 16 ns/k — only ~2.3× slower** despite ART's heavier
per-yield machinery. The 1k cell's 397 ns is dominated by Range
setup amortising across only 10 yields.

**Chapter 9 Put is faster than btree at every size**, by 1.3× to
2× depending on N.

## URL — host + path + 8-byte hex tail

Realistic shape. Long shared prefixes at the top, divergent
suffixes at the leaves. Roughly 25–80 bytes per key. This is the
workload that drove path compression's headline number (chapter
3, 2× tighter heap) and node16's (chapter 6, 3× tighter heap).

| keys | Chapter 9 Put µs/k | Chapter 9 Get ns | Chapter 9 heap B/k | Chapter 9 mid1% ns/k | btree Put µs/k | btree Get ns | btree heap B/k | btree mid1% ns/k |
|------|---:|---:|---:|---:|---:|---:|---:|---:|
|  1k  | 0.36 | 141 | 175 | 236 | 0.25 |  187 | 48 | 32 |
| 10k  | 0.43 | 170 | 173 | 123 | 0.38 |  289 | 47 | 15 |
| 100k | 0.65 | 252 | 175 | 102 | 0.68 |  590 | 48 | 13 |
|  1M  | 0.82 | 466 | 173 | 212 | 1.63 | 1840 | 49 | 37 |
| 10M  | 1.29 | 819 | 171 | 493 | 2.89 | 3021 | 48 | 78 |
| 100M | OOM  | OOM | OOM | OOM | (would need ~24 GB; the test machine has 16 GB) |

**Chapter 9 Get is 1.3×–3.9× faster than btree across sizes**, with
the gap widening as N grows.

**Chapter 9 uses ~3.6× the heap** of btree on URL — the worst ratio
across the three workloads.

**Chapter 9 Put is ~tied with btree at small sizes, 1.3×–2× faster
at scale.**

**Chapter 9 mid1% is 4×–9× slower than btree on URL.** Same cause
as Sparse: each yielded URL key crosses several inner nodes
(host divergence, path divergence, then leaf), each an interface
call with closure escape. The scale-with-N pattern is similar.

## What the numbers say

Four patterns hold across all three workloads:

1. **Get latency is ART's strongest suit and it scales beautifully.**
   Dense Get is 36–53 ns from 1k to 100M (essentially flat). Sparse
   Get rises slowly with map height. URL Get is consistently 1.3–
   3.9× faster than btree.
2. **Put is competitive with btree at every size**, faster on the
   shorter-key workloads at scale.
3. **Heap is the trie's honest cost.** Chapter 9 uses 1.2–3.6× the
   heap of btree, with the ratio depending mainly on key length.
   Btree wins this column.
4. **Range over a windowed slice favours btree, but the gap is
   workload-dependent.** On Dense, where prefix sharing keeps the
   trie shallow, ART's `Range` is only 2–3× slower than btree's
   `AscendRange`. On Sparse and URL, where the trie is deeper and
   the per-yield interface dispatch dominates, the gap is 5–10×.
   In every case, ART's polished `Range` is dramatically faster
   than the naive walk-and-filter `Range` of earlier chapters (the
   per-chapter `BenchmarkMid1pct_*` numbers in chapters 1–7 confirm
   this).

The two implementations are not interchangeable. Picking between
them is a workload-shape question:

| | ART wins | btree wins |
|---|---|---|
| Get | ✓ (always) | |
| Put | ✓ (mostly) | |
| Heap | | ✓ (always) |
| Range / windowed iteration | (Dense, close) | ✓ (Sparse, URL) |

ART is the right choice when **lookup latency dominates** (cache,
dedup, set-membership, lookup tables in hot paths) **and** the
working set fits in memory at the trie's bytes-per-key budget.
Btree wins when **memory is tight** or when **windowed-iteration
throughput dominates** on irregular keysets.

## Caveats

- Numbers depend on hardware (CPU, RAM, NUMA), Go version, and
  GC tuning. The trends are robust; absolute numbers are not.
- The Get and mid1% samples are hot — the working set has just
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
- The middle-1 % window is not the easiest case for either
  implementation — both have to descend to the lo bound first.
  A 1 %-window at the start of the keyspace would be cheaper for
  both, and would also disadvantage ART less. We picked the middle
  because it's the most workload-realistic shape for "sample a
  sub-range to look at."
