# Chapter 3 — Path compression

Chapter 2 left two structural waste cases on the table. The first
was the URL row of the bytes/key table:

> URL: 1 787 B/key, 9× tighter than chapter 1, still 25× looser than btree

Lazy expansion fixed the unique-tail problem (one leaf per key,
no chain). It did *not* fix the shared-prefix problem: two URL
keys agreeing on `"https://api.example.com/v1/users/"` (33 bytes)
still allocated 33 inner node256s along that shared path, each
with one child. Each one weighs ~2 KB.

Path compression collapses each such chain into a single inner
node whose `prefix` field stores the run of bytes consumed before
the first branching byte. One node, one allocation, regardless of
how long the shared prefix is.

## What changes

```go {src=art.go decl=node256}
type node256[V any] struct {
	prefix   []byte
	children [256]node
	terminal *leaf[V]
}
```

That's the whole structural change. An inner node now represents
*either* a single byte position (`prefix == ""`) or any number of
consecutive byte positions (`len(prefix) > 0`). The empty prefix
is legal and common — the root of a tree whose first bytes
diverge has it.

Three semantic rules follow:

- **Get and Delete** must verify the prefix matches the next bytes
  of the key before descending. A mismatch is proof of absence.
- **Put** has three new outcomes when it meets an existing inner
  node: the prefix matches in full (descend past it); the prefix
  matches and the key ran out (split prefix at the cut, new
  parent gets a terminal); the prefix matches partially and both
  diverge (split prefix at the divergence, new parent has two
  branching children).
- **Delete reshape** picks up a new collapse case: when an inner
  node ends up with one *inner* child and no terminal, merge the
  child's prefix in. Without prefix compression (chapter 2) this
  case had to be left alone because there was nowhere to record
  the byte the parent consumed.

## consumePrefix: the one performance-critical helper

The naive prefix-match in `Get` is

```go
end := depth + len(n.prefix)
if end > len(key) || !bytes.Equal(n.prefix, key[depth:end]) {
    return zero, false
}
depth = end
```

That's correct, but it costs a `bytes.Equal` function call (and a
slice-header construction for `key[depth:end]`) at every inner
node — even when `len(n.prefix) == 0`. On Sparse keys, where
almost every prefix is empty, the naive version is **6× slower**
than chapter 2 on `Get`. The fix is a single short-circuit:

```go {src=art.go decl=consumePrefix}
func consumePrefix(prefix, key []byte, depth int) (int, bool) {
	if len(prefix) == 0 {
		return depth, true
	}
	end := depth + len(prefix)
	if end > len(key) || !bytes.Equal(prefix, key[depth:end]) {
		return 0, false
	}
	return end, true
}
```

The production `art` package uses the same idiom for the same
reason. This is the chapter's first foreshadowing of the chapter-8
polish: small structural decisions can have outsized perf
consequences when they fall on the hottest path.

## Put with prefix splits

The interesting cases are in `splitPrefixedNode`:

```go {src=art.go decl=splitPrefixedNode}
func splitPrefixedNode[V any](n *node256[V], key []byte, value V, depth, common int, size *int) node {
	sharedPrefix := append([]byte(nil), n.prefix[:common]...)
	oldBranch := n.prefix[common]
	n.prefix = append([]byte(nil), n.prefix[common+1:]...)

	parent := &node256[V]{prefix: sharedPrefix}
	parent.children[oldBranch] = n

	*size++
	newLeaf := &leaf[V]{key: append([]byte(nil), key...), value: value}
	cut := depth + common
	if cut == len(key) {
		parent.terminal = newLeaf
	} else {
		parent.children[key[cut]] = newLeaf
	}
	return parent
}
```

Read it bottom-up. We're inserting a new key that matched only the
first `common` bytes of the existing node's `prefix`. We need a
*new* parent node hosting:

- `prefix = sharedPrefix` — the bytes both sides agreed on
- under one branch byte (`oldBranch`, the next byte of the old
  prefix), the *old* node, with its prefix shortened to start past
  the branch byte
- as either a terminal (the new key ended exactly at the cut) or
  a second branching child (the new key continued on a different
  byte)

`splitTwoLeaves` simplifies dramatically compared with chapter 2:
no chain of inner nodes, just a single parent carrying the shared
bytes.

## Delete: the merge case

The new collapse:

```go {src=art.go}
if count == 1 && n.terminal == nil {
    if l, ok := only.(*leaf[V]); ok {
        return l                                     // hoist the leaf
    }
    child := only.(*node256[V])
    merged := make([]byte, 0, len(n.prefix)+1+len(child.prefix))
    merged = append(merged, n.prefix...)
    merged = append(merged, onlyByte)
    merged = append(merged, child.prefix...)
    child.prefix = merged
    return child                                     // merge prefixes
}
```

When an inner node has one inner-node child and no terminal, the
child can absorb its parent: the parent's prefix, plus the branch
byte that linked them, plus the child's existing prefix, becomes
the child's new prefix. The child then takes the parent's place.

