# Chapter 6 — Add node16 (the easy change)

This is chapter 5's payoff.

Chapter 5 promised that adding the next inner-node type would be a
new struct file plus minimal edits. Here is the diff that landed
that promise:

| File | Change | Lines |
|---|---|---|
| `art.go` | new `node16[V]` struct + 11 method implementations + `growToNode16` + `shrinkToNode16` | +130 |
| `art.go` | `node4.addOrGrowChild`: grow target changed from `node256` to `node16` | 1 line |
| `art.go` | `node256.reshape`: demote target changed from `node4` to `node16`, threshold from 4 to 16 | 2 lines |
| `art.go` | `growToNode256` rebound from `*node4 -> *node256` to `*node16 -> *node256`; `shrinkToNode4` rebound from `*node256 -> *node4` to `*node16 -> *node4` | 2 lines |
| `art.go` | `CountByKind` widened to `(n4, n16, n256 int)` | 5 lines |

Everything else — `Put`, `Get`, `Delete`, `All`, `splitTwoLeaves`,
`splitPrefixedNode`, `consumePrefix`, `longestCommonPrefix`,
`collapseEmpty`, `mergePrefixIntoChild` — is character-for-character
unchanged from chapter 5. That's what the polymorphism investment
bought.

## node16 is just node4 at a bigger size

The new struct mirrors `node4` exactly, scaled up:

```go {src=art.go decls=node16,node16Capacity}
type node16[V any] struct {
	prefix      []byte
	keys        [node16Capacity]byte
	children    [node16Capacity]node
	terminal    node
	numChildren uint8
}

const node16Capacity = 16
```

Eleven methods, each one mechanical (`findChild`, `addChild`,
`addOrGrowChild`, `replaceChild`, `removeChild`, `eachAscending`,
`reshape`, plus the four header accessors). Reading them is reading
node4's methods with `4` rewritten to `16`. The single interesting
choice is in `reshape`:

```go {src=art.go decl=node16.reshape}
func (n *node16[V]) reshape() node {
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
	if n.numChildren <= node4Capacity {
		return shrinkToNode4[V](n)
	}
	return n
}
```

A node16 can demote *to* node4, but the ladder stops there going
down. Going up, `addOrGrowChild` promotes to node256.

## The new ladder

```
                    promote at +1 child
node4  ──────► node16 ──────► node256
node4  ◄────── node16 ◄────── node256
                    demote at boundary
```

Capacity boundaries: a node4 with 4 children grows to node16 on
the next add; a node16 with 16 grows to node256; a node256 with
≤ 16 demotes to node16; a node16 with ≤ 4 demotes to node4.
`shrinkToNode4` and `shrinkToNode16` walk children in ascending
edge-byte order so the demoted node's `keys` array is sorted
automatically.

## What the new band catches — measured

Reproduce with
`go test -bench=. -benchmem -benchtime=300ms ./tutorial/06-add-node16/`.

### Per-node sizes

```
Type       Bytes   Slot
node4        120   sorted [4]keys + [4]children
node16       320   sorted [16]keys + [16]children
node256    4 144   indexed [256]children
leaf          32   key slice header + value (V == int)
```

### Inner-node mix

```
Workload    Stage 5 (n4 + n256)        Stage 6 (n4 + n16 + n256)
Dense          1 + 4                       1 + 0 +   4
Sparse       141 + 93                    141 + 92 + 1
URL          330 + 63                    330 + 63 + 0
```

URL is the most striking: every chapter-5 node256 — 63 of them —
fit in node16. Those 63 nodes shrank from 4 144 B to 320 B each,
saving roughly 240 KB on a 1 000-key tree.

Sparse is similar: 92 of the 93 chapter-5 node256s settled into
node16. Only one root node256 remains, holding ~250 first-byte
children that genuinely need node256's array indexing.

Dense is unchanged: its node256s hold 256 leaf children at the
last byte, where node16 cannot help.

### Live heap

```
Workload    Stage 5    Stage 6    improvement     btree
Dense        59 B/key   59 B/key      1.00×       ~70 B/key
Sparse      518 B/key  100 B/key      5.17×       ~70 B/key
URL         429 B/key  143 B/key      2.99×       ~70 B/key
```

Sparse is now within **1.4× of btree's heap** (100 B/key vs
~70 B/key). URL is within **2.0×**. The trie was 30× over btree
on Sparse at the end of chapter 1, 1.7× over at the end of
chapter 2, and is now within striking distance.

### Time per operation

```
Op    Workload   Stage 5         Stage 6         change
Put    Dense        88 µs          83 µs            -6%
Put    Sparse      240 µs         138 µs            -42%
Put    URL         347 µs         274 µs            -21%
Get    Dense        29 ns          24 ns            -17%
Get    Sparse       27 ns          35 ns            +27%
Get    URL          99 ns         118 ns            +19%
All    Dense       5.0 µs         5.0 µs            tied
All    Sparse       36 µs          16 µs            -56%
All    URL          38 µs          23 µs            -40%
```

Put and All got faster on Sparse and URL. Smaller per-node mallocs
(node16's 320 B vs node256's 4 144 B) is the main driver for Put;
fewer slots to scan per node is the driver for All.

The honest cost is on `Get`: Sparse goes from 27 ns to 35 ns and
URL from 99 ns to 118 ns. **That's the tradeoff we deliberately
made.** A node16's `findChild` scans up to 16 keys linearly:

```go {src=art.go decl=node16.findChild}
func (n *node16[V]) findChild(b byte) node {
	for i := uint8(0); i < n.numChildren; i++ {
		if n.keys[i] == b {
			return n.children[i]
		}
	}
	return nil
}
```

A node256's `findChild` is one array index:

```go {src=art.go decl=node256.findChild}
func (n *node256[V]) findChild(b byte) node { return n.children[b] }
```

So at every formerly-node256 inner node that became a node16, Get
now pays an O(fanout) loop instead of O(1) array indexing. On
Sparse and URL we did exactly that conversion at scale. We accepted
~20% more `Get` time for ~3-5× less heap. We are engineering: the
numbers describe the trade, and we made it deliberately.

### Allocations

```
Op       Workload     Stage 5 B/op    Stage 6 B/op    btree B/op
Put      Dense          60 KB           61 KB            102 KB
Put      Sparse        530 KB          113 KB             70 KB
Put      URL           438 KB          152 KB             73 KB
```

Sparse Put now allocates **4.7× fewer bytes** than chapter 5 —
within 1.6× of btree.

## What's still wrong

Look at the inner-node mix table again. URL has 330 node4s and
63 node16s. Many of those node16s probably hold 5–10 children:
not big enough for node256 to be needed, but the per-node-16
overhead is still ~3× node4's. There's room for a *fourth* node
type sized for 17–48 children to keep things tight, and that's
chapter 7's `node48`.

There is also still a meaningful Get cost we can claw back at
the polish stage (chapter 8): inline-key buffers, embedded
header-struct via Go promotion, a reused path buffer in `Range`,
and so on.

The chapter-7 diff will look like this chapter's: one new struct
file, two surgical edits, and that's it.
