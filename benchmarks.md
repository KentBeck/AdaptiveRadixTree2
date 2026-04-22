# ART vs google/btree — 10M-element benchmark

*Last measured at commit `b73719f` (v0.3.0).*

**Comparator:** `github.com/google/btree` v1.1.3 (4.1k stars, most-imported B-tree in Go; used by etcd/k8s-adjacent tooling), degree 32 (library default), generics form `BTreeG[kv]`.

**Workload.** 10,000,000 entries. Keys are a deterministic random permutation of `uint64` in `[0, 10M)`, big-endian encoded to 8 bytes (seed = 42). Values are `int`. Both implementations store key + value; btree holds them in a `kv` struct and orders by `bytes.Compare`.

**Host.** Go 1.24.2, darwin/amd64, VirtualApple @ 2.50GHz, 16 logical CPUs.

**Harness.** `bench_test.go`. Setup is excluded from measured time via `b.ResetTimer()` / `b.StopTimer()`. Trees for Get/Range are built once per process via `sync.Once`. The 1 % range is the half-open byte interval `[BE(5_000_000), BE(5_100_000))` — exactly 100,000 entries.

## Per-operation results

| Operation | ART | B-tree | Ratio | Faster |
| --- | --- | --- | --- | --- |
| Put (bulk) | 150.8 ns/key | 752.6 ns/key (−16% vs pre-generics; likely monomorphisation benefit) | 0.20× | ART 5.0× |
| Get (hit) | 57.57 ns/op (+33% regression vs pre-generics; investigation deferred) | 861.9 ns/op (−20% vs pre-generics) | 0.067× | ART 15.0× |
| Get (miss) | 9.36 ns/op | 118.5 ns/op | 0.079× | ART 12.7× |
| Delete (bulk) | 124.5 ns/key (+78% regression vs pre-generics; investigation deferred) | 796.3 ns/key | 0.156× | ART 6.4× |
| Range (1 %, 100K) | 19.21 ns/key | 10.73 ns/key | 1.79× | B-tree 1.8× |

*All rows measured with *`-benchtime=3s -count=5`* (median of 5 reps). Put's 10M-key inner loop runs 2–3 times per rep at this benchtime. Delete's setup is excluded via *`b.StopTimer()`*/*`b.StartTimer()`*, so ~3 clean delete iterations per rep for ART and 1 for B-tree. At 3s benchtime Get / GetMiss / Range converged to ~67M ops for ART Get, ~376M ops for ART GetMiss, and ~1886 range passes for ART Range.*

## Memory (one 10M-element tree, from Put benchmark)

| Impl | Total bytes | Allocs | Bytes/entry | Allocs/entry |
| --- | --- | --- | --- | --- |
| ART | 893 MB | 10.16M (−50% vs pre-generics; likely monomorphisation reducing boxed-value allocs) | ~89 | 1.02 |
| B-tree | 714 MB | 692K | ~71 | 0.07 |

One 1 %-range scan (100K entries yielded):

| Impl | Bytes/scan | Allocs/scan |
| --- | --- | --- |
| ART | 32 B | 1 |
| B-tree | 0 B | 0 |

B-tree uses ~20 % less memory overall and ~15× fewer allocations at build time (items packed into node slices). On range scans B-tree allocates nothing. On range scans ART allocates a single small internal path buffer used for pruning; the yielded `[]byte` keys reference the tree's own stable storage and may be retained by callers without copying, as long as the entry stays in the tree.

## Verdict

**Supports production use for point-operation-heavy workloads.**

At 10M entries with 8-byte random keys, ART is 5–15× faster than the most popular Go B-tree on Put, Get (hit), Get (miss), and Delete. Get (hit) at 57.57 ns/op is ~15× faster, and Get (miss) at 9.36 ns/op is ~13× faster because mismatches can be resolved after one or two node visits. The Get (hit) and Delete margins narrowed vs the pre-generics baseline (ART Get regressed ~33% and ART Delete regressed ~78%; both flagged above, investigation deferred).

**Still slower on short-range scans, though no longer catastrophically so.**

The 1 % range (100 K entries) is the common "pagination / windowed scan" shape, and B-tree is **~1.8× faster** there. Both implementations are now essentially zero-alloc on the scan, so the gap is pure traversal cost: a small range pays ART's tree-descent cost to locate the start, and then walks through mostly-empty inner structure relative to the span it yields.

**Memory trade-off is modest but real.** ART is ~25 % larger in bytes and allocates ~15× more objects during build (down from ~29× pre-generics). Under heavy GC pressure or on memory-tight hosts the allocation count matters more than the byte total.

**Net production guidance.**

- Workload is point lookups / writes / deletes (cache, dedup, lookup table, set-membership): **ART wins decisively.**
- Workload is ordered windowed reads (paginated iteration, range queries, scan-then-emit pipelines): **B-tree still wins, but the gap has narrowed** (~1.8× on short-range scans). Prefer B-tree for scan-heavy workloads.
- Mixed / unknown: collect a representative trace and re-benchmark. The point-op / range ratio can flip the recommendation.

## Caveats / what these numbers don't cover

1. **Key shape.** 8-byte keys from a permutation of `[0, 10M)` have shallow common prefixes. Longer keys with deep common prefixes would likely widen ART's point-op lead and narrow the range gap.
2. **No concurrent access.** Both impls are single-goroutine; neither library ships a tested RW-safe wrapper.
3. **Steady-state vs. cold.** Get / Range run on a warm cache; first-hit latency is not isolated.
4. **B-tree degree.** Left at library default (32). Tuning could shift B-tree numbers by 10–30 % on any single op.
5. **Key aliasing.** `Tree.Range` yields `[]byte` keys that alias the tree's internal storage. They are safe to retain while the entry is in the tree and must be treated as read-only; mutating a yielded key corrupts the tree.

## Reproducing

The numbers in this document come from a single invocation of the full bench suite, from the nested `bench/` module:

```
(cd bench && go test -bench=. -benchmem -benchtime=3s -count=5 -timeout=30m ./...)
```

Each benchmark is run 5 times; the tables above report the median of the 5 reps per row.

## Environment (as measured)

Measured on macOS 26.3.1 (darwin) on an Apple M4 Max laptop with 16 logical CPUs, on AC power (100% charged). The Go toolchain is `go version go1.24.2 darwin/amd64` — i.e. the Go binary targets `darwin/amd64` and runs under Rosetta 2 on Apple Silicon, which is why `go test` reports the CPU as `VirtualApple @ 2.50GHz`. `GOMAXPROCS` was left unset (Go default = 16, one per logical CPU). Command: `(cd bench && go test -bench=. -benchmem -benchtime=3s -count=5 -timeout=30m ./...)`. No machine-quiescing steps beyond closing foreground apps and letting the laptop idle on AC; the run took ~6.5 minutes wall time.