# Chapter 5 — Adding node4

Chapter [4](../04-path-compression/tutorial.md) ended with one
number above all the others:

> Sparse: 234 inner nodes for 1 000 keys → ~950 KB of child slots
> carried just so each node can index the alphabet directly.

That's the cost of `[256]node` per inner node, regardless of the
node's actual fanout. Across all three workloads, the inner-node
footprint dwarfs the leaf footprint. We need a smaller inner node.

Chapter 5 introduces *node4* — an inner node that stores up to
four branching children in a sorted four-element array. Same
prefix slot, same terminal slot, same role in the tree. A node4
costs 112 B versus a node256's 4 136 B.

The cost: we now have *two* inner-node types. Every operation
must dispatch on which type it has. Chapter 5 uses explicit
type-switch helpers for that dispatch — `nodePrefix`,
`nodeFindChild`, `nodeAddOrGrowChild`, etc. Ten of them. With two
cases each.

If two cases is uncomfortable, four cases (chapters
[7](../07-add-node16/tutorial.md) and
[8](../08-add-node48/tutorial.md) add two more node types) is
intolerable. That's exactly why chapter
[6](../06-introduce-polymorphism/tutorial.md) exists between this
chapter and the third addition: refactor the dispatch to method
polymorphism *first*, then add the rest. **Make the change easy,
then make the easy change.**

This chapter is the "make the change hard" one. Read the type
switches and remember how many of them there are.

## What changes

Two node types now:

```go {src=art.go decls=node4,node256}
type node4[V any] struct {
	prefix      []byte
	keys        [4]byte
	children    [4]node
	terminal    *leaf[V]
	numChildren uint8
}

type node256[V any] struct {
	prefix      []byte
	children    [256]node
	terminal    *leaf[V]
	numChildren uint16
}
```

The new invariant: **`node4.keys[:numChildren]` is sorted ascending
by edge byte.** Sorted storage is what lets `Range` yield children
in byte order without a sort step at iteration time. `node256`
gets the same property for free because the array index *is* the
byte; node4 has to maintain it explicitly during `addChild`.

Promotion when a `node4` exceeds capacity:

```go {src=art.go}
case *node4[V]:
    if r.numChildren < node4Capacity {
        r.addChild(b, child)
        return r
    }
    grown := growToNode256[V](r)
    grown.children[b] = child
    grown.numChildren++
    return grown
```

Demotion when a `node256` falls to ≤ 4 children, in `reshape`:

```go {src=art.go}
if r, ok := current.(*node256[V]); ok && r.numChildren <= node4Capacity {
    return shrinkToNode4[V](r)
}
```

`shrinkToNode4` walks `children[0..255]` in ascending order so the
demoted node4 inherits sorted keys for free.

## The dispatch helpers

Every operation that used to access `n.prefix`, `n.terminal`, or
`n.children[b]` now goes through a typed helper:

```go {src=art.go decls=nodePrefix,nodeFindChild,nodeAddOrGrowChild}
func nodePrefix[V any](n node) []byte {
	switch r := n.(type) {
	case *node4[V]:
		return r.prefix
	case *node256[V]:
		return r.prefix
	}
	panic("nodePrefix: unknown inner-node type")
}

func nodeFindChild[V any](n node, b byte) node {
	switch r := n.(type) {
	case *node4[V]:
		return r.findChild(b)
	case *node256[V]:
		return r.children[b]
	}
	panic("nodeFindChild: unknown inner-node type")
}

func nodeAddOrGrowChild[V any](n node, b byte, child node) node {
	switch r := n.(type) {
	case *node4[V]:
		if r.numChildren < node4Capacity {
			r.addChild(b, child)
			return r
		}
		grown := growToNode256[V](r)
		grown.children[b] = child
		grown.numChildren++
		return grown
	case *node256[V]:
		r.children[b] = child
		r.numChildren++
		return r
	}
	panic("nodeAddOrGrowChild: unknown inner-node type")
}
```

(There are seven more dispatch helpers like these — `setNodePrefix`,
`nodeTerminal`, `setNodeTerminal`, `nodeReplaceChild`,
`nodeRemoveChild`, `numChildren`, `eachAscending`. Each is a
two-case type switch; the shape is uniform.)

