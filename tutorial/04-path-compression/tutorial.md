# Chapter 4 — Path compression

Chapter [3](../03-lazy-expansion/tutorial.md) left two structural
waste cases on the table. The first was the URL row of its
footprint table: ~4 KB/key on the heap, 4.5× tighter than chapter
2 but still ~40× looser than btree.

Lazy expansion fixed the unique-tail problem (one leaf per key,
no chain). It did *not* fix the shared-prefix problem: two URL
keys agreeing on `"https://api.example.com/v1/users/"` (33 bytes)
still allocated 33 inner node256s along that shared path, each
with one child. Each one weighs ~4 KB.

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
  child's prefix in. Without prefix compression (chapter 3) this
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
than chapter 3 on `Get`. The fix is a single short-circuit:

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
reason. This is the chapter's first foreshadowing of the polish
in chapter [9](../09-polish/tutorial.md): small structural
decisions can have outsized perf consequences when they fall on
the hottest path.

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

`splitTwoLeaves` simplifies dramatically compared with chapter
[3](../03-lazy-expansion/tutorial.md): no chain of inner nodes,
just a single parent carrying the shared bytes.

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

This is exactly the operation that keeps the URL footprint tight
under deletion: removing the only branching key in a host bucket
leaves a chain of prefix-merge candidates that collapse all the
way up.

## What path compression bought, measured

Same acceptance criteria, same yardsticks: the tables below are
rendered by `go test -update-bench` from the shared harness
benchmarks — this chapter's tree alongside chapter
[3](../03-lazy-expansion/tutorial.md)'s, with `google/btree` for
context. Reproduce any cell with
`go test -bench=. -benchmem -benchtime=300ms ./04-path-compression/`.

### Structural footprint

<!-- bench:innernodemix:start -->
```
Workload    Chapter 3 inner   Chapter 4 inner   prefix bytes
Dense              11                 5            6 B
Sparse            234               234            0 B
URL               834               393          441 B
```
<!-- bench:innernodemix:end -->

Per-node sizes (from `unsafe.Sizeof`): chapter-3 node256 is
4 104 B; this chapter's node256 is 4 128 B (added 24 B for the
prefix slice header); leaf is 32 B plus key bytes; prefix bytes
live in separately-allocated slices.

Two numbers per workload below: **structural** (sum of
`unsafe.Sizeof` contributions) and **heap** (actual
`runtime.HeapAlloc` delta after the build, including malloc
rounding).

<!-- bench:footprint:start -->
```
Workload   Chapter3 struct       heap  Chapter4 struct       heap  improvement
Dense                 85 B       93 B             60 B       64 B        1.45×
Sparse             1 008 B    1 186 B          1 013 B    1 186 B        1.00×
URL                3 495 B    4 136 B          1 695 B    1 992 B        2.08×
```
<!-- bench:footprint:end -->

Sparse is the honest case where path compression buys nothing:
random 16-byte keys diverge at byte 0 ~99.6% of the time, so
there are no chains of one-child nodes to collapse. The bytes/key
gap versus btree (~70 B/key on the heap) is now squarely down to
**inner-node waste**: every node256 still reserves space for 256
interface slots even when it uses 4. At 4 128 B per node256, that
waste dominates everything else. Chapters
[5](../05-add-node4/tutorial.md), [7](../07-add-node16/tutorial.md),
and [8](../08-add-node48/tutorial.md) are the fix.

### Time and allocations per operation

<!-- bench:optime:start -->
```
Op           Workload      Chapter4     Chapter3        btree
Put          Dense          90.7 µs     148.4 µs     223.6 µs
Put          Sparse        586.2 µs     601.7 µs     260.3 µs
Put          URL            1.15 ms      3.61 ms     332.1 µs
Get          Dense          26.0 ns      30.0 ns     149.0 ns
Get          Sparse         23.0 ns      22.0 ns     170.0 ns
Get          URL            90.0 ns     223.0 ns     202.0 ns
Range        Dense           7.7 µs       8.7 µs       6.8 µs
Range        Sparse         64.3 µs      63.2 µs       6.4 µs
Range        URL           109.6 µs     219.9 µs       6.5 µs
RangeWindow  Dense          12.7 µs      14.1 µs     408.0 ns
RangeWindow  Sparse         76.9 µs      76.4 µs     389.0 ns
RangeWindow  URL           132.2 µs     262.1 µs     490.0 ns
```
<!-- bench:optime:end -->

<!-- bench:opspace:start -->
```
Op     Workload    Chapter4 B   allocs   Chapter3 B   allocs      btree B   allocs
Put    Dense          64.4 KB    2 008      93.5 KB    2 012     109.6 KB    1 115
Put    Sparse          1.2 MB    2 235       1.2 MB    2 235      86.3 KB    1 085
Put    URL             2.0 MB    2 540       4.1 MB    2 835     121.4 KB    1 088
Range  Dense            112 B        3        112 B        3         96 B        3
Range  Sparse           112 B        3        112 B        3         96 B        3
Range  URL              112 B        3        112 B        3         96 B        3
```
<!-- bench:opspace:end -->

Three highlights:

- **Get on URL is the chapter's headline: markedly faster than
  chapter 3 and competitive with btree.** Long URL-shaped
  prefixes used to require dozens of inner-node hops; now one
  prefix-match handles a 33-byte run in one `bytes.Equal` call.
- **Get is at or below btree's time on every workload.** The
  trie's array-index descent was always going to win on lookup,
  but chapter 3's leaf-compare cost had partially erased that
  advantage on URL; this chapter recovers it.
- **Put on Sparse doesn't improve.** Path compression adds a
  `consumePrefix` check at every level of every Put; on Sparse,
  where almost every prefix is empty, the short-circuit keeps the
  cost near zero but there's no compensating benefit either — the
  two chapters land within noise of each other. Honest trade.
  Allocations tell the same story: down on URL (chains collapse
  into one prefix-bearing node), unchanged on Sparse and Dense.

### Capacity

<!-- bench:capacity:start -->
```
Workload    Chapter4 keys     B/key   Chapter3 keys     B/key      btree keys     B/key
Dense           1 563 933      67.1       1 563 020      67.1       1 239 814      84.6
Sparse            125 000     1 524         125 000     1 527       1 634 046      64.6
URL                58 317     1 807          54 997     1 916       1 091 012      96.4
```
<!-- bench:capacity:end -->

A subtlety worth noticing: URL barely moves here even though the
1 000-key footprint halved. At the 100 MB scale the tree holds
tens of thousands of URL keys, so each shared-prefix chain was
already amortized over many more keys — chapter 3's per-key chain
tax shrinks with scale on its own. Path compression's win shows
up at small scale and in operation time; the remaining capacity
gap to btree is inner-node waste, not path shape.

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
will mean four. Chapter [6](../06-introduce-polymorphism/tutorial.md)
will refactor that dispatch from type switches to method calls on
an `innerNode` interface, so the third and fourth additions cost
no new code in `Put`/`Get`/`Delete`/`Range`.

Chapter [5](../05-add-node4/tutorial.md) introduces node4 with the
awkward two-case dispatch deliberately, so chapter 6's refactor
has something to refactor. Make the change easy first; then make
the easy change.
