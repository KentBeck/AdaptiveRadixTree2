# Chapter 1 — A node256-only tree

The simplest possible trie. One node type. Every node has 256
children slots and an optional terminal value. No leaves, no
prefix compression. A key of length k always traverses k inner
nodes. An inner node always allocates 256 child slots whether it
uses 1 of them or all 256.

This chapter exists to be the disaster baseline. The numbers below
are the cost of doing nothing clever; every later chapter takes
one decision against this baseline and measures the saving.

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

`children[b]` is a pointer to the child reached by following edge
byte `b`. `terminal` is non-nil exactly when some key ends at this
node's path — which is the byte sequence consumed from the root.
`*V` rather than `V` so the zero value of `V` doesn't collide with
"no entry here".

## Get walks down, returning early on a missing edge

```go {src=art.go decl=Get}
func (t *Tree[V]) Get(key []byte) (V, bool) {
	var zero V
	if t.root == nil {
		return zero, false
	}
	n := t.root
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

`children[b]` is a direct array index, not a search. `node256` is
fast at lookup precisely because the alphabet *is* the index. That
speed is why later chapters keep `node256` around for the cases
where it earns its keep — a hot, dense sub-trie — instead of
replacing it everywhere.

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
	v := value
	n.terminal = &v
}
```

There is no resize, no rebalance, no comparison. Every byte of
`key` becomes one step downward. Every step costs a pointer
dereference and possibly an allocation.

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

The recursion returns two booleans: did we find and remove the
key, and is the node we just touched now empty (no terminal, no
children). The parent uses the second flag to decide whether to
detach. Without this pruning, deleting `"hello"` would leave a
six-deep chain of empty `node256`s in the tree forever.

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
		// Copy the path -- the caller may retain the key, and we
		// reuse the path buffer below.
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

`Range(from, to)` yields every (key, value) pair whose key falls
in the half-open interval `[from, to)`; a `nil` bound is unbounded
on that side, so `Range(nil, nil)` walks the whole tree. The
recursion in `walk` iterates `b` from 0 to 255 at every node, so
children are visited in ascending byte order and the yielded keys
are byte-wise sorted with no comparison and no balancing tree. In
this chapter the bounds are enforced at the leaf, after the full
descent; chapter 8 will prune subtrees by prefix so out-of-range
work is skipped instead of filtered. The `make + copy` per yield
is also wasteful; chapter 8 fixes that with a reusable path
buffer.

## The disaster, measured

Workload sizes are 1 000 entries each. Numbers were taken on a
4-core 64-bit machine; reproduce with
`go test -bench=. -benchmem -benchtime=300ms ./tutorial/01-node256-only/`.

### Structural footprint

The footprint here is *just* the inner-node memory: 256 child
pointers + the terminal pointer per node, on a 64-bit machine.
The reader's keys and values are not counted.

<!-- bench:footprint:start -->
```
Workload    nodes/key  bytes/key   per node
─────────────────────────────────────────────
Dense        1.01        2.0 KB      2056 B
Sparse      15.25       30.6 KB      2056 B
URL          8.09       16.2 KB      2056 B
```
<!-- bench:footprint:end -->

Sparse keys are the worst case: every byte forces a fresh node256
because random keys share no prefixes. ~15 nodes per 16-byte key
matches "one node per byte" almost exactly. URL keys share their
host prefixes and so amortise to ~8 nodes/key despite being
twice as long. Dense integer keys all share their high-order
zero bytes, so one chain at the top is reused by every key —
nodes/key is barely above 1.

For comparison, `google/btree` stores about **70 B/key** on the
Sparse workload — roughly **400× tighter** than this trie. That
is the gap that motivates chapters 2 through 7.

### Time per operation (vs `google/btree`)

```
Op    Workload    Stage 1            google/btree     winner
─────────────────────────────────────────────────────────────
Put    Dense        719 µs            120 µs           btree (6×)
Put    Sparse    24 036 µs            196 µs           btree (122×)
Put    URL       18 020 µs            210 µs           btree (86×)
Get    Dense          8.8 ns          109 ns           Stage 1 (12×)
Get    Sparse       142   ns          128 ns           btree (slightly)
Get    URL          199   ns          140 ns           btree (1.4×)
Range  Dense        197 µs              4 µs           btree (50×)
Range  Sparse     3 835 µs              4 µs           btree (1 060×)
Range  URL        1 784 µs              4 µs           btree (490×)
```

The interesting cells:

- **Get on Dense is 12× faster than btree.** The trie does 8
  array indexes; btree does a tree of binary searches. When keys
  are short and cache-friendly, the trie wins lookups handily —
  this is the whole reason ART exists.
- **Put is awful.** Allocating ~16 node256s per Put on Sparse
  costs ~16 allocations × ~2 KB each. Most of the 24 ms is
  `runtime.mallocgc` and zeroing memory. Lazy expansion (chapter
  2) collapses each chain to a single leaf.
- **Range is awful.** The recursion sweeps every one of the
  thousands of empty child slots on every node256. Even on Dense
  (where there are only ~1 000 nodes), `Range(nil, nil)` does
  1 000 × 256 ≈ 256 000 nil checks. Smaller node types
  (chapters 4 — 7) reduce that to scanning only the children that
  exist.

### Allocations

```
Op       Workload   Stage 1 allocs    btree allocs
─────────────────────────────────────────────────
Put      Dense       2 012 / 1k         113 / 1k
Put      Sparse     16 247 / 1k          83 / 1k
Put      URL         9 086 / 1k          86 / 1k
```

Put allocates roughly nodes-per-key + 1, plus an allocation per
terminal pointer. `btree` allocates one node per ~12 entries (its
fanout) plus its leaf items. The order-of-magnitude gap is what
chapters 2 — 7 close.

## What's wrong, in one sentence

Every node carries the cost of all 256 possible children whether
or not it uses them, and every byte of a unique tail forces a
fresh node even though there's nothing to branch on.

Chapter 2 fixes the second half by adding a leaf type for unique
tails. Chapter 3 fixes the first half partially by giving each
node a path-compressed prefix so a long shared run of bytes
doesn't allocate intermediate inner nodes. Chapters 4 — 7 fix the
rest by introducing smaller node types when the actual fanout is
much less than 256.

The two tables above are the baseline against which every later
chapter will be compared.
