# Public API Contract

This document is the single source of truth for the observable behavior of
every exported symbol in `art` and `artmap`. It describes what the code does
today, not what it ought to do; behavior gaps that violate stated project
goals are recorded here as `🚩 follow-up:` notes for later remediation
rather than fixed in this document. Contract entries are intentionally
mechanical so reviewers can find the exact behavior of any public method
in seconds.

See `doc.go` for prose-style package documentation and `README.md` for
usage and architecture. Structural rules (children sorted, terminal-key
equals path, post-`Delete` reshape, capacity-boundary promotion and
demotion, etc.) are catalogued separately in `INVARIANTS.md`, with each
rule paired 1:1 to a `TestInvariant_*` test in `invariants_test.go`.

## Conventions

Each contract entry has six fields:

1. **Signature** — copied verbatim from the source.
2. **Behavior** — one-line summary of what the method does.
3. **Preconditions** — what the caller must guarantee before calling.
4. **Postconditions** — what the method guarantees on return.
5. **Panics** — the exact panic message text (if any), with the input
   shape that triggers it. "None" means the method never panics on its
   own; nil-receiver and nil-tree panics produce the standard Go runtime
   message and are noted separately where relevant.
6. **Edge cases** — observed behavior on boundary inputs (nil key, empty
   slice, empty tree, zero-value receiver, etc.).

Throughout: a `nil` `[]byte` key and an empty-slice `[]byte{}` key are
treated as the same empty key by every method on `Tree[V]` (this is
enforced by `bytes.Equal` and inline-key normalization in `newLeaf`).
The tree can hold at most one entry under the empty key.

---

## Package `art`

### Type `Tree[V any]`

Sorted map from `[]byte` to `V`, backed by an Adaptive Radix Tree. The
struct has only unexported fields. A zero-value `Tree[V]{}` (declared
without `New[V]()`) is fully usable: `root` is `nil` and `size` is `0`,
which is the same state `New[V]()` produces.

### Type `LockedTree[V any]`

`sync.RWMutex`-guarded wrapper around `*Tree[V]`. All exported methods
acquire the wrapper's mutex (read lock for read-only methods, write lock
for mutators).

A zero-value `LockedTree[V]{}` (declared without `NewLocked[V]()`) has
a `nil` inner `tree`. Every method panics eagerly with the typed
message `art: LockedTree must be constructed with NewLocked` before
taking the mutex. The check lives at the top of every public method
(`requireConstructed`).

### `func New[V any]() *Tree[V]`

1. **Signature**: `func New[V any]() *Tree[V]`
2. **Behavior**: returns a freshly allocated empty `*Tree[V]`.
3. **Preconditions**: none.
4. **Postconditions**: returned tree has `Len() == 0`; the returned
   pointer is non-nil; the underlying root is `nil`.
5. **Panics**: none.
6. **Edge cases**: a zero-value `Tree[V]{}` is observationally
   equivalent to the value returned by `New[V]()`; calling `New[V]()` is
   recommended only for clarity.

### `func NewLocked[V any]() *LockedTree[V]`

1. **Signature**: `func NewLocked[V any]() *LockedTree[V]`
2. **Behavior**: returns a freshly allocated empty `*LockedTree[V]`
   wrapping a freshly allocated `*Tree[V]`.
3. **Preconditions**: none.
4. **Postconditions**: returned wrapper has `Len() == 0`; the wrapper
   and its inner tree are non-nil.
5. **Panics**: none.
6. **Edge cases**: callers should always use `NewLocked` rather than a
   zero-value `LockedTree[V]{}`; see the type entry above.

### `func (t *Tree[V]) Len() int`

1. **Signature**: `func (t *Tree[V]) Len() int`
2. **Behavior**: returns the number of key-value pairs currently in the
   tree.
3. **Preconditions**: none.
4. **Postconditions**: returns a non-negative count; runs in O(1).
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**: returns `0` on a freshly constructed or zero-value
   `Tree[V]`; returns `0` after `Clear`.

### `func (t *Tree[V]) Put(key []byte, value V)`

1. **Signature**: `func (t *Tree[V]) Put(key []byte, value V)`
2. **Behavior**: associates `value` with `key`, replacing any previous
   value for the same key.
3. **Preconditions**: none. Any `[]byte` value (including `nil` and
   `[]byte{}`) is accepted as a key.
4. **Postconditions**: after return, `Get(key)` returns `(value, true)`;
   `Len()` is unchanged on overwrite and incremented by one on insert;
   the caller may freely mutate or reuse `key` because the tree copies
   key bytes (inline up to 24 bytes, heap-copied otherwise).
