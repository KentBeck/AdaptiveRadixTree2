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
- Descending and open-ended iteration: `AllDescending`, `RangeFrom`, `RangeTo`, `RangeDescending`
- Sorted-map accessors: `Min`, `Max`, `Ceiling`, `Floor`, plus `Len`, `Clear`, `Clone`
- Path compression (shared key prefixes stored once)
- Terminal values (keys that are prefixes of other keys are stored correctly)
- Adaptive node types (node4 / node16 / node48 / node256) with automatic promote/demote
- `LockedTree[V]` wrapper for `sync.RWMutex`-guarded concurrent access
- `artmap.Ordered[K, V]` typed façade for `cmp.Ordered` keys (integers, floats, strings) with byte-order-preserving encoding
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
	tree := art.New[int]()
	tree.Put([]byte("apple"), 1)
	tree.Put([]byte("apricot"), 2)
	tree.Put([]byte("banana"), 3)

	if v, ok := tree.Get([]byte("apple")); ok {
		fmt.Println("apple ->", v)
	}

	fmt.Println("size:", tree.Len())

	if k, v, ok := tree.Min(); ok {
		fmt.Printf("min: %s -> %d\n", k, v)
	}
	if k, v, ok := tree.Max(); ok {
		fmt.Printf("max: %s -> %d\n", k, v)
	}

	// Sorted iteration (ascending).
	for k, v := range tree.All() {
		fmt.Printf("%s -> %d\n", k, v)
	}

	// Reverse iteration.
	for k, v := range tree.AllDescending() {
		fmt.Printf("%s -> %d\n", k, v)
	}

	// Range scan: keys in [start, end).
	for k, v := range tree.Range([]byte("ap"), []byte("b")) {
		fmt.Printf("%s -> %d\n", k, v)
	}

	// Open-ended range: all keys >= "b".
	for k, v := range tree.RangeFrom([]byte("b")) {
		fmt.Printf("%s -> %d\n", k, v)
	}

	tree.Delete([]byte("banana"))
}
```

## API reference

### `art.Tree[V any]`

- `func New[V any]() *Tree[V]` — create an empty tree.
- `func (t *Tree[V]) Put(key []byte, value V)` — insert or overwrite the value at `key`.
- `func (t *Tree[V]) Get(key []byte) (V, bool)` — look up `key`; `ok` is false if absent.
- `func (t *Tree[V]) Delete(key []byte) bool` — remove `key`; returns whether it was present.
- `func (t *Tree[V]) Len() int` — current number of entries, O(1).
- `func (t *Tree[V]) Clear()` — drop every entry in O(1).
- `func (t *Tree[V]) Clone() *Tree[V]` — independent structural copy.
- `func (t *Tree[V]) Min() (key []byte, value V, ok bool)` — smallest entry.
- `func (t *Tree[V]) Max() (key []byte, value V, ok bool)` — largest entry.
- `func (t *Tree[V]) Ceiling(target []byte) (key []byte, value V, ok bool)` — smallest key ≥ `target`.
- `func (t *Tree[V]) Floor(target []byte) (key []byte, value V, ok bool)` — largest key ≤ `target`.
- `func (t *Tree[V]) All() iter.Seq2[[]byte, V]` — every `(key, value)` pair in ascending order.
- `func (t *Tree[V]) AllDescending() iter.Seq2[[]byte, V]` — every pair in descending order.
- `func (t *Tree[V]) Range(start, end []byte) iter.Seq2[[]byte, V]` — pairs with key in `[start, end)`, ascending.
- `func (t *Tree[V]) RangeFrom(start []byte) iter.Seq2[[]byte, V]` — pairs with key ≥ `start`, ascending.
- `func (t *Tree[V]) RangeTo(end []byte) iter.Seq2[[]byte, V]` — pairs with key < `end`, ascending.
- `func (t *Tree[V]) RangeDescending(start, end []byte) iter.Seq2[[]byte, V]` — pairs with key in `[start, end)`, descending.

### `art.LockedTree[V any]`

A `sync.RWMutex`-guarded wrapper around `Tree[V]` — see the Concurrency section below.

- `func NewLocked[V any]() *LockedTree[V]` — create an empty locked tree.
- `func (t *LockedTree[V]) Put(key []byte, value V)`
- `func (t *LockedTree[V]) Get(key []byte) (V, bool)`
- `func (t *LockedTree[V]) Delete(key []byte) bool`
- `func (t *LockedTree[V]) Len() int`
- `func (t *LockedTree[V]) Clear()`
- `func (t *LockedTree[V]) Clone() *Tree[V]` — returns an unlocked `*Tree[V]` snapshot.

### `artmap.Ordered[K, V]`

For typed keys (any `cmp.Ordered` type: integers, floats, strings), the [`artmap`](artmap/ordered.go) subpackage provides `Ordered[K, V]`, a façade that encodes keys with a byte-order-preserving codec on top of `art.Tree[V]`. It exposes the same sorted-map surface as `Tree[V]` — `Put`, `Get`, `Delete`, `Len`, `Clone`, `Min`, `Max`, `Ceiling`, `Floor`, `All`, `AllDescending`, `Range`, `RangeFrom`, `RangeTo`, `RangeDescending` — with `K` in place of `[]byte`. See [`artmap/ordered.go`](artmap/ordered.go) for the exact signatures.

`Range` nil semantics:

- `Range(nil, nil)` is equivalent to `All()`.
- `Range(start, nil)` yields all keys ≥ `start`.
- `Range(nil, end)` yields all keys < `end`.
- `Range(start, end)` with `bytes.Compare(start, end) >= 0` yields nothing.
- The empty slice (`[]byte{}`) is a valid key and is distinct from `nil`.

## Architecture

**Node types.** There are four inner node types (`node4`, `node16`, `node48`, `node256`) plus a `leaf`. Inner nodes grow and shrink based on child count. Each inner node may carry a `prefix` (for path compression) and an optional `terminal` leaf holding a value for a key that ends exactly at that node.

**The `innerNode` interface.** All four inner node types implement a minimal `innerNode` interface covering `findChild` and `removeChild` (plus `kind()` via the embedded `node` interface). Operations (`Put`, `Get`, `Delete`, iteration) are implemented as standalone functions with switch statements dispatching on node type.

**File organization.**

| File | Purpose |
|------|---------|
| `types.go` | Node structs, `innerNode` interface, node lifecycle (grow/shrink/addChild/replaceChild/removeChild), `Tree[V]` definition, `New` constructor, `Len` |
| `put.go` | `Tree.Put` + `putInto` dispatcher + `putIntoNode4/16/48/256` helpers |
| `get.go` | `Tree.Get` with inline switch over node types |
| `delete.go` | `Tree.Delete` + `deleteFrom` switch + `postDeleteReshape` collapse logic |
| `iterate.go` | `Tree.All`, `Tree.AllDescending`, `Tree.Range`, `Tree.RangeFrom`, `Tree.RangeTo`, `Tree.RangeDescending` + `iterate`/`iterateRange` switches |
| `sorted.go` | `Tree.Min`, `Tree.Max`, `Tree.Ceiling`, `Tree.Floor`, `Tree.Clone`, `Tree.Clear` + shared `*LeafOf` / `cloneNode` helpers |
| `locked.go` | `LockedTree[V]` wrapper and `NewLocked` constructor |
| `helpers.go` | Shared pure functions: `longestCommonPrefix`, `newNode4With`, `splitPrefixedInner`, `newLeaf` |
| `doc.go` | Package doc comment |
| `art_test.go` | 116 unit tests |
| `art_fuzz_test.go` | `FuzzSortedMap` differential fuzzer + 9 seed inputs |
| `artmap/` | Typed `Ordered[K, V]` façade over `art.Tree[V]` with byte-order-preserving key codec (`codec.go`, `ordered.go`) |

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

## Concurrency

`art.Tree[V]` and `artmap.Ordered[K, V]` are **not** safe for concurrent use by multiple goroutines when any goroutine is writing. Concurrent reads (`Get`, `All`, `Range`, `Min`, `Max`, `Ceiling`, `Floor`, `Len`) are safe only while no goroutine is calling a mutating method (`Put`, `Delete`, `Clear`). The tree has no internal synchronization; races are undefined behaviour, not a panic.

For the common read-mostly case, wrap your tree with a `sync.RWMutex` you own:

```go
var (
    mu   sync.RWMutex
    tree = art.New[int]()
)

