# Chapter 3 — Lazy expansion (leaves)

Chapter [2](../02-node256-only/tutorial.md) ended with one
number above all the others:

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
an explicit type assertion. Chapter
[6](../06-introduce-polymorphism/tutorial.md) will introduce a
sibling interface `innerNode` with real methods so that the four
ART node sizes (chapters [2](../02-node256-only/tutorial.md),
[5](../05-add-node4/tutorial.md), [7](../07-add-node16/tutorial.md),
[8](../08-add-node48/tutorial.md)) can be dispatched without a
switch.

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

## Put walks down, splitting on collision

`Put` keeps the one-method public surface from chapter 2, but the
work shifts into a recursive `putInto` helper that takes
`(current, depth)` and returns the (possibly different) node
that should occupy the slot it was called on. The recursion is
what lets a leaf split into a node256 — the call site swaps the
returned node into place without knowing what kind it is:

```go {src=art.go decl=Put}
func (t *Tree[V]) Put(key []byte, value V) {
	t.root = putInto(t.root, key, value, 0, &t.size)
}
```

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
allocates one `node256` instead of chapter 2's chain of sixteen.

For two URL keys sharing `"https://api.example.com/v1/users/"` —
33 bytes — the loop builds a chain of 33 `node256`s. Lazy
expansion does not save us from URL-shaped prefix sharing; that's
chapter [4](../04-path-compression/tutorial.md)'s job.

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
  consumed. Chapter [4](../04-path-compression/tutorial.md) fixes
  that.

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

## Range yields the leaf's key directly

Chapter 2 had to allocate-and-copy the path on every yield because
the key only existed as the path of edges traversed. Here, every
leaf carries its own key — yield it directly.