5. **Panics**: none on the documented contract. Internally,
   `newNode4With` panics with `"art: newNode4With called with equal
   keys - invariant violation"` as defense in depth; this branch is
   unreachable from `Put` because both call sites filter equal-key
   overwrites first.
6. **Edge cases**:
   - `Put(nil, v)` and `Put([]byte{}, v)` both write to the empty key
     and are interchangeable.
   - Repeated `Put` of the same key overwrites the value; `Len()` does
     not change.
   - The tree may rebalance node capacity (`node4` → `node16` → `node48`
     → `node256`) and split prefixes during insertion; this is
     transparent to the caller.

### `func (t *Tree[V]) Get(key []byte) (value V, ok bool)`

1. **Signature**: `func (t *Tree[V]) Get(key []byte) (value V, ok bool)`
2. **Behavior**: returns the value previously stored under `key`.
3. **Preconditions**: none.
4. **Postconditions**: if `key` is present, returns `(storedValue,
   true)`; otherwise returns `(zeroValue, false)`. The tree is not
   modified.
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**:
   - `Get(nil)` and `Get([]byte{})` are equivalent and both look up the
     empty key.
   - On an empty tree (including zero-value `Tree[V]{}`) returns
     `(zeroValue, false)`.
   - Returned `value` is the stored value; any `[]byte` value would be
     the stored value, not a copy.

### `func (t *Tree[V]) Delete(key []byte) bool`

1. **Signature**: `func (t *Tree[V]) Delete(key []byte) bool`
2. **Behavior**: removes `key` from the tree.
3. **Preconditions**: none.
4. **Postconditions**: returns `true` if `key` was present (and is now
   absent); returns `false` if `key` was not present (tree unchanged).
   `Len()` decreases by exactly one when `true` is returned. After a
   successful removal the affected subtree is reshaped: empty inner
   nodes are replaced by their terminal (or removed), and inner nodes
   that drop below the next-smaller capacity boundary are demoted; a
   `node4` with one remaining child and no terminal is collapsed into
   that child with the prefix merged.
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**:
   - `Delete(nil)` and `Delete([]byte{})` both remove the empty-key
     entry if present.
   - Delete on an empty tree (including zero-value `Tree[V]{}`) returns
     `false`.
   - Deleting an absent key returns `false` without modifying the tree.

### `func (t *Tree[V]) Clear()`

1. **Signature**: `func (t *Tree[V]) Clear()`
2. **Behavior**: removes every entry from the tree by dropping the root
   reference and resetting the size.
3. **Preconditions**: none.
4. **Postconditions**: `Len()` returns `0`; the tree is in the same
   observable state as a freshly constructed `Tree[V]`. Runs in O(1).
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**: idempotent — calling `Clear` on an already empty
   tree (including a zero-value `Tree[V]{}`) is a no-op.

### `func (t *Tree[V]) Clone() *Tree[V]`

1. **Signature**: `func (t *Tree[V]) Clone() *Tree[V]`
2. **Behavior**: returns an independent structural copy of `t`.
3. **Preconditions**: none.
4. **Postconditions**: the returned tree has the same `Len()` and the
   same set of `(key, value)` pairs as `t`; subsequent writes to either
   tree do not affect the other. Inner-node structures are freshly
   allocated and leaves are freshly allocated; key bytes may be shared
   between the two trees, matching the read-only-key contract of
   `All`/`Range`. Values are copied by assignment (shallow copy of `V`).
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**:
   - Clone of an empty tree (including zero-value `Tree[V]{}`) returns a
     non-nil empty `*Tree[V]`.
   - For value types `V` containing pointers/slices/maps, the clone
     shares those underlying references with the source — `Clone` is
     structural, not deep on `V`.

### `func (t *Tree[V]) Min() (key []byte, value V, ok bool)`

1. **Signature**: `func (t *Tree[V]) Min() (key []byte, value V, ok bool)`
2. **Behavior**: returns the smallest key in the tree (byte-wise) and
   its value.
3. **Preconditions**: none.
4. **Postconditions**: when the tree is non-empty, returns
   `(storedKey, storedValue, true)`; the returned `key` aliases the
   tree's internal storage and must be treated as read-only. When the
   tree is empty, returns `(nil, zeroValue, false)`.
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**:
   - On an empty tree (including zero-value `Tree[V]{}`), returns
     `(nil, zeroValue, false)`.
   - When the empty key is the only key, `Min()` returns it (the
     yielded `key` slice has length 0).

### `func (t *Tree[V]) Max() (key []byte, value V, ok bool)`

1. **Signature**: `func (t *Tree[V]) Max() (key []byte, value V, ok bool)`
2. **Behavior**: returns the largest key in the tree (byte-wise) and
   its value.
