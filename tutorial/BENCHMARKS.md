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

To reproduce the small tier (up to 1M):

```
cd tutorial && go test ./08-polish/ -run TestScalingAnnex -v -timeout 5m
```

To include the 10M and 100M rows (needs ~14 GB free RAM and
~10 minutes):

```
cd tutorial && go test ./08-polish/ -run TestScalingAnnex -v -huge -timeout 30m
```

The harness reports per-cell results to `t.Log`. The tables
below are copied from one captured run on a 16 GB Linux box with
Go 1.23.

## Sparse — random 16-byte keys, no shared prefixes

The hard case for a trie. Random first bytes mean the root must
fan out to ~250 children; deeper nodes settle into the smaller
node types. This is the workload where the chapter 6 (node16) and
chapter 7 (node48) decisions earn their keep.

| keys | Stage 8 Put µs/key | Stage 8 Get ns | Stage 8 heap B/key | btree Put µs/key | btree Get ns | btree heap B/key |
|---|---|---|---|---|---|---|
| 1k   | _filled below_ | | | | | |
| 10k  | | | | | | |
| 100k | | | | | | |
| 1M   | | | | | | |
| 10M  | | | | | | |
| 100M | | | | | | |

## Dense — contiguous 8-byte big-endian integers

Maximum prefix sharing — every adjacent pair differs only in the
trailing byte or two. This is the workload where path compression
(chapter 3) and lazy expansion (chapter 2) most help; the
adaptive node sizes contribute little because the leaf-bearing
nodes still use node256 to hold 256 sequential leaves.

| keys | Stage 8 Put µs/key | Stage 8 Get ns | Stage 8 heap B/key | btree Put µs/key | btree Get ns | btree heap B/key |
|---|---|---|---|---|---|---|
| 1k   | | | | | | |
| 10k  | | | | | | |
| 100k | | | | | | |
| 1M   | | | | | | |
| 10M  | | | | | | |
| 100M | | | | | | |

## URL — host + path + 8-byte hex tail

Realistic shape. Long shared prefixes at the top, divergent
suffixes at the leaves. Roughly 25–80 bytes per key. This is the
workload that drove path compression's headline number (chapter
3, 2× tighter heap) and node16's (chapter 6, 3× tighter heap).

| keys | Stage 8 Put µs/key | Stage 8 Get ns | Stage 8 heap B/key | btree Put µs/key | btree Get ns | btree heap B/key |
|---|---|---|---|---|---|---|
| 1k   | | | | | | |
| 10k  | | | | | | |
| 100k | | | | | | |
| 1M   | | | | | | |
| 10M  | | | | | | |
| 100M | | | | | | |

## What the numbers say

_Filled in after the captured run lands below._

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
- The bench machine had 16 GB free RAM. 100M Sparse runs at
  ~12 GB heap on the trie side and is borderline; 100M URL is
  ~17.5 GB and cannot fit. Cells that didn't fit are marked
  *OOM* in the tables.
