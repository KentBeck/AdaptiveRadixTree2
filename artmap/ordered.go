// Package artmap provides a typed façade over the []byte-keyed
// [art.Tree]: an [Ordered] sorted map keyed by any Go [cmp.Ordered]
// type, using byte-order-preserving encoders so the underlying ART
// preserves the natural ordering of K.
package artmap

import (
	"cmp"
	"iter"

	art "github.com/KentBeck/AdaptiveRadixTree2"
)

// OrderedKey is the key constraint for [Ordered]. It is an alias for
// Go's [cmp.Ordered]: the set of types whose underlying type is a
// signed or unsigned integer, a floating-point type, or a string.
//
// Every OrderedKey value is encoded as a []byte before being stored
// in the underlying [art.Tree]. The encoder is chosen at [New] time
// from the key's [reflect.Kind] and is byte-order-preserving: for any
// two values a, b of type K, [cmp.Compare](a, b) and
// [bytes.Compare](encode(a), encode(b)) agree in sign. Every encoder
// round-trips: decode(encode(k)) == k for every representable k.
//
// Encoding rules:
//   - string (including ~string): the raw string bytes. Lexicographic
//     byte order matches the lexicographic order on strings.
//   - uint8, uint16, uint32, uint64, uint, uintptr: fixed-width
//     big-endian of the natural width; uint and uintptr use the
//     platform word size.
//   - int8, int16, int32, int64, int: fixed-width big-endian after
//     flipping the sign bit, so signed values sort in ascending
//     signed order (negatives before non-negatives).
//   - float32, float64: the IEEE 754 bit-pattern with the sign bit
//     flipped for non-negative values and all bits flipped for
//     negative values, then big-endian. Positive zero sorts above
//     negative zero. NaN bit patterns round-trip bitwise; their order
//     relative to other floats is unspecified but deterministic.
//
// Slice types such as []byte are not [cmp.Ordered] and therefore
// cannot be used with Ordered; call [art.Tree] directly for raw
// []byte keys.
type OrderedKey = cmp.Ordered

// Ordered is a sorted map from K to V, backed by an [art.Tree]. Keys
// are encoded via a byte-order-preserving encoder selected at [New]
// time (see [OrderedKey]) so range operations return entries in the
// natural ascending order of K.
//
// An Ordered is not safe for concurrent use by multiple goroutines
// when any goroutine is writing, with the same contract as the
// underlying [art.Tree]. Callers that need concurrent access should
// guard an Ordered with their own sync.RWMutex or wrap [art.Tree]
// directly with [art.LockedTree]. See the Concurrency section of the
// project README for the full discussion.
type Ordered[K OrderedKey, V any] struct {
	tree *art.Tree[V]
	dec  func([]byte) K
	kind keyKind
}

// New returns an empty [Ordered] keyed by K. It panics if K's kind is
// not one of the [cmp.Ordered] kinds, which cannot happen for a
// well-typed caller.
func New[K OrderedKey, V any]() *Ordered[K, V] {
	kind, dec := pickKind[K]()
	return &Ordered[K, V]{tree: art.New[V](), dec: dec, kind: kind}
}

// Len returns the number of key-value pairs. It runs in O(1).
func (o *Ordered[K, V]) Len() int { return o.tree.Len() }

// Put associates value with key, replacing any previous value.
func (o *Ordered[K, V]) Put(key K, value V) {
	var buf [maxFixedKey]byte
	o.tree.Put(o.encode(key, buf[:]), value)
}

// Get returns the value previously stored under key, if any.
func (o *Ordered[K, V]) Get(key K) (V, bool) {
	var buf [maxFixedKey]byte
	return o.tree.Get(o.encode(key, buf[:]))
}

// Delete removes key, returning whether it was present.
func (o *Ordered[K, V]) Delete(key K) bool {
	var buf [maxFixedKey]byte
	return o.tree.Delete(o.encode(key, buf[:]))
}

// Min returns the smallest key, its value, and ok=true. ok is false
// if the map is empty, in which case key and value are their zero
// values.
func (o *Ordered[K, V]) Min() (key K, value V, ok bool) {
	kb, v, present := o.tree.Min()
	if !present {
		return key, value, false
	}
	return o.dec(kb), v, true
}