3. **Preconditions**: none.
4. **Postconditions**: when the tree is non-empty, returns
   `(storedKey, storedValue, true)`; the returned `key` aliases the
   tree's internal storage and must be treated as read-only. When the
   tree is empty, returns `(nil, zeroValue, false)`.
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**:
   - On an empty tree (including zero-value `Tree[V]{}`), returns
     `(nil, zeroValue, false)`.
   - When the empty key is the only key, `Max()` returns it.

### `func (t *Tree[V]) Ceiling(target []byte) (key []byte, value V, ok bool)`

1. **Signature**: `func (t *Tree[V]) Ceiling(target []byte) (key []byte, value V, ok bool)`
2. **Behavior**: returns the smallest key that compares byte-wise `>=
   target`, with its value.
3. **Preconditions**: none.
4. **Postconditions**: when such a key exists, returns
   `(matchedKey, matchedValue, true)` with `bytes.Compare(matchedKey,
   target) >= 0`; the returned `key` aliases the tree's internal
   storage. When no such key exists, returns `(nil, zeroValue, false)`.
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**:
   - `target == nil` and `target == []byte{}` are equivalent (both
     represent the empty key); on a non-empty tree both return `Min()`'s
     entry.
   - On an empty tree, returns `(nil, zeroValue, false)` for any
     `target`.
   - When `target` exactly matches a stored key, that key is returned.

### `func (t *Tree[V]) Floor(target []byte) (key []byte, value V, ok bool)`

1. **Signature**: `func (t *Tree[V]) Floor(target []byte) (key []byte, value V, ok bool)`
2. **Behavior**: returns the largest key that compares byte-wise `<=
   target`, with its value.
3. **Preconditions**: none.
4. **Postconditions**: when such a key exists, returns
   `(matchedKey, matchedValue, true)` with `bytes.Compare(matchedKey,
   target) <= 0`; the returned `key` aliases the tree's internal
   storage. When no such key exists, returns `(nil, zeroValue, false)`.
5. **Panics**: standard nil-receiver panic if `t` is `nil`.
6. **Edge cases**:
   - `target == nil` and `target == []byte{}` are equivalent: `Floor`
     returns the empty-key entry if present, else `(nil, zeroValue,
     false)` (any non-empty stored key is `>` the empty target).
   - On an empty tree, returns `(nil, zeroValue, false)`.
   - When `target` exactly matches a stored key, that key is returned.

### `func (t *Tree[V]) All() iter.Seq2[[]byte, V]`

1. **Signature**: `func (t *Tree[V]) All() iter.Seq2[[]byte, V]`
2. **Behavior**: returns an iterator that yields every `(key, value)`
   pair in ascending byte-wise key order.
3. **Preconditions**: the caller must not mutate the tree while ranging
   over the returned iterator. Caller must not mutate the yielded `key`
   slice.
4. **Postconditions**: yields each pair exactly once in ascending key
   order; returning `false` from the range body stops the traversal
   immediately. The yielded `key` slice aliases the tree's internal
   storage (safe to retain only while the entry remains in the tree).
5. **Panics**: standard nil-receiver panic if `t` is `nil` when the
   iterator is invoked.
6. **Edge cases**:
   - On an empty tree (including zero-value `Tree[V]{}`), yields
     nothing.
   - The returned `iter.Seq2` is safe to range over more than once;
     each invocation walks the tree's current state.
   - When the empty key is present, it is yielded first.

### `func (t *Tree[V]) AllDescending() iter.Seq2[[]byte, V]`

1. **Signature**: `func (t *Tree[V]) AllDescending() iter.Seq2[[]byte, V]`
2. **Behavior**: returns an iterator that yields every `(key, value)`
   pair in descending byte-wise key order.
3. **Preconditions**: same as `All`.
4. **Postconditions**: yields each pair exactly once in descending key
   order; returning `false` stops traversal immediately. Same key-slice
   aliasing contract as `All`.
5. **Panics**: standard nil-receiver panic if `t` is `nil` when the
   iterator is invoked.
6. **Edge cases**:
   - On an empty tree, yields nothing.
   - When the empty key is present, it is yielded last (it sorts before
     every non-empty key).

### `func (t *Tree[V]) Range(start, end []byte) iter.Seq2[[]byte, V]`

1. **Signature**: `func (t *Tree[V]) Range(start, end []byte) iter.Seq2[[]byte, V]`
2. **Behavior**: returns an iterator over every `(key, value)` pair
   whose key lies in the half-open interval `[start, end)`, in ascending
   byte-wise key order.
