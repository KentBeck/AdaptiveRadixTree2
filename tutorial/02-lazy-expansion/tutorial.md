# Chapter 2 — Lazy expansion (leaves)

Chapter 1 ended with one number above all the others:

> Sparse: 15.25 nodes/key, ~31 KB/key

The disaster came from one rule: every byte of every key forces a
fresh `node256`, even when there's nothing at that level to branch
on. A 16-byte random key allocates 16 inner nodes. ~2 KB each.

The fix is direct: when a key has no sibling — when it occupies a
unique tail in the trie — store the (key, value) pair as a single
*leaf* and skip the chain of one-child node256s entirely.

## What changes

A second node kind enters the design.

```go {src=art.go decls=node,leaf,leaf.isNode,node256,node256.isNode}
type node interface {
	isNode()
}

type leaf[V any] struct {
	key   []byte
	value V
}

func (*leaf[V]) isNode() {}

type node256[V any] struct {
	children [256]node
	terminal *leaf[V]
}

func (*node256[V]) isNode() {}
```

Three changes that follow:

- A `Tree.root` is now `node` (interface), not `*node256` (struct):
  a tree of one entry is a single leaf.
- A `node256.children[b]` slot can hold either kind: a leaf for a
  unique tail, or another inner node when more branching follows.
- A new `terminal *leaf[V]` slot on `node256` carries the leaf for
  a key that ends exactly at that node's path. Without it, the key
  `"hell"` could not coexist with `"hello"` — the inner node
  reached by walking `"hell"` would have nowhere to put `"hell"`'s
  value while still hosting the `'o'` child for `"hello"`.

A note on the empty-method `node` interface: this is a *sum-type
marker*, not method polymorphism. Every dispatch in this chapter is
an explicit type assertion. Chapter 5 will introduce a sibling
interface `innerNode` with real methods so that the four ART node
sizes (chapters 4–7) can be dispatched without a switch.

## Put walks down, splitting on collision

The three cases in `putInto` map directly to the three structural
states a child slot can be in:

```go {src=art.go decl=putInto}
func putInto[V any](current node, key []byte, value V, depth int, size *int) node {
	if current == nil {
		*size++
		return &leaf[V]{key: append([]byte(nil), key...), value: value}
	}

	if l, ok := current.(*leaf[V]); ok {
		if bytes.Equal(l.key, key) {
			l.value = value
			return l
		}
		return splitTwoLeaves(l, key, value, depth, size)
	}

	n := current.(*node256[V])
	if depth == len(key) {
		if n.terminal == nil {
			*size++
			n.terminal = &leaf[V]{key: append([]byte(nil), key...), value: value}
		} else {
			n.terminal.value = value
		}
		return n
	}
	b := key[depth]
	n.children[b] = putInto(n.children[b], key, value, depth+1, size)
	return n
}
```

The interesting case is `splitTwoLeaves`. When two keys land in the
same slot, we must build the inner-node structure to host them
both. They share some leading bytes; somewhere they diverge. The
divergence point determines how many node256s we need.

```go {src=art.go decl=splitTwoLeaves}
func splitTwoLeaves[V any](existing *leaf[V], newKey []byte, newValue V, depth int, size *int) node {
	diverge := depth
	for diverge < len(existing.key) && diverge < len(newKey) && existing.key[diverge] == newKey[diverge] {
		diverge++
	}

	head := &node256[V]{}
	n := head
	for i := depth; i < diverge; i++ {
		next := &node256[V]{}
		n.children[existing.key[i]] = next
		n = next
	}

	*size++
	newLeaf := &leaf[V]{key: append([]byte(nil), newKey...), value: newValue}

	switch {
	case diverge == len(existing.key):
		n.terminal = existing
		n.children[newKey[diverge]] = newLeaf
	case diverge == len(newKey):
		n.terminal = newLeaf
		n.children[existing.key[diverge]] = existing
	default:
		n.children[existing.key[diverge]] = existing
		n.children[newKey[diverge]] = newLeaf
	}
	return head
}
```

For two random 16-byte Sparse keys the loop body runs ~zero times
(they diverge at byte 0 ~99.6% of the time), so `splitTwoLeaves`
allocates one `node256` instead of chapter 1's chain of sixteen.

