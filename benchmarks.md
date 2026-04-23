# ART vs google/btree — 10M-element benchmark

*Last measured at commit `d8df19e` (post-v0.4.1; node interface de-parameterised so V lives only on `Tree[V]` and `leaf[V]`).*

**Comparator:** `github.com/google/btree` v1.1.3 (4.1k stars, most-imported B-tree in Go; used by etcd/k8s-adjacent tooling), degree 32 (library default), generics form `BTreeG[kv]`.

**Workload.** 10,000,000 entries. Keys are a deterministic random permutation of `uint64` in `[0, 10M)`, big-endian encoded to 8 bytes (seed = 42). Values are `int`. Both implementations store key + value; btree holds them in a `kv` struct and orders by `bytes.Compare`.

**Host.** Go 1.24.2, darwin/amd64, VirtualApple @ 2.50GHz, 16 logical CPUs.

**Harness.** `bench_test.go`. Setup is excluded from measured time via `b.ResetTimer()` / `b.StopTimer()`. Trees for Get/Range are built once per process via `sync.Once`. The 1 % range is the half-open byte interval `[BE(5_000_000), BE(5_100_000))` — exactly 100,000 entries.

## Per-operation results

Throughput **and** allocation cost are reported side-by-side so a careful reader can compare point-op speed against GC pressure in one place.

| Operation | ART ns/op | ART B/op | ART allocs/op | B-tree ns/op | B-tree B/op | B-tree allocs/op | Faster |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Put (per key, 10M build) | 161.8 | 89.4 | 1.016 | 748.3 | 71.4 | 0.069 | ART 4.6× |
| Get hit (per lookup) | 52.74 | 0 | 0 | 908.6 | 0 | 0 | ART 17.2× |
| Get miss (per lookup) | 8.22 | 0 | 0 | 120.3 | 0 | 0 | ART 14.6× |
| Delete (per key, 10M) | 77.70 | 6.27 | 0.012 | 836.1 | 0.73 | ~4e-4 | ART 10.8× |
| Range (1 %, 100K scan) | 19.25 ns/key | 32 B/scan | 1 alloc/scan | 10.24 ns/key | 0 B/scan | 0 allocs/scan | B-tree 1.9× |

