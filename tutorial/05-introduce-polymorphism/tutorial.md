# Chapter 5 — Introduce polymorphism

This chapter is engineering. It adds no features and changes no
edge cases. It refactors chapter 4's nine type-switch dispatch
helpers into method calls on an `innerNode` interface that both
`*node4` and `*node256` implement.

We did this because chapter 4 was already feeling crowded with
two cases per helper, and we know two more node types are coming
(node16, node48). Method dispatch through an interface makes those
additions land as **new struct files with zero edits to `Put`,
`Get`, `Delete`, or `All`**. That's the change we want to make
easy. Chapter 6 collects the easy change.

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
them. `Put`, `Get`, `Delete`, and `All` use only the methods on
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
  interface is one fewer method for chapters 6 and 7 to
  implement.
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

Chapter 4's `reshape` was a single free function that did
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

Chapter 5 splits the responsibilities. Each concrete type's
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

Adding node16 in chapter 6 means writing a third such method,
specifying its own demote/promote thresholds. The other code
doesn't move.

## What it cost — measured

Reproduce with
`go test -bench=. -benchmem -benchtime=300ms ./tutorial/05-introduce-polymorphism/`.

### Per-node sizes

Storing `terminal` as `node` (interface, 16 B) instead of
`*leaf[V]` (pointer, 8 B) grew each inner-node struct by 8 B:

```
Type        Stage 4    Stage 5
node4         112 B      120 B
node256     4 136 B    4 144 B
leaf           32 B       32 B
```

In practice Go's allocator rounds these to the same size class,
so the live-heap footprint is unchanged.

### Live heap and allocation

```
Workload    Stage 4 heap    Stage 5 heap    ratio
Dense          59 B/key        59 B/key      1.00×
Sparse        516 B/key       518 B/key      1.00×
URL           424 B/key       429 B/key      1.01×
```

Essentially unchanged.

### Time per operation

```
Op    Workload   Stage 4         Stage 5         change
Put    Dense        79 µs           82 µs          +4%
Put    Sparse      248 µs          260 µs          +5%
Put    URL         339 µs          380 µs         +12%
Get    Dense        23 ns           26 ns         +13%
Get    Sparse       24 ns           29 ns         +24%
Get    URL          89 ns           99 ns         +10%
All    Dense       4.98 µs         5.20 µs         +4% (+ 7 allocs/op)
All    Sparse     27   µs         39   µs        +43% (+ 236 allocs/op)
All    URL        23   µs         38   µs        +63% (+ 395 allocs/op)
```

Two distinct costs:

- **Method dispatch ~10–25% slower than the concrete type
  switch.** Go's interface method call is one indirect jump
  through the itable. The compiler can't inline through it. On
  hot, short paths like `Get` on Sparse, the per-iteration
  dispatch dominates the work and shows up in the bench.
- **Closures passed to interface methods escape to the heap.**
  `eachAscending(yield func(byte, node) bool)` takes a
  `func`-typed argument; when the callee is reached through an
  interface, Go's escape analysis cannot prove the closure stays
  on the caller's stack and allocates it. Chapter 4's
  `eachAscending` was a free function with concrete-type case
  bodies; the compiler could inline the cases and keep closures
  on the stack. The new All allocations (~one per inner node
  visited) come from this. They're small — tens of bytes each —
  but they are visible.

### What did the tradeoff buy?

Code shape, measured by counts that don't lie:

```
                       Chapter 4    Chapter 5
free dispatch fns          9            0
type-switch cases         18            0
per-type method sets       0            2
inner-node interface
  methods                  —           11
```

The decision was: spend ~5–25% on hot-path latency and ~one
allocation per inner-node visit during `All`, in exchange for
deleting nine type-switch helpers, splitting `reshape` along
type boundaries, and **making the chapter 6 / chapter 7 diffs
new-file-only**.

Whether that's a good trade depends on what we're optimising for.
We are not optimising for raw point-lookup latency — chapter 4 was
already faster than btree on `Get` everywhere except URL, and
stage 5 still is. We are optimising for **reading and changing**
the code: a future maintainer (human or AI) seeing
`n.findChild(b)` in `Put` doesn't need to know whether `n` is a
node4 or node256 or, in chapters 6 and 7, a node16 or node48.
That maintainer also doesn't need to remember to update nine
helpers when adding a fifth node type.

## The chapter-6 promise

Adding `node16` in chapter 6 is, end to end:

1. Define `type node16[V any] struct { ... }`.
2. Implement the eleven `innerNode` methods on it.
3. Decide which inner-node sizes promote and demote into node16,
   and update `growToNode16` / `shrinkToNode4` boundaries.

`Put`, `Get`, `Delete`, `All`, `splitTwoLeaves`,
`splitPrefixedNode`, `consumePrefix`, `longestCommonPrefix`,
`collapseEmpty`, `mergePrefixIntoChild` — none of them changes.
That's the easy change chapter 5 made possible.