3. **Preconditions**: same caller contract as `All`.
4. **Postconditions**: yields each in-range pair exactly once in
   ascending order; returning `false` stops traversal immediately. A
   `nil` bound is unbounded on that side. The yielded `key` slice
   aliases the tree's internal storage.
5. **Panics**: `art: Range called with reversed bounds (start > end)`
   when both bounds are non-nil and `bytes.Compare(start, end) > 0`.
   The panic is raised eagerly at the call to `Range`, before the
   iterator is invoked, so the bug surfaces at the call site rather
   than at the loop. Standard nil-receiver panic if `t` is `nil` when
   the iterator is invoked.
6. **Edge cases**:
   - `Range(nil, nil)` is equivalent to `All()`.
   - `Range(start, nil)` yields keys `>= start`.
   - `Range(nil, end)` yields keys `< end`.
   - `Range(start, end)` with `bytes.Compare(start, end) == 0` (equal
     bounds) yields nothing — the well-defined empty half-open
     interval `[s, s)`. This is **not** considered malformed input.
   - On an empty tree, yields nothing for any bounds (still subject
     to the reversed-bounds panic for non-nil malformed inputs).
   - The empty-slice bound `[]byte{}` is *not* the same as `nil` here:
     `start == []byte{}` is a defined lower bound (every byte slice is
     `>= []byte{}`, so the iteration is unrestricted on the low side),
     but `bytes.Compare([]byte{}, end)` participates in the equal/
     reversed checks above. With `nil` the bound is treated as "no
     bound at all" and the comparison is skipped. In practice both
     produce the same yield set; the difference matters only at the
     reversed-bounds panic.

### `func (t *Tree[V]) RangeFrom(start []byte) iter.Seq2[[]byte, V]`

1. **Signature**: `func (t *Tree[V]) RangeFrom(start []byte) iter.Seq2[[]byte, V]`
2. **Behavior**: returns an iterator over every `(key, value)` pair
   whose key is `>= start`, in ascending order. Defined as
   `Range(start, nil)`.
3. **Preconditions**: same as `Range`.
4. **Postconditions**: same as `Range(start, nil)`.
5. **Panics**: none.
6. **Edge cases**:
   - `RangeFrom(nil)` is equivalent to `All()`.
   - On an empty tree, yields nothing.

### `func (t *Tree[V]) RangeTo(end []byte) iter.Seq2[[]byte, V]`

1. **Signature**: `func (t *Tree[V]) RangeTo(end []byte) iter.Seq2[[]byte, V]`
2. **Behavior**: returns an iterator over every `(key, value)` pair
   whose key is `< end`, in ascending order. Defined as `Range(nil,
   end)`.
3. **Preconditions**: same as `Range`.
4. **Postconditions**: same as `Range(nil, end)`.
5. **Panics**: none.
6. **Edge cases**:
   - `RangeTo(nil)` is equivalent to `All()`.
   - `RangeTo([]byte{})` yields nothing (no key compares strictly less
     than the empty slice).
   - On an empty tree, yields nothing.

### `func (t *Tree[V]) RangeDescending(start, end []byte) iter.Seq2[[]byte, V]`

1. **Signature**: `func (t *Tree[V]) RangeDescending(start, end []byte) iter.Seq2[[]byte, V]`
2. **Behavior**: returns an iterator over every `(key, value)` pair
   whose key lies in the half-open interval `[start, end)`, in
   descending byte-wise key order.
3. **Preconditions**: same as `Range`.
4. **Postconditions**: yields each in-range pair exactly once in
   descending order; returning `false` stops traversal immediately. A
   `nil` bound is unbounded on that side. Same key-slice aliasing as
   `All`.
5. **Panics**: `art: RangeDescending called with reversed bounds
   (start > end)` when both bounds are non-nil and
   `bytes.Compare(start, end) > 0`, raised eagerly at the call site
   like `Range`. Standard nil-receiver panic if `t` is `nil` when the
   iterator is invoked.
6. **Edge cases**:
   - `RangeDescending(nil, nil)` is equivalent to `AllDescending()`.
   - `RangeDescending(start, end)` with `bytes.Compare(start, end) == 0`
     yields nothing — same well-defined empty half-open interval
     `[s, s)` as `Range`, not a malformed input.
   - On an empty tree, yields nothing.

### Methods on `LockedTree[V]`

Each method takes a write lock on mutators (`Put`, `Delete`, `Clear`)
and a read lock on read-only methods (`Get`, `Len`, `Clone`), then
delegates to the corresponding `Tree[V]` method. Behavior, panic
conditions, and edge cases match the underlying `Tree[V]` method
exactly. Every method additionally panics with
`art: LockedTree must be constructed with NewLocked` when called on a
zero-value `LockedTree[V]{}`; the guard runs before the lock is taken,
so a malformed caller fails before any synchronization side effects.