*All rows measured with *`-benchtime=1s -count=3`* (median of 3 reps) on the nested `bench/` module. For Put and Delete, B/op and allocs/op are per-key (divide the raw bench counter by `benchN=10_000_000`). For Range, ns/op is normalized per key yielded while B/op and allocs/op remain per full 100K-entry scan. Put's 10M-key inner loop runs 1 time per rep at this benchtime. Delete's setup is excluded via *`b.StopTimer()`*/*`b.StartTimer()`*, giving 2 clean delete iterations per rep for ART and 1 for B-tree.*

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

At 10M entries with 8-byte random keys, ART is 4.6–17× faster than the most popular Go B-tree on Put, Get (hit), Get (miss), and Delete. Get (hit) at 52.74 ns/op is ~17× faster, and Get (miss) at 8.22 ns/op is ~15× faster because mismatches can be resolved after one or two node visits. Delete at 77.70 ns/key on the de-parameterised node interface is materially faster than the 115.3 ns/key baseline that motivated PR #7, and has closed most of the remaining gap against the pre-generics baseline (70.0 ns/key) tracked for future work.

**Still slower on short-range scans, though no longer catastrophically so.**

The 1 % range (100 K entries) is the common "pagination / windowed scan" shape, and B-tree is **~1.8× faster** there. Both implementations are now essentially zero-alloc on the scan, so the gap is pure traversal cost: a small range pays ART's tree-descent cost to locate the start, and then walks through mostly-empty inner structure relative to the span it yields.

**Memory trade-off is modest but real.** ART is ~25 % larger in bytes and allocates ~15× more objects during build (down from ~29× pre-generics). Under heavy GC pressure or on memory-tight hosts the allocation count matters more than the byte total.

**Net production guidance.**

- Workload is point lookups / writes / deletes (cache, dedup, lookup table, set-membership): **ART wins decisively.**
- Workload is ordered windowed reads (paginated iteration, range queries, scan-then-emit pipelines): **B-tree still wins, but the gap has narrowed** (~1.8× on short-range scans). Prefer B-tree for scan-heavy workloads.
- Mixed / unknown: collect a representative trace and re-benchmark. The point-op / range ratio can flip the recommendation.

## Key-shape sensitivity

ART's speed and memory cost depend on the shape of the key distribution — how much prefix siblings share, how dense the byte space is, and how long each key is. `bench/keyshapes_test.go` sweeps four representative shapes at a 100K working set so the whole sweep finishes in well under a minute.

- **seqInt64** — sequential `uint64` `[0, 100K)` big-endian. Dense, short, heavy shared prefix.
- **randInt64** — permutation of `[0, 100K)` big-endian. Short, sparse on the leading bytes, same tree shape as the main 10M workload.
- **uuid** — 16 random bytes per key. Long, effectively prefix-less.
- **urlPath** — deep `/api/v1/org/NNN/user/NNN/session/NNNNNN` strings. Long keys with heavy common prefixes across siblings.

| Shape | Put ns/key | Put B/key | Get ns/op | Get B/op | Delete ns/key | Delete B/key | Range ns/key | Range B/scan |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| seqInt64 | 37.52 | 89.4 | 12.55 | 0 | 24.10 | 6.29 | 16.48 | 32 |
| randInt64 | 45.87 | 89.4 | 15.00 | 0 | 31.46 | 6.29 | 16.48 | 32 |
| uuid | 62.86 | 122.9 | 22.89 | 0 | 39.56 | 5.72 | 23.63 | 32 |
| urlPath | 130.5 | 161.9 | 46.42 | 0 | 64.67 | 14.30 | 24.29 | 32 |

*Measured with *`(cd bench && go test -run=^$ -bench=BenchmarkKeyShape -benchmem -benchtime=1s -count=1 ./...)`* at 100K keys per shape. Put B/key, Delete B/key, and the raw Go counters scale roughly linearly with working-set size; see `keyshapes_test.go` for the exact key generators. Range yields the middle 1 % of the sorted keyspace (~1000 entries) so each row's Range window is comparable across shapes.*

Read the table as a map of where ART's costs live:

- **Put / Get / Delete widen by ~3–4× from seqInt64 to urlPath.** Longer keys mean deeper traversal, more prefix comparisons on each inner node, and more memory per leaf.
- **Get is remarkably cheap on short dense keys** (12.55 ns/op on seqInt64) — tighter than the random-8-byte main bench (52.74 ns/op) because the dense-prefix tree fits more nicely in cache at 100K.
- **Memory per key scales with key length, not with entropy.** seqInt64 and randInt64 both pay ~89 B/key; uuid jumps to ~123 B/key; urlPath to ~162 B/key.
- **Range cost is shape-stable** at 16–24 ns/key and always exactly one 32-byte allocation per scan (the path buffer used for pruning). Longer-key shapes add a small constant for the deeper tree descent but the per-key yield cost is flat.

B-tree equivalents are intentionally omitted from this table — the point here is to characterize *ART's* internal sensitivity. Rerun the harness against a `BTreeG[kv]` in your own workload if you need the direct per-shape comparison.

## Caveats / what these numbers don't cover

1. **Key shape.** 8-byte keys from a permutation of `[0, 10M)` have shallow common prefixes. See the Key-shape sensitivity section above for how Put / Get / Delete / Range costs change across `seqInt64`, `randInt64`, `uuid`, and `urlPath` workloads; in particular, longer keys slow ART's point ops down in absolute terms (an `urlPath` Get is ~3.7× slower than a `seqInt64` Get at 100K).
2. **No concurrent access.** Both impls are single-goroutine; neither library ships a tested RW-safe wrapper.
3. **Steady-state vs. cold.** Get / Range run on a warm cache; first-hit latency is not isolated.
4. **B-tree degree.** Left at library default (32). Tuning could shift B-tree numbers by 10–30 % on any single op.
5. **Key aliasing.** `Tree.Range` yields `[]byte` keys that alias the tree's internal storage. They are safe to retain while the entry is in the tree and must be treated as read-only; mutating a yielded key corrupts the tree.

## Reproducing

The main-table numbers come from a single invocation of the original per-op bench suite, from the nested `bench/` module:

```
(cd bench && go test -run=^$ -bench='^Benchmark(Put|Get|GetMiss|Delete|Range)_(ART|BTree)$' -benchmem -benchtime=1s -count=3 -timeout=20m ./...)
```

Each benchmark is run 3 times; the main table reports the median of the 3 reps per row. The Key-shape sensitivity table is produced by a separate, faster pass:

```
(cd bench && go test -run=^$ -bench=BenchmarkKeyShape -benchmem -benchtime=1s -count=1 ./...)
```

To run everything at once (~3 minutes), use `-bench=.` in place of either regex.

## Environment (as measured)

Measured on macOS 26.3.1 (darwin) on an Apple M4 Max laptop with 16 logical CPUs, on AC power (100% charged). The Go toolchain is `go version go1.24.2 darwin/amd64` — i.e. the Go binary targets `darwin/amd64` and runs under Rosetta 2 on Apple Silicon, which is why `go test` reports the CPU as `VirtualApple @ 2.50GHz`. `GOMAXPROCS` was left unset (Go default = 16, one per logical CPU). Main-table command: `(cd bench && go test -run=^$ -bench='^Benchmark(Put|Get|GetMiss|Delete|Range)_(ART|BTree)$' -benchmem -benchtime=1s -count=3 -timeout=20m ./...)` — ~2.5 minutes wall time. Key-shape command: `(cd bench && go test -run=^$ -bench=BenchmarkKeyShape -benchmem -benchtime=1s -count=1 ./...)` — ~36 seconds wall time. No machine-quiescing steps beyond closing foreground apps and letting the laptop idle on AC.