This is exactly the operation that recovers Stage 2's URL footprint:
deleting the only branching key in a host bucket leaves a chain of
prefix-merge candidates that collapse all the way up.

## What path compression bought, measured

Reproduce with
`go test -bench=. -benchmem -benchtime=300ms ./tutorial/03-path-compression/`.
Stage 2 is benchmarked alongside Stage 3 for the per-decision
impact. (`-benchtime=300ms` is the project standard so fast
operations like `Get` reach steady-state and are not dominated by
framework startup.)

### Structural footprint

```
Workload    Stage 2 inner   Stage 3 inner   prefix bytes
Dense           11                5              6 B
Sparse         234              234              0 B
URL            834              393            441 B
```

Per-node sizes (from `unsafe.Sizeof`): stage-2 node256 is 4 104 B;
stage-3 node256 is 4 128 B (added 24 B for the prefix slice
header); leaf is 32 B plus key bytes; prefix bytes live in
separately-allocated slices.

Two numbers per workload below: **structural** (sum of
unsafe.Sizeof contributions) and **heap** (actual
`runtime.HeapAlloc` delta after the build, including malloc
rounding). Heap matches the `B/op` from `Put` benchmarks.

```
Workload    Stage 2                       Stage 3                      heap improvement
            structural    heap            structural    heap
Dense           85 B        93 B             60 B        64 B            1.45×
Sparse       1 008 B    1 186 B          1 013 B    1 186 B            1.00× (no shared prefix)
URL          3 495 B    4 136 B          1 695 B    1 992 B            2.08×
```

Sparse is the honest case where path compression buys nothing:
random 16-byte keys diverge at byte 0 ~99.6% of the time, so
there are no chains of one-child nodes to collapse. The bytes/key
gap versus btree (~70 B/key on the heap) is now squarely down to
**inner-node waste**: every node256 still reserves space for 256
interface slots even when it uses 4. At 4 128 B per node256, that
waste dominates everything else. Chapters 4–7 are the fix.

### Time per operation

```
Op    Workload     Stage 2          Stage 3         btree
Put    Dense          90 µs           72 µs (1.3×)   114 µs
Put    Sparse        425 µs          446 µs (0.95×)  171 µs
Put    URL         2 396 µs          929 µs (2.6×)   197 µs
Get    Dense          17.7 ns         13.5 ns (1.3×) 113 ns
Get    Sparse         10.9 ns         10.8 ns (1.0×) 129 ns
Get    URL           200   ns         71   ns (2.8×) 137 ns
All    Dense           5.8 µs          5.3 µs           4 µs
All    Sparse         54   µs         51   µs           3.5 µs
All    URL          194   µs         94   µs (2.1×)     3.7 µs
```

Three highlights:

- **Get on URL goes 2.8× faster than Stage 2 and 1.9× faster than
  btree.** Long URL-shaped prefixes used to require dozens of
  inner-node hops; now one prefix-match handles a 33-byte run in
  one bytes.Equal call.
- **Stage 3 Get is faster than btree on every workload.** Dense:
  8.4×. Sparse: 12×. URL: 1.9×. The trie's array-index descent
  was always going to win on lookup, but chapter 2's leaf-compare
  cost had partially erased that advantage on URL; chapter 3
  recovers it.
- **Put on Sparse is 5% slower than Stage 2.** Path compression
  adds a `consumePrefix` call at every Put. On Sparse, where the
  shortcut fires immediately on the empty prefix, the cost is
  small but real and there's no compensating benefit. Honest
  trade.

### Allocations

```
Op       Workload     Stage 2 allocs   Stage 3 allocs   btree allocs
Put      Dense          2 011             2 008             113
Put      Sparse         2 234             2 235              83
Put      URL            2 834             2 540              86
```

Allocations drop modestly on URL (2 834 → 2 540) because chains of
inner-node allocations collapse into one prefix-bearing node plus
the leaf and prefix-byte slice. Sparse and Dense are essentially
unchanged.

## What's still wrong

Look at the bytes/key columns above. Even after path compression,
**every inner node still reserves 256 interface slots** — ~4 KB
of nil-mostly array per node. Sparse uses 234 inner nodes for
1 000 keys; that's 234 × ~4 KB ≈ 950 KB of mostly-empty interface
slots carried just so each node can index the alphabet directly.

The next four chapters chip away at this. They introduce three
smaller node types — node4, node16, node48 — each tuned for a
different fanout band. A node with 2 children pays for 2 child
slots, not 256. The space saving is expected to be the largest of
any single decision in the tutorial.

But two node types means two cases in every dispatch — and four
will mean four. Chapter 5 between 4 and 16 will refactor that
dispatch from a `nodeKind` switch to method calls on an
`innerNode` interface, so the third and fourth additions cost no
new code in `Put`/`Get`/`Delete`/`All`.

Chapter 4 introduces node4 with the awkward two-case dispatch
deliberately, so chapter 5's refactor has something to refactor.
Make the change easy first; then make the easy change.
