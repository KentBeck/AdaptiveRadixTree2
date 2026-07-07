# Chapter 8 — Adding node48

The completed ladder. node48 fills the 17–48 fanout band: too
many children for node16's linear scan, too few to justify
node256's full 4 KB array.

The diff shape mirrors chapter [7](../07-add-node16/tutorial.md)
exactly: one new struct file
(`node48` + 11 method implementations + `growToNode48` +
`shrinkToNode48`), plus surgical edits at three places:

| Change | Lines |
|---|---|
| New `node48[V]` struct + 11 method impls + 2 ladder helpers | +180 |
| `node16.addOrGrowChild`: grow target → `node48` (was `node256`) | 1 |
| `node256.reshape`: demote target → `node48`, threshold 16 → 48 | 2 |
| `growToNode256` rebound to take `*node48`; `shrinkToNode16` rebound to take `*node48` | ~10 |
| `CountByKind` widened to four return values | 5 |

`Put`, `Get`, `Delete`, `Range`, `splitTwoLeaves`,
`splitPrefixedNode`, `consumePrefix`, `longestCommonPrefix`,
`collapseEmpty`, and `mergePrefixIntoChild` are unchanged from
chapter 7 character-for-character.

## node48 has a different layout

node4 and node16 store children in sorted arrays. node256
indexes children directly by edge byte. node48 splits the
difference:

```go {src=art.go decl=node48}
type node48[V any] struct {
	prefix      []byte
	childIndex  [256]byte
	children    [node48Capacity]node
	childEdge   [node48Capacity]byte
	terminal    node
	numChildren uint8
}
```

`childIndex[b]` says where to find the child reached via edge
`b`: zero means no such child, any other value `k` says
`children[k-1]`. **`findChild` is O(1) like node256, but the index
is a single 256-byte array instead of 256 sixteen-byte interface
slots.** That's the structural saving: 256 B + 48 × 16 B + 48 B =
roughly 1 KB per node48 vs 4 KB per node256.

Why the parallel `childEdge[48]` array? Because of removeChild.
To keep `children[:numChildren]` dense (so addChild can simply
append), removeChild swaps the last live child into the freed
slot. After the swap, the swapped-in child's `childIndex` entry
must be updated to point at its new slot — and we need to know
*which edge byte* the swapped-in child held. `childEdge[i]`
records that for the slot at index `i`:

```go {src=art.go decl=node48.removeChild}
func (n *node48[V]) removeChild(b byte) {
	slot := n.childIndex[b]
	if slot == 0 {
		return
	}
	last := n.numChildren
	if slot != last {
		lastEdge := n.childEdge[last-1]
		n.children[slot-1] = n.children[last-1]
		n.childEdge[slot-1] = lastEdge
		n.childIndex[lastEdge] = slot
	}
	n.children[last-1] = nil
	n.childEdge[last-1] = 0
	n.childIndex[b] = 0
	n.numChildren--
}
```

This is the most subtle data-structure code in the tutorial. It
is also the only operation that earns its complexity — every
other node48 method reads in a couple of lines.

## What does node48 actually catch?

**For our 1 000-key fixture workloads: nothing.** The 17–48
fanout band is empty at that scale.

<!-- bench:innernodemix1k:start -->
```
Workload    Chapter 7 mix              Chapter 8 mix
Dense       1n4 + 0n16 + 4n256        1n4 + 0n16 + 0n48 + 4n256
Sparse      141n4 + 92n16 + 1n256        141n4 + 92n16 + 0n48 + 1n256
URL         330n4 + 63n16 + 0n256        330n4 + 63n16 + 0n48 + 0n256
```
<!-- bench:innernodemix1k:end -->

Inner-node mixes are identical between chapters 7 and 8 at this
size. URL never triggers node48 at any scale because its branching
structure is shaped by the workload generator (6 hosts ⨯ 10 paths
⨯ 8-byte random tail) — branching points have either ≤ 16 children
or 256 random tail children.

Sparse at 1 000 also doesn't trigger it because its average
first-byte bucket has ~ 4 keys (1000 / 256 ≈ 3.9), so depth-1
nodes settle at node4 or small node16. The root has ~250
distinct first bytes — way above 48 — so it stays node256.

**At Sparse 5 000** the workload finally falls into node48's sweet
spot: 5000 / 256 ≈ 20 keys per first-byte bucket, so depth-1
nodes hold about 20 children — too many for node16, too few for
node256. **182 inner nodes settle into node48.**

<!-- bench:innernodemix5k:start -->
```
Sparse/5000  Chapter 7 mix:  164 n4 + 74 n16 + 183 n256
             Chapter 8 mix:  164 n4 + 74 n16 + 182 n48 + 1 n256
```
<!-- bench:innernodemix5k:end -->

Heap measurement at that scale (rendered, like every table here,
by `go test -update-bench` from the shared harness benchmarks):

<!-- bench:heapfootprint5k:start -->
```
Workload      Chapter7      Chapter8      improvement     btree
Sparse/5000   236 B/key      99 B/key       2.38×          65 B/key
```
<!-- bench:heapfootprint5k:end -->

