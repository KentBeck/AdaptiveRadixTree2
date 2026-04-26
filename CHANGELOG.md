# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.2] - 2026-04-26

### Changed (docs)
- `benchmarks.md` per-operation results now compare ART against three
  implementations rather than one. The `bench/` harness gained `_Tidwall`
  (`github.com/tidwall/btree` v1.8.1, configured with
  `Options{NoLocks: true, Degree: 32}` for parity with `google/btree`'s default
  degree and to keep its sync.RWMutex out of the measurement) and `_Plar`
  (`github.com/plar/go-adaptive-radix-tree` v1.0.7, whose public API stores
  `interface{}` values — the per-Put boxing alloc that costs is structural to
  the API and is documented in-table) siblings of `Put`, `Get`, `GetMiss`,
  `Delete`, and `Range`. The Verdict prose has been rewritten to reflect the
  new three-comparator landscape rather than the prior google/btree-only
  framing. No core `.go` files were edited; the change is confined to the
  nested `bench/` module and the `benchmarks.md` doc.

## [0.5.1] - 2026-04-25

### Changed (internal)
- Deduplicated the four-way prefix-consume / terminal-check preambles in
  `get.go`, `delete.go`, and `iterate.go` behind three free helpers in
  `helpers.go` (`consumePrefix`, `terminalValue[V]`, `yieldTerminalInRange[V]`).
  −40 net LOC. No additions to the `node` / `innerNode` interfaces; every
  helper is a free function over concrete types so no interface-method
  indirection is introduced (`runtime.getitab` remains absent from Get and
  Delete pprof profiles). `put.go` is intentionally left expanded — sharing
  a helper across the prefix-split allocation boundary would reintroduce
  interface dispatch on the hot path, per the reverted polymorphism spike
  recorded in `polymorphism-failed.md`. Hot-path benchmarks at parity with
  v0.5.0 within run variance.

### Fixed
- `artmap/ordered_test.go`: the `TestOrdered_Float64_OrderAcrossZero` fixture
  now uses `math.Copysign(0, -1)` for actual IEEE-754 negative zero. The
  prior `-0.0` literal was folded to `+0.0` by the parser (staticcheck
  SA4026); the test now exercises the intended sign-bit case.

### CI
- The fuzz-smoke workflow step now discovers `Fuzz*` targets dynamically
  across all packages and runs each one scoped to its own package. Replaces
  a `go test -fuzz=… ./...` invocation that was rejected by Go with
  "cannot use -fuzz flag with multiple packages" once the `artmap`
  subpackage was introduced. Future fuzz targets in any package are picked
  up automatically; an explicit empty-set guard fails the step if no
  targets are discovered.

## [0.5.0] - 2026-04-23

### Added
- `Tree[V]` descending iteration and open-ended range methods: `AllDescending`, `RangeFrom`, `RangeTo`, `RangeDescending`.
- `artmap` subpackage exposing `Ordered[K cmp.Ordered, V any]`, a typed sorted-map façade over byte-keyed ART with order-preserving encoders for the `cmp.Ordered` types. Encoder overhead on int64 keys: +2.6 ns/op (Put), +2.5 ns/op (Get), +1.3 ns/op (Delete).
- `art.LockedTree[V]`: a `sync.RWMutex` wrapper exposing `Put`, `Get`, `Delete`, `Len`, `Clear`, and `Clone`. Uncontended `Get` overhead ~0.8 ns/op.

### Performance
- De-parameterised the internal node interface so that `V` appears only on `Tree[V]` and the leaf. `Delete` (bulk 10M, 8-byte keys): −39 ns/key (~34 % faster). `Get` (hit, 10M, 8-byte keys) showed a +8.5 ns/op regression at landing (`d8df19e`), but investigation (findings note `2a29d2a7`) attributed the gap to code-layout / i-cache sensitivity rather than the node-interface shape; the regression self-healed across subsequent layout-affecting commits. Re-measured at the release commit, `Get` is 43.68 ns/op — at parity with (slightly below) the pre-E-D2 baseline.

### Documented
- `benchmarks.md` expanded with three-column-per-engine tables (ns/op · B/op · allocs/op) and a key-shape sensitivity section covering `seqInt64` / `randInt64` / `uuid` / `urlPath`.

### Stability
- README now carries a `## Stability` section enumerating the exported surface of both packages that will be frozen at v1.0 and committing to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) from that tag forward. Target for v1.0.0 is no earlier than 2026-07-23.

## [0.4.1] - 2026-04-22

### Performance
- `Delete` (bulk 10M, 8-byte keys): 124.5 → 115.3 ns/key (−9 ns/key, ~7 %). Size tracking now happens at the `insertLeaf` / `clearTerminalIfMatches` chokepoints instead of being propagated through recursive return tuples; parent frames detect no-op by pointer equality. A residual ~45 ns/key gap vs the pre-generics baseline (70.0 ns/key) remains, tracked for future work.
- `Get` (hit, 10M, 8-byte keys): 57.57 → 44.81 ns/op, fully recovered to the pre-generics baseline (43.22 ns/op). Code-layout / register-pressure side effect of the put/delete refactor.

### Documented
- `benchmarks.md` re-baselined against this release; "regression" annotations removed / adjusted.

## [0.4.0] - 2026-04-21

### Documented
- `benchmarks.md` re-baselined against `b73719f`. Flagged `Get` (hit) (+33 %) and `Delete` (+78 %) regressions vs the pre-generics baseline for investigation.

## [0.3.0] - 2026-04-21

### Added
- GitHub Actions CI workflow (build, vet, staticcheck, test, short fuzz).
- Nested `bench/` module so the main module has zero runtime dependencies.
- Seed corpus committed for `FuzzSortedMap`.

### Changed
- Removed unused `isEmpty()` methods from inner node types to unblock `staticcheck`.

## [0.2.0] - 2026-04-21

### Added
- Sorted-map surface: `Min`, `Max`, `Ceiling`, `Floor`, `Clone`, `Clear`.
- Six `ExampleTree_*` functions for the new methods.
- Fuzzer now exercises the new operations against a sorted oracle.

### Documented
- Nil-key / empty-key equivalence contract.

## [0.1.0] - 2026-04-21

### Added
- Initial generic `Tree[V any]` public API (`New`, `Put`, `Get`, `Delete`, `Len`, `All`, `Range`).
- Adaptive radix tree core: node4/16/48/256 with promotion and demotion.
- Path compression with prefix splitting and terminal-carrying collapse.
- Inline small-key buffer (≤ 24 bytes) to halve Put allocations.
- Differential fuzzer against `map[string]V` + sorted oracle (45M+ execs, zero divergences).
- Mutation testing (`gremlins`) with 96.55 % measured efficacy (100 % of killable mutants).
- `example_test.go` with six verified examples.
- Package documentation (`doc.go`) and goroutine-safety contract on `Tree`.

[0.5.1]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/KentBeck/AdaptiveRadixTree2/releases/tag/v0.1.0

