# art — Adaptive Radix Tree for Go

A sorted map implementation using an Adaptive Radix Tree (ART). Fast lookups, path compression, and sorted iteration with Go 1.23 range-over-func.

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
go get github.com/kentbeck/art
```

Requires Go 1.23 or later (for `iter.Seq2` / range-over-func).

## Quick start

```go
package main

import (
	"fmt"

	"github.com/kentbeck/art"
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

**Node types.** There are four inner node types (`node4`, `node16`, `node48`, `node256`) plus a `leaf`. Inner nodes grow and shrink based on child count, so each node's capacity matches its fanout. Each inner node may also carry a `prefix` (for path compression) and an optional `terminal` leaf holding a value for a key that ends exactly at that node.

**The `innerNode` interface.** All four inner node types implement a common `innerNode` interface covering put/get/delete/iterate plus prefix and terminal accessors. Public methods on `Tree` dispatch through this interface, so each operation is expressed once per node type.

**File organization.**

| File | Purpose |
|------|---------|
| `types.go` | Node structs, `innerNode` interface, lifecycle (grow/shrink/addChild), operation methods |
| `put.go` | `Tree.Put` dispatcher |
| `get.go` | `Tree.Get` dispatcher |
| `delete.go` | `Tree.Delete` dispatcher + `postDeleteReshape` collapse logic |
| `iterate.go` | `Tree.All`, `Tree.Range`, iteration helpers |
| `helpers.go` | Shared pure functions: `longestCommonPrefixLen`, `newNode4With`, `splitPrefixedInner`, `newLeaf` |
| `art_test.go` | 44 unit tests |
| `art_fuzz_test.go` | `FuzzSortedMap` differential fuzzer + 6 seed inputs |

**Invariants.**

- Children of `node4` and `node16` are stored sorted ascending by edge byte.
- A `terminal` leaf at an inner node has a key equal to that node's full path from the root.
- After `Delete`, a node with 0 children and no terminal is removed; a node with exactly 1 remaining child collapses (terminal-only, leaf-only, or prefix-merge into its sole child).

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

