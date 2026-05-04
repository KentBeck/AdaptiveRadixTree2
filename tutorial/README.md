# Tutorial: building an Adaptive Radix Tree, one decision at a time

This is the literate tutorial that ships alongside the
`art.Tree` source. Audience: Go programmers who know the sorted
map API (Go's `map` plus an ordered iterator, or `google/btree`'s
shape) but have never built or studied a trie.

## Reading order

Each chapter is its own runnable Go package under
`tutorial/<chapter>/`. Read `tutorial.md` for the prose, look at
`art.go` and `art_test.go` for the working code, and run
`go test -bench=. -benchmem ./tutorial/<chapter>/` to reproduce
the headline numbers.

| # | Path | What's added | Status |
|---|---|---|---|
| 0 | [`00-what-is-a-trie/`](00-what-is-a-trie/tutorial.md) | Prose primer: what a trie is, why byte-by-byte descent, where it shines and where it doesn't. No code. | ✅ shipped |
| 1 | [`01-node256-only/`](01-node256-only/tutorial.md) | One node type, full 256-fanout, no leaves, no prefix compression. The disaster baseline: ~31 KB per key on sparse workloads. | ✅ shipped |
| 2 | [`02-lazy-expansion/`](02-lazy-expansion/tutorial.md) | Add a leaf type for tail-only paths. Sparse bytes/key drops 59×; All allocations drop to zero. | ✅ shipped |
| 3 | [`03-path-compression/`](03-path-compression/tutorial.md) | `prefix []byte` on inner nodes; one node can consume a run of bytes that don't branch. URL bytes/key drops 2×; URL Get drops 2.8×; Stage 3 Get is faster than btree on every workload. | ✅ shipped |
| 4 | [`04-add-node4/`](04-add-node4/tutorial.md) | Add a 4-child sorted-array node and dispatch via type-switch helpers. URL bytes/key drops 3.9×; All on URL is 4.4× faster. Get gets slower (1.3–2.3×) — the dispatch cost. | ✅ shipped |
| 5 | [`05-introduce-polymorphism/`](05-introduce-polymorphism/tutorial.md) | Refactor: nine type-switch helpers become 11 methods on an `innerNode` interface. Behaviour identical; chapter 6's diff for adding node16 becomes new-file-only. The price: ~10–25% on hot-path Get latency and a closure allocation per inner node during All. Engineering — quality of decision, not size of speedup. | ✅ shipped |
| 6 | [`06-add-node16/`](06-add-node16/tutorial.md) | New struct + 11 method implementations + 5 lines of edits to existing types. Sparse heap drops 5.17×; URL drops 3.0×. Stage 6 is within 1.4–2.0× of btree's heap on Sparse / URL. The price: ~20% more time on Get for medium-fanout workloads (linear scan vs array index). | ✅ shipped |
| 7 | [`07-add-node48/`](07-add-node48/tutorial.md) | New struct + 11 method impls + 3 surgical edits. Same diff shape as chapter 6. node48 is unused at 1k fixture size — the 17–48 fanout band is empty there — but at Sparse-5k the heap drops 2.35× (234 → 99 B/key, 182 inner nodes settle into node48) and Put goes 1.7× faster than btree. The lesson: the four-type ladder is workload-adaptive; not every type earns its seat for every workload. | ✅ shipped |
| 8 | `08-polish/` | Inline-key buffer, embedded `innerHeader`, reused path buffer for `Range`. Allocations per key drop to ~1. Becomes a reading guide to the parent package's `art.Tree`. | 🚧 planned |

## How the comparison numbers work

`tutorial/bench/` is a shared workload package every chapter imports.
It exposes three deterministic key generators — `Dense`, `Sparse`,
`URL` — plus a `google/btree` constructor for side-by-side
comparison. Chapter `N` benchmarks the same key set on the same
machine against:

- **`google/btree`**, the well-known sorted-map alternative
- **chapter `N-1`**, when applicable, to make each
  decision's contribution visible

Numbers are committed in each chapter's `tutorial.md`. They were
captured on a 4-core 64-bit machine with Go 1.23. To reproduce:

```
cd tutorial && go test -bench=. -benchmem -benchtime=300ms ./...
```

`-benchtime=300ms` is the convention. Without it, fast operations
(`Get` ~10 ns) finish before Go's bench framework reaches a stable
sample and report numbers heavily inflated by startup overhead.

Bench output is meant to be read alongside the prose, not as a
performance leaderboard. Each chapter's headline is "what did this
decision cost or save?", measured against the previous chapter, in
the workload where the decision matters most.