mu.Lock(); tree.Put(key, v); mu.Unlock()

mu.RLock(); v, ok := tree.Get(key); mu.RUnlock()
```

As a convenience, the package ships `art.LockedTree[V]`: a thin `sync.RWMutex`-guarded wrapper exposing `Put`, `Get`, `Delete`, `Len`, `Clear`, and `Clone`. It is intentionally narrow — iteration is not wrapped, since holding an `RLock` across a user-controlled `yield` is easy to mis-use. Code that needs an ordered scan under a lock should `Clone()` the tree and iterate the unlocked snapshot.

```go
t := art.NewLocked[int]()
t.Put([]byte("apple"), 1)
v, ok := t.Get([]byte("apple"))

snap := t.Clone()
for k, v := range snap.All() { /* safe: snap is not shared */ _, _ = k, v }
```

Copy-on-write, lock-free, and RCU variants are out of scope for this release; `LockedTree` is the supported concurrent surface.

## Stability

art is pre-1.0 (currently v0.4.x). Until v1.0.0 is tagged, the public API may change in minor-version bumps; patch releases stay API-compatible. The target for tagging **v1.0.0 is no earlier than 2026-07-23**, giving the recent additions (the typed `artmap.Ordered[K, V]` façade, the descending and half-open `Range*` helpers, and the `LockedTree[V]` wrapper) time to settle before the signatures are locked down.

From v1.0 onward the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html): breaking changes to the surface listed below only land in a new major version.

**Frozen at v1.0.** The exported surface of both packages as of v0.4.1:

- Package `art` (module root):
  - Types: `Tree[V any]`, `LockedTree[V any]`.
  - Constructors: `New[V any]() *Tree[V]`, `NewLocked[V any]() *LockedTree[V]`.
  - `Tree[V]` methods: `Put`, `Get`, `Delete`, `Len`, `Clear`, `Clone`, `Min`, `Max`, `Ceiling`, `Floor`, `All`, `AllDescending`, `Range`, `RangeFrom`, `RangeTo`, `RangeDescending`.
  - `LockedTree[V]` methods: `Put`, `Get`, `Delete`, `Len`, `Clear`, `Clone`.
- Package `artmap`:
  - Types: `OrderedKey` (alias for `cmp.Ordered`), `Ordered[K OrderedKey, V any]`.
  - Constructor: `New[K OrderedKey, V any]() *Ordered[K, V]`.
  - `Ordered[K, V]` methods: `Put`, `Get`, `Delete`, `Len`, `Clone`, `Min`, `Max`, `Ceiling`, `Floor`, `All`, `AllDescending`, `Range`, `RangeFrom`, `RangeTo`, `RangeDescending`.

Signatures and documented behaviour (including the `Range` nil / half-open semantics and the goroutine-safety contract above) of these symbols are covered by the SemVer compatibility promise from v1.0 forward.

**Not frozen.** The following remain free to change under a minor or patch release, before and after v1.0:

- Internal package layout — inner node types (`node4`/`node16`/`node48`/`node256`), file organization, and unexported helpers.
- Performance characteristics and specific benchmark numbers; improvements or regressions are not breaking changes.
- The `artmap` byte-order-preserving key encoding is an implementation detail — treat encoded keys as opaque and do not persist them across versions.
- Fuzz corpus layout under `testdata/`.

Breaking changes during the 0.x series are recorded in [CHANGELOG.md](CHANGELOG.md).

## Testing

```sh
go test ./...                                           # 116 unit tests + 9 fuzz seed runs
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