For two URL keys sharing `"https://api.example.com/v1/users/"` —
33 bytes — the loop builds a chain of 33 `node256`s. Lazy
expansion does not save us from URL-shaped prefix sharing; that's
chapter 3's job.

## Get walks down, comparing at the leaf

```go {src=art.go decl=Get}
func (t *Tree[V]) Get(key []byte) (V, bool) {
	var zero V
	current := t.root
	depth := 0
	for current != nil {
		if l, ok := current.(*leaf[V]); ok {
			if bytes.Equal(l.key, key) {
				return l.value, true
			}
			return zero, false
		}
		n := current.(*node256[V])
		if depth == len(key) {
			if n.terminal == nil {
				return zero, false
			}
			return n.terminal.value, true
		}
		current = n.children[key[depth]]
		depth++
	}
	return zero, false
}
```

The shape of a Get is now: descend until you reach a leaf or run
out of edges, then either compare the leaf's full key or return
the terminal. The compare is a single `bytes.Equal` on a slice up
to `len(key)`. We will see in the bench numbers that on short keys
this trade — fewer descents but one full-key compare — costs a
little time on Dense (the descent was already cheap) and saves a
lot on Sparse (the descent was 16 cache misses).

## Delete now collapses

The reshape rules grow:

- **0 children, no terminal:** node is dead; return nil.
- **0 children, terminal:** node is now just a single key — return
  that terminal leaf in place of the inner node.
- **1 leaf child, no terminal:** the leaf carries the full key and
  the path that led here is recoverable from those bytes. Hoist
  the leaf — the parent's slot now points straight at it.
- **1 inner child, no terminal:** keep the chain. Without prefix
  compression we cannot represent the byte that this inner node
  consumed. Chapter 3 fixes that.

The recursive `deleteFrom` returns the new (possibly collapsed)
node and a `deleted` flag; `reshape` evaluates the rules above:

```go {src=art.go decl=reshape}
func reshape[V any](n *node256[V]) node {
	var only node
	count := 0
	for _, c := range n.children {
		if c != nil {
			count++
			only = c
		}
	}
	if count == 0 {
		if n.terminal != nil {
			return n.terminal
		}
		return nil
	}
	if count == 1 && n.terminal == nil {
		if l, ok := only.(*leaf[V]); ok {
			return l
		}
	}
	return n
}
```

The collapse cascades: when an inner node hoists a leaf upward,
its parent may now also satisfy the 1-leaf-child collapse and hoist
again. A whole chain can dissolve into a single leaf, which is
exactly what should happen after `Delete("help")` from a tree
containing only `{"hello", "help"}`.

## All yields the leaf's key directly

Chapter 1 had to allocate-and-copy the path on every yield because
the key only existed as the path of edges traversed. Here, every
leaf carries its own key — yield it directly.

```go {src=art.go decl=iterate}
func iterate[V any](n node, yield func([]byte, V) bool) bool {
	if l, ok := n.(*leaf[V]); ok {
		return yield(l.key, l.value)
	}
	r := n.(*node256[V])
	if r.terminal != nil {
		if !yield(r.terminal.key, r.terminal.value) {
			return false
		}
	}
	for b := 0; b < 256; b++ {
		c := r.children[b]
		if c == nil {
			continue
		}
		if !iterate(c, yield) {
			return false
		}
	}
	return true
}
```

The terminal yields *first* because its key is a strict prefix of
every child's key, and prefixes sort earlier byte-wise. This is the
same invariant the production `art.Tree.All` relies on.

## What lazy expansion bought, measured

Same workloads as chapter 1, same machine, same Go version. Run
`go test -bench=. -benchmem -benchtime=300ms ./tutorial/02-lazy-expansion/`
to reproduce. Stage 2 is benchmarked alongside Stage 1 for the
per-decision impact.

### Structural footprint

<!-- bench:innernodemix:start -->
```
Workload    Stage 1 inner    Stage 2 inner + leaves
Dense          1011           11 + 1000
Sparse        15246          234 + 1000
URL            8085          834 + 1000
```
<!-- bench:innernodemix:end -->

