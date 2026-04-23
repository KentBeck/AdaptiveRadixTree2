# art — Adaptive Radix Tree for Go

[![test](https://github.com/KentBeck/AdaptiveRadixTree2/actions/workflows/test.yml/badge.svg)](https://github.com/KentBeck/AdaptiveRadixTree2/actions/workflows/test.yml)

A sorted map implementation using an Adaptive Radix Tree (ART). Fast lookups, path compression, and sorted iteration with Go 1.23 range-over-func.

See [CHANGELOG.md](CHANGELOG.md) for release notes.

The ulterior goals of this project are to get the genie to write code that is:
- Fast
- Reliable
- Readable (by humans & genies)

## What is an Adaptive Radix Tree?

An Adaptive Radix Tree is a trie in which every inner node uses a variable-size container sized to its actual fanout. Instead of reserving 256 child slots at every level, ART picks among four node types — holding up to 4, 16, 48, or 256 children — and promotes or demotes a node as children are added and removed. Memory stays proportional to the branching that actually occurs in the data, not to the alphabet size.

Because keys are indexed byte by byte, ART iterates keys in sorted (byte-wise lexicographic) order for free, lookups take O(k) time in the key length k, and path compression collapses chains of single-child nodes so that long shared prefixes are stored once. These properties make ART a practical in-memory ordered map: small enough to compete with hash tables on lookup, yet ordered like a balanced BST.

The data structure was introduced by Leis, Kemper, and Neumann in ["The Adaptive Radix Tree: ARTful Indexing for Main-Memory Databases"](https://db.in.tum.de/~leis/papers/ART.pdf) (ICDE 2013).

## Features

- `Put`, `Get`, `Delete` with O(k) complexity where k is key length
- Sorted iteration via Go 1.23 range-over-func: `All()` and `Range(start, end)`
- Path compression (shared key prefixes stored once)
- Terminal values (keys that are prefixes of other keys are stored correctly)
- Adaptive node types (node4 / node16 / node48 / node256) with automatic promote/demote
- Fuzz-tested against Go's built-in map (45M+ executions, 0 divergences as of last campaign)

## Installation

```sh
go get github.com/KentBeck/AdaptiveRadixTree2
```

Requires Go 1.23 or later (for `iter.Seq2` / range-over-func).

## Quick start

```go
package main

import (
	"fmt"

	"github.com/KentBeck/AdaptiveRadixTree2"
)

func main() {
	tree := art.New()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)
	tree.Put([]byte("banana"), 3)

	if v, ok := tree.Get([]byte("apple")); ok {
		fmt.Println("apple ->", v)
	}

	// Sorted iteration.
	for k, v := range tree.All() {
		fmt.Printf("%s -> %v\n", k, v)
	}

	// Range scan: keys in [start, end).
	for k, v := range tree.Range([]byte("ap"), []byte("b")) {
		fmt.Printf("%s -> %v\n", k, v)
	}

	tree.Delete([]byte("banana"))
}
```

## API reference

- `func New() *Tree` — create an empty tree.
- `func (t *Tree) Put(key []byte, value any)` — insert or overwrite the value at `key`.
- `func (t *Tree) Get(key []byte) (any, bool)` — look up `key`; `ok` is false if absent.
- `func (t *Tree) Delete(key []byte) bool` — remove `key`; returns whether it was present.
- `func (t *Tree) All() iter.Seq2[[]byte, any]` — range over all `(key, value)` pairs in sorted order.
- `func (t *Tree) Range(start, end []byte) iter.Seq2[[]byte, any]` — range over keys in `[start, end)`.

`Range` nil semantics:

- `Range(nil, nil)` is equivalent to `All()`.
- `Range(start, nil)` yields all keys ≥ `start`.
- `Range(nil, end)` yields all keys < `end`.
- `Range(start, end)` with `bytes.Compare(start, end) >= 0` yields nothing.
- The empty slice (`[]byte{}`) is a valid key and is distinct from `nil`.

## Architecture

**Node types.** There are four inner node types (`node4`, `node16`, `node48`, `node256`) plus a `leaf`. Inner nodes grow and shrink based on child count. Each inner node may carry a `prefix` (for path compression) and an optional `terminal` leaf holding a value for a key that ends exactly at that node.

**The `innerNode` interface.** All four inner node types implement a minimal `innerNode` interface covering `findChild`, `removeChild`, and `isEmpty`. Operations (`Put`, `Get`, `Delete`, iteration) are implemented as standalone functions with switch statements dispatching on node type.

**File organization.**

| File | Purpose |
|------|---------|
| `types.go` | Node structs, `innerNode` interface, node lifecycle (grow/shrink/addChild/replaceChild/removeChild) |
| `put.go` | `Tree.Put` + `putInto` dispatcher + `putIntoNode4/16/48/256` helpers |
| `get.go` | `Tree.Get` with inline switch over node types |
| `delete.go` | `Tree.Delete` + `deleteFrom` switch + `postDeleteReshape` collapse logic |
| `iterate.go` | `Tree.All`, `Tree.Range` + `iterate`/`iterateRange` switches |
| `helpers.go` | Shared pure functions: `longestCommonPrefix`, `newNode4With`, `splitPrefixedInner`, `newLeaf` |
| `art_test.go` | 44 unit tests |
| `art_fuzz_test.go` | `FuzzSortedMap` differential fuzzer + 6 seed inputs |

**Invariants.**

- Children of `node4` and `node16` are stored sorted ascending by edge byte.
- A `terminal` leaf at an inner node has a key equal to that node's full path from the root.
- After `Delete`, a node with 0 children and no terminal is removed; a node with exactly 1 remaining child collapses (terminal-only, leaf-only, or prefix-merge into its sole child).

## When to choose artmap

This project ships two related types: the raw-bytes `art.Tree` and the typed façade [`artmap.Ordered[K, V]`](artmap/ordered.go) built on top of it. Both are in-memory ordered maps with one clear sweet spot and two honest trade-offs against `google/btree`. Full numbers live in [benchmarks.md](benchmarks.md); the short guidance is below.

If your keys are a single `cmp.Ordered` type (integers, floats, strings) rather than raw `[]byte`, reach for `artmap.Ordered[K, V]`: it uses byte-order-preserving encoders so the underlying tree preserves the natural ascending order of `K` without the caller hand-encoding keys.

| Workload shape | Recommended |
| --- | --- |
| Point-heavy reads / writes (cache, dedup, lookup, set-membership) | `art.Tree` / `artmap.Ordered` |
| Short random-access lookups on long or UUID-shaped keys | `art.Tree` / `artmap.Ordered` |
| Large ordered scans or windowed pagination (scan-heavy) | `google/btree` |
| Heavy mutation under GC pressure (tight alloc budget) | `google/btree` |
| Small dataset (< 10k keys) | Either — pick on language fit |
| Mostly-ordered iteration with occasional point ops | `google/btree` |

**The Range trade-off.** On short ordered scans (the 1 %, 100k-entry window in the bench), `google/btree` is ~1.74× faster than `art.Tree`. Both are essentially zero-alloc on the scan, so the gap is pure traversal cost: the ART descent pays to locate the start and then walks through mostly-empty inner structure relative to the span it yields. `Range`, `RangeFrom`, `RangeTo`, `RangeDescending`, and `AllDescending` cover the common ordered-iteration shapes, but if ordered scans dominate your workload, `google/btree` is still the right pick.

**The allocation trade-off.** `Put` on `art.Tree` averages ~1.02 allocs/key at 10M; `google/btree` averages ~0.07 allocs/key — about 15× fewer allocations per key at build time. Under heavy GC pressure or on memory-tight hosts, the allocation count matters more than the ~25 % byte-total difference.

### Quality

- CI: the [`test` workflow](https://github.com/KentBeck/AdaptiveRadixTree2/actions/workflows/test.yml) (badge at the top of this README) runs build, vet, staticcheck, unit tests, and a short fuzz campaign on every push.
- Fuzz corpus: [`testdata/fuzz/FuzzSortedMap`](testdata/fuzz/FuzzSortedMap); cumulative executions have exceeded 45M across campaigns with zero divergences against the `map[string]V` + sorted-oracle reference.
- Mutation testing: ~95 % measured efficacy under `gremlins` (100 % of killable mutants at the v0.1.0 baseline; see [`CHANGELOG.md`](CHANGELOG.md) for per-release figures).

## Testing

```sh
go test ./...                                           # 44 unit tests + 6 fuzz seed runs
go test -run=^$ -fuzz=FuzzSortedMap -fuzztime=30s ./... # differential fuzz
go vet ./... && gofmt -l .                              # static checks
```

The fuzz harness cross-checks every operation against Go's built-in `map[string]any` plus a sorted-slice oracle, so any divergence in value, presence, or iteration order surfaces immediately.

## Contributing

- Run the full test suite plus a fuzz campaign of at least 30 seconds before opening a PR.
- Keep `go vet` and `gofmt` clean.
- Prefer the existing file organization: one file per public operation.
- If you add a new operation, expect to add a method to `innerNode` and implement it on all four inner node types.

## License

Licensed under the MIT License. See [`LICENSE`](LICENSE) for details.

