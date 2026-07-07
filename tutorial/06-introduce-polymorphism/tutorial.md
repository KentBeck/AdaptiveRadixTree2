# Chapter 6 — Introduce polymorphism

This chapter is engineering. It adds no features and changes no
edge cases. It refactors chapter 5's ten type-switch dispatch
helpers into method calls on an `innerNode` interface that both
`*node4` and `*node256` implement.

We did this because chapter [5](../05-add-node4/tutorial.md) was
already feeling crowded with two cases per helper, and we know two
more node types are coming (node16, node48). Method dispatch
through an interface makes those additions land as **new struct
files with zero edits to `Put`, `Get`, `Delete`, or `Range`**.
That's the change we want to make easy. Chapter
[7](../07-add-node16/tutorial.md) collects the easy change.

The numbers below show what the refactor cost — not because the
numbers should drive the decision (they shouldn't, here) but
because we are engineering, not guessing. The point of measuring
is the *quality of the decision*, not the size of the speedup.

## What the interface looks like

```go {src=art.go decl=innerNode}
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

Eleven methods. Both `*node4` and `*node256` implement all of
them. `Put`, `Get`, `Delete`, and `Range` use only the methods on
this interface; they have no idea what concrete type they're
operating on.

A few small design decisions in this interface earn their own
sentences:

- **`getTerminal()` returns `node`, not `*leaf[V]`.** That keeps
  the inner-node structs V-erased so they can implement a single
  V-free interface and share the same compiled code across every
  `Tree[V]` instantiation. Consuming code casts the result to
  `*leaf[V]` at point of use.
- **No `numChildren()` on the interface.** No consumer outside
  `reshape` needs the count, and `reshape` is a method on each
  concrete type — it accesses its own field directly. A narrower
  interface is one fewer method for chapters
  [7](../07-add-node16/tutorial.md) and
  [8](../08-add-node48/tutorial.md) to implement.
- **`reshape()` is on the interface.** The collapse and
  demote/promote rules are type-specific (node4 has no demotion
  case; node256 demotes to node4 at four children); each type
  owns its own logic. Common cases (the 0-children and
  1-only-child collapses) live in two free helpers,
  `collapseEmpty` and `mergePrefixIntoChild`, that both reshape
  methods call.

## What the operations look like now

`Put`'s inner-node body, before:

```go
prefix := nodePrefix[V](current)
common := longestCommonPrefix(prefix, key[depth:])
if common < len(prefix) {
    return splitPrefixedNode(current, key, value, depth, common, size)
}
depth += common
if depth == len(key) {
    if t := nodeTerminal[V](current); t == nil {
        *size++
        setNodeTerminal[V](current, &leaf[V]{key: ..., value: value})
    } else {
        t.value = value
    }
    return current
}
b := key[depth]
child := nodeFindChild[V](current, b)
if child == nil {
    *size++
    return nodeAddOrGrowChild[V](current, b, &leaf[V]{...})
}
newChild := putInto[V](child, key, value, depth+1, size)
if newChild != child {
    nodeReplaceChild[V](current, b, newChild)
}
return current
```

After:

```go
n := current.(innerNode)
prefix := n.getPrefix()
common := longestCommonPrefix(prefix, key[depth:])
if common < len(prefix) {
    return splitPrefixedNode[V](n, key, value, depth, common, size)
}
depth += common
if depth == len(key) {
    if t := n.getTerminal(); t != nil {
        t.(*leaf[V]).value = value
        return n
    }
    *size++
    n.setTerminal(&leaf[V]{key: ..., value: value})
    return n
}
b := key[depth]
child := n.findChild(b)
if child == nil {
    *size++
    return n.addOrGrowChild(b, &leaf[V]{...})
}
newChild := putInto[V](child, key, value, depth+1, size)
if newChild != child {
    n.replaceChild(b, newChild)
}
return n
```

The shape is identical. Every `nodeFoo[V](current, ...)` became
`n.foo(...)`. The `[V]` type witness stays on `splitPrefixedNode`
(it allocates a new `*leaf[V]` and a new `*node4[V]`) but every
inner-node-as-receiver method is `V`-free. The free dispatch
helpers — `nodePrefix`, `nodeFindChild`, `nodeAddOrGrowChild`,
`nodeReplaceChild`, `nodeRemoveChild`, `nodeTerminal`,
`setNodeTerminal`, `setNodePrefix`, `numChildren`,
`eachAscending` — **are gone**.

## The reshape diff is the easiest one to read

Chapter 5's `reshape` was a single free function that did
everything — the count, the terminal check, the collapse cases,
*and* the node256→node4 demotion via a type assertion at the end:

```go
func reshape[V any](current node) node {
    count := numChildren[V](current)
    terminal := nodeTerminal[V](current)
    if count == 0 { ... collapse ... }
    if count == 1 && terminal == nil { ... hoist or merge ... }
    if r, ok := current.(*node256[V]); ok && r.numChildren <= node4Capacity {
        return shrinkToNode4[V](r)
    }
    return current
}
```

Chapter 6 splits the responsibilities. Each concrete type's
`reshape()` knows only its own collapse-and-demote rules:

```go {src=art.go decl=node4.reshape}
func (n *node4[V]) reshape() node {
	if n.numChildren == 0 {
		return collapseEmpty(n.terminal)
	}
	if n.numChildren == 1 && n.terminal == nil {
		only := n.children[0]
		if l, ok := only.(*leaf[V]); ok {
			return l
		}
		return mergePrefixIntoChild(n.prefix, n.keys[0], only.(innerNode))
	}
	return n
}
```

`node256.reshape` has the same shape with one extra branch — the
demotion to node4 once it falls to four children:

```go
func (n *node256[V]) reshape() node {
    if n.numChildren == 0 {
        return collapseEmpty(n.terminal)
    }
    if n.numChildren == 1 && n.terminal == nil {
        // (find sole child, hoist or merge -- same shape)
    }
    if n.numChildren <= node4Capacity {
        return shrinkToNode4[V](n)
    }
    return n
}
```

Adding node16 in chapter [7](../07-add-node16/tutorial.md) means
writing a third such method, specifying its own demote/promote
thresholds. The other code doesn't move.

## What it cost — measured

Same acceptance criteria, same yardsticks: the tables below are
rendered by `go test -update-bench` from the shared harness
benchmarks — this chapter's tree alongside chapter
[5](../05-add-node4/tutorial.md)'s, with `google/btree` for
context. Reproduce any cell with
`go test -bench=. -benchmem -benchtime=300ms ./06-introduce-polymorphism/`.

### Per-node sizes

Storing `terminal` as `node` (interface, 16 B) instead of
`*leaf[V]` (pointer, 8 B) grew each inner-node struct by 8 B:

<!-- bench:nodesizes:start -->
```
Type        Chapter 5    Chapter 6
node4         112 B        120 B
node256      4136 B       4144 B
leaf           32 B         32 B
```
<!-- bench:nodesizes:end -->

In practice Go's allocator rounds these to the same size class,
so the live-heap footprint is unchanged:

<!-- bench:heapfootprint:start -->
```
Workload    Chapter5 heap   Chapter6 heap    ratio
Dense            59 B/key        59 B/key    1.00×
Sparse          516 B/key       518 B/key    1.00×
URL             429 B/key       429 B/key    1.00×
```
<!-- bench:heapfootprint:end -->

### Time and allocations per operation

<!-- bench:optime:start -->
```
Op           Workload      Chapter6     Chapter5        btree
Put          Dense         130.7 µs     116.5 µs     212.2 µs
Put          Sparse        401.6 µs     399.0 µs     270.0 µs
Put          URL           509.3 µs     481.6 µs     325.1 µs
Get          Dense          39.0 ns      33.0 ns     138.0 ns
Get          Sparse         44.0 ns      40.0 ns     174.0 ns
Get          URL           120.0 ns     102.0 ns     219.0 ns
Range        Dense           9.6 µs      10.9 µs       6.4 µs
Range        Sparse         45.3 µs      34.5 µs       6.2 µs
Range        URL            48.6 µs      31.9 µs       6.3 µs
RangeWindow  Dense          16.9 µs      14.5 µs     408.0 ns
RangeWindow  Sparse         62.1 µs      41.3 µs     372.0 ns
RangeWindow  URL            55.2 µs      37.8 µs     456.0 ns
```
<!-- bench:optime:end -->

<!-- bench:opspace:start -->
```
Op     Workload    Chapter6 B   allocs   Chapter5 B   allocs      btree B   allocs
Put    Dense          60.1 KB    2 012      60.1 KB    2 012     109.6 KB    1 115
Put    Sparse        530.3 KB    2 328     526.6 KB    2 328      86.3 KB    1 085
Put    URL           438.0 KB    2 603     431.8 KB    2 603     121.4 KB    1 088
Range  Dense            312 B        9        112 B        3         96 B        3
Range  Sparse          5.8 KB      238        112 B        3         96 B        3
Range  URL             9.6 KB      397        112 B        3         96 B        3
```
<!-- bench:opspace:end -->

Two distinct costs:

- **Method dispatch is somewhat slower than the concrete type
  switch, worst on hot short paths.** Go's interface method call
  is one indirect jump
  through the itable. The compiler can't inline through it. On
  hot, short paths like `Get` on Sparse, the per-iteration
  dispatch dominates the work and shows up in the bench.
- **Closures passed to interface methods escape to the heap.**
  `eachAscending(yield func(byte, node) bool)` takes a
  `func`-typed argument; when the callee is reached through an
  interface, Go's escape analysis cannot prove the closure stays
  on the caller's stack and allocates it. Chapter 5's
  `eachAscending` was a free function with concrete-type case
  bodies; the compiler could inline the cases and keep closures
  on the stack. The new Range allocations (~one per inner node
  visited — compare the Range rows' allocs columns above) come
  from this. They're small — tens of bytes each — but they are
  visible.

### Capacity

<!-- bench:capacity:start -->
```
Workload    Chapter6 keys     B/key   Chapter5 keys     B/key      btree keys     B/key
Dense           1 563 926      67.1       1 563 930      67.1       1 239 809      84.6
Sparse            514 952     603.7         526 482     597.1       1 634 039      64.6
URL               214 683     489.1         219 401     478.4       1 091 012      96.4
```
<!-- bench:capacity:end -->

The refactor is capacity-neutral, as it should be.

### What did the tradeoff buy?

Code shape, measured by counts that don't lie:

```
                       Chapter 5    Chapter 6
free dispatch fns         10            0
type-switch cases         20            0
per-type method sets       0            2
inner-node interface
  methods                  —           11
```

The decision was: spend a modest hot-path latency tax and ~one
allocation per inner-node visit during `Range`, in exchange for
deleting ten type-switch helpers, splitting `reshape` along type
boundaries, and **making the chapter
[7](../07-add-node16/tutorial.md) / chapter
[8](../08-add-node48/tutorial.md) diffs a new file plus a few
boundary edits, with no changes to the operation bodies**.

Whether that's a good trade depends on what we're optimising for.
We are not optimising for raw point-lookup latency — chapter 5
was already faster than btree on `Get` on every workload, and
this chapter still is. We are optimising for **reading and
changing** the code: a future maintainer (human or AI) seeing
`n.findChild(b)` in `Put` doesn't need to know whether `n` is a
node4 or node256 or, in chapters [7](../07-add-node16/tutorial.md)
and [8](../08-add-node48/tutorial.md), a node16 or node48. That
maintainer also doesn't need to remember to update ten helpers
when adding a fifth node type.

## The chapter-6 promise

Adding `node16` in chapter [7](../07-add-node16/tutorial.md) is,
end to end:

1. Define `type node16[V any] struct { ... }`.
2. Implement the eleven `innerNode` methods on it.
3. Decide which inner-node sizes promote and demote into node16,
   and update `growToNode16` / `shrinkToNode4` boundaries.

`Put`, `Get`, `Delete`, `Range`, `splitTwoLeaves`,
`splitPrefixedNode`, `consumePrefix`, `longestCommonPrefix`,
`collapseEmpty`, `mergePrefixIntoChild` — none of them changes.
That's the easy change chapter 6 made possible.
