package harness

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

// OpKind identifies which method of SortedMap an Op runs.
type OpKind int

const (
	OpPut OpKind = iota
	OpGet
	OpDelete
	OpRange
	OpLen
)

// Op is one operation in a diff trace. Op.Kind says what to do; the
// other fields are inputs.
type Op struct {
	Kind  OpKind
	Key   []byte
	Value int
	From  []byte // for Range
	To    []byte
}

// Builder helpers keep regression scenarios readable.

// Put builds an OpPut.
func Put(key []byte, value int) Op { return Op{Kind: OpPut, Key: key, Value: value} }

// Get builds an OpGet.
func Get(key []byte) Op { return Op{Kind: OpGet, Key: key} }

// Del builds an OpDelete.
func Del(key []byte) Op { return Op{Kind: OpDelete, Key: key} }

// Rng builds an OpRange.
func Rng(from, to []byte) Op { return Op{Kind: OpRange, From: from, To: to} }

// Length builds an OpLen.
func Length() Op { return Op{Kind: OpLen} }

// reporter is the subset of testing.TB that runDiff needs. The
// indirection lets us unit-test that the harness actually fires on
// divergence (see TestDiff_DetectsDivergence).
type reporter interface {
	Errorf(format string, args ...any)
}

// RunDiff executes ops on candidate and reference, asserting that
// every visible result matches at every step. After all ops, it
// also asserts that a full Range(nil, nil) yields the same (key,
// value) sequence on both. Failures are reported via t.Errorf with
// the op index and a tail of recent ops so the cause is locatable.
func RunDiff(t *testing.T, candidate, reference SortedMap, ops []Op) {
	t.Helper()
	runDiff(t, candidate, reference, ops)
}

func runDiff(r reporter, candidate, reference SortedMap, ops []Op) {
	for i, op := range ops {
		switch op.Kind {
		case OpPut:
			candidate.Put(op.Key, op.Value)
			reference.Put(op.Key, op.Value)
		case OpGet:
			gv, gok := candidate.Get(op.Key)
			rv, rok := reference.Get(op.Key)
			if gv != rv || gok != rok {
				r.Errorf("op %d Get(%q): candidate=(%d,%v) reference=(%d,%v)\n%s",
					i, op.Key, gv, gok, rv, rok, traceTail(ops, i))
			}
		case OpDelete:
			gok := candidate.Delete(op.Key)
			rok := reference.Delete(op.Key)
			if gok != rok {
				r.Errorf("op %d Delete(%q): candidate=%v reference=%v\n%s",
					i, op.Key, gok, rok, traceTail(ops, i))
			}
		case OpRange:
			gPairs := collect(candidate.Range(op.From, op.To))
			rPairs := collect(reference.Range(op.From, op.To))
			if !pairsEqual(gPairs, rPairs) {
				r.Errorf("op %d Range(%q,%q):\n  candidate=%s\n  reference=%s\n%s",
					i, op.From, op.To, formatPairs(gPairs), formatPairs(rPairs), traceTail(ops, i))
			}
		case OpLen:
			// Len parity is asserted by the post-step check below.
		}
		if g, ref := candidate.Len(), reference.Len(); g != ref {
			r.Errorf("op %d (%s): Len mismatch: candidate=%d reference=%d\n%s",
				i, formatOp(op), g, ref, traceTail(ops, i))
			return
		}
	}
	gPairs := collect(candidate.Range(nil, nil))
	rPairs := collect(reference.Range(nil, nil))
	if !pairsEqual(gPairs, rPairs) {
		r.Errorf("final Range(nil,nil):\n  candidate=%s\n  reference=%s",
			formatPairs(gPairs), formatPairs(rPairs))
	}
}

type kv struct {
	K []byte
	V int
}

func collect(seq func(yield func([]byte, int) bool)) []kv {
	var out []kv
	for k, v := range seq {
		out = append(out, kv{K: append([]byte(nil), k...), V: v})
	}
	return out
}

func pairsEqual(a, b []kv) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].V != b[i].V || !bytes.Equal(a[i].K, b[i].K) {
			return false
		}
	}
	return true
}

func formatPairs(p []kv) string {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, x := range p {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%q=%d", x.K, x.V)
	}
	b.WriteByte(']')
	return b.String()
}

func traceTail(ops []Op, upto int) string {
	const window = 8
	start := upto - window + 1
	if start < 0 {
		start = 0
	}
	var b bytes.Buffer
	b.WriteString("trace tail:\n")
	for i := start; i <= upto; i++ {
		fmt.Fprintf(&b, "  [%d] %s\n", i, formatOp(ops[i]))
	}
	return b.String()
}

func formatOp(op Op) string {
	switch op.Kind {
	case OpPut:
		return fmt.Sprintf("Put(%q, %d)", op.Key, op.Value)
	case OpGet:
		return fmt.Sprintf("Get(%q)", op.Key)
	case OpDelete:
		return fmt.Sprintf("Delete(%q)", op.Key)
	case OpRange:
		return fmt.Sprintf("Range(%q, %q)", op.From, op.To)
	case OpLen:
		return "Len()"
	}
	return "??"
}

// RandomTrace generates numOps pseudo-random ops. Keys are 1-8
// bytes over the alphabet "abc", so collisions and shared prefixes
// are frequent. The mix is Put-heavy: 4 Put : 2 Get : 2 Delete :
// 1 Range. A zero seed draws one from the clock; the effective
// seed is returned so a failing trace can be reproduced by passing
// it back in.
func RandomTrace(seed uint64, numOps int) (uint64, []Op) {
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	randKey := func() []byte {
		k := make([]byte, 1+r.IntN(8))
		for i := range k {
			k[i] = "abc"[r.IntN(3)]
		}
		return k
	}
	ops := make([]Op, numOps)
	for i := range ops {
		switch pick := r.IntN(9); {
		case pick < 4:
			ops[i] = Put(randKey(), r.IntN(1<<20))
		case pick < 6:
			ops[i] = Get(randKey())
		case pick < 8:
			ops[i] = Del(randKey())
		default:
			a, b := randKey(), randKey()
			if bytes.Compare(a, b) > 0 {
				a, b = b, a
			}
			ops[i] = Rng(a, b)
		}
	}
	return seed, ops
}

// RandomTraceForT generates a random trace and logs the effective
// seed via t.Logf so failing tests are reproducible. Pin the seed
// by calling RandomTrace directly when reducing a failure.
func RandomTraceForT(t *testing.T, numOps int) []Op {
	t.Helper()
	seed, ops := RandomTrace(0, numOps)
	t.Logf("RandomTrace seed=%d numOps=%d", seed, len(ops))
	return ops
}
