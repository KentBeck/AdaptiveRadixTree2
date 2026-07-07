# Chapter 2 — A node256-only tree

This chapter builds the simplest trie that could possibly
compile. One inner-node type, with a 256-slot child table. No
leaves. No prefix compression. Every byte of every key forces a
fresh node, so a 16-byte random key allocates 16 inner nodes —
about 31 KB of pointer table per key on the Sparse workload. This is a disaster, and that is the point: chapter 2
sets the cost-to-beat that every later chapter measures itself
against.

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

Chapter 1 introduced a small differential test harness. Chapter
2's `art_test.go` is the first chapter to wire into it.
`TestRegression` runs every named scenario from
`harness.Scenarios()` against both this tree and `google/btree`,
asserting that every observable result agrees. `TestRandomDiff`
does the same against a randomly generated op trace. The same
two tests, against the same oracle, run in every chapter from
here on; the correctness floor never moves even as the data
structure underneath changes shape four times. The harness has
its own meta-test (`harness.TestDiff_DetectsDivergence`) that
confirms the runner actually fails when the candidate
misbehaves — without that, a green build would prove nothing.

## The disaster, measured

Run `go test -bench=. -benchmem -benchtime=300ms ./02-node256-only/`
to reproduce. The numbers below are for 1 000 keys of each
workload, against `google/btree` for context.

```
Op    Workload    Stage 2 ns/op   Stage 2 B/op   Stage 2 allocs   btree ns/op   btree B/op   btree allocs
Put   Dense          378 µs        2 337 426        2 012             61 µs       101 600         113
Put   Sparse       3 543 µs       35 134 898       16 247            105 µs        70 240          83
Put   URL          1 795 µs       18 635 890        9 086            123 µs        73 376          86
Get   Dense            4.3 ns             0            0              87 ns             0           0
Get   Sparse          65.6 ns             0            0              74 ns             0           0
Get   URL            190   ns             0            0              87 ns             0           0
Range Dense           85 µs         8 008          1 001               2.6 µs           0           0
Range Sparse       1 444 µs        33 976          2 247               2.5 µs           0           0
Range URL            773 µs        75 096          1 431               2.5 µs           0           0
```

Get-on-Sparse beats btree (65.6 ns vs 74 ns) — the alphabet-as-index
descent is genuinely fast on randomly distributed keys, where the
trie is short. Get-on-Dense beats btree by 20× for the same
reason. Everything else is a bloodbath: Put on Sparse is 34×
slower than btree and allocates 16 000 nodes for 1 000 keys.
Range on Sparse is 580× slower than btree.

The headline number is bytes/key, captured by a 100 MB capacity
probe (`harness.MeasureCapacity`):

```
Workload    Stage 2 keys-fit   Stage 2 B/key   btree keys-fit   btree B/key
Dense           45 000             2 330         1 240 000           84.7
Sparse           4 000            34 653         1 620 000           64.6
URL              7 000            16 139         1 090 000           96.4
```

≈ 4 000 keys before 100 MB on Sparse. That number is the chapter's
headline and the motivator for chapter 3.

## What's wrong, in one sentence

Every node carries a 2 KB pointer table whether it has 1 child or
256, and unique tails (random keys with no shared prefix) pay
that cost at every level — chapter 3 fixes the unique-tail half
by introducing leaves, and chapter 4 fixes the shared-prefix half
via path compression.