Per-node sizes (from `unsafe.Sizeof`): a stage-1 node is 2 056 B
(`[256]*node + *V`); a stage-2 node256 is 4 104 B (`[256]node` —
the slot is now an interface = 16 B each, not 8); a leaf is 32 B
plus its key bytes.

Two numbers per workload below: **structural** (sum of
unsafe.Sizeof contributions for every live node) and **heap**
(actual `runtime.HeapAlloc` delta after building the tree, which
includes malloc rounding and per-allocation bookkeeping). Heap
matches the `B/op` you see in the bench output for `Put`. The
heap numbers are what your process actually pays.

```
Workload    Stage 1                       Stage 2                      heap improvement
            structural    heap            structural    heap
Dense        2 078 B    2 337 B             85 B        93 B           25×
Sparse      31 345 B   35 134 B          1 008 B    1 186 B           30×
URL         16 622 B   18 636 B          3 495 B    4 136 B            4.5×
```

Sparse, the chapter-1 disaster, dropped from ~35 KB/key on the
heap to ~1.2 KB/key — a 30× reduction in actual memory. URL saw
only a 4.5× improvement because long URL prefixes still demand
long chains of node256s, and a chain of one-child node256s costs
the same 4 KB per node whether it has 1 child or 256. That's
chapter 3's headline target.

### Time per operation

```
Op    Workload     Stage 1            Stage 2          btree
Put    Dense          709 µs            99 µs (7.2×)    119 µs
Put    Sparse       9 708 µs           447 µs (22×)     176 µs
Put    URL          6 203 µs         2 467 µs (2.5×)    202 µs
Get    Dense            9.1 ns          17.2 ns (0.5×)  107 ns
Get    Sparse         145   ns          10.9 ns (13×)   134 ns
Get    URL            213   ns         199   ns (1.07×) 140 ns
All    Dense          200 µs            5.6 µs (36×)      4 µs
All    Sparse       3 686 µs           50   µs (74×)      4 µs
All    URL          1 762 µs          215   µs (8×)       4 µs
```

Three outcomes worth pointing at:

- **Get on Sparse is 13× faster than Stage 1 and 12× faster than
  btree.** Stage 1 walked 16 levels (16 cache misses); Stage 2
  walks one or two before reaching a leaf, then does a single
  `bytes.Equal` on the full key. Fewer cache misses dominate.
- **Get on Dense is *slower* than Stage 1.** Two costs rise: the
  interface type assertion at every loop iteration, and the leaf
  compare reads the full key rather than just walking
  node-by-node. On Dense where the chapter-1 walk was already
  ~9 ns, the new overhead doubles the time. Chapter 3 recovers
  this on Dense by collapsing the chain of leading-zero nodes
  into a single prefix, cutting the walk *and* keeping the leaf
  compare on the same short key.
- **Put on Dense nearly matches btree.** Stage 1 was 6× slower;
  Stage 2 is within 17%. Lazy expansion removed the per-Put
  allocation blizzard.

### Allocations

```
Op        Workload     Stage 1 allocs   Stage 2 allocs   btree allocs
Put       Dense          2 011               2 012             113
Put       Sparse        16 246               2 235              83
Put       URL            9 085               2 835              86
All       Dense          1 001                   0               0
All       Sparse         2 247                   0               0
All       URL            1 431                   0               0
```

Put-Sparse allocations dropped from ~16 / key to ~2 / key (one
leaf, plus its inline key copy). Put-URL still allocates ~3 /
key because long-prefix keys still need chains of inner nodes.
All allocations vanish entirely: leaves carry their own keys,
nothing to copy on yield.

## What's still wrong

Two structural waste cases remain:

1. **Long shared prefixes still cost one node256 per byte.** On
   URL keys sharing 33-byte hosts, every shared byte allocates an
   inner node with one child — 4 KB per node either way. Chapter
   3 introduces a `prefix []byte` field on inner nodes so a single
   node can consume a run of bytes that don't branch.
2. **Even after path compression, every inner node still reserves
   256 child slots.** A node with 2 children pays for 254 nil
   interface slots. Chapters 4–7 introduce smaller node sizes
   that allocate room for what's actually used.

Chapter 3's headline target is the URL row above: from ~4 KB/key
on the heap to something closer to `~70 B/key` (btree's number).
