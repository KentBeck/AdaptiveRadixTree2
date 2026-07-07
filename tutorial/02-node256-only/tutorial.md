# Chapter 2 — A node256-only tree

This chapter builds the simplest trie that could possibly
compile. One inner-node type, with a 256-slot child table. No
leaves. No prefix compression. Every byte of every key forces a
fresh node, so a 16-byte random key allocates 16 inner nodes —
about 31 KB of pointer table per key on the Sparse workload.
This is a disaster, and that is the point: chapter 2 sets the
cost-to-beat that every later chapter measures itself against.

## The data type

```go {src=art.go decls=node,Tree}
type node[V any] struct {
	children [256]*node[V]
	terminal *V
}

type Tree[V any] struct {
	root *node[V]
	size int
}
```

`children` is a fixed array of 256 slots — one for every possible
byte value. The byte itself is the index, so descending one level
of the trie is `n = n.children[b]`: an O(1) array load, not a
search. That alphabet-as-index is the trie's one genuinely fast
property in this chapter, and it is what every other node type
in chapters 5-8 will be trying to emulate at smaller widths.
`terminal` is `*V` rather than `V` so the absence of a value at
this node is a nil pointer, distinguishable from a present zero
value.

## Get walks down, returning early on a missing edge

```go {src=art.go decl=Get}
func (t *Tree[V]) Get(key []byte) (V, bool) {
	var zero V
	n := t.root
	if n == nil {
		return zero, false
	}
	for _, b := range key {
		n = n.children[b]
		if n == nil {
			return zero, false
		}
	}
	if n.terminal == nil {
		return zero, false
	}
	return *n.terminal, true
}
```

One byte at a time. Each iteration either finds a child slot
populated and continues, or finds nil and returns `(zero, false)`
immediately. There is no comparison and no search at any level —
`children[b]` is the alphabet-as-index again. After the descent,
the only remaining question is whether *this* node is itself a
terminus for the key.

## Put walks down, allocating as needed

```go {src=art.go decl=Put}
func (t *Tree[V]) Put(key []byte, value V) {
	if t.root == nil {
		t.root = &node[V]{}
	}
	n := t.root
	for _, b := range key {
		if n.children[b] == nil {
			n.children[b] = &node[V]{}
		}
		n = n.children[b]
	}
	if n.terminal == nil {
		t.size++
	}
	n.terminal = &value
}
```

Every byte position that doesn't already have a child allocates a
fresh node256 — 2 KB of pointer table — and walks into it. No
comparisons, no resize, no rebalance. A length-k key inserted
into an empty tree allocates k+1 nodes, every one of them holding
a single child until the terminus. This is what makes the Sparse
workload (random 16-byte keys, almost no shared prefix) a
disaster: the second key shares one byte with the first ~0.4% of
the time, so almost every key pays its own ~16-node tax.

## Delete walks down, then prunes back up

```go {src=art.go decls=Delete,deleteFrom}
func (t *Tree[V]) Delete(key []byte) bool {
	if t.root == nil {
		return false
	}
	deleted, empty := deleteFrom(t.root, key, 0)
	if deleted {
		t.size--
	}
	if empty {
		t.root = nil
	}
	return deleted
}

func deleteFrom[V any](n *node[V], key []byte, depth int) (deleted, empty bool) {
	if depth == len(key) {
		if n.terminal == nil {
			return false, false
		}
		n.terminal = nil
		return true, isEmpty(n)
	}
	child := n.children[key[depth]]
	if child == nil {
		return false, false
	}
	deleted, childEmpty := deleteFrom(child, key, depth+1)
	if !deleted {
		return false, false
	}
	if childEmpty {
		n.children[key[depth]] = nil
	}
	return true, isEmpty(n)
}
```

`deleteFrom` walks down to the terminus, clears the `terminal`
pointer, then unwinds. At each level on the way back up, if the
child the recursion just returned from went empty (no terminal,
no children), the parent's slot is cleared too; if the parent in
turn has nothing left, it propagates upward. Without this
unwind, deleting `"hello"` would leave six empty node256s along
the path — 12 KB of dead pointer tables that no other key ever
reaches. `TestDeletePrunesAllTheWay` enforces it: insert one key,
delete it, assert the node count is back to zero.

## Range yields in sorted order for free

```go {src=art.go decls=Range,walk}
func (t *Tree[V]) Range(from, to []byte) iter.Seq2[[]byte, V] {
	return func(yield func([]byte, V) bool) {
		if t.root == nil {
			return
		}
		walk(t.root, nil, func(k []byte, v V) bool {
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

func walk[V any](n *node[V], path []byte, yield func([]byte, V) bool) bool {
	if n.terminal != nil {
		out := make([]byte, len(path))
		copy(out, path)
		if !yield(out, *n.terminal) {
			return false
		}
	}
	for b := 0; b < 256; b++ {
		c := n.children[b]
		if c == nil {
			continue
		}
		if !walk(c, append(path, byte(b)), yield) {
			return false
		}
	}
	return true
}
```

