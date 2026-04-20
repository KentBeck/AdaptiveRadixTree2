# ART vs google/btree — 10M-element benchmark

**Comparator:** [`github.com/google/btree`](https://github.com/google/btree) v1.1.3
(4.1k stars, most-imported B-tree in Go; used by etcd/k8s-adjacent tooling),
degree 32 (library default), generics form `BTreeG[kv]`.

**Workload.** 10,000,000 entries. Keys are a deterministic random permutation of
`uint64` in `[0, 10M)`, big-endian encoded to 8 bytes (seed = 42). Values are
`int`. Both implementations store key + value; btree holds them in a `kv`
struct and orders by `bytes.Compare`.

**Host.** Go 1.24.2, darwin/amd64, VirtualApple @ 2.50GHz, 16 logical CPUs.

**Harness.** `bench_test.go`. Setup is excluded from measured time via
`b.ResetTimer()` / `b.StopTimer()`. Trees for Get/Iterate are built once per
process via `sync.Once`.

## Per-operation results

| Operation      | ART (ns/key) | B-tree (ns/key) | ART / B-tree | Faster |
|----------------|-------------:|----------------:|-------------:|:------:|
| Put (bulk)     |        192.0 |           795.2 |        0.24× | **ART 4.1×** |
| Get (hit)      |         83.9 |           888.0 |        0.09× | **ART 10.6×** |
| Delete (bulk)  |        121.2 |           854.2 |        0.14× | **ART 7.1×** |
| Iterate (full) |         14.5 |             7.9 |        1.83× | **B-tree 1.8×** |

*Put / Delete / Iterate measured with `-benchtime=1x` (a single 10M-key pass is
itself ~8 s of work); Get and Iterate numbers above reflect re-runs with
`-benchtime=3s` so `b.N` scales (42.7M ops for ART Get; 22 full-scan passes
for ART Iterate).*

## Memory (one 10M-element tree, from Put benchmark)

| Impl   | Total bytes | Allocs    | Bytes/entry | Allocs/entry |
|--------|------------:|----------:|------------:|-------------:|
| ART    |      893 MB |     30.2M |        ~89  |          3.0 |
| B-tree |      714 MB |      692K |        ~71  |         0.07 |

B-tree uses ~20 % less memory and ~44× fewer allocations (items packed into
node slices). ART pays for per-edge/per-leaf allocations.

## Verdict

**Supports production use for Get / Put / Delete-heavy workloads.**
Across the three point-operation benchmarks ART is 4–10× faster than the most
popular Go B-tree at 10M entries with 8-byte random keys. Get in particular is
~10× faster, which is the regime ART was designed for (path compression + fan-
out up to 256 + cache-friendly node layouts).

**Does not support production use where full-range iteration dominates.**
B-tree's packed node layout gives it ~1.8× faster full scans. For a workload
that reads the whole map (or large ranges) repeatedly, B-tree is the better
choice.

**Memory trade-off is modest but real.** ART is ~25 % larger in bytes and
allocates ~44× more objects. Under heavy GC pressure or on memory-tight hosts
the allocation count matters more than the byte total.

## Caveats / what these numbers don't cover

1. **Key shape.** These 8-byte keys have roughly uniform high bits (random
   permutation of [0, 10M)), so common prefixes are shallow. ART's strengths
   (prefix compression) and weaknesses (many inner nodes for sparse key sets)
   are both muted here. Longer keys with deep common prefixes would likely
   widen ART's lead on Get/Put and possibly narrow its iteration deficit.
2. **No concurrent access.** Both impls are single-goroutine here; neither
   library ships with an RW-safe wrapper tested.
3. **Steady-state vs. cold.** Get was measured on a warm cache; first-hit
   latency after process start is not isolated.
4. **No Range benchmark.** The `Tree.Range` iterator exists but wasn't
   exercised; bounded scans likely sit between point-Get and full-Iterate for
   both libraries.
5. **B-tree degree.** Left at library default (32). Tuning degree could shift
   B-tree's absolute numbers by 10–30 % on any single op.

## Reproducing

```
go test -run=^$ -bench=. -benchmem -benchtime=1x -timeout=30m ./...
go test -run=^$ -bench='^BenchmarkGet_' -benchmem -benchtime=3s ./...
go test -run=^$ -bench='^BenchmarkIterate_' -benchmem -benchtime=3s ./...
```

