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

For each cell, three numbers:

- **Put µs/key** — wall-clock time per `Put`, averaged over the
  whole build phase.
- **Get ns** — wall-clock time per `Get`, sampled over a 1 second
  hot-loop after the build.
- **heap B/key** — process heap delta after the build, divided by
  the key count. Captured via `runtime.ReadMemStats` after a
  triggered GC.

These three together summarise the cost of *holding* a sorted map
at a given size and the cost of *touching* it. The two
implementations differ in shape (trie vs B-tree) so absolute
numbers diverge across the columns; the trends tell the story.

## Methodology

Captured by `TestScalingAnnex` in
[`08-polish/scaling_test.go`](08-polish/scaling_test.go), which
builds each (workload, size, implementation) cell exactly once,
times the build, GCs, snapshots heap, then samples Get for a
fixed wall-clock window. No bench-framework iteration loop — at
the 100M scale, building the tree once is already minutes.

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
Go 1.23. The full run took 277 seconds.

## Sparse — random 16-byte keys, no shared prefixes

The hard case for a trie. Random first bytes mean the root must
fan out to ~250 children; deeper nodes settle into the smaller
node types. This is the workload where the chapter 6 (node16) and
chapter 7 (node48) decisions earn their keep.

| keys | Stage 8 Put µs/key | Stage 8 Get ns | Stage 8 heap B/key | btree Put µs/key | btree Get ns | btree heap B/key |
|------|-------------------:|---------------:|-------------------:|-----------------:|-------------:|-----------------:|
|  1k  |  0.19 |   37 | 116 | 0.22 |   138 |  46 |
| 10k  |  0.16 |   30 | 105 | 0.28 |   227 |  48 |
| 100k |  0.25 |   63 | 117 | 0.49 |   412 |  49 |
|  1M  |  0.37 |  149 | 107 | 0.91 |   966 |  48 |
| 10M  |  0.55 |  223 | 122 | 2.01 |  2361 |  48 |
| 30M  |  0.73 |  335 | 117 | 2.73 |  3126 |  49 |
| 100M | OOM   | OOM  | OOM | (would need ~14 GB free; the test machine had ~11 GB available at the cell) |

**Stage 8 Get is 3.7×–10× faster than btree.** The advantage grows
with map size: at 1k keys the trie is 3.7× ahead; at 30M it is
9.3× ahead. The trie's lookup is k cache lines (k = key length =
16) regardless of N; btree's is `log_b(N)` comparisons each
reading multiple cache lines. As N grows, the trie's advantage
compounds.

**Stage 8 uses ~2.4× the heap.** ~110–120 B/key vs btree's
~48 B/key, both essentially flat across sizes.

**Stage 8 Put is 1.2×–3.7× faster than btree.** Smaller node
mallocs amortised across the build.

## Dense — contiguous 8-byte big-endian integers

Maximum prefix sharing — every adjacent pair differs only in the
trailing byte or two. This is the workload where path compression
(chapter 3) and lazy expansion (chapter 2) most help; the
adaptive node sizes contribute little because the leaf-bearing
nodes still use node256 to hold 256 sequential leaves.

| keys | Stage 8 Put µs/key | Stage 8 Get ns | Stage 8 heap B/key | btree Put µs/key | btree Get ns | btree heap B/key |
|------|-------------------:|---------------:|-------------------:|-----------------:|-------------:|-----------------:|
|  1k  |  0.16 |  30 | 84 | 0.19 | 121 | 68 |
| 10k  |  0.09 |  27 | 83 | 0.18 | 147 | 68 |
| 100k |  0.10 |  37 | 83 | 0.21 | 178 | 69 |
|  1M  |  0.10 |  43 | 83 | 0.22 | 205 | 69 |
| 10M  |  0.10 |  41 | 83 | 0.27 | 235 | 69 |
| 100M |  0.14 |  47 | 83 | 0.29 | 249 | 69 |