### `func (t *LockedTree[V]) Put(key []byte, value V)`

1. **Signature**: `func (t *LockedTree[V]) Put(key []byte, value V)`
2. **Behavior**: takes a write lock and delegates to `Tree.Put`.
3. **Preconditions**: same as `Tree.Put`.
4. **Postconditions**: same as `Tree.Put`. The wrapper's mutex is
   released before return.
5. **Panics**: same as `Tree.Put`; `art: LockedTree must be
   constructed with NewLocked` on a zero-value `LockedTree[V]{}`.
6. **Edge cases**: same as `Tree.Put`.

### `func (t *LockedTree[V]) Get(key []byte) (V, bool)`

1. **Signature**: `func (t *LockedTree[V]) Get(key []byte) (V, bool)`
2. **Behavior**: takes a read lock and delegates to `Tree.Get`.
3. **Preconditions**: same as `Tree.Get`.
4. **Postconditions**: same as `Tree.Get`.
5. **Panics**: same as `Tree.Get`; `art: LockedTree must be
   constructed with NewLocked` on a zero-value `LockedTree[V]{}`.
6. **Edge cases**: same as `Tree.Get`.

### `func (t *LockedTree[V]) Delete(key []byte) bool`

1. **Signature**: `func (t *LockedTree[V]) Delete(key []byte) bool`
2. **Behavior**: takes a write lock and delegates to `Tree.Delete`.
3. **Preconditions**: same as `Tree.Delete`.
4. **Postconditions**: same as `Tree.Delete`.
5. **Panics**: same as `Tree.Delete`; `art: LockedTree must be
   constructed with NewLocked` on a zero-value `LockedTree[V]{}`.
6. **Edge cases**: same as `Tree.Delete`.

### `func (t *LockedTree[V]) Len() int`

1. **Signature**: `func (t *LockedTree[V]) Len() int`
2. **Behavior**: takes a read lock and delegates to `Tree.Len`.
3. **Preconditions**: none.
4. **Postconditions**: same as `Tree.Len`.
5. **Panics**: same as `Tree.Len`; `art: LockedTree must be
   constructed with NewLocked` on a zero-value `LockedTree[V]{}`.
6. **Edge cases**: same as `Tree.Len`.

### `func (t *LockedTree[V]) Clear()`

1. **Signature**: `func (t *LockedTree[V]) Clear()`
2. **Behavior**: takes a write lock and delegates to `Tree.Clear`.
3. **Preconditions**: none.
4. **Postconditions**: same as `Tree.Clear`.
5. **Panics**: same as `Tree.Clear`; `art: LockedTree must be
   constructed with NewLocked` on a zero-value `LockedTree[V]{}`.
6. **Edge cases**: same as `Tree.Clear`.

### `func (t *LockedTree[V]) Clone() *Tree[V]`

1. **Signature**: `func (t *LockedTree[V]) Clone() *Tree[V]`
2. **Behavior**: takes a read lock and returns an unlocked snapshot
   `*Tree[V]` (not a `*LockedTree[V]`).
3. **Preconditions**: none.
4. **Postconditions**: returns a non-nil `*Tree[V]` independent of `t`;
   subsequent writes to `t` or to the returned tree do not affect each
   other. The returned tree has no mutex, so callers can iterate it
   without any locking.
5. **Panics**: same as `Tree.Clone`; `art: LockedTree must be
   constructed with NewLocked` on a zero-value `LockedTree[V]{}`.
6. **Edge cases**:
   - `LockedTree` does not wrap iteration methods (`All`, `Range`,
     `RangeDescending`, etc.) by design; callers should `Clone` and
     iterate the snapshot.

---

## Package `artmap`

### Type `OrderedKey`

1. **Signature**: `type OrderedKey = cmp.Ordered`
2. **Behavior**: alias for `cmp.Ordered` — the constraint admitted as
   the key type for `Ordered[K, V]`.
3. **Preconditions**: any type satisfying `cmp.Ordered` (signed/unsigned
   integers, floats, strings, and named types whose underlying type is
   one of those).
4. **Postconditions**: every `OrderedKey` value can be encoded by a
   byte-order-preserving encoder selected at `New` time and decoded
   back to its original value bitwise.
5. **Panics**: n/a (this is a type alias, not a callable).
6. **Edge cases**: slice types such as `[]byte` are not `cmp.Ordered`
   and therefore cannot be used as `K`; callers with raw `[]byte` keys
   should use `art.Tree` directly.

### Type `Ordered[K OrderedKey, V any]`

