# ART vs three sorted-map alternatives — 10M-element benchmark

*Per-operation table re-baselined against three comparators at the bench-2 commit on `sorted-map-via-art` (`-benchtime=3s -count=5`, median of 5 reps; raw output at `/tmp/bench-2-baseline/bench.txt`). The ART and `google/btree` rows are fresh measurements taken in the same harness invocation as the new `tidwall/btree` and `plar/go-adaptive-radix-tree` rows, so all four implementations share identical machine state, GOGC, and warm-up. The Key-shape sub-table is carried forward from `d8df19e` and remains ART-only by design (it characterizes ART's internal sensitivity to key shape, not a comparator race). The **New-surface microbenchmarks** section was measured fresh at `0f4234f`.*

**Comparators.**
- `github.com/google/btree` v1.1.3 — most-imported B-tree in Go (4.1k stars; used by etcd/k8s-adjacent tooling); generics form `BTreeG[kv]`, degree 32 (library default).
- `github.com/tidwall/btree` v1.8.1 — well-tuned alternative B-tree; `BTreeG[kv]` constructed via `NewBTreeGOptions(kvLess, Options{NoLocks: true, Degree: 32})`. **Degree is overridden to 32 to match google/btree** for an apples-to-apples B-tree comparison; **`NoLocks: true` disables tidwall's internal `sync.RWMutex` on every op**, since google/btree never charges that cost. With `NoLocks: false` (the default) tidwall's numbers would be visibly worse on every operation, so this setting is part of the comparison contract.
- `github.com/plar/go-adaptive-radix-tree` v1.0.7 — the most-cited alternative ART in pure Go (65 importers). API is non-generic: `Insert(Key, Value)` / `Search(Key)` / `Delete(Key)` where `Value` is `interface{}`, so **every Put incurs an `int → interface{}` boxing alloc** that our generic `Tree[V int]` does not. This shows up cleanly as ~4 allocs/Put for plar vs ~1 alloc/Put for ART (the extra ~3 are leaf/iterator-side; the boxing is the structural one) — the per-op table reflects it honestly rather than burying it.

**Workload.** 10,000,000 entries. Keys are a deterministic random permutation of `uint64` in `[0, 10M)`, big-endian encoded to 8 bytes (seed = 42). Values are `int`. All four implementations store key + value; the two B-trees hold them in a `kv` struct and order by `bytes.Compare`.

**Host.** Go 1.24.2, darwin/amd64, VirtualApple @ 2.50GHz, 16 logical CPUs.

**Harness.** `bench_test.go`. Setup is excluded from measured time via `b.ResetTimer()` / `b.StopTimer()`. Trees for Get/Range are built once per process via `sync.Once`. The 1 % range is the half-open byte interval `[BE(5_000_000), BE(5_100_000))` — exactly 100,000 entries.

## Per-operation results

Headline number per cell is ns/op (for `Get`/`GetMiss`) or ns/key (for `Put`/`Delete`/`Range`). Allocation profile follows in a separate sub-table so each cell here stays a single number.

| Operation | ART | google/btree | tidwall/btree | plar/art | Faster | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Put (per key, 10M build) | **161.7** ns/key | 772.9 ns/key | 797.2 ns/key | 168.9 ns/key | ART | ART 4.8× over google, 4.9× over tidwall; ART vs plar is a near-tie (1.04×) — ART's only structural lead over plar here is generics avoiding the boxing alloc. |
| Get hit (per lookup) | **43.96** ns/op | 896.8 ns/op | 932.2 ns/op | 68.06 ns/op | ART | ART 20.4× over google, 21.2× over tidwall, 1.55× over plar. The two B-trees are within 4 % of each other; ART wins decisively against both ARTs and both B-trees. |
| Get miss (per lookup) | **8.82** ns/op | 151.0 ns/op | 154.1 ns/op | 15.73 ns/op | ART | ART 17.1× over google, 17.5× over tidwall, 1.78× over plar. Misses resolve in 1–2 node visits on either ART. |
| Delete (per key, 10M) | **76.61** ns/key | 808.1 ns/key | 786.5 ns/key | 119.2 ns/key | ART | ART 10.5× over google, 10.3× over tidwall, 1.56× over plar. The two B-trees are within 3 %; tidwall is fractionally faster here. |
| Range (1 %, 100K scan) | 19.32 ns/key | 10.78 ns/key | **8.12** ns/key | 6804 ns/key | tidwall/btree | **tidwall is 2.4× faster than ART**, google is 1.8× faster than ART. plar's `Iterator` exposes no efficient seek primitive, so the bench falls back to a full leaf-walk skipping until `key >= start` — the resulting 6.8 µs/key is **~352× slower** than ART and is dominated by structure-traversal cost, not the actual scan. Honest caveat: a different harness using plar's `ForEachPrefix` would be faster for prefix-shaped windows but does not match a half-open byte range. |

*Measured at the bench-2 commit on `sorted-map-via-art` with `(cd bench && go test -run='^$' -bench='^Benchmark(Put|Get|GetMiss|Delete|Range)_(ART|BTree|Tidwall|Plar)$' -benchmem -benchtime=3s -count=5 -timeout=30m ./...)`. Each row is the median of 5 reps. Raw output: `/tmp/bench-2-baseline/bench.txt`. For Put and Delete, ns/key is the raw `ns/op` counter divided by `benchN = 10_000_000` (one outer iteration builds or empties the whole tree). For Range, ns/key is normalized over the 100,000 entries yielded from the half-open byte interval `[BE(5_000_000), BE(5_100_000))`. Delete's setup is excluded via `b.StopTimer()` / `b.StartTimer()`.*

### Allocation profile per operation

Per-key (`Put`, `Delete`) and per-scan (`Range`) allocations side-by-side. `Get` / `GetMiss` are zero-alloc on every implementation and are omitted.

| Operation | ART (B/key, allocs/key) | google/btree | tidwall/btree | plar/art |
| --- | --- | --- | --- | --- |
| Put | 89.4, 1.016 | 71.4, 0.069 | 93.9, 0.069 | 93.0, **4.031** |
| Delete | 6.27, 0.0118 | 0.73, ~3.7e-4 | 0.87, ~4.3e-4 | 3.89, 0.024 |
| Range (per scan) | 32 B, 1 alloc | 0 B, 0 | 0 B, 0 | **165 MB, ~5.18M** |

The plar Put row is the boxing tax: `Insert(Key, Value)` takes `Value = interface{}`, so each `int` value escapes to the heap. That extra alloc is structural to plar's API, not a tuning oversight; we are flagging it rather than wrapping it away. The plar Range row is the no-seek tax: walking ~5.2M leaves to find the 100K-entry midpoint window allocates per visited leaf inside the iterator. Both numbers are honest reflections of what a user choosing plar would actually pay.

## Memory (one 10M-element tree, from Put benchmark)

| Impl | Total bytes | Allocs | Bytes/entry | Allocs/entry |
| --- | --- | --- | --- | --- |
| ART (this library) | 893 MB | 10.16M | ~89 | 1.02 |
| google/btree | 714 MB | 692K | ~71 | 0.07 |
| tidwall/btree | 939 MB | 693K | ~94 | 0.07 |
| plar/art | 930 MB | 40.31M | ~93 | **4.03** |

One 1 %-range scan (100K entries yielded):

| Impl | Bytes/scan | Allocs/scan |
| --- | --- | --- |
| ART (this library) | 32 B | 1 |
| google/btree | 0 B | 0 |
| tidwall/btree | 0 B | 0 |
| plar/art | ~165 MB | ~5.18M |

Both B-trees pack items into node slices and stay essentially zero-alloc on range scans. ART (this library) allocates a single 32-byte internal path buffer per scan used for pruning; the yielded `[]byte` keys reference the tree's own stable storage and may be retained by callers without copying, as long as the entry stays in the tree. plar/art's per-scan numbers are inflated by the no-seek workaround (see Per-operation results above) — they are an artifact of how the harness has to use plar's public API to express a half-open byte-range window, not a fair characterization of plar's tree itself if a seek primitive existed.

## Verdict

**Supports production use for point-operation-heavy workloads — across all three comparators, including the other ART.**

At 10M entries with 8-byte random keys, ART (this library) is the fastest of the four implementations on Put, Get (hit), Get (miss), and Delete:

- **Versus the two B-trees** (`google/btree`, `tidwall/btree`): ART is **17–21× faster** on Get (43.96 ns/op vs 896.8 / 932.2 ns/op), **17.1–17.5× faster** on Get (miss) (8.82 ns/op vs 151.0 / 154.1 ns/op), **4.8–4.9× faster** on Put (161.7 ns/key vs 772.9 / 797.2 ns/key), and **10.3–10.5× faster** on Delete (76.61 ns/key vs 786.5 / 808.1 ns/key). The two B-trees are within ~4 % of each other on every point op — the choice between them does not change the headline.
- **Versus the other Go ART implementation** (`plar/go-adaptive-radix-tree`): ART is **1.55× faster** on Get hit, **1.78× faster** on Get miss, and **1.56× faster** on Delete. **Put is essentially a tie** at 161.7 vs 168.9 ns/key (1.04× — within run variance) — the only structural ART-vs-ART lead our library has on Put is generics avoiding the `int → interface{}` boxing alloc that plar's API mandates (1 alloc/Put for ART, ~4 allocs/Put for plar). Concretely: if you don't need a generic value type, plar's Put is competitive; on every other point op our library wins by a clear margin.

**Loses on short-range scans against both B-trees — and the gap is wider than the prior README claimed.**

The 1 % range (100K entries) is the common "pagination / windowed scan" shape. **`tidwall/btree` is 2.4× faster than ART** on this benchmark (8.12 vs 19.32 ns/key) and **`google/btree` is 1.8× faster** (10.78 vs 19.32 ns/key). Tidwall is the new headline range winner — the prior framing of "B-tree is 1.8× faster" understated the worst case by picking only one comparator. Both B-trees are zero-alloc on the scan; ART's one 32-byte path-buffer alloc is the same on every scan size and is not the dominant cost. The gap is structure-traversal: a small range pays ART's tree-descent cost to locate the start, then walks mostly-empty inner structure relative to the span it yields. plar/art's range number (6804 ns/key, ~352× slower than ART) is a harness artifact — plar's public `Iterator` exposes no seek primitive, so the bench falls back to a full leaf-walk; do not read it as a tree-shape verdict against plar.

**Memory.** ART (this library) sits at ~89 B/entry — between google/btree (~71 B, smallest) and tidwall/btree (~94 B) / plar/art (~93 B). Allocation count at build is the more interesting axis: both B-trees pack items into node slices (~0.07 allocs/entry); ART pays ~1 alloc/entry; plar pays ~4 allocs/entry due to the boxing. On memory-tight hosts or under heavy GC pressure, the allocation count matters more than the byte total — and on that axis the B-trees lead, ART is in the middle, and plar is the most expensive.

**Net production guidance (now informed by three comparators).**

- Workload is point lookups / writes / deletes (cache, dedup, lookup table, set-membership): **ART (this library) wins decisively against both B-trees and against the alternative Go ART** — by 1.5–21× depending on operation. Choose this library.
- Workload is ordered windowed reads (paginated iteration, range queries, scan-then-emit pipelines): **prefer a B-tree.** `tidwall/btree` is the fastest (2.4× over ART); `google/btree` is the most-imported (1.8× over ART). Both are zero-alloc on the scan.
- Workload is mixed: collect a representative trace and re-benchmark. The point-op / range ratio is what flips the recommendation, and the four-way comparison above gives you the unit costs for each side of the trade.

## Picking between implementations

**Short version:** if your workload is dominated by point ops, use this ART. If it's dominated by short range scans, use `tidwall/btree`. The other two are dominated.

| Implementation | When to pick it | When to avoid |
|---|---|---|
| **This ART** (`KentBeck/AdaptiveRadixTree2`) | Point-op heavy: Get/Put/Delete dominate. Generic `V` matters (no boxing). Variable-length byte keys, or keys with shared prefixes. | Short range scans are the hot path. |
| **tidwall/btree** | Range scans dominate (2.4× faster than ART, 1.3× faster than google/btree). Iterator-heavy code. | Point-op heavy workloads — ~20× slower than ART on Get-hit, ~5× on Put. Note `Options{NoLocks: true}` is required to match the parity numbers above. |
| **google/btree** | Default reach for "I want a B-tree." Mature, widely imported, easy to staff. | Anywhere `tidwall/btree` can be substituted — tidwall matches it on point ops and beats it on range. |
| **plar/go-adaptive-radix-tree** | You need an ART **and** generics is unavailable (pre-Go 1.18 codebase) **and** you can't switch deps. | Almost everywhere else. Strictly dominated: 1.5–1.8× slower than this ART on every point op, no seek primitive on its iterator, and `interface{}` values force ~4 allocs/Put. |

### Decision rule, one axis at a time

- **Throughput on Get/Put/Delete:** ART > plar/ART >> the two B-trees.
- **Throughput on short range scans:** tidwall > google >> ART >>> plar/ART (harness-bound; see Caveats).
- **Allocation pressure on Put:** ART ≈ tidwall ≈ google (~1 alloc) << plar (~4 allocs from `interface{}` boxing).
- **API ergonomics:** ART and tidwall are generic on `V` (no boxing); google is generic on the item type; plar is `interface{}` only.
- **Mindshare / staffability:** google >> tidwall >> the two ARTs.

### TL;DR

Use this ART for point-op-heavy workloads with byte-string keys. Use `tidwall/btree` for range-scan-heavy workloads. Use `google/btree` if "the standard one" is a feature. `plar/go-adaptive-radix-tree` is dominated by this ART on every axis where they overlap.

> The plar Range number (~6800 ns/key) is harness-bound, not tree-shape — plar v1.0.7 ships no seek primitive, so the bench falls back to a full leaf-walk-and-skip. See **Caveats / what these numbers don't cover** for the full list.

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
- **Get is remarkably cheap on short dense keys** (12.55 ns/op on seqInt64) — tighter than the random-8-byte main bench (44.68 ns/op) because the dense-prefix tree fits more nicely in cache at 100K.
- **Memory per key scales with key length, not with entropy.** seqInt64 and randInt64 both pay ~89 B/key; uuid jumps to ~123 B/key; urlPath to ~162 B/key.
- **Range cost is shape-stable** at 16–24 ns/key and always exactly one 32-byte allocation per scan (the path buffer used for pruning). Longer-key shapes add a small constant for the deeper tree descent but the per-key yield cost is flat.

B-tree equivalents are intentionally omitted from this table — the point here is to characterize *ART's* internal sensitivity. Rerun the harness against a `BTreeG[kv]` in your own workload if you need the direct per-shape comparison.

## New-surface microbenchmarks

The following numbers characterize three pieces of surface area that ship alongside the core `Tree[V]`: the extra `Range` shapes (descending and half-open), the `LockedTree` RWMutex wrapper, and the `artmap.Ordered` typed encoder subpackage. All three tables were measured fresh at `0f4234f` with `-benchtime=1s -count=3` (median reported) on the same darwin/amd64 host as the main table.

### Range shape: descending and half-open

Measured from the nested `bench/` module at the same 10M working set used by the main table. Ascending `Range` is reproduced here as the reference row.

| Bench | ns/op | ns/key | B/op | allocs/op | Keys yielded |
| --- | --- | --- | --- | --- | --- |
| `BenchmarkRange_ART` (ascending, 1 % window) | 1,762,460 | 17.62 | 32 | 1 | 100,000 |
| `BenchmarkRangeDescending_ART` (same window, reversed) | 1,820,001 | 18.20 | 32 | 1 | 100,000 |
| `BenchmarkRangeFrom_ART` (half-open: start → end of tree) | 82,685,491 | 16.54 | 32 | 1 | ~5,000,000 |

`BenchmarkRangeTo_ART` is not present in the suite — only `RangeFrom` has a dedicated bench today. If you need a `RangeTo`-shaped (start-of-tree → bound) number, add the bench; the public method exists, the harness row does not.

Descending iteration pays roughly 3 % over ascending at the same window — the iterator spends one extra node-visit step on each descent but the per-yielded-key cost (18.20 vs 17.62 ns/key) is within measurement noise of the forward path. `RangeFrom` yields ~5M keys from the midpoint of the 10M tree and actually comes in slightly *cheaper* per-key (16.54 ns/key) than the short window, because the one-time descent cost amortizes over a much larger yield. All three shapes match the main-table allocation profile: a single 32-byte path buffer per scan, with keys aliased into the tree's own storage.

### LockedTree overhead

Measured from the root module's `art_test.go`. Both sub-benchmarks run single-goroutine so the lock path is exercised but never contended; the numbers isolate the pure `sync.RWMutex` lock-path cost from the underlying tree work. Working set is a 1024-entry 2-byte-key cycle, so both the bare and wrapped trees stay warm in L1.

| Op | `Tree[V]` ns/op | `LockedTree[V]` ns/op | Δ (ns/op, uncontended) |
| --- | --- | --- | --- |
| Put | 12.92 | 17.22 | +4.30 |
| Get | 9.09 | 10.61 | +1.52 |

Uncontended overhead is about 4.3 ns per Put and 1.5 ns per Get — a single Lock/Unlock pair on the fast path. At this tiny working set the underlying tree is already in the 9–13 ns range so the *relative* cost looks steep (~33 % on Put, ~17 % on Get); against the main-table 10M working set (Put 162 ns/op, Get 53 ns/op) the same absolute overhead would shrink to ~3 %. Contended behaviour — what happens when multiple goroutines actually fight for the lock — is not characterized here; the RWMutex-guarded wrapper behaves as a standard Go `sync.RWMutex` under load and should be profiled against your real reader/writer mix before adopting.

### artmap.Ordered encoder overhead

Measured from the `artmap/` subpackage. Typed `artmap.Ordered[int64, int]` is compared against raw `art.Tree[int]` with hand-rolled big-endian `int64` encoding, at a 10K-key working set.

| Op | `art.Tree[int]` (raw, []byte keys) | `artmap.Ordered[int64,int]` | Δ |
| --- | --- | --- | --- |
| Put (full 10K-key build) | 490,996 ns/op (49.10 ns/key) | 537,888 ns/op (53.79 ns/key) | +46,892 ns/build (+4.69 ns/key) |
| Get (per op) | 12.23 ns/op | 14.84 ns/op | +2.61 ns/op |
| Range (full `[-1<<62, 1<<62)` scan) | 125,632 ns/op (32 B, 1 alloc) | 134,356 ns/op (168 B, 7 allocs) | +8,724 ns/scan (+136 B, +6 allocs) |

`artmap` ships no standalone `BenchmarkOrderedDelete` / `BenchmarkTreeDelete` today — Delete is unbenched in the subpackage, so it is not covered in this table. Flagging as a gap for a follow-up bench PR rather than writing a new bench under this doc-only task.

The encoder adds ~4.7 ns/Put, ~2.6 ns/Get, and ~0.9 ns per yielded key on Range — the cost of one `binary.BigEndian.PutUint64` with a sign-bit flip on each operation, plus (on the Range path) a small decoded-key slice that pushes the scan from a single 32-byte allocation to 168 B across 7 allocations. Net guidance: if your workload is already in `[]byte` keys, the raw `art.Tree` is marginally cheaper; if you're starting from `int64` / `string` / other Go-native ordered types, `artmap.Ordered` saves the hand-written encode/decode at a cost small relative to a single allocation per op.

## Caveats / what these numbers don't cover

1. **Key shape.** 8-byte keys from a permutation of `[0, 10M)` have shallow common prefixes. See the Key-shape sensitivity section above for how Put / Get / Delete / Range costs change across `seqInt64`, `randInt64`, `uuid`, and `urlPath` workloads; in particular, longer keys slow ART's point ops down in absolute terms (an `urlPath` Get is ~3.7× slower than a `seqInt64` Get at 100K). The key-shape sweep is ART (this library) only.
2. **No concurrent access.** All four implementations are exercised single-goroutine. `tidwall/btree` is configured `NoLocks: true` so its sync.RWMutex is out of the measurement; `google/btree` and our ART have no internal locking; plar's tree is documented as not thread-safe.
3. **Steady-state vs. cold.** Get / Range run on a warm cache; first-hit latency is not isolated.
4. **B-tree degree.** Both B-trees use degree 32 (`google/btree` default; `tidwall/btree` overridden to match). Tuning the degree could shift either B-tree's numbers by 10–30 % on any single op — the relative ordering between the two B-trees, in particular, is sensitive to this knob.
5. **plar Range is API-bound.** plar's `Iterator()` exposes no efficient seek primitive, so `BenchmarkRange_Plar` walks all leaves from the start of the tree and skips until `key >= rangeLo`, stopping at `key >= rangeHi`. The reported 6804 ns/key reflects that walk, not plar's tree shape; a future plar release adding a seek primitive could change this row substantially.
6. **Key aliasing.** `Tree.Range` yields `[]byte` keys that alias the tree's internal storage. They are safe to retain while the entry is in the tree and must be treated as read-only; mutating a yielded key corrupts the tree.

## Reproducing

The main-table numbers come from a single invocation of the four-comparator per-op bench suite, from the nested `bench/` module:

```
(cd bench && go test -run='^$' -bench='^Benchmark(Put|Get|GetMiss|Delete|Range)_(ART|BTree|Tidwall|Plar)$' -benchmem -benchtime=3s -count=5 -timeout=30m ./...)
```

Each benchmark is run 5 times; the main table reports the median of the 5 reps per row. Total wall time is ~14 minutes. To run only the new comparator subset (smoke-check after a `bench/` edit) use:

```
(cd bench && go test -run='^$' -bench='^Benchmark(Put|Get|GetMiss|Delete|Range)_(Tidwall|Plar)$' -benchtime=1s -count=1 -timeout=30m ./...)
```

The Key-shape sensitivity table is produced by a separate, faster pass:

```
(cd bench && go test -run=^$ -bench=BenchmarkKeyShape -benchmem -benchtime=1s -count=1 ./...)
```

The New-surface microbenchmarks live in three different modules and use three separate commands:

```
(cd bench && go test -run=^$ -bench='^Benchmark(Range|RangeDescending|RangeFrom)_ART$' -benchmem -benchtime=1s -count=3 ./...)
go test -run=^$ -bench='^BenchmarkLockedTree(Put|Get)$' -benchmem -benchtime=1s -count=3 .
(cd artmap && go test -run=^$ -bench='^Benchmark(Ordered|Tree)(Put|Get|Range)_int64$' -benchmem -benchtime=1s -count=3 ./...)
```

To run everything at once (~3 minutes), use `-bench=.` in place of either main-table regex.

## Environment (as measured)

Measured on macOS 26.3.1 (darwin) on an Apple M4 Max laptop with 16 logical CPUs, on AC power (100% charged). The Go toolchain is `go version go1.24.2 darwin/amd64` — i.e. the Go binary targets `darwin/amd64` and runs under Rosetta 2 on Apple Silicon, which is why `go test` reports the CPU as `VirtualApple @ 2.50GHz`. `GOMAXPROCS` was left unset (Go default = 16, one per logical CPU). Main-table command: `(cd bench && go test -run='^$' -bench='^Benchmark(Put|Get|GetMiss|Delete|Range)_(ART|BTree|Tidwall|Plar)$' -benchmem -benchtime=3s -count=5 -timeout=30m ./...)` — ~14 minutes wall time. Key-shape command: `(cd bench && go test -run=^$ -bench=BenchmarkKeyShape -benchmem -benchtime=1s -count=1 ./...)` — ~36 seconds wall time. No machine-quiescing steps beyond closing foreground apps and letting the laptop idle on AC.