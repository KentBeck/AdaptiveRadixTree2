# ART vs google/btree — 10M-element benchmark

**Comparator:** `github.com/google/btree` v1.1.3 (4.1k stars, most-imported B-tree in Go; used by etcd/k8s-adjacent tooling), degree 32 (library default), generics form `BTreeG[kv]`.

**Workload.** 10,000,000 entries. Keys are a deterministic random permutation of `uint64` in `[0, 10M)`, big-endian encoded to 8 bytes (seed = 42). Values are `int`. Both implementations store key + value; btree holds them in a `kv` struct and orders by `bytes.Compare`.

**Host.** Go 1.24.2, darwin/amd64, VirtualApple @ 2.50GHz, 16 logical CPUs.

**Harness.** `bench_test.go`. Setup is excluded from measured time via `b.ResetTimer()` / `b.StopTimer()`. Trees for Get/Range are built once per process via `sync.Once`. The 1 % range is the half-open byte interval `[BE(5_000_000), BE(5_100_000))` — exactly 100,000 entries.

## Per-operation results

| Operation | ART | B-tree | Ratio | Faster |
| --- | --- | --- | --- | --- |
| Put (bulk) | 162.2 ns/key | 897.8 ns/key | 0.18× | ART 5.5× |
| Get (hit) | 43.22 ns/op | 1081 ns/op | 0.04× | ART 25.0× |
| Get (miss) | 8.34 ns/op | 126.9 ns/op | 0.07× | ART 15.2× |
| Delete (bulk) | 70.0 ns/key | 772.3 ns/key | 0.09× | ART 11.0× |
| Range (1 %, 100K) | 17.75 ns/key | 9.6 ns/key | 1.85× | B-tree 1.8× |

*Put measured with *`-benchtime=1x`* (one 10M-key pass, *`b.N=1`*). Delete measured with *`-benchtime=3s`* (setup excluded via *`b.StopTimer()`*/*`b.StartTimer()`*, so ~3 clean delete iterations per run; median of 5). Get / GetMiss / Range measured with *`-benchtime=3s`*: 35.5M ops for ART Get, 307M ops for ART GetMiss, 1212 range passes for ART Range.*

## Memory (one 10M-element tree, from Put benchmark)

| Impl | Total bytes | Allocs | Bytes/entry | Allocs/entry |
| --- | --- | --- | --- | --- |
| ART | 973 MB | 20.2M | ~97 | 2.02 |
| B-tree | 714 MB | 692K | ~71 | 0.07 |

One 1 %-range scan (100K entries yielded):

| Impl | Bytes/scan | Allocs/scan |
| --- | --- | --- |
| ART | 32 B | 1 |
| B-tree | 0 B | 0 |

B-tree uses ~27 % less memory overall and ~29× fewer allocations at build time (items packed into node slices). On range scans B-tree allocates nothing. On range scans ART allocates a single small internal path buffer used for pruning; the yielded `[]byte` keys reference the tree's own stable storage and may be retained by callers without copying, as long as the entry stays in the tree.

## Verdict

**Supports production use for point-operation-heavy workloads.**

At 10M entries with 8-byte random keys, ART is 5–25× faster than the most popular Go B-tree on Put, Get (hit), Get (miss), and Delete. Get (hit) at 43.22 ns/op is ~25× faster, and Get (miss) at 8.34 ns/op is ~15× faster because mismatches can be resolved after one or two node visits.

**Still slower on short-range scans, though no longer catastrophically so.**

The 1 % range (100 K entries) is the common "pagination / windowed scan" shape, and B-tree is **~1.8× faster** there. Both implementations are now essentially zero-alloc on the scan, so the gap is pure traversal cost: a small range pays ART's tree-descent cost to locate the start, and then walks through mostly-empty inner structure relative to the span it yields.

**Memory trade-off is modest but real.** ART is ~36 % larger in bytes and allocates ~29× more objects during build. Under heavy GC pressure or on memory-tight hosts the allocation count matters more than the byte total.

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

```
go test -run=^$ -bench='^BenchmarkPut_' \
  -benchmem -benchtime=1x -timeout=30m ./...
go test -run=^$ -bench='^BenchmarkDelete_' \
  -benchmem -benchtime=3s -timeout=30m -count=5 ./...
go test -run=^$ -bench='^BenchmarkGet_|^BenchmarkGetMiss_|^BenchmarkRange_' \
  -benchmem -benchtime=3s -timeout=30m ./...
```