Sorted map from `K` to `V`, backed by `art.Tree[V]` with a
byte-order-preserving encoder selected from `K`'s `reflect.Kind`. The
struct has only unexported fields.

A zero-value `Ordered[K, V]{}` (declared without `New[K, V]()`) has a
`nil` inner `tree` and `nil` decoder. Every method panics eagerly with
`artmap: Ordered must be constructed with New` (`requireConstructed`,
called at the top of every public method) before touching either
field.

### `func New[K OrderedKey, V any]() *Ordered[K, V]`

1. **Signature**: `func New[K OrderedKey, V any]() *Ordered[K, V]`
2. **Behavior**: returns a freshly allocated empty `*Ordered[K, V]`
   with the encoder selected from `K`'s `reflect.Kind`.
3. **Preconditions**: `K` must be one of the `cmp.Ordered` kinds
   recognized by `pickKind` (string, all int/uint widths, float32,
   float64). The type system enforces this for any well-typed caller.
4. **Postconditions**: returned map has `Len() == 0`; the returned
   pointer is non-nil; the encoder/decoder pair is set and is byte-
   order-preserving for `K`.
5. **Panics**: `panic("artmap: unsupported OrderedKey kind " +
   t.Kind().String())` if `K`'s underlying kind is not one of the
   supported kinds. This branch is unreachable for any `K` satisfying
   `cmp.Ordered`; it exists as defense in depth.
6. **Edge cases**: `New[int, V]` and `New[uint, V]` use 64-bit or
   32-bit encodings depending on the platform word size; this is an
   implementation detail and should not be relied on across platforms
   for persisted encodings.

### `func (o *Ordered[K, V]) Len() int`

1. **Signature**: `func (o *Ordered[K, V]) Len() int`
2. **Behavior**: returns the number of key-value pairs.
3. **Preconditions**: none.
4. **Postconditions**: returns a non-negative count; runs in O(1).
5. **Panics**: `artmap: Ordered must be constructed with New` on a zero-value `Ordered[K, V]{}`.
6. **Edge cases**: returns `0` on a freshly constructed map.

### `func (o *Ordered[K, V]) Put(key K, value V)`

1. **Signature**: `func (o *Ordered[K, V]) Put(key K, value V)`
2. **Behavior**: encodes `key` with the byte-order-preserving encoder
   selected at `New` time, then delegates to `art.Tree.Put`.
3. **Preconditions**: none.
4. **Postconditions**: same as `art.Tree.Put` on the encoded key.
5. **Panics**: `panic("artmap: unreachable")` if the internal `kind`
   field is out of range (only reachable on a corrupted struct);
   `artmap: Ordered must be constructed with New` on a zero-value `Ordered[K, V]{}`.
6. **Edge cases**:
   - For `K = string`, the empty string is a valid key and is stored
     as the encoded `[]byte{}` (i.e. the empty key on the underlying
     tree).
   - For numeric `K`, the zero value is a valid key with a fixed-width
     non-empty encoding.
   - NaN `float32`/`float64` values round-trip bitwise but their order
     relative to other floats is unspecified.

### `func (o *Ordered[K, V]) Get(key K) (V, bool)`

1. **Signature**: `func (o *Ordered[K, V]) Get(key K) (V, bool)`
2. **Behavior**: encodes `key` and delegates to `art.Tree.Get`.
3. **Preconditions**: none.
4. **Postconditions**: same as `art.Tree.Get` on the encoded key.
5. **Panics**: same as `Put`; `artmap: Ordered must be constructed with New` on a zero-value receiver.
6. **Edge cases**: same as `Put`.

### `func (o *Ordered[K, V]) Delete(key K) bool`

1. **Signature**: `func (o *Ordered[K, V]) Delete(key K) bool`
2. **Behavior**: encodes `key` and delegates to `art.Tree.Delete`.
3. **Preconditions**: none.
4. **Postconditions**: same as `art.Tree.Delete` on the encoded key.
5. **Panics**: same as `Put`; `artmap: Ordered must be constructed with New` on a zero-value receiver.
6. **Edge cases**: same as `Put`.

### `func (o *Ordered[K, V]) Min() (key K, value V, ok bool)`

1. **Signature**: `func (o *Ordered[K, V]) Min() (key K, value V, ok bool)`
2. **Behavior**: returns the smallest key in the map (in `K`'s natural
   ascending order) and its value.
3. **Preconditions**: none.
4. **Postconditions**: when non-empty, returns `(decodedMinKey,
   storedValue, true)`. When empty, returns the zero value of `K`,
   the zero value of `V`, and `false`.
5. **Panics**: `artmap: Ordered must be constructed with New` on a zero-value `Ordered[K, V]{}`.
6. **Edge cases**: on an empty map, the returned `key` is the zero
   value of `K` (e.g. `""` for string, `0` for numeric K), not `nil`.

