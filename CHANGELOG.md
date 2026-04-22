# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/KentBeck/AdaptiveRadixTree2/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/KentBeck/AdaptiveRadixTree2/releases/tag/v0.1.0

