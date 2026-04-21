# ART vs google/btree — 10M-element benchmark

**Comparator:** `github.com/google/btree` v1.1.3 (4.1k stars, most-imported B-tree in Go; used by etcd/k8s-adjacent tooling), degree 32 (library default), generics form `BTreeG[kv]`.

**Workload.** 10,000,000 entries. Keys are a deterministic random permutation of `uint64` in `[0, 10M)`, big-endian encoded to 8 bytes (seed = 42). Values are `int`. Both implementations store key + value; btree holds them in a `kv` struct and orders by `bytes.Compare`.

**Host.** Go 1.24.2, darwin/amd64, VirtualApple @ 2.50GHz, 16 logical CPUs.

**Harness.** `bench_test.go`. Setup is excluded from measured time via `b.ResetTimer()` / `b.StopTimer()`. Trees for Get/Range are built once per process via `sync.Once`. The 1 % range is the half-open byte interval `[BE(5_000_000), BE(5_100_000))` — exactly 100,000 entries.

## Per-operation results

| Operation | ART | B-tree | Ratio | Faster |
| --- | --- | --- | --- | --- |
| Put (bulk) | 219.8 ns/key | 897.8 ns/key | 0.24× | ART 4.1× |
| Get (hit) | 98.9 ns/op | 1081 ns/op | 0.09× | ART 10.9× |
| Get (miss) | 11.7 ns/op | 126.9 ns/op | 0.09× | ART 10.9× |
| Delete (bulk) | 124.6 ns/key | 773.3 ns/key | 0.16× | ART 6.2× |
| Range (1 %, 100K) | 28.6 ns/key | 9.6 ns/key | 2.97× | B-tree 3.0× |

*Put / Delete measured with `-benchtime=1x` (one 10M-key pass is itself ~2–10 s of work, so `b.N=1` is all the framework gets). Get / GetMiss / Range measured with `-benchtime=3s`: 35.5M ops for ART Get, 307M ops for ART GetMiss, 1212 range passes for ART Range.*

## Memory (one 10M-element tree, from Put benchmark)

| Impl | Total bytes | Allocs | Bytes/entry | Allocs/entry |
| --- | --- | --- | --- | --- |
| ART | 893 MB | 30.2M | ~89 | 3.0 |
| B-tree | 714 MB | 692K | ~71 | 0.07 |

One 1 %-range scan (100K entries yielded):

| Impl | Bytes/scan | Allocs/scan |
| --- | --- | --- |
| ART | 32 B | 1 |
| B-tree | 0 B | 0 |

B-tree uses ~20 % less memory overall and ~44× fewer allocations at build time (items packed into node slices). On range scans B-tree allocates nothing. ART now allocates a single reusable key buffer per scan: the `[]byte` yielded for each pair is a view into that buffer and is only valid until the next iteration step, so callers that retain the key must copy it.

## Verdict

**Supports production use for point-operation-heavy workloads.**

At 10M entries with 8-byte random keys, ART is 4–11× faster than the most popular Go B-tree on Put, Get (hit), Get (miss), and Delete. Get-miss is particularly strong (11.7 ns/op) because mismatches can be resolved after one or two node visits.

**Still slower on short-range scans, though no longer catastrophically so.**

The 1 % range (100 K entries) is the common "pagination / windowed scan" shape, and B-tree is **~3× faster** there. Both implementations are now essentially zero-alloc on the scan, so the gap is pure traversal cost: this is still worse than the full-scan ratio (1.8×) because a small range pays ART's tree-descent cost to locate the start, and then walks through mostly-empty inner structure relative to the span it yields.

**Memory trade-off is modest but real.** ART is ~25 % larger in bytes and allocates ~44× more objects during build. Under heavy GC pressure or on memory-tight hosts the allocation count matters more than the byte total.

**Net production guidance.**

- Workload is point lookups / writes / deletes (cache, dedup, lookup table, set-membership): **ART wins decisively.**
- Workload is ordered windowed reads (paginated iteration, range queries, scan-then-emit pipelines): **B-tree still wins, but the gap has narrowed** (~3× on short-range scans). Prefer B-tree for scan-heavy workloads.
- Mixed / unknown: collect a representative trace and re-benchmark. The point-op / range ratio can flip the recommendation.

## Caveats / what these numbers don't cover

1. **Key shape.** 8-byte keys from a permutation of `[0, 10M)` have shallow common prefixes. Longer keys with deep common prefixes would likely widen ART's point-op lead and narrow the range gap.
2. **No concurrent access.** Both impls are single-goroutine; neither library ships a tested RW-safe wrapper.
3. **Steady-state vs. cold.** Get / Range run on a warm cache; first-hit latency is not isolated.
4. **B-tree degree.** Left at library default (32). Tuning could shift B-tree numbers by 10–30 % on any single op.
5. **Yield-buffer sharing.** `Tree.Range` yields a `[]byte` view into a single reusable key buffer; callers that need to retain keys across iteration steps must copy them.

## Reproducing

```
go test -run=^$ -bench='^BenchmarkPut_|^BenchmarkDelete_' \
  -benchmem -benchtime=1x -timeout=30m ./...
go test -run=^$ -bench='^BenchmarkGet_|^BenchmarkGetMiss_|^BenchmarkRange_' \
  -benchmem -benchtime=3s -timeout=30m ./...
```