// Max returns the largest key, its value, and ok=true. ok is false
// if the map is empty.
func (o *Ordered[K, V]) Max() (key K, value V, ok bool) {
	kb, v, present := o.tree.Max()
	if !present {
		return key, value, false
	}
	return o.dec(kb), v, true
}

// Ceiling returns the smallest key >= target and its value. ok is
// false if no such key exists.
func (o *Ordered[K, V]) Ceiling(target K) (key K, value V, ok bool) {
	var buf [maxFixedKey]byte
	kb, v, present := o.tree.Ceiling(o.encode(target, buf[:]))
	if !present {
		return key, value, false
	}
	return o.dec(kb), v, true
}

// Floor returns the largest key <= target and its value. ok is false
// if no such key exists.
func (o *Ordered[K, V]) Floor(target K) (key K, value V, ok bool) {
	var buf [maxFixedKey]byte
	kb, v, present := o.tree.Floor(o.encode(target, buf[:]))
	if !present {
		return key, value, false
	}
	return o.dec(kb), v, true
}

// Clone returns an independent structural copy. Writes to o or to the
// returned map do not affect each other.
func (o *Ordered[K, V]) Clone() *Ordered[K, V] {
	return &Ordered[K, V]{tree: o.tree.Clone(), dec: o.dec, kind: o.kind}
}

// All returns an iterator over every (key, value) pair in ascending
// key order. See [art.Tree.All] for the underlying contract.
func (o *Ordered[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for kb, v := range o.tree.All() {
			if !yield(o.dec(kb), v) {
				return
			}
		}
	}
}

// AllDescending returns an iterator over every (key, value) pair in
// descending key order.
func (o *Ordered[K, V]) AllDescending() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for kb, v := range o.tree.AllDescending() {
			if !yield(o.dec(kb), v) {
				return
			}
		}
	}
}

// Range returns an iterator over every (key, value) pair whose key
// lies in the half-open interval [start, end), in ascending key
// order. start >= end yields nothing. The encoded start and end are
// captured eagerly so the returned iterator is safe to range over
// multiple times.
func (o *Ordered[K, V]) Range(start, end K) iter.Seq2[K, V] {
	var sbuf, ebuf [maxFixedKey]byte
	sb := append([]byte(nil), o.encode(start, sbuf[:])...)
	eb := append([]byte(nil), o.encode(end, ebuf[:])...)
	return func(yield func(K, V) bool) {
		for kb, v := range o.tree.Range(sb, eb) {
			if !yield(o.dec(kb), v) {
				return
			}
		}
	}
}

// RangeFrom returns an iterator over entries whose key is >= start,
// in ascending order.
func (o *Ordered[K, V]) RangeFrom(start K) iter.Seq2[K, V] {
	var sbuf [maxFixedKey]byte
	sb := append([]byte(nil), o.encode(start, sbuf[:])...)
	return func(yield func(K, V) bool) {
		for kb, v := range o.tree.RangeFrom(sb) {
			if !yield(o.dec(kb), v) {
				return
			}
		}
	}
}

// RangeTo returns an iterator over entries whose key is < end, in
// ascending order.
func (o *Ordered[K, V]) RangeTo(end K) iter.Seq2[K, V] {
	var ebuf [maxFixedKey]byte
	eb := append([]byte(nil), o.encode(end, ebuf[:])...)
	return func(yield func(K, V) bool) {
		for kb, v := range o.tree.RangeTo(eb) {
			if !yield(o.dec(kb), v) {
				return
			}
		}
	}
}

// RangeDescending returns an iterator over entries whose key lies in
// [start, end), in descending key order. start >= end yields nothing.
func (o *Ordered[K, V]) RangeDescending(start, end K) iter.Seq2[K, V] {
	var sbuf, ebuf [maxFixedKey]byte
	sb := append([]byte(nil), o.encode(start, sbuf[:])...)
	eb := append([]byte(nil), o.encode(end, ebuf[:])...)
	return func(yield func(K, V) bool) {
		for kb, v := range o.tree.RangeDescending(sb, eb) {
			if !yield(o.dec(kb), v) {
				return
			}
		}
	}
}