### `func (o *Ordered[K, V]) Max() (key K, value V, ok bool)`

1. **Signature**: `func (o *Ordered[K, V]) Max() (key K, value V, ok bool)`
2. **Behavior**: returns the largest key (in `K`'s natural ascending
   order) and its value.
3. **Preconditions**: none.
4. **Postconditions**: same shape as `Min`.
5. **Panics**: `artmap: Ordered must be constructed with New` on a zero-value receiver.
6. **Edge cases**: same shape as `Min`.

### `func (o *Ordered[K, V]) Ceiling(target K) (key K, value V, ok bool)`

1. **Signature**: `func (o *Ordered[K, V]) Ceiling(target K) (key K, value V, ok bool)`
2. **Behavior**: returns the smallest key `>= target` (by `K`'s
   natural order) and its value.
3. **Preconditions**: none.
4. **Postconditions**: when such a key exists, returns
   `(decodedKey, storedValue, true)`. When none exists, returns the
   zero value of `K`, the zero value of `V`, and `false`.
5. **Panics**: same as `Put`; `artmap: Ordered must be constructed with New` on a zero-value receiver.
6. **Edge cases**:
   - For numeric `K`, every encoded key has fixed width, so `Ceiling`
     of the smallest `K` value (e.g. `math.MinInt64`) on a non-empty
     map returns `Min`.
   - For `K = string`, `Ceiling("")` on a non-empty map returns `Min`.

### `func (o *Ordered[K, V]) Floor(target K) (key K, value V, ok bool)`

1. **Signature**: `func (o *Ordered[K, V]) Floor(target K) (key K, value V, ok bool)`
2. **Behavior**: returns the largest key `<= target` (by `K`'s natural
   order) and its value.
3. **Preconditions**: none.
4. **Postconditions**: same shape as `Ceiling`.
5. **Panics**: same as `Put`; `artmap: Ordered must be constructed with New` on a zero-value receiver.
6. **Edge cases**:
   - For `K = string`, `Floor("")` returns the empty-string entry if
     present, else `(zeroK, zeroV, false)`.

### `func (o *Ordered[K, V]) Clone() *Ordered[K, V]`

1. **Signature**: `func (o *Ordered[K, V]) Clone() *Ordered[K, V]`
2. **Behavior**: returns an independent structural copy of `o` with
   the same encoder/decoder.
3. **Preconditions**: none.
4. **Postconditions**: same set of `(key, value)` pairs and same
   `Len()`; subsequent writes to either map do not affect the other.
5. **Panics**: `artmap: Ordered must be constructed with New` on a zero-value receiver.
6. **Edge cases**: same `V`-shallow caveat as `art.Tree.Clone`.

### `func (o *Ordered[K, V]) All() iter.Seq2[K, V]`

1. **Signature**: `func (o *Ordered[K, V]) All() iter.Seq2[K, V]`
2. **Behavior**: returns an iterator yielding every `(key, value)`
   pair in ascending `K` order; each underlying `[]byte` key is
   decoded back to `K` before being yielded.
