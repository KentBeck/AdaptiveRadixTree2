# ART vs google/btree — 10M-element benchmark

*Last measured at commit `d14c9c6` (v0.4.1).*

**Comparator:** `github.com/google/btree` v1.1.3 (4.1k stars, most-imported B-tree in Go; used by etcd/k8s-adjacent tooling), degree 32 (library default), generics form `BTreeG[kv]`.

**Workload.** 10,000,000 entries. Keys are a deterministic random permutation of `uint64` in `[0, 10M)`, big-endian encoded to 8 bytes (seed = 42). Values are `int`. Both implementations store key + value; btree holds them in a `kv` struct and orders by `bytes.Compare`.

**Host.** Go 1.24.2, darwin/amd64, VirtualApple @ 2.50GHz, 16 logical CPUs.

**Harness.** `bench_test.go`. Setup is excluded from measured time via `b.ResetTimer()` / `b.StopTimer()`. Trees for Get/Range are built once per process via `sync.Once`. The 1 % range is the half-open byte interval `[BE(5_000_000), BE(5_100_000))` — exactly 100,000 entries.

## Per-operation results

| Operation | ART | B-tree | Ratio | Faster |
| --- | --- | --- | --- | --- |
| Put (bulk) | 160.3 ns/key | 800.1 ns/key | 0.20× | ART 5.0× |
| Get (hit) | 44.81 ns/op (recovered in v0.4.1; see CHANGELOG) | 929.4 ns/op | 0.048× | ART 20.7× |
| Get (miss) | 8.66 ns/op | 118.5 ns/op | 0.073× | ART 13.7× |
| Delete (bulk) | 115.3 ns/key (PR #7 recovered ~9 ns/key vs v0.4.0; residual ~45 ns/key gap vs pre-generics tracked) | 796.3 ns/key | 0.145× | ART 6.9× |
| Range (1 %, 100K) | 19.53 ns/key | 10.73 ns/key | 1.82× | B-tree 1.8× |

*All rows measured with *`-benchtime=3s -count=5`* (median of 5 reps). Put's 10M-key inner loop runs 2–3 times per rep at this benchtime. Delete's setup is excluded via *`b.StopTimer()`*/*`b.StartTimer()`*, so ~3 clean delete iterations per rep for ART and 1 for B-tree. At 3s benchtime Get / GetMiss / Range converged to ~77M ops for ART Get, ~425M ops for ART GetMiss, and ~1874 range passes for ART Range.*

## Memory (one 10M-element tree, from Put benchmark)

| Impl | Total bytes | Allocs | Bytes/entry | Allocs/entry |
| --- | --- | --- | --- | --- |
| ART | 893 MB | 10.16M | ~89 | 1.02 |
| B-tree | 714 MB | 692K | ~71 | 0.07 |

One 1 %-range scan (100K entries yielded):

| Impl | Bytes/scan | Allocs/scan |
| --- | --- | --- |
| ART | 32 B | 1 |
| B-tree | 0 B | 0 |

B-tree uses ~20 % less memory overall and ~15× fewer allocations at build time (items packed into node slices). On range scans B-tree allocates nothing. On range scans ART allocates a single small internal path buffer used for pruning; the yielded `[]byte` keys reference the tree's own stable storage and may be retained by callers without copying, as long as the entry stays in the tree.

## Verdict

**Supports production use for point-operation-heavy workloads.**

At 10M entries with 8-byte random keys, ART is 5–21× faster than the most popular Go B-tree on Put, Get (hit), Get (miss), and Delete. Get (hit) at 44.81 ns/op is ~21× faster, and Get (miss) at 8.66 ns/op is ~14× faster because mismatches can be resolved after one or two node visits. Get (hit) has fully recovered to the pre-generics baseline (44.81 ns/op vs the pre-generics 43.22 ns/op); Delete is partially recovered via PR #7 (124.5 → 115.3 ns/key), with a residual ~45 ns/key gap vs the pre-generics baseline (70.0 ns/key) tracked for future work.

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