Ten helpers in total: `nodePrefix`, `setNodePrefix`,
`nodeTerminal`, `setNodeTerminal`, `nodeFindChild`,
`nodeAddOrGrowChild`, `nodeReplaceChild`, `nodeRemoveChild`,
`numChildren`, plus `eachAscending` for iteration. Each is a
two-case type switch. The operations look reasonably clean
because the dispatch is hidden in the helpers — but the dispatch
cost is real, as the bench numbers below show, and **every new
node type means a new case in every helper**.

`Put`, `Get`, `Delete`, `Range` are otherwise unchanged from
chapter 4 in shape — they just call the helpers instead of
accessing struct fields directly.

## What node4 buys, measured

Same acceptance criteria, same yardsticks: the tables below are
rendered by `go test -update-bench` from the shared harness
benchmarks — this chapter's tree alongside chapter
[4](../04-path-compression/tutorial.md)'s, with `google/btree` for
context. Reproduce any cell with
`go test -bench=. -benchmem -benchtime=300ms ./05-add-node4/`.

### Per-node sizes

`unsafe.Sizeof` reports:

<!-- bench:nodesizes:start -->
```
Type      Bytes    What it holds
node4       112    prefix slice, 4 sorted keys, 4 child slots, terminal, count
node256    4136    prefix slice, 256 child slots, terminal, count
leaf         32    key slice header, value (V == int)
ratio       36x    node256 / node4
```
<!-- bench:nodesizes:end -->

Every inner-node demotion from node256 to node4 saves ~4 KB.

### Structural footprint

<!-- bench:innernodemix:start -->
```
Workload    Chapter 4 inner   Chapter 5 (n4 + n256)
Dense               5               1 + 4
Sparse            234             141 + 93
URL               393             330 + 63
```
<!-- bench:innernodemix:end -->

Two numbers per workload below: **structural** (sum of
`unsafe.Sizeof` contributions) and **heap** (actual
`runtime.HeapAlloc` delta after building, including malloc
rounding).

<!-- bench:footprint:start -->
```
Workload   Chapter4 struct       heap  Chapter5 struct       heap  improvement
Dense                 60 B       69 B             56 B       59 B        1.17×
Sparse             1 013 B    1 186 B            448 B      516 B        2.30×
URL                1 695 B    1 992 B            370 B      424 B        4.70×
```
<!-- bench:footprint:end -->

This chapter lands within **~6× of btree's heap footprint on URL**
and ~7× on Sparse, down from chapter 3's ~40× and ~17×.

The Sparse arithmetic, made concrete: 141 of the 234 inner nodes
from chapter 4 became node4s. Each demoted node went from 4 136 B
to 112 B — saved 4 024 B per node. Total inner-node savings:
141 × 4 024 ≈ 567 KB, giving the heap reduction in the table. The
93 inner nodes that stayed node256 are the ones with > 4
children — depth-1 buckets that collected enough random keys to
overflow node4.

URL is the headline: 330 of the 393 inner nodes are node4s now,
saving 330 × 4 024 ≈ 1.3 MB of the chapter-4 tree's ~2 MB.

### Time and allocations per operation

<!-- bench:optime:start -->
```
Op           Workload      Chapter5     Chapter4        btree
Put          Dense         114.2 µs      95.0 µs     186.3 µs
Put          Sparse        315.1 µs     558.8 µs     249.9 µs
Put          URL           409.1 µs      1.15 ms     315.9 µs
Get          Dense          33.0 ns      26.0 ns     140.0 ns
Get          Sparse         38.0 ns      25.0 ns     191.0 ns
Get          URL           106.0 ns      89.0 ns     205.0 ns
Range        Dense           9.5 µs       7.8 µs       6.4 µs
Range        Sparse         39.5 µs      63.6 µs       6.2 µs
Range        URL            33.2 µs     116.6 µs       6.3 µs
RangeWindow  Dense          16.6 µs      12.6 µs     370.0 ns
RangeWindow  Sparse         45.2 µs      70.5 µs     374.0 ns
RangeWindow  URL            38.2 µs     121.0 µs     454.0 ns
```
<!-- bench:optime:end -->

