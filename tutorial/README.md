# Tutorial: building an Adaptive Radix Tree, one decision at a time

This is the literate tutorial that ships alongside the
`art.Tree` source. Audience: Go programmers who know the sorted
map API (Go's `map` plus an ordered iterator, or `google/btree`'s
shape) but have never built or studied a trie.

## Why this is written as a linear series of decisions

Real software design is iterative — backtracking, dead ends,
opinions that flip three times before they stick. **The story you
tell about a design afterward is not the story of how the design
was actually arrived at.** That gap is the subject of David
Parnas and Paul Clements's 1986 paper [*A Rational Design Process:
How and Why to Fake
It*](https://www.cs.tufts.edu/~nr/cs257/archive/david-parnas/fake-it.pdf).
Their argument: even though real design is messy, the *write-up*
should present a rational, linear sequence — because that is the
form a reader can follow, criticise, and learn from. We "fake" the
rational design by documenting it as if it had been planned from
the start.

This tutorial is exactly that fake. The eight chapters present
ART as a linear sequence of decisions, each one motivated by the
previous chapter's measured shortfall, each one closing a
specific gap. The actual history of ART (and of this tutorial's
construction) was not nearly so tidy. But for a reader trying to
*understand* ART — what it is, why it has the shape it has, what
each piece earns its keep doing — the rational design is the
useful one.

Each chapter is its own runnable Go package under
`tutorial/<chapter>/`. Read `tutorial.md` for the prose, look at
`art.go` and `art_test.go` for the working code, and run
`go test -bench=. -benchmem -benchtime=300ms ./tutorial/<chapter>/`
to reproduce the headline numbers.

## What each chapter teaches

The technique introduced in each chapter is the title; the
underlying engineering lesson is below it.

| # | Technique | Lesson |
|---|---|---|
| 0 | What a trie is | Shorter explanation than expected. Byte-by-byte descent gives sorted iteration for free, and trades one node-traversal per byte for the lookup-by-comparison cost of a B-tree. |
| 1 | Test harness | Build the differential tester (against `google/btree`) + a regression scenario suite + a 100 MB capacity probe. Every chapter from 2 onward is exercised by this harness. (Prose pending; package code lives at [`tutorial/harness/`](harness/).) |
| 2 | The simplest possible trie (node256-only) | Disaster baseline. One node type, full 256-fanout, no leaves, no prefix compression. ~31 KB/key on Sparse, ~4 000 keys before the 100 MB budget. The cost-to-beat for every later chapter. |
| 3 | Lazy expansion | When unique tails are common, a leaf is cheaper than a chain of inner nodes. Stop expanding past the last branching point. |
| 4 | Path compression | When prefixes are shared, encode the run in one prefix field instead of one inner node per byte. Mirror image of chapter 3: chapter 3 saved tail bytes; chapter 4 saves prefix bytes. |
| 5 | Smaller node types (node4) with two-case dispatch | Smaller nodes save space when fanout is low. The naive way to dispatch — a type switch per operation — is bearable at two cases and visibly painful at four. Hold that pain for one chapter. |
| 6 | Polymorphism | "Make the change easy, then make the easy change." Refactor before adding the third and fourth node types, not after. The numbers you measure during a refactor describe what the trade cost; they do not drive the decision. |
| 7 | The easy change (node16) | One new struct + 11 method implementations + 5 lines of surgical edits. Adding the third node type costs no edits to operation bodies. The chapter-6 investment is collected here. |
| 8 | The completed ladder (node48) | The "Adaptive" in *Adaptive Radix Tree* means the *shape of the data* picks which sizes to use. Not every node type earns its seat for every workload. |
| 9 | Polish + reading guide | Three small refinements (inline-key buffer, embedded `innerHeader`, reused path buffer in `Range`) close the gap to the production code. Each is a focused trade — bytes for allocs, repetition for promotion, per-yield allocation for per-call buffer. |

The measurements in each chapter's `tutorial.md` quantify each
trade. They are presented because *we are engineering* —
juggling tradeoffs whose effects we have measured, not guessing.
The quality of each decision is what matters; the size of any
single speedup is bookkeeping.

## What got added in each chapter

| # | Path | What's added | Status |
|---|---|---|---|
| 0 | [`00-what-is-a-trie/`](00-what-is-a-trie/tutorial.md) | Prose primer: what a trie is, why byte-by-byte descent, where it shines and where it doesn't. No code. | ✅ shipped |
| 1 | [`harness/`](harness/) | Test harness: `SortedMap` interface + adapters (btree, map[string]int), `RunDiff` op-trace runner with random + 14 regression scenarios + meta-test, `MeasureCapacity` for a 100 MB budget. | 🚧 prose pending |
| 2 | [`02-node256-only/`](02-node256-only/tutorial.md) | Disaster baseline: one node type, full 256-fanout, no leaves, no prefix compression. Get / Put / Delete / Range walked through end-to-end. First chapter wired into the chapter-1 differential test harness (`harness.RunRegression` + `harness.RunDiff`). ~31 KB/key on Sparse; ≈ 4 000 keys before 100 MB. | ✅ shipped |
| 3 | [`03-lazy-expansion/`](03-lazy-expansion/tutorial.md) | Add a leaf type for tail-only paths. Sparse bytes/key drops 30×; All allocations drop to zero. | ✅ shipped |
| 4 | [`04-path-compression/`](04-path-compression/tutorial.md) | `prefix []byte` on inner nodes; one node can consume a run of bytes that don't branch. URL bytes/key drops 2×; URL Get drops 2.8×; Stage 3 Get is faster than btree on every workload. | ✅ shipped |
| 5 | [`05-add-node4/`](05-add-node4/tutorial.md) | Add a 4-child sorted-array node and dispatch via type-switch helpers. URL bytes/key drops 3.9×; Range on URL is 4.4× faster. Get gets slower (1.3–2.3×) — the dispatch cost. | ✅ shipped |
| 6 | [`06-introduce-polymorphism/`](06-introduce-polymorphism/tutorial.md) | Refactor: nine type-switch helpers become 11 methods on an `innerNode` interface. Behaviour identical; chapter 7's diff for adding node16 becomes new-file-only. The price: ~10–25% on hot-path Get latency and a closure allocation per inner node during Range. Engineering — quality of decision, not size of speedup. | ✅ shipped |
| 7 | [`07-add-node16/`](07-add-node16/tutorial.md) | New struct + 11 method implementations + 5 lines of edits to existing types. Sparse heap drops 5.17×; URL drops 3.0×. Stage 6 is within 1.4–2.0× of btree's heap on Sparse / URL. The price: ~20% more time on Get for medium-fanout workloads (linear scan vs array index). | ✅ shipped |
| 8 | [`08-add-node48/`](08-add-node48/tutorial.md) | New struct + 11 method impls + 3 surgical edits. Same diff shape as chapter 7. node48 is unused at 1k fixture size — the 17–48 fanout band is empty there — but at Sparse-5k the heap drops 2.35× (234 → 99 B/key, 182 inner nodes settle into node48) and Put goes 1.7× faster than btree. The lesson: the four-type ladder is workload-adaptive; not every type earns its seat for every workload. | ✅ shipped |
| 9 | [`09-polish/`](09-polish/tutorial.md) | Three polishes: inline-key buffer (Put allocs drop ~2× on short keys), embedded `innerHeader` (16 trivial methods deleted), `Range` with reused path buffer (zero per-yield allocations). Plus a reading guide to the parent `art.Tree` source. | ✅ shipped |

## How the per-chapter numbers work

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

Bench-output labels `Stage1`/`Stage2`/... in chapters 3-9 are
historical: they retain pre-renumber numbering. New labels will
appear when chapter content is rewritten.

## Beyond per-chapter — the scaling annex

[`BENCHMARKS.md`](BENCHMARKS.md) tracks the chapter-9 implementation
(equivalent to the production `art.Tree`) against `google/btree`
across map sizes from 1 000 to 100 000 000 keys. It answers the
question "does the per-chapter story still hold at production
scale?" with one rolled-up table per workload.

## A note on what these numbers mean

Bench output is meant to be read alongside the prose, not as a
performance leaderboard. Each chapter's headline is "what did this
decision cost or save?", measured against the previous chapter, in
the workload where the decision matters most. The tutorial's goal
is *understanding*, not winning a benchmark.

## Reading the whole thing as one document

To assemble all chapters plus the appendix into a single markdown
file (and a self-contained styled HTML), run:

```
python3 tutorial/build_book.py
```

The script writes `tutorial/_book/art-tutorial.md` and
`tutorial/_book/art-tutorial.html`. The output directory is
gitignored — it is meant for local reading, not commit. Re-run any
time after editing chapters or after `go test ./... -update-prose`
refreshes the bench regions.