Chapter 8 cuts the Sparse-5k heap by more than half, landing
within ~1.5× of btree.

### Time per operation at 5k Sparse

<!-- bench:optime5k:start -->
```
Op           Workload      Chapter8     Chapter7        btree
Put          Sparse         1.11 ms      1.65 ms      1.61 ms
Get          Sparse         40.0 ns      41.0 ns     220.0 ns
Range        Sparse        138.7 µs     150.3 µs      31.0 µs
RangeWindow  Sparse        168.2 µs     190.6 µs       1.0 µs
```
<!-- bench:optime5k:end -->

Put beats btree outright at 5k Sparse and improves on chapter 7.
Get is essentially unchanged from chapter 7 and several times
faster than btree.

### What about the smaller workloads?

<!-- bench:optime:start -->
```
Op           Workload      Chapter8     Chapter7        btree
Put          Dense         127.4 µs     124.6 µs     203.3 µs
Put          Sparse        193.1 µs     189.5 µs     269.7 µs
Put          URL           370.7 µs     384.1 µs     322.3 µs
Get          Dense          40.0 ns      39.0 ns     148.0 ns
Get          Sparse         50.0 ns      50.0 ns     168.0 ns
Get          URL           140.0 ns     141.0 ns     201.0 ns
Range        Dense           9.9 µs       9.5 µs       6.4 µs
Range        Sparse         23.7 µs      23.7 µs       6.2 µs
Range        URL            32.5 µs      34.2 µs       6.4 µs
RangeWindow  Dense          14.3 µs      14.3 µs     375.0 ns
RangeWindow  Sparse         31.0 µs      31.7 µs     387.0 ns
RangeWindow  URL            40.9 µs      45.2 µs     511.0 ns
```
<!-- bench:optime:end -->

<!-- bench:opspace:start -->
```
Op     Workload    Chapter8 B   allocs   Chapter7 B   allocs      btree B   allocs
Put    Dense          66.0 KB    2 020      61.4 KB    2 016     109.6 KB    1 115
Put    Sparse        113.8 KB    2 330     112.6 KB    2 329      86.3 KB    1 085
Put    URL           151.8 KB    2 603     151.8 KB    2 603     121.4 KB    1 088
Range  Dense            312 B        9        312 B        9         96 B        3
Range  Sparse          5.8 KB      238       5.8 KB      238         96 B        3
Range  URL             9.6 KB      397       9.6 KB      397         96 B        3
```
<!-- bench:opspace:end -->

Heap is unchanged at 1k size because the inner-node mix didn't
shift. Any small Put / Get movement is the cost of having a
fourth case in the dispatch — the runtime now resolves through a
slightly larger set of concrete types. Same kind of cost as
chapter [6](../06-introduce-polymorphism/tutorial.md), same shape
of trade.

### Capacity

<!-- bench:capacity:start -->
```
Workload    Chapter8 keys     B/key   Chapter7 keys     B/key      btree keys     B/key
Dense           1 564 092      67.1       1 564 099      67.1       1 239 814      84.6
Sparse          1 215 571      98.6       1 141 608     207.1       1 634 039      64.6
URL               755 034     139.6         755 037     139.6       1 091 012      96.4
```
<!-- bench:capacity:end -->

At the 100 MB scale the tree holds hundreds of thousands of
Sparse keys, so the first-byte buckets hold hundreds of children
each and sit in node256 — the 17–48 band matters at intermediate
bucket sizes, not at every scale. Adaptivity again: which rungs
of the ladder earn their keep depends on the data.

## What this chapter teaches

Adding node48 cost zero behavioural change and zero edits to the
operation bodies. The diff was new-struct-plus-three-edits, the
same shape as chapter [7](../07-add-node16/tutorial.md). **That
is the chapter [6](../06-introduce-polymorphism/tutorial.md)
polymorphism investment compounding.**

What the new node type buys depends on the workload. At 1k
fixture size, the 17–48 band is empty across all three workloads;
the chapter measurably costs a few percent on hot paths and saves
nothing. At 5k Sparse, the same code suddenly pays for itself
3× over.

This is the deeper engineering point. The four-type ladder is not
universally optimal — for any given workload, only some of the
types earn their seats at the table. The Adaptive in *Adaptive*
Radix Tree means: the *shape of the data* picks which sizes to
use. We pay the dispatch cost for all four; we collect the heap
savings only where they appear.

## What's left

Chapter 8 reaches the same node-type structure as the production
`art.Tree`. The remaining gap to production is *polish*:

- An inline-key buffer on `*leaf[V]` so short keys (≤ 24 B) avoid
  a second allocation.
- An embedded `innerHeader` so each inner node's prefix/terminal
  accessors come from method promotion instead of being written
  out four times.
- A reused path buffer in `Range` so range-iteration becomes
  zero-alloc.
- A unified `kind() nodeKind` method for cheap leaf-vs-inner
  branching.

Chapter [9](../09-polish/tutorial.md) will be a reading guide to
the production source plus a small set of polish demos. There is
no fifth node type.