<!-- bench:opspace:start -->
```
Op     Workload    Chapter5 B   allocs   Chapter4 B   allocs      btree B   allocs
Put    Dense          60.1 KB    2 012      64.4 KB    2 008     109.6 KB    1 115
Put    Sparse        526.6 KB    2 328       1.2 MB    2 235      86.3 KB    1 085
Put    URL           431.8 KB    2 603       2.0 MB    2 540     121.4 KB    1 088
Range  Dense            112 B        3        112 B        3         96 B        3
Range  Sparse           112 B        3        112 B        3         96 B        3
Range  URL              112 B        3        112 B        3         96 B        3
```
<!-- bench:opspace:end -->

Three honest observations:

- **Get got slower across the board**, worst on Sparse. The cost
  is the type switch executed at every inner-node visit. On
  Sparse, where the walk is one or two inner nodes followed by a
  leaf compare, the type switch is a large share of the work. We
  knew adding a second node type would cost dispatch; the
  question for chapter [6](../06-introduce-polymorphism/tutorial.md)
  is whether a polymorphic interface is faster than the switch.
  (Spoiler: not meaningfully. Not the reason we'll do the
  refactor.)
- **Range got dramatically faster on Sparse and URL.** Chapter
  4's `Range` iterated 256 child slots per inner node (mostly
  nil); a node4 iterates only the 4 occupied slots. On URL the
  inner-node mix is 330 node4s + 63 node256s — **84 % of inner
  nodes are node4s**, so most of `Range`'s per-node iteration
  cost dropped by 64×. On Sparse the mix is 141 + 93 (60 %
  node4s), a smaller but still large win.
- **Put got faster on Sparse and URL, and a little slower on
  Dense.** Faster because allocating a 112-byte node4 costs less
  malloc and zeroing time than a 4 KB node256, and Sparse / URL
  trees do a lot of those allocations. Dense pays the dispatch
  overhead without saving allocations — its tree is a handful of
  nodes either way. Put *bytes* tell the
  cleaner story: Sparse drops ~2.3×, URL ~4.6× — the allocation
  *count* is unchanged (one node per branching point) but the
  per-allocation size collapsed.

### Capacity

<!-- bench:capacity:start -->
```
Workload    Chapter5 keys     B/key   Chapter4 keys     B/key      btree keys     B/key
Dense           1 563 933      67.1       1 563 930      67.1       1 239 814      84.6
Sparse            526 488     597.1         125 000     1 524       1 634 042      64.6
URL               219 403     478.4          58 318     1 807       1 091 010      96.4
```
<!-- bench:capacity:end -->

Sparse and URL capacity both jump — the smaller inner nodes are
pure win at the 100 MB scale.

## What's still wrong: the dispatch is starting to hurt

`Get` on Sparse roughly doubled from chapter 4. Two type switches
per loop iteration, fired on every inner-node visit. The compiler
does what it can with two-case switches but can't eliminate the
type-tag check.

The dispatch problem will get worse with more node types. Adding
node16 (chapter [7](../07-add-node16/tutorial.md)) means 3 cases
× 9 helpers = 27 case branches. Adding node48 (chapter
[8](../08-add-node48/tutorial.md)) means 36. Each addition forces
an edit to every dispatch helper.

Chapter [6](../06-introduce-polymorphism/tutorial.md) introduces
an `innerNode` interface with one method per operation:

```go {src=../06-introduce-polymorphism/art.go decl=innerNode}
type innerNode interface {
	node
	getPrefix() []byte
	setPrefix(p []byte)
	getTerminal() node
	setTerminal(t node)
	findChild(b byte) node
	addOrGrowChild(b byte, child node) innerNode
	replaceChild(b byte, child node)
	removeChild(b byte)
	eachAscending(yield func(byte, node) bool) bool
	reshape() node
}
```

`node4` and `node256` both implement it. Every operation calls
methods directly. The type switches in the helpers go away. Bench
numbers should stay roughly the same (or better — the Go compiler
specializes interface calls in some cases) and the chapter-7 diff
for adding node16 will be **one new struct file**, no edits to
existing operations.

That's the chapter-5 commitment: **same behavior, easier change**.
Then chapter [7](../07-add-node16/tutorial.md) collects the easy
change.
