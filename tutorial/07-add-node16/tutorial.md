# Chapter 7 — Add node16 (the easy change)

This is chapter [6](../06-introduce-polymorphism/tutorial.md)'s
payoff.

Chapter 6 promised that adding the next inner-node type would be a
new struct file plus minimal edits. Here is the diff that landed
that promise:

| File | Change | Lines |
|---|---|---|
| `art.go` | new `node16[V]` struct + 11 method implementations + `growToNode16` + `shrinkToNode16` | +130 |
| `art.go` | `node4.addOrGrowChild`: grow target changed from `node256` to `node16` | 1 line |
| `art.go` | `node256.reshape`: demote target changed from `node4` to `node16`, threshold from 4 to 16 | 2 lines |
| `art.go` | `growToNode256` rebound from `*node4 -> *node256` to `*node16 -> *node256`; `shrinkToNode4` rebound from `*node256 -> *node4` to `*node16 -> *node4` | 2 lines |
| `art.go` | `CountByKind` widened to `(n4, n16, n256 int)` | 5 lines |

Everything else — `Put`, `Get`, `Delete`, `Range`, `splitTwoLeaves`,
`splitPrefixedNode`, `consumePrefix`, `longestCommonPrefix`,
`collapseEmpty`, `mergePrefixIntoChild` — is character-for-character
unchanged from chapter 6. That's what the polymorphism investment
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

Same acceptance criteria, same yardsticks: the tables below are
rendered by `go test -update-bench` from the shared harness
benchmarks — this chapter's tree alongside chapter
[6](../06-introduce-polymorphism/tutorial.md)'s, with
`google/btree` for context. Reproduce any cell with
`go test -bench=. -benchmem -benchtime=300ms ./07-add-node16/`.

### Per-node sizes

<!-- bench:nodesizes:start -->
```
Type       Bytes   Slot
node4        120   sorted [4]keys + [4]children
node16       320   sorted [16]keys + [16]children
node256     4144   indexed [256]children
leaf          32   key slice header + value (V == int)
```
<!-- bench:nodesizes:end -->

### Inner-node mix

<!-- bench:innernodemix:start -->
```
Workload    Chapter 6 (n4 + n256)      Chapter 7 (n4 + n16 + n256)
Dense           1 + 4                      1 + 0 + 4
Sparse        141 + 93                   141 + 92 + 1
URL           330 + 63                   330 + 63 + 0
```
<!-- bench:innernodemix:end -->

URL is the most striking: every chapter-6 node256 — 63 of them —
fit in node16. Those 63 nodes shrank from 4 144 B to 320 B each,
saving roughly 240 KB on a 1 000-key tree.

Sparse is similar: 92 of the 93 chapter-6 node256s settled into
node16. Only one root node256 remains, holding ~250 first-byte
children that genuinely need node256's array indexing.

Dense is unchanged: its node256s hold 256 leaf children at the
last byte, where node16 cannot help.

### Live heap

<!-- bench:heapfootprint:start -->
```
Workload    Chapter6 heap   Chapter7 heap improvement
Dense            59 B/key        59 B/key    1.00×
Sparse          518 B/key       100 B/key    5.18×
URL             429 B/key       143 B/key    3.00×
```
<!-- bench:heapfootprint:end -->

Sparse lands within **~1.5× of btree's heap**; URL within ~2×.
The trie was two orders of magnitude over btree on Sparse at the
end of chapter [2](../02-node256-only/tutorial.md), ~17× over at
the end of chapter [3](../03-lazy-expansion/tutorial.md), ~7×
after node4, and is now within striking distance.

### Time and allocations per operation

<!-- bench:optime:start -->
```
Op           Workload      Chapter7     Chapter6        btree
Put          Dense         114.9 µs     115.3 µs     190.5 µs
Put          Sparse        189.6 µs     340.9 µs     257.2 µs
Put          URL           359.2 µs     403.0 µs     308.7 µs
Get          Dense          37.0 ns      36.0 ns     155.0 ns
Get          Sparse         47.0 ns      44.0 ns     167.0 ns
Get          URL           137.0 ns     136.0 ns     196.0 ns
Range        Dense           9.9 µs       9.6 µs       6.3 µs
Range        Sparse         25.7 µs      45.4 µs       6.2 µs
Range        URL            32.2 µs      47.5 µs       6.3 µs
RangeWindow  Dense          14.5 µs      14.5 µs     368.0 ns
RangeWindow  Sparse         30.2 µs      52.5 µs     357.0 ns
RangeWindow  URL            40.3 µs      55.0 µs     452.0 ns
```
<!-- bench:optime:end -->

<!-- bench:opspace:start -->
```
Op     Workload    Chapter7 B   allocs   Chapter6 B   allocs      btree B   allocs
Put    Dense          61.4 KB    2 016      60.1 KB    2 012     109.6 KB    1 115
Put    Sparse        112.6 KB    2 329     530.3 KB    2 328      86.3 KB    1 085
Put    URL           151.8 KB    2 603     438.0 KB    2 603     121.4 KB    1 088
Range  Dense            312 B        9        312 B        9         96 B        3
Range  Sparse          5.8 KB      238       5.8 KB      238         96 B        3
Range  URL             9.6 KB      397       9.6 KB      397         96 B        3
```
<!-- bench:opspace:end -->

Put and Range got faster on Sparse and URL. Smaller per-node
mallocs (node16's 320 B vs node256's 4 144 B) is the main driver
for Put; fewer slots to scan per node is the driver for Range.
Put bytes on Sparse drop ~4.7× — within a small factor of btree.

The honest cost is on `Get`: Sparse and URL each give back
roughly 10-20%, run to run. **That's the tradeoff we deliberately made.** A
node16's `findChild` scans up to 16 keys linearly:

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
Sparse and URL we did exactly that conversion at scale. We
accepted ~10-20% more `Get` time for ~3-5× less heap. We are
engineering: the numbers describe the trade, and we made it
deliberately.

### Capacity

<!-- bench:capacity:start -->
```
Workload    Chapter7 keys     B/key   Chapter6 keys     B/key      btree keys     B/key
Dense           1 564 103      67.1       1 563 929      67.1       1 239 811      84.6
Sparse          1 141 608     207.1         514 951     603.7       1 634 046      64.6
URL               755 036     139.6         214 685     489.1       1 091 012      96.4
```
<!-- bench:capacity:end -->

The 100 MB ceilings tell the same story at scale: Sparse and URL
capacity multiply, and Sparse now approaches btree's.

## What's still wrong

Look at the inner-node mix table again. URL has 330 node4s and
63 node16s. Many of those node16s probably hold 5–10 children:
not big enough for node256 to be needed, but the per-node-16
overhead is still ~3× node4's. There's room for a *fourth* node
type sized for 17–48 children to keep things tight, and that's
chapter [8](../08-add-node48/tutorial.md)'s `node48`.

There is also still a meaningful Get cost we can claw back at
the polish stage (chapter [9](../09-polish/tutorial.md)):
inline-key buffers, an embedded header struct via Go promotion, a
reused path buffer in `Range`, and so on.

The chapter-8 diff will look like this chapter's: one new struct
file, a few surgical edits, and that's it.
