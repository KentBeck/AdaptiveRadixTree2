package harness

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"
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

// RandomConfig parameterizes RandomTrace. Zero-value fields fall
// back to defaults that produce a small alphabet (so collisions are
// frequent), 1000 mixed ops, and Put-heavy weights.
type RandomConfig struct {
	Seed                                            uint64
	NumOps                                          int
	KeyAlphabet                                     []byte
	MaxKeyLen                                       int
	PutWeight, GetWeight, DeleteWeight, RangeWeight int
}

// RandomTrace generates a deterministic op sequence based on cfg.
func RandomTrace(cfg RandomConfig) []Op {
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}
	if cfg.NumOps == 0 {
		cfg.NumOps = 1000
	}
	if len(cfg.KeyAlphabet) == 0 {
		cfg.KeyAlphabet = []byte("abc")
	}
	if cfg.MaxKeyLen == 0 {
		cfg.MaxKeyLen = 8
	}
	if cfg.PutWeight == 0 && cfg.GetWeight == 0 && cfg.DeleteWeight == 0 && cfg.RangeWeight == 0 {
		cfg.PutWeight, cfg.GetWeight, cfg.DeleteWeight, cfg.RangeWeight = 4, 2, 2, 1
	}
	r := rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15))
	total := cfg.PutWeight + cfg.GetWeight + cfg.DeleteWeight + cfg.RangeWeight
	ops := make([]Op, 0, cfg.NumOps)
	randKey := func() []byte {
		n := 1 + r.IntN(cfg.MaxKeyLen)
		k := make([]byte, n)
		for i := range k {
			k[i] = cfg.KeyAlphabet[r.IntN(len(cfg.KeyAlphabet))]
		}
		return k
	}
	for i := 0; i < cfg.NumOps; i++ {
		pick := r.IntN(total)
		switch {
		case pick < cfg.PutWeight:
			ops = append(ops, Put(randKey(), r.IntN(1<<20)))
		case pick < cfg.PutWeight+cfg.GetWeight:
			ops = append(ops, Get(randKey()))
		case pick < cfg.PutWeight+cfg.GetWeight+cfg.DeleteWeight:
			ops = append(ops, Del(randKey()))
		default:
			a, b := randKey(), randKey()
			if bytes.Compare(a, b) > 0 {
				a, b = b, a
			}
			ops = append(ops, Rng(a, b))
		}
	}
	return ops
}