```go {src=art.go decls=Range,iterate}
func (t *Tree[V]) Range(from, to []byte) iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if t.root == nil {
			return
		}
		iterate(t.root, func(k []byte, v V) bool {
			if from != nil && bytes.Compare(k, from) < 0 {
				return true
			}
			if to != nil && bytes.Compare(k, to) >= 0 {
				return true
			}
			return yield(k, v)
		})
	}
}

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

`Range(from, to)` yields every (key, value) pair whose key falls
in the half-open interval `[from, to)`; a `nil` bound is unbounded
on that side, so `Range(nil, nil)` walks the whole tree. The
terminal yields *first* because its key is a strict prefix of
every child's key, and prefixes sort earlier byte-wise. This is the
same invariant the production `art.Tree.Range` (and its `All()`
shorthand) relies on. In this chapter the bounds are enforced at
the leaf, after the full descent; chapter
[9](../09-polish/tutorial.md) will make `Range` prune subtrees by
prefix instead of walking every leaf and filtering.

## What lazy expansion bought, measured

Same workloads as chapter [2](../02-node256-only/tutorial.md),
same acceptance criteria, same yardsticks: the tables below are
rendered by `go test -update-bench` from the shared harness
benchmarks. This chapter's tree is measured alongside chapter 2's
so the decision's impact is visible, with `google/btree` for
context. Reproduce any cell with
`go test -bench=. -benchmem -benchtime=300ms ./03-lazy-expansion/`.

### Structural footprint

<!-- bench:innernodemix:start -->
```
Workload    Chapter 2 inner    Chapter 3 inner + leaves
Dense            1011             11 + 1000
Sparse          15246            234 + 1000
URL              8085            834 + 1000
```
<!-- bench:innernodemix:end -->

Per-node sizes (from `unsafe.Sizeof`): a chapter-2 node is
2 056 B (`[256]*node + *V`); this chapter's node256 is 4 104 B
(`[256]node` — the slot is now an interface = 16 B each, not 8);
a leaf is 32 B plus its key bytes.

Two numbers per workload below: **structural** (sum of
`unsafe.Sizeof` contributions for every live node) and **heap**
(actual `runtime.HeapAlloc` delta after building the tree, which
includes malloc rounding and per-allocation bookkeeping). The
heap numbers are what your process actually pays.

<!-- bench:footprint:start -->
```
Workload   Chapter2 struct       heap  Chapter3 struct       heap  improvement
Dense              2 078 B    2 337 B             85 B       93 B        25.1×
Sparse            31 345 B   35 134 B          1 008 B    1 186 B        29.6×
URL               16 622 B   18 635 B          3 495 B    4 136 B         4.5×
```
<!-- bench:footprint:end -->

Sparse, the chapter-2 disaster, dropped from ~35 KB/key on the
heap to ~1.2 KB/key — roughly 30× less actual memory. URL
improved far less, because long URL prefixes still demand long
chains of node256s, and a chain of one-child node256s costs the
same 4 KB per node whether it has 1 child or 256. That's
chapter [4](../04-path-compression/tutorial.md)'s headline
target.

### Time and allocations per operation

<!-- bench:optime:start -->
```
Op           Workload      Chapter3     Chapter2        btree
Put          Dense          98.2 µs     772.9 µs     139.7 µs
Put          Sparse        466.1 µs      9.84 ms     215.3 µs
Put          URL            2.44 ms      5.39 ms     248.1 µs
Get          Dense          18.0 ns      11.0 ns     107.0 ns
Get          Sparse         11.0 ns     175.0 ns     131.0 ns
Get          URL           203.0 ns     245.0 ns     143.0 ns
Range        Dense           7.6 µs     217.1 µs       6.1 µs
Range        Sparse         53.8 µs      4.16 ms       6.0 µs
Range        URL           188.4 µs      1.84 ms       5.9 µs
RangeWindow  Dense          10.8 µs     209.5 µs     307.0 ns
RangeWindow  Sparse         56.8 µs      4.43 ms     304.0 ns
RangeWindow  URL           204.2 µs      1.80 ms     368.0 ns
```
<!-- bench:optime:end -->

<!-- bench:opspace:start -->
```
Op     Workload    Chapter3 B   allocs   Chapter2 B   allocs      btree B   allocs
Put    Dense          93.5 KB    2 012       2.3 MB    2 012     109.6 KB    1 115
Put    Sparse          1.2 MB    2 235      35.1 MB   16 247      86.3 KB    1 085
Put    URL             4.1 MB    2 835      18.6 MB    9 086     121.4 KB    1 088
Range  Dense            112 B        3       8.1 KB    1 004         96 B        3
Range  Sparse           112 B        3      34.1 KB    2 250         96 B        3
Range  URL              112 B        3      75.2 KB    1 434         96 B        3
```
<!-- bench:opspace:end -->

Three outcomes worth pointing at:

- **Get on Sparse is an order of magnitude faster than chapter
  2's tree.** Chapter 2 walked 16 levels (16 cache misses); this
  chapter walks one or two before reaching a leaf, then does a
  single `bytes.Equal` on the full key. Fewer cache misses
  dominate.
- **Get on Dense is *slower* than chapter 2's.** Two costs rise:
  the interface type assertion at every loop iteration, and the
  leaf compare reads the full key rather than just walking
  node-by-node. On Dense where the chapter-2 walk was already
  cheap, the new overhead roughly doubles the time. Chapter
  [4](../04-path-compression/tutorial.md) recovers this on Dense
  by collapsing the chain of leading-zero nodes into a single
  prefix, cutting the walk *and* keeping the leaf compare on the
  same short key.
- **Put allocations on Sparse dropped from ~16 per key to ~2 per
  key** (one leaf, plus its key copy) — the allocation blizzard
  was the disaster's engine, and it's gone. Put on URL still
  allocates ~3 per key because long-prefix keys still need
  chains of inner nodes. Range allocations vanish entirely:
  leaves carry their own keys, nothing to copy on yield.

### Capacity

The same 100 MB question as chapter
[2](../02-node256-only/tutorial.md):

<!-- bench:capacity:start -->
```
Workload    Chapter3 keys     B/key   Chapter2 keys     B/key      btree keys     B/key
Dense           1 563 028      67.1          46 009     2 329       1 239 809      84.6
Sparse            125 000     1 527           3 984    34 654       1 634 035      64.6
URL                54 997     1 916           7 373    16 091       1 091 012      96.4
```
<!-- bench:capacity:end -->

The Sparse ceiling rose from a few thousand keys past a hundred
thousand — the same ~30× as the heap column above — yet still an
order of magnitude short of btree. Dense now fits *more* keys
than btree: a short unique tail collapses to a single leaf just
under the root. URL is the workload where lazy expansion helped
least; the shared-prefix chains are the remaining structural
waste.

## What's still wrong

Two structural waste cases remain:

1. **Long shared prefixes still cost one node256 per byte.** On
   URL keys sharing 33-byte hosts, every shared byte allocates an
   inner node with one child — 4 KB per node either way. Chapter
   [4](../04-path-compression/tutorial.md) will introduce a
   `prefix []byte` field on inner nodes so a single node can
   consume a run of bytes that don't branch.
2. **Even after path compression, every inner node still reserves
   256 child slots.** A node with 2 children pays for 254 nil
   interface slots. Chapters [5](../05-add-node4/tutorial.md),
   [7](../07-add-node16/tutorial.md), and
   [8](../08-add-node48/tutorial.md) will introduce smaller node
   sizes that allocate room for what's actually used.

Chapter [4](../04-path-compression/tutorial.md)'s headline target
is the URL row above: from ~4 KB/key on the heap to something
closer to btree's ~70 B/key.
