# Chapter 4 — Adding node4

Chapter 3 ended with one number above all the others:

> Sparse: 234 inner nodes for 1 000 keys → ~468 KB of pointers
> carried just so each node can index the alphabet directly.

That's the cost of `[256]node` per inner node, regardless of the
node's actual fanout. Across all three workloads, the inner-node
footprint dwarfs the leaf footprint. We need a smaller inner node.

Chapter 4 introduces *node4* — an inner node that stores up to
four branching children in a sorted four-element array. Same
prefix slot, same terminal slot, same role in the tree. A node4
costs about 80 B versus a node256's ~2 080 B.

The cost: we now have *two* inner-node types. Every operation
must dispatch on which type it has. Chapter 4 uses explicit
type-switch helpers for that dispatch — `nodePrefix`,
`nodeFindChild`, `nodeAddOrGrowChild`, etc. Nine of them. With two
cases each.

If two cases is uncomfortable, four cases (chapters 6 and 7 add
two more node types) is intolerable. That's exactly why chapter 5
exists between this chapter and the third addition: refactor the
dispatch to method polymorphism *first*, then add the rest. **Make
the change easy, then make the easy change.**

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
by edge byte.** Sorted storage is what lets `All` yield children
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

Nine helpers in total: `nodePrefix`, `setNodePrefix`,
`nodeTerminal`, `setNodeTerminal`, `nodeFindChild`,
`nodeAddOrGrowChild`, `nodeReplaceChild`, `nodeRemoveChild`,
`numChildren`, plus `eachAscending` for iteration. Each is a
two-case type switch. The operations look reasonably clean
because the dispatch is hidden in the helpers — but the dispatch
cost is real, as the bench numbers below show, and **every new
node type means a new case in every helper**.

`Put`, `Get`, `Delete`, `All` are otherwise unchanged from chapter
3 in shape — they just call the helpers instead of accessing
struct fields directly.

## What node4 buys, measured

Reproduce with
`go test -bench=. -benchmem -benchtime=300ms ./tutorial/04-add-node4/`.

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
Workload    Stage 3 inner   Stage 4 (n4 + n256)
Dense             5             1 + 4
Sparse          234           141 + 93
URL             393           330 + 63
```
<!-- bench:innernodemix:end -->

Two numbers per workload below: **structural** (sum of
unsafe.Sizeof contributions) and **heap** (actual
`runtime.HeapAlloc` delta after building, including malloc
rounding). Heap matches the `B/op` from `Put` benchmarks.

```
Workload    Stage 3                       Stage 4                      heap improvement
            structural    heap            structural    heap
Dense           60 B        64 B             56 B        59 B           1.08×
Sparse       1 015 B    1 186 B            448 B       516 B           2.30×
URL          1 698 B    1 992 B            370 B       424 B           4.69×
```

Stage 4 is now within **6× of btree's heap footprint on URL**
(424 B/key vs ~70 B/key) and ~7× on Sparse, down from chapter
3's 28× and 17×.

The Sparse arithmetic, made concrete: 141 of the 234 inner nodes
from chapter 3 became node4s. Each demoted node went from 4 136 B
to 112 B — saved 4 024 B per node. Total inner-node savings:
141 × 4 024 ≈ 567 KB on a 1 186 KB tree (chapter 3 heap), giving
the 2.3× heap reduction observed. The 93 inner nodes that stayed
node256 are the ones with > 4 children — depth-1 buckets that
collected enough random keys to overflow node4.

URL is the headline. 330 of the 393 inner nodes are node4s now.
Per-node savings: 330 × 4 024 ≈ 1 328 KB on a 1 992 KB tree =
the 4.69× heap reduction.

### Time per operation

```
Op    Workload     Stage 3          Stage 4           btree
Put    Dense          79 µs           89 µs (0.9×)    118 µs
Put    Sparse        455 µs          248 µs (1.8×)    175 µs
Put    URL           888 µs          343 µs (2.6×)    196 µs
Get    Dense          13.5 ns         21.6 ns (0.6×)  108 ns
Get    Sparse         11.0 ns         24.8 ns (0.4×)  127 ns
Get    URL            65   ns         83   ns (0.8×)  133 ns
All    Dense           5.3 µs          6.0 µs (0.9×)    4 µs
All    Sparse         54   µs         24   µs (2.2×)    4 µs
All    URL            92   µs         21   µs (4.4×)    4 µs
```

Three honest observations:

- **Get got slower across the board** — between 1.3× and 2.3×,
  worst on Sparse. The cost is the type switch executed at every
  inner-node visit. On Sparse, where the walk is one or two
  inner nodes followed by a leaf compare, the type switch is the
  work. We knew adding a second node type would cost dispatch;
  the question for chapter 5 is whether a polymorphic interface
  is faster than the switch. (Spoiler: it isn't, slightly. Not
  the reason we'll do the refactor.)
- **All got dramatically faster on Sparse and URL.** On URL, 4.4×
  faster than chapter 3. Reason: chapter 3's `All` iterated 256
  child slots per inner node (mostly nil); chapter 4's `All` on a
  node4 iterates only the 4 occupied slots. On URL the inner-node
  mix is 330 node4s + 63 node256s — **84 % of inner nodes are
  node4s**, so 84 % of `All`'s per-node iteration cost dropped by
  64×. On Sparse the mix is 141 node4s + 93 node256s (60 % node4s),
  giving the smaller 2.2× All speedup.
- **Put got faster on Sparse (1.8×) and URL (2.6×) and slightly
  slower on Dense (1.1×).** Faster because allocating an 80-byte
  node4 costs less malloc time than a 2 080-byte node256, and
  Sparse / URL trees do a lot of those allocations. Slower on
  Dense because the dispatch overhead applies even when Put falls
  through to the existing-node-256 path.

### Allocations and bytes allocated

```
Op       Workload     Stage 3 B/op    Stage 4 B/op    btree B/op
Put      Dense          64 KB           60 KB            102 KB
Put      Sparse      1 186 KB          527 KB             70 KB
Put      URL         1 993 KB          432 KB             73 KB
```

Sparse Put now allocates **2.3× fewer bytes** than chapter 3
(though still ~7.5× more than btree). URL is similar: **4.6×
fewer bytes** than chapter 3 (~5.9× more than btree). The
allocation *count* is essentially unchanged — we still allocate
one node per branching point — but the per-allocation size
dropped dramatically.

## What's still wrong: the dispatch is starting to hurt

`Get` on Sparse went from 11 ns (chapter 3) to 25 ns (chapter 4).
Two type switches per loop iteration, fired on every inner-node
visit. The compiler does what it can with two-case switches but
can't eliminate the type-tag check.

The dispatch problem will get worse with more node types. Adding
node16 (chapter 6) means 3 cases × 9 helpers = 27 case branches.
Adding node48 (chapter 7) means 36. Each addition forces an edit
to every dispatch helper.

Chapter 5 introduces an `innerNode` interface with one method per
operation:

```go {src=../05-introduce-polymorphism/art.go decl=innerNode}
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
specializes interface calls in some cases) and the chapter-6
diff for adding node16 will be **one new struct file**, no edits
to existing operations.

That's the chapter-5 commitment: **same behavior, easier change**.
Then chapter 6 collects the easy change.