3. **Preconditions**: same as `art.Tree.All`.
4. **Postconditions**: yields each pair exactly once in ascending `K`
   order; returning `false` stops traversal immediately. The yielded
   `K` is owned by the caller (decoded from the tree's internal bytes).
5. **Panics**: `artmap: Ordered must be constructed with New` on a zero-value receiver when the
   iterator is invoked.
6. **Edge cases**: on an empty map, yields nothing.

### `func (o *Ordered[K, V]) AllDescending() iter.Seq2[K, V]`

1. **Signature**: `func (o *Ordered[K, V]) AllDescending() iter.Seq2[K, V]`
2. **Behavior**: returns an iterator yielding every `(key, value)` pair
   in descending `K` order.
3. **Preconditions**: same as `All`.
4. **Postconditions**: same as `All`, but in descending order.
5. **Panics**: same as `All`.
6. **Edge cases**: on an empty map, yields nothing.

### `func (o *Ordered[K, V]) Range(start, end K) iter.Seq2[K, V]`

1. **Signature**: `func (o *Ordered[K, V]) Range(start, end K) iter.Seq2[K, V]`
2. **Behavior**: returns an iterator yielding every `(key, value)` pair
   whose key lies in the half-open interval `[start, end)` in `K`'s
   natural ascending order. `start` and `end` are encoded eagerly when
   `Range` is called, so the returned iterator is safe to range over
   multiple times.
3. **Preconditions**: same as `All`.
4. **Postconditions**: same as `art.Tree.Range` on the encoded bounds.
5. **Panics**: same as `Put` (kind switch); `artmap: Ordered must be
   constructed with New` on a zero-value receiver, raised eagerly at
   the call site.
6. **Edge cases**:
   - For every supported `K`, the encoded bounds are wrapped through
     `bytes.Clone` so a zero-length encoding (the empty string) stays
     a non-nil bound rather than collapsing to a tree-level
     "unbounded" nil. Equal bounds (including `Range("", "")` for
     strings) yield nothing — the half-open interval is empty.
   - For numeric `K`, encoded bounds are always non-nil fixed-width
     slices.
   - Reversed bounds (`bytes.Compare(encoded_start, encoded_end) > 0`)
     surface the underlying `art.Tree.Range` panic
     `art: Range called with reversed bounds (start > end)`. For
     numeric `K` this means `start > end` in `K`'s natural ordering;
     for strings it is byte-wise comparison after encoding.

### `func (o *Ordered[K, V]) RangeFrom(start K) iter.Seq2[K, V]`

1. **Signature**: `func (o *Ordered[K, V]) RangeFrom(start K) iter.Seq2[K, V]`
2. **Behavior**: returns an iterator yielding every `(key, value)` pair
   whose key is `>= start`, in ascending `K` order. `start` is encoded
   eagerly.
3. **Preconditions**: same as `All`.
4. **Postconditions**: equivalent to `art.Tree.RangeFrom(encodedStart)`
   followed by per-pair decoding.
5. **Panics**: same as `Put`; `artmap: Ordered must be constructed with New` on a zero-value receiver
   when the iterator is invoked.
6. **Edge cases**:
   - For `K = string`, `RangeFrom("")` is equivalent to `All()`.
   - For numeric `K`, `RangeFrom(min K)` (e.g. `math.MinInt64`) is
     equivalent to `All()`.

### `func (o *Ordered[K, V]) RangeTo(end K) iter.Seq2[K, V]`

1. **Signature**: `func (o *Ordered[K, V]) RangeTo(end K) iter.Seq2[K, V]`
2. **Behavior**: returns an iterator yielding every `(key, value)` pair
   whose key is `< end`, in ascending `K` order. `end` is encoded
   eagerly.
3. **Preconditions**: same as `All`.
4. **Postconditions**: equivalent to `art.Tree.RangeTo(encodedEnd)`
   followed by per-pair decoding.
5. **Panics**: same as `Put`; `artmap: Ordered must be constructed with New` on a zero-value receiver
   when the iterator is invoked.
6. **Edge cases**:
   - For `K = string`, `RangeTo("")` yields nothing (no string sorts
     strictly before the empty string).
   - For numeric `K`, `RangeTo(min K)` yields nothing (no value sorts
     strictly before the minimum).

### `func (o *Ordered[K, V]) RangeDescending(start, end K) iter.Seq2[K, V]`

1. **Signature**: `func (o *Ordered[K, V]) RangeDescending(start, end K) iter.Seq2[K, V]`
2. **Behavior**: returns an iterator yielding every `(key, value)` pair
   whose key lies in `[start, end)`, in descending `K` order. Bounds
   are encoded eagerly.
3. **Preconditions**: same as `All`.
4. **Postconditions**: same shape as `Range`, but yields in descending
   order.
5. **Panics**: same as `Range`.
6. **Edge cases**:
   - Equal bounds yield nothing for every `K`, including
     `RangeDescending("", "")` for strings (which uses the same
     `bytes.Clone` encoding path as `Range` so the bound stays
     non-nil).
   - Reversed bounds raise the `art: RangeDescending called with
     reversed bounds (start > end)` panic, eagerly at the call site.

---

## Follow-ups

No open Goal #1 follow-ups at present. The W2-era list has been
resolved; entries below are kept as a changelog cross-reference:

- *Reversed range bounds* (formerly: silently empty) — now panic with
  `art: Range called with reversed bounds (start > end)` and
  `art: RangeDescending called with reversed bounds (start > end)`.
  Equal bounds still yield nothing because `[s, s)` is a well-defined
  empty interval and not malformed input.
- *`Ordered.Range("", "")` falling through to `All()`* — fixed by
  switching the bounds-clone idiom from
  `append([]byte(nil), encoded...)` to `bytes.Clone(encoded)`, which
  preserves the non-nil-ness of a zero-length encoding.
- *Zero-value `LockedTree[V]{}` panics generically* — every method now
  guards with `requireConstructed` and panics with
  `art: LockedTree must be constructed with NewLocked`.
- *Zero-value `Ordered[K, V]{}` panics generically* — same fix in
  `artmap`: `artmap: Ordered must be constructed with New`.
