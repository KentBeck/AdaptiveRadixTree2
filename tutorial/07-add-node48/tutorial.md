# Chapter 7 — Adding node48

The completed ladder. node48 fills the 17–48 fanout band: too
many children for node16's linear scan, too few to justify
node256's full 4 KB array.

The diff shape mirrors chapter 6 exactly: one new struct file
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
chapter 6 character-for-character.

## node48 has a different layout

node4 and node16 store children in sorted arrays. node256
indexes children directly by edge byte. node48 splits the
difference:

```go
type node48[V any] struct {
    prefix      []byte
    childIndex  [256]byte         // 1-based slot per edge byte; 0 = absent
    children    [48]node          // densely packed
    childEdge   [48]byte          // parallel to children: each slot's edge byte
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

```go
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

```
Workload    Stage 6 mix              Stage 7 mix
Dense       1n4 + 4n256              1n4 + 0n16 + 0n48 + 4n256
Sparse      141n4 + 92n16 + 1n256    141n4 + 92n16 + 0n48 + 1n256
URL         330n4 + 63n16 + 0n256    330n4 + 63n16 + 0n48 + 0n256
```

Inner-node mixes are identical between stages 6 and 7 at this
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

```
Sparse/5000  S6 mix:  ?  + 0  n48 + 209 n256
             S7 mix:  164 n4 + 74 n16 + 182 n48 + 1 n256
```

Heap measurement at that scale:

```
Workload      Stage 6      Stage 7      improvement     btree
Sparse/5000   234 B/key    99 B/key       2.35×         ~75 B/key
```

Stage 7 is now within **1.3× of btree's heap on Sparse-5k**.

### Time per operation at 5k Sparse

```
Op    Stage 6        Stage 7        change       btree
Put     1 107 µs       826 µs        -25%        1 392 µs
Get        30 ns        31 ns        +3%           211 ns
Range    129 µs        108 µs        -16%          21 µs
```

Put is **1.7× faster than btree** at 5k Sparse and 25% faster
than chapter 6. Get is essentially unchanged from chapter 6 (and
6.7× faster than btree).

### What about the smaller workloads?

```
Op    Workload   Stage 6        Stage 7        change
Put    Dense        85 µs           90 µs          +6%
Put    Sparse/1k   135 µs          145 µs          +7%
Put    URL         275 µs          301 µs          +9%
Get    Dense        27 ns           27 ns          tied
Get    Sparse/1k    33 ns           35 ns          +5%
Get    URL         115 ns          128 ns         +11%
Range  Dense       5.8 µs          5.4 µs          -7%
Range  Sparse/1k    16 µs           16 µs          tied
Range  URL          24 µs           23 µs          -4%
```

Heap is unchanged at 1k size because the inner-node mix didn't
shift. The small Put / Get regressions are the cost of having a
fourth case in the dispatch — the runtime now resolves through a
slightly larger interface table. Same kind of cost as chapter 5,
same shape of trade.

## What this chapter teaches

Adding node48 cost zero behavioural change and zero edits to the
operation bodies. The diff was new-struct-plus-three-edits, the
same shape as chapter 6. **That is the chapter 5 polymorphism
investment compounding.**

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

Chapter 7 reaches the same node-type structure as the production
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

Chapter 8 will be a reading guide to the production source plus
a small set of polish demos. There is no eighth node type.
