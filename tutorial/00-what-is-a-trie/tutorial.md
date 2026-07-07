# Chapter 0 — What's a trie?

This tutorial assumes you're a Go programmer who's used `map[K]V`
and has reached for a sorted map at some point — `google/btree`,
or one of the third-party red-black tree libraries —
and got a feel for the API. It does *not* assume you know what a
trie is or how an Adaptive Radix Tree differs from one.

Chapter 1 builds the test harness. Chapter 2 builds the simplest
imaginable trie — a working sorted map, and a memory disaster.
Chapters 3 through 8 each add one decision: lazy expansion, path
compression, smaller node types, polymorphism. By chapter 9 you can
read the project's main `art.Tree` source as a known artifact
rather than a wall of code. This chapter is just the primer — no
Go yet.

## The premise

A sorted map is a `map[K]V` whose iteration order is the natural
ascending order of `K`, not insertion order or hash order. The
operations are familiar:

```
Put(k, v)       // insert or replace
Get(k)          // (v, ok)
Delete(k)       // remove if present
Len()           // count
Range(from, to) // iterator in ascending key order, from <= k < to
```

Either bound on `Range` may be omitted (nil) to mean "unbounded on
that side", so `Range(nil, nil)` walks every key.

Two well-known data structures cover this surface:

- **Hash maps with side indexes** (e.g. `map[K]V` plus a sorted
  slice you maintain by hand). Lookups are O(1), but every mutation
  costs O(n) on the slice or O(log n) on a heap, and the iteration
  order is whatever you paid to maintain.
- **Self-balancing BSTs** — red-black, AVL, B-tree. Lookups,
  inserts, and deletes are O(log n); iteration is a tree walk.
  These compare keys as opaque blobs: every comparison reads
  *every byte* of the key until it finds a difference.

Tries take the third path: **don't compare keys; walk them.**

## The trie idea

A trie ("retrieval tree", traditionally pronounced *try*) is a tree
where each *edge* is labelled with one element of the key (a byte,
in our case), and the path from root to a node is the prefix of
every key reachable below it. There is no key comparison. Lookup
reads the key byte by byte and follows the edge labelled with that
byte.

The trie for the map `{hello: 1, help: 2, hi: 3}`, one byte per
edge:

```
(root)
└─ h
   ├─ e
   │  └─ l
   │     └─ l
   │        ├─ o        "hello" → 1
   │        └─ p        "help"  → 2
   └─ i                 "hi"    → 3
```

The keys `hello` and `help` share the four-byte prefix `hell`. They
diverge at the fifth byte: `o` vs `p`. So the trie has one chain of
edges `h → e → l → l` shared between them, and at the node reached
after consuming `hell` there are two outgoing edges, `o` and `p`,
each leading to its own value.

Two consequences fall out for free:

1. **Sorted iteration is automatic.** At each node, visit children
   in ascending byte order. The yielded keys appear in byte-wise
   sorted order with no comparisons and no balancing.
2. **Shared work for shared prefixes.** A lookup of `help` and a
   lookup of `hello` traverse the same first four edges. The
   per-byte cost depends on the keys' shape, not on the size of the
   map.

## What a trie costs

The honest tradeoffs are equally direct:

- **One node-traversal per byte.** A 16-byte key is 16 pointer
  dereferences down the tree. If the tree is bigger than your L1
  cache, that's 16 cache misses. A B-tree comparing the same key
  against a few interior keys is fewer dereferences but each
  comparison reads more bytes; the right answer depends on the
  workload.
- **Wasted nodes for sparse keysets.** Every byte position where
  any key has a unique value gets its own node. If your keys are
  random 16-byte blobs, almost every level branches, and you pay
  for one inner node per byte per key. Chapter 2 makes this
  visible: ~35 MB to store 1 000 random 16-byte keys in the
  simplest possible trie.

The chapters that follow are a series of decisions that keep the
**byte-by-byte descent** and remove the wasted nodes:

- **Lazy expansion** (chapter 3) — stop allocating inner nodes
  along a tail with no siblings.
- **Path compression** (chapter 4) — let one node represent
  several consecutive bytes when none of them branch.
- **Smaller node types** (chapters 5, 7, and 8) — when a node has
  only a handful of children, don't allocate room for 256.
- **Polymorphism** (chapter 6) — between adding the second and
  third node types, refactor so that adding the rest is mechanical.
- **Polish** (chapter 9) — inline-key buffers, embedded headers,
  a reused path buffer for `Range`. Allocations per key drop to
  roughly one.

By chapter 9 the implementation is the same shape as the
production `art.Tree` in the parent package. You will have built
it, decision by decision, and you will have measured what each
decision was worth.

Onward to chapter 1, where we build the lie-detector — and then
chapter 2, where we build the disaster.
