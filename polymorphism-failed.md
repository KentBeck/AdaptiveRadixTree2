# Polymorphism refactor: rejected

**Commit reverted:** `ff7fba5 refactor: move per-node-type operations into methods on each node type`

**Decision:** revert. The structural move did not pay for itself and introduced
coupling that was not present before.

## What the refactor did

Replaced free functions `putInto`, `putIntoNode{4,16,48,256}`, `deleteFrom`, and
the inline type switches in `Tree.Get` and `iterate` with methods on an
expanded `innerNode` interface:

```go
type innerNode interface {
    node

    // Operations
    put(key []byte, value any, depth int) (node, bool)
    get(key []byte, depth int) (any, bool)
    delete(key []byte, depth int) (node, bool)
    iterate(yield func([]byte, any) bool) bool

    // Structural
    findChild(b byte) node
    removeChild(b byte)
    isEmpty() bool

    // Path compression accessors
    getPrefix() []byte
    setPrefix([]byte)
    getTerminal() *leaf
    setTerminal(*leaf)
}
```

`Tree.Put/Get/Delete/All` became thin dispatchers over `*leaf` + `innerNode`.

Verification was clean: `go build`, 50 tests PASS, 2.4M fuzz execs with 0 failures,
`go vet` clean, `gofmt -l .` clean.

## Why it was rejected

### 1. Total line count increased

| File        | Before | After | Δ      |
|-------------|-------:|------:|-------:|
| types.go    |    395 |   944 | **+549** |
| put.go      |    219 |    65 |   −154 |
| get.go      |     76 |    22 |    −54 |
| delete.go   |    237 |   121 |   −116 |
| iterate.go  |    255 |   203 |    −52 |
| helpers.go  |     71 |    71 |      0 |
| **Total**   | **1253** | **1426** | **+173 (+14%)** |

The per-file shrinkage in put/get/delete/iterate was real but was more than offset
by types.go growing 2.4×. The stated goal of "reducing duplication" did not
materialize in line terms because the task's explicit out-of-scope rule kept the
prefix-match / terminal / exhausted-key logic duplicated per node type.

### 2. `types.go` lost its isolation

Before: types.go defined types and their structural operations only. Zero
outbound calls beyond the standard library (and it did not need any imports).

After: types.go calls into

- `helpers.go` — `longestCommonPrefix`, `newLeaf`, `newNode4With`, `splitPrefixedInner`
- `put.go`     — `node4AddOrGrow`, `node16AddOrGrow`, `node48AddOrGrow`
- `delete.go`  — `postDeleteReshape`

and imports `bytes`. The file now carries the entire tree's operation logic in
addition to type definitions, so the "types" file has become the largest and
most connected file in the package.

### 3. File-level dependency cycle (in spirit)

types.go → put.go (`*AddOrGrow`) → types.go (receiver methods on struct types).
Legal in one Go package, but the "one file, one concern" mental model breaks:
operation logic on node types lives in types.go, but the grow helpers it calls
live in put.go.

### 4. Interface widened with dead code

`getPrefix / setPrefix / getTerminal / setTerminal` were added to the interface
and implemented 4× (one per node type), for 16 one-line methods with **zero
callers** today. They were included on speculation. Interface widening is a soft
ratchet: every future node type must implement them, and removing an unused
interface member after the fact is harder than not adding it.

### 5. Dispatch logic duplicated 16×

Each of the 16 operation methods (4 types × `put/get/delete/iterate`) carries
its own `switch child.(type) { nil / *leaf / innerNode }` block to dispatch to
children. Previously `putInto`, `deleteFrom`, the `Get` loop, and the `iterate`
helper each owned this in one place. The change multiplied the dispatch sites
from ~4 to 16 without removing the underlying need to handle each case.

### 6. Prefix-check fan-out

12 near-identical prefix-check blocks (4 `put` + 4 `get` + 4 `delete`) vs. ~8
before (4 in `putInto` prefix-split + 4 in `deleteFrom`; `Get`'s single loop
had 4 switch branches). Small absolute delta, but the task's "pure structural
move" carve-out locked this duplication in as a precondition for a later task.

## What would have justified the move

The refactor only pays off if the follow-up — extracting
`handlePrefixAndTerminal` or similar shared helpers and collapsing the
per-method duplication — actually lands and drives total LOC below baseline
while keeping the per-file isolation. As a standalone change it trades a
compact, loop-driven implementation for a scattered, method-dispatched one at
a cost of +173 lines and new cross-file coupling centered on `types.go`.

## Outcome

- Commit `ff7fba5` reverted.
- Tree on `sorted-map-via-art` returns to the state of `900513f` plus this
  write-up.
- Follow-up consideration: if the polymorphic form is still desired, do it
  together with the prefix/terminal deduplication so the combined change
  reduces total LOC rather than inflating types.go.

