# Tree invariants

This document is the single, machine-checkable list of structural and
accounting invariants the `art` package maintains. It exists so that:

1. A human reading the code can map each rule to the lines that enforce
   it.
2. An AI-assisted refactor can be checked against an explicit list
   rather than against scattered comments.
3. Each invariant has at least one test named `TestInvariant_<Name>` in
   `art_test.go` that exercises it directly. Behavioural tests
   (`TestPutThenGet`, `TestRange...`) cover the same ground from the
   outside; the invariant tests pin down the structural reason those
   behaviours hold.

If you change internals, every invariant below must still be true at
every public-API boundary (after each `Put`, `Delete`, `Clear`, and
`Clone`). Mid-operation transient states are exempt — only the
externally observable tree must satisfy them.

The fuzz harness (`art_fuzz_test.go`) verifies behavioural equivalence
to a Go `map` plus sorted oracle, so behavioural drift is caught there.
The invariants below are *structural* and not visible behaviourally,
which is why they need their own tests.

---

## 1. Sorted child arrays in `node4` and `node16`

`node4` and `node16` keep `keys[:numChildren]` strictly ascending by
edge byte. `addChild` / `insertChild` insert at the right slot;
`removeChild` shifts later entries left to keep the array dense and
sorted. `node48` and `node256` index by edge byte directly and are not
ordered arrays, so this rule does not apply to them.

- Enforced: `types.go:100-110` (`node4.addChild`),
  `types.go:138-149` (`node4.removeChild`),
  `types.go:211-221` (`node16.insertChild`),
  `types.go:248-259` (`node16.removeChild`).
- Test: `TestInvariant_Node4ChildrenSortedAscending`,
  `TestInvariant_Node16ChildrenSortedAscending`.

## 2. `numChildren` matches occupied slots

For every inner node, `numChildren` equals the number of child slots
currently in use:

- `node4`/`node16`: the count of populated entries in the
  `keys`/`children` prefix.
- `node48`: the count of non-zero entries in `childIndex`, and equals
  the count of populated `children[:numChildren]`.
- `node256`: the count of non-nil entries in `children`.

The terminal slot is **not** counted in `numChildren` — it is the leaf
stored at the node's exact path, not a branching child.

- Enforced: every increment/decrement in `addChild`/`removeChild`
  across `types.go`.
- Test: `TestInvariant_NumChildrenMatchesOccupiedSlots`.

## 3. Terminal key equals the node's path from root

