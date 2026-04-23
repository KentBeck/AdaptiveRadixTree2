# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.4.1...HEAD
[0.4.1]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/KentBeck/AdaptiveRadixTree2/releases/tag/v0.1.0