`walk` visits the children of every node in ascending byte order
(`for b := 0; b < 256; b++`). Because the children-by-byte order
is also the lexicographic order of keys passing through this
node, an in-order traversal yields keys sorted with no comparison
and no balancing. This is the property that makes a trie a sorted
map at all: it is paid for by the node layout, once, at design
time. The `from`/`to` filter is applied at the leaf because at
this stage we have no way to skip a subtree by prefix; chapter 9
adds prefix-aware pruning. One cost worth flagging: the `out :=
make([]byte, len(path)); copy(...)` allocates a fresh key buffer
per yielded pair, because `path` is mutated by the recursion and
cannot leave the walker alive. Chapter 9 will eliminate that with
a reused buffer.

## How we know it's correct

Chapter [1](../01-test-harness/tutorial.md) introduced a
differential test harness, and this chapter is the first to wire
into it. `TestAcceptance` calls `harness.RunAcceptance`: every
named scenario plus a 1000-op random trace, each diffed op-by-op
against `google/btree`. The same single test, against the same
oracle, runs in every chapter from here on; the acceptance bar
never moves even as the data structure underneath changes shape
four times. The harness has its own meta-test
(`harness.TestDiff_DetectsDivergence`) that confirms the runner
actually fails when the candidate misbehaves — without that, a
green build would prove nothing.

## The disaster, measured

The tables below are rendered by `go test -update-bench` from the
shared harness benchmarks (see chapter
[1](../01-test-harness/tutorial.md)) — 1 000 keys per workload,
this chapter's tree against `google/btree`. `Put` rows describe a
full 1000-key build; `RangeWindow` iterates the middle 1% of the
keyspace. Reproduce any cell with
`go test -bench=. -benchmem -benchtime=300ms ./02-node256-only/`.

<!-- bench:optime:start -->
```
Op           Workload      Chapter2        btree
Put          Dense         769.3 µs     141.5 µs
Put          Sparse        39.27 ms     213.5 µs
Put          URL           14.11 ms     244.4 µs
Get          Dense          11.0 ns     107.0 ns
Get          Sparse        157.0 ns     130.0 ns
Get          URL           228.0 ns     145.0 ns
Range        Dense         136.0 µs       5.9 µs
Range        Sparse         4.00 ms       5.9 µs
Range        URL            1.41 ms       5.7 µs
RangeWindow  Dense         146.4 µs     284.0 ns
RangeWindow  Sparse         3.02 ms     288.0 ns
RangeWindow  URL            1.45 ms     360.0 ns
```
<!-- bench:optime:end -->

<!-- bench:opspace:start -->
```
Op     Workload    Chapter2 B   allocs      btree B   allocs
Put    Dense           2.3 MB    2 012     109.6 KB    1 115
Put    Sparse         35.1 MB   16 247      86.3 KB    1 085
Put    URL            18.6 MB    9 086     121.4 KB    1 088
Range  Dense           8.1 KB    1 004         96 B        3
Range  Sparse         34.1 KB    2 250         96 B        3
Range  URL            75.2 KB    1 434         96 B        3
```
<!-- bench:opspace:end -->

Get on Dense beats btree by an order of magnitude, and Get on
Sparse sits in btree's neighborhood — the alphabet-as-index
descent is genuinely fast when each level is a single array load.
Everything else is a bloodbath: Put on Sparse is two orders of
magnitude slower than btree and allocates sixteen 2 KB nodes per
key; Range on Sparse is hundreds of times slower; and a
`RangeWindow` over 1% of the keys costs as much as a full walk,
because without prefix pruning the iterator visits every node and
filters at the leaf.

The headline number is bytes/key, captured by the 100 MB capacity
probe (`harness.MeasureCapacity`):

<!-- bench:capacity:start -->
```
Workload    Chapter2 keys     B/key      btree keys     B/key
Dense              46 009     2 329       1 239 809      84.6
Sparse              3 984    34 654       1 634 046      64.6
URL                 7 373    16 091       1 091 007      96.4
```
<!-- bench:capacity:end -->

A few thousand keys before 100 MB on Sparse, against btree's
million and a half. That gap is the chapter's headline and the
motivator for chapter [3](../03-lazy-expansion/tutorial.md).

## What's wrong, in one sentence

Every node carries a 2 KB pointer table whether it has 1 child or
256, and unique tails (random keys with no shared prefix) pay
that cost at every level — chapter 3 fixes the unique-tail half
by introducing leaves, and chapter 4 fixes the shared-prefix half
via path compression.