If an inner node has a non-nil terminal, the terminal's key is exactly
the byte sequence consumed from the root to that node (root prefixes
+ branch bytes + this node's prefix, with no further byte). This is
why `Min` of a subtree is the terminal when present (a terminal key
is shorter than any extending child key under byte-wise order).

- Enforced: the terminal is only set in two places, both passing the
  full key:
  - `helpers.go:107-112` (`newNode4With` puts the exhausted side into
    `terminal`).
  - `put.go:55-61` (`putIntoInner` sets a fresh terminal when the key
    is exhausted at this node).
  - `helpers.go:130-132` (`splitPrefixedInner` puts the exhausted key
    into the new parent's terminal).
- Test: `TestInvariant_TerminalKeyEqualsNodePath`.

## 4. Post-`Delete` reshape: 0 children + no terminal → removed

After any successful `Delete`, no inner node in the tree has both
zero branching children **and** a nil terminal. Such a node would be
unreachable structure and is dropped by `reshape` returning `nil`
(via `collapseEmpty`) so the parent removes the edge.

- Enforced: `types.go:172-184` (node4),
  `types.go:279-287` (node16),
  `types.go:415-423` (node48),
  `types.go:536-544` (node256), all calling
  `collapseEmpty` (`types.go:586-591`).
- Test: `TestInvariant_NoEmptyInnerNodesAfterDelete`.

## 5. Post-`Delete` reshape: 1 child + no terminal collapses

A `node4` with exactly one branching child and no terminal is
collapsed: the lone child replaces the parent, with the parent's
prefix and the branch byte merged into the child's prefix (or the
leaf is hoisted directly when the only child is a leaf). Larger inner
node types do not have this case because they reach `numChildren <=
node4Capacity` first and demote to `node4`, where the collapse rule
then applies.

- Enforced: `types.go:176-182` (`node4.reshape`),
  `types.go:597-604` (`mergePrefixIntoChild`).
- Test: `TestInvariant_SingleChildNode4Collapses`.

## 6. Post-`Delete` reshape: demotion when count crosses capacity

When child count drops to (or below) the next-smaller node type's
capacity, the node demotes:

- `node16` → `node4` at `numChildren == 4`
  (`types.go:283-285`, `shrinkToNode4`).
- `node48` → `node16` at `numChildren == 16`
  (`types.go:419-421`, `shrinkToNode16` — note that `shrinkToNode16`
  walks `childIndex` in ascending edge-byte order so the resulting
  `node16` satisfies invariant #1).
- `node256` → `node48` at `numChildren == 48`
  (`types.go:540-542`, `shrinkToNode48`).

Demotion preserves prefix, terminal, and the full child set.

- Test: `TestInvariant_DemotesAtCapacityBoundaries`.

## 7. Promotion when capacity is exceeded

`addOrGrow` is the only way to add a child. When the current type is
full it grows to the next size, copying prefix, terminal, and all
children. `node256` never grows.

- `node4` (cap 4) → `node16` via `growToNode16`
  (`types.go:115-123`, `types.go:291-299`).
- `node16` (cap 16) → `node48` via `growToNode48`
  (`types.go:225-233`, `types.go:427-438`).
- `node48` (cap 48) → `node256` via `growToNode256`
  (`types.go:347-355`, `types.go:548-560`).

Test: `TestInvariant_PromotesAtCapacityBoundaries`.

## 8. `node48` index/edge consistency

For every non-zero `childIndex[b]`, the slot it points to is occupied
and `childEdge[slot-1] == b`. For every occupied slot `i <
numChildren`, `childIndex[childEdge[i]] == i+1`. The 0 sentinel in
`childIndex` means "absent" so live slots are 1-indexed.
`removeChild` keeps `children[:numChildren]` dense by swapping the
last live slot into the freed position and updating its `childIndex`.

- Enforced: `types.go:338-343` (`addChild`),
  `types.go:371-387` (`removeChild`).
- Test: `TestInvariant_Node48IndexAndEdgeConsistency`.

## 9. `Tree.size` equals the number of live leaves

Every `Tree[V].size` adjustment lives at exactly two chokepoints:

- `helpers.go:24-27` (`insertLeaf`) increments on every brand-new
  leaf allocation (Put paths).
- `helpers.go:33-41` (`clearTerminalIfMatches`) decrements when a
  terminal leaf is cleared during Delete.
- `delete.go:30-34` decrements when a branching leaf is removed
  during Delete.

Replace-value paths (Put on an existing key) do not touch size. The
oracle test enumerates all reachable leaves and compares to
`Tree.Len()`.

- Test: `TestInvariant_SizeEqualsLeafCount`.

## 10. Leaf keys are stable copies

`newLeaf` (`helpers.go:5-17`) always copies the caller's key, either
into the inline buffer (≤ 24 bytes) or onto the heap. Mutating the
slice the caller passed to `Put` after the call must not change any
key in the tree.

- Test: `TestInvariant_LeafKeyIsStableCopy`.

## 11. `Clone` produces a structurally independent tree

After `t2 := t.Clone()`, every inner node along any path differs in
identity from the corresponding node in `t`, so a structural mutation
(`Put`, `Delete`) on either tree leaves the other unaffected. Leaves
are also freshly allocated. Key bytes may be shared because keys are
treated as read-only by the contract.

- Enforced: `sorted.go:282-299` (`cloneNode`), using each node type's
  `shallow()` to produce a fresh copy.
- Test: `TestInvariant_CloneIsStructurallyIndependent`.

## 12. Children appear in ascending byte order under iteration

For every inner node and every node type, `eachAscending` yields
children in strictly ascending edge-byte order, and `eachDescending`
yields them in strictly descending edge-byte order. This is the
foundation of `All`, `Range`, `Min`/`Max`, and `Ceiling`/`Floor`.

- Enforced: `types.go:151-167` (node4),
  `types.go:261-277` (node16),
  `types.go:389-413` (node48),
  `types.go:510-534` (node256).
- Test: `TestInvariant_EachAscendingIsSorted`,
  `TestInvariant_EachDescendingIsSorted`.

## 13. Inline-key boundary

A leaf's `key` slice points into its own `inline` array iff
`len(key) <= inlineKeyMax` (24). Otherwise `key` is a heap slice
disjoint from `inline`. This affects nothing observable but is
load-bearing for the no-extra-alloc claim on short keys.

- Enforced: `helpers.go:5-17` (`newLeaf`).
- Test: `TestInvariant_InlineKeyBoundary`.

---

## How to add an invariant

1. State it here in one sentence, in the imperative ("Children of
   node4 are sorted ascending"), then a short paragraph explaining
   why and the file:line that enforces it.
2. Add a `TestInvariant_<Name>` test in `art_test.go` that builds a
   tree exhibiting the case and asserts the rule directly (not a
   round-trip Get).
3. If the invariant is observable behaviourally, the existing
   `FuzzSortedMap` already covers it. If it is purely structural
   (most of the list above), the new test is the only line of
   defence.