**Stage 8 Get is essentially flat at 27–47 ns across 5 orders of
magnitude.** That's the trie's structural property: each new level
of map size adds at most one node-traversal, and on Dense keys
the height grows by ~1 every 256× in size. btree's Get scales as
expected — 121 → 249 ns — adding work for every level of B-tree
height.

**Stage 8 is 5.3× faster than btree on Get at 100M** (47 ns vs
249 ns), with only 1.2× more heap (83 B/key vs 69 B/key). This is
ART at its best: dense prefix-sharing keys, large N.

**Stage 8 Put is faster than btree at every size**, by 1.2× to
2× depending on N.

## URL — host + path + 8-byte hex tail

Realistic shape. Long shared prefixes at the top, divergent
suffixes at the leaves. Roughly 25–80 bytes per key. This is the
workload that drove path compression's headline number (chapter
3, 2× tighter heap) and node16's (chapter 6, 3× tighter heap).

| keys | Stage 8 Put µs/key | Stage 8 Get ns | Stage 8 heap B/key | btree Put µs/key | btree Get ns | btree heap B/key |
|------|-------------------:|---------------:|-------------------:|-----------------:|-------------:|-----------------:|
|  1k  |  0.49 |  122 | 175 | 0.24 |  138 | 48 |
| 10k  |  0.37 |  163 | 173 | 0.36 |  266 | 47 |
| 100k |  0.51 |  245 | 175 | 0.60 |  511 | 48 |
|  1M  |  0.80 |  509 | 173 | 1.10 | 1243 | 49 |
| 10M  |  1.18 |  944 | 171 | 2.47 | 2536 | 48 |
| 100M | OOM   | OOM  | OOM | (would need ~24 GB; the test machine has 16 GB) |

**Stage 8 Get is 1.1×–2.7× faster than btree across sizes**, with
the gap widening as N grows. URL keys' length means each Get does
more work per node (the prefix compare), so the trie's
per-traversal cost is higher than on Sparse — but the advantage
over btree's longer comparisons is also larger.

**Stage 8 uses ~3.6× the heap** of btree on URL — the worst ratio
across the three workloads. URL keys' length plus moderate
fanout at every depth means more inner nodes per key.

**Stage 8 Put is 1.4×–2.1× faster than btree** at 100k and above;
slower at 1k where the trie's structural setup overhead doesn't
amortise.

## What the numbers say

Three patterns hold across all three workloads:

1. **Get latency is ART's strongest suit and it scales beautifully.**
   Dense Get is 27–47 ns from 1k to 100M (essentially flat). Sparse
   Get rises slowly with map height. URL Get is consistently 1.5–
   2.5× faster than btree. The trie's worst-case lookup is k
   pointer-chases for a key of length k; the absolute number is
   small and stays small as N grows.
2. **Put is competitive with btree at every size**, faster on the
   shorter-key workloads (Dense, Sparse). The chapter-8 polishes
   (inline-key buffer especially) keep allocator pressure low.
3. **Heap is the trie's honest cost.** Stage 8 uses 1.2–3.6× the
   heap of btree, with the ratio depending mainly on key length.
   Btree wins this column.

The two implementations are not interchangeable. ART is the
right choice when **lookup latency dominates** (cache, dedup,
set-membership, lookup tables in hot paths) **and** the working
set fits in memory at the trie's bytes-per-key budget. Btree
wins when **memory is tight** or when **iteration throughput**
dominates (each chapter's per-`All` table shows btree faster, and
the production `art.Tree`'s `Range` is also slower than btree's —
see chapter 8 for the details).

## Caveats

- Numbers depend on hardware (CPU, RAM, NUMA), Go version, and
  GC tuning. The trends are robust; absolute numbers are not.
- The Get sample is hot — the working set has just been built and
  is warm in the cache. Real workloads hit cold trees more often.
  Expect Get to be 2–5× slower under cache pressure at the larger
  sizes.
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
