package art

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// FuzzSortedMap drives Tree with a randomly generated op stream and
// cross-checks every observable answer against two oracles: Go's
// built-in map for Put/Get/Delete, and that map with its keys sorted
// byte-wise for All/Range. Any discrepancy is a bug.
//
// The fuzzer's input bytes are parsed as a stream of typed operations
// (see fuzzCursor). Keys are drawn from a 64-symbol alphabet (bytes &
// fuzzKeyByteMask) and are 0..7 bytes long, so short keys collide,
// shared prefixes form, and node promotions fire - the scenarios unit
// tests cannot exhaust by hand.

const (
	fuzzOpPut    byte = 0
	fuzzOpGet    byte = 1
	fuzzOpDelete byte = 2
	fuzzOpAll    byte = 3
	fuzzOpRange  byte = 4
	fuzzOpCount       = 5

	fuzzMaxOps      = 1000
	fuzzKeyLenMask  = 0x07 // keys are 0..7 bytes long
	fuzzKeyByteMask = 0x3F // 64-symbol alphabet: dense enough to reach node48
	fuzzNilBoundLen = 0xFF // sentinel length byte meaning "nil bound" in Range
)

// opRecord captures one executed operation so a failing assertion can
// print a replayable log.
type opRecord struct {
	code     byte
	key      []byte
	val      byte
	start    []byte
	end      []byte
	startNil bool
	endNil   bool
}

func (r opRecord) String() string {
	switch r.code {
	case fuzzOpPut:
		return fmt.Sprintf("Put(%x, %d)", r.key, r.val)
	case fuzzOpGet:
		return fmt.Sprintf("Get(%x)", r.key)
	case fuzzOpDelete:
		return fmt.Sprintf("Delete(%x)", r.key)
	case fuzzOpAll:
		return "All()"
	case fuzzOpRange:
		return fmt.Sprintf("Range(%s, %s)", fuzzBoundString(r.start, r.startNil), fuzzBoundString(r.end, r.endNil))
	}
	return fmt.Sprintf("unknown(%d)", r.code)
}

func fuzzBoundString(b []byte, isNil bool) string {
	if isNil {
		return "nil"
	}
	return fmt.Sprintf("%x", b)
}

func formatOpLog(log []opRecord) string {
	var sb strings.Builder
	for i, op := range log {
		fmt.Fprintf(&sb, "  %03d: %s\n", i, op)
	}
	return sb.String()
}

// fuzzCursor consumes the fuzzer's input byte-by-byte. A short read
// signals end-of-stream and stops op processing cleanly.
type fuzzCursor struct {
	data []byte
	i    int
}

func (c *fuzzCursor) readByte() (byte, bool) {
	if c.i >= len(c.data) {
		return 0, false
	}
	b := c.data[c.i]
	c.i++
	return b, true
}

// readKey reads a key whose length byte is masked to 0..7 and whose
// body bytes are masked into the shared alphabet.
func (c *fuzzCursor) readKey() ([]byte, bool) {
	l, ok := c.readByte()
	if !ok {
		return nil, false
	}
	n := int(l & fuzzKeyLenMask)
	key := make([]byte, n)
	for i := 0; i < n; i++ {
		b, ok := c.readByte()
		if !ok {
			return nil, false
		}
		key[i] = b & fuzzKeyByteMask
	}
	return key, true
}

// readBound is like readKey but reserves fuzzNilBoundLen as a "nil
// bound" sentinel so Range is exercised with and without each bound.
func (c *fuzzCursor) readBound() (key []byte, isNil bool, ok bool) {
	l, lok := c.readByte()
	if !lok {
		return nil, false, false
	}
	if l == fuzzNilBoundLen {
		return nil, true, true
	}
	n := int(l & fuzzKeyLenMask)
	key = make([]byte, n)
	for i := 0; i < n; i++ {
		b, bok := c.readByte()
		if !bok {
			return nil, false, false
		}
		key[i] = b & fuzzKeyByteMask
	}
	return key, false, true
}

func FuzzSortedMap(f *testing.F) {
	addFuzzSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		runFuzzOps(t, data)
	})
}

// runFuzzOps replays the op stream against tree + oracle in lockstep
// and t.Fatalfs (with the full op log) on the first disagreement.
func runFuzzOps(t *testing.T, data []byte) {
	tree := New[byte]()
	oracle := make(map[string]byte)
	cur := &fuzzCursor{data: data}
	var log []opRecord

	for len(log) < fuzzMaxOps {
		codeByte, ok := cur.readByte()
		if !ok {
			return
		}
		code := codeByte % fuzzOpCount
		switch code {
		case fuzzOpPut:
			key, ok := cur.readKey()
			if !ok {
				return
			}
			v, ok := cur.readByte()
			if !ok {
				return
			}
			log = append(log, opRecord{code: code, key: key, val: v})
			tree.Put(key, v)
			oracle[string(key)] = v
			assertLen(t, tree, oracle, log)
		case fuzzOpGet:
			key, ok := cur.readKey()
			if !ok {
				return
			}
			log = append(log, opRecord{code: code, key: key})
			got, gotOK := tree.Get(key)
			wantV, wantOK := oracle[string(key)]
			if gotOK != wantOK {
				t.Fatalf("Get(%x) presence: got=%v want=%v\nops:\n%s", key, gotOK, wantOK, formatOpLog(log))
			}
			if gotOK && got != wantV {
				t.Fatalf("Get(%x) value: got=%v want=%d\nops:\n%s", key, got, wantV, formatOpLog(log))
			}
		case fuzzOpDelete:
			key, ok := cur.readKey()
			if !ok {
				return
			}
			log = append(log, opRecord{code: code, key: key})
			_, wantPresent := oracle[string(key)]
			gotDeleted := tree.Delete(key)
			if gotDeleted != wantPresent {
				t.Fatalf("Delete(%x) return: got=%v want=%v\nops:\n%s", key, gotDeleted, wantPresent, formatOpLog(log))
			}
			delete(oracle, string(key))
			assertLen(t, tree, oracle, log)
		case fuzzOpAll:
			log = append(log, opRecord{code: code})
			checkDrainAll(t, tree, oracle, log)
		case fuzzOpRange:
			start, startNil, ok := cur.readBound()
			if !ok {
				return
			}
			end, endNil, ok := cur.readBound()
			if !ok {
				return
			}
			log = append(log, opRecord{code: code, start: start, end: end, startNil: startNil, endNil: endNil})
			var s, e []byte
			if !startNil {
				s = start
			}
			if !endNil {
				e = end
			}
			checkRange(t, tree, oracle, s, e, log)
		}
	}
}

// assertLen asserts tree.Len() matches the oracle's size. This is
// called after every Put/Delete and at each iteration checkpoint so a
// miscounted insertion or deletion fails fast with the full op log.
func assertLen(t *testing.T, tree *Tree[byte], oracle map[string]byte, log []opRecord) {
	t.Helper()
	if got, want := tree.Len(), len(oracle); got != want {
		t.Fatalf("Len(): got=%d want=%d\nops:\n%s", got, want, formatOpLog(log))
	}
}

// checkDrainAll asserts Tree.All yields exactly the oracle's keys in
// byte-wise ascending order, with matching values.
func checkDrainAll(t *testing.T, tree *Tree[byte], oracle map[string]byte, log []opRecord) {
	t.Helper()
	assertLen(t, tree, oracle, log)
	want := oracleSortedKeys(oracle)
	var got []string
	var gotVals []byte
	for k, v := range tree.All() {
		got = append(got, string(k))
		gotVals = append(gotVals, v)
	}
	assertSortedKV(t, "All()", got, gotVals, want, oracle, log)
}

// checkRange asserts Tree.Range(start, end) yields exactly the oracle
// keys in [start, end) in byte-wise ascending order, with matching
// values.
func checkRange(t *testing.T, tree *Tree[byte], oracle map[string]byte, start, end []byte, log []opRecord) {
	t.Helper()
	assertLen(t, tree, oracle, log)
	want := oracleRange(oracle, start, end)
	var got []string
	var gotVals []byte
	for k, v := range tree.Range(start, end) {
		got = append(got, string(k))
		gotVals = append(gotVals, v)
	}
	label := fmt.Sprintf("Range(%s, %s)", fuzzBoundString(start, start == nil), fuzzBoundString(end, end == nil))
	assertSortedKV(t, label, got, gotVals, want, oracle, log)
}

func assertSortedKV(t *testing.T, label string, gotKeys []string, gotVals []byte, wantKeys []string, oracle map[string]byte, log []opRecord) {
	t.Helper()
	if len(gotKeys) != len(wantKeys) {
		t.Fatalf("%s length: got=%d want=%d\ngot keys=%x\nwant keys=%x\nops:\n%s",
			label, len(gotKeys), len(wantKeys), keysAsHex(gotKeys), keysAsHex(wantKeys), formatOpLog(log))
	}
	for i := range gotKeys {
		if gotKeys[i] != wantKeys[i] {
			t.Fatalf("%s key[%d]: got=%x want=%x\nops:\n%s",
				label, i, gotKeys[i], wantKeys[i], formatOpLog(log))
		}
		if gotVals[i] != oracle[wantKeys[i]] {
			t.Fatalf("%s value[%d] for key %x: got=%d want=%d\nops:\n%s",
				label, i, gotKeys[i], gotVals[i], oracle[wantKeys[i]], formatOpLog(log))
		}
	}
}

// oracleSortedKeys returns the oracle's keys in byte-wise ascending
// order. Go's sort.Strings compares strings byte-wise, matching the
// tree's key ordering.
func oracleSortedKeys(oracle map[string]byte) []string {
	keys := make([]string, 0, len(oracle))
	for k := range oracle {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// oracleRange returns oracle keys in [start, end), sorted. A nil bound
// is unbounded on that side. If start >= end (both non-nil) the result
// is empty - Tree.Range returns nothing in that case.
func oracleRange(oracle map[string]byte, start, end []byte) []string {
	if start != nil && end != nil && bytes.Compare(start, end) >= 0 {
		return nil
	}
	all := oracleSortedKeys(oracle)
	var out []string
	for _, k := range all {
		kb := []byte(k)
		if start != nil && bytes.Compare(kb, start) < 0 {
			continue
		}
		if end != nil && bytes.Compare(kb, end) >= 0 {
			continue
		}
		out = append(out, k)
	}
	return out
}

func keysAsHex(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = fmt.Sprintf("%x", k)
	}
	return out
}

// addFuzzSeeds adds inputs that exercise the edge cases a naive fuzzer
// might take a while to rediscover: empty keys, prefix-overlap keys,
// splits that create new shared prefixes, enough distinct children to
// drive node4 -> node16 -> node48 promotion, and a Delete that empties
// the tree. Each seed is a byte stream in the format runFuzzOps reads.
func addFuzzSeeds(f *testing.F) {
	f.Add(seedEmptyKey())
	f.Add(seedPrefixKeys())
	f.Add(seedSplitKeys())
	f.Add(seedPromoteToNode48())
	f.Add(seedDeleteToEmpty())
	f.Add(seedRangeBounds())
}

// --- seed builders ---

type seedBuf struct{ b []byte }

func (s *seedBuf) put(key []byte, v byte) {
	s.b = append(s.b, fuzzOpPut, byte(len(key)))
	s.b = append(s.b, key...)
	s.b = append(s.b, v)
}
func (s *seedBuf) get(key []byte) {
	s.b = append(s.b, fuzzOpGet, byte(len(key)))
	s.b = append(s.b, key...)
}
func (s *seedBuf) del(key []byte) {
	s.b = append(s.b, fuzzOpDelete, byte(len(key)))
	s.b = append(s.b, key...)
}
func (s *seedBuf) all() { s.b = append(s.b, fuzzOpAll) }
func (s *seedBuf) rangeOp(start, end []byte, startNil, endNil bool) {
	s.b = append(s.b, fuzzOpRange)
	s.appendBound(start, startNil)
	s.appendBound(end, endNil)
}
func (s *seedBuf) appendBound(key []byte, isNil bool) {
	if isNil {
		s.b = append(s.b, fuzzNilBoundLen)
		return
	}
	s.b = append(s.b, byte(len(key)))
	s.b = append(s.b, key...)
}

func seedEmptyKey() []byte {
	var s seedBuf
	s.put(nil, 7)
	s.get(nil)
	s.all()
	s.del(nil)
	s.get(nil)
	return s.b
}

func seedPrefixKeys() []byte {
	var s seedBuf
	s.put([]byte{1}, 1)
	s.put([]byte{1, 2}, 2)
	s.put([]byte{1, 2, 3}, 3)
	s.get([]byte{1})
	s.get([]byte{1, 2})
	s.get([]byte{1, 2, 3})
	s.all()
	return s.b
}

func seedSplitKeys() []byte {
	var s seedBuf
	s.put([]byte{1, 2, 3}, 1)
	s.put([]byte{1, 2, 4}, 2)
	s.put([]byte{1, 2}, 3)
	s.put([]byte{1, 7}, 4)
	s.put([]byte{5}, 5)
	s.all()
	s.rangeOp([]byte{1}, []byte{1, 9}, false, false)
	return s.b
}

func seedPromoteToNode48() []byte {
	var s seedBuf
	for i := byte(0); i < 20; i++ {
		s.put([]byte{0, i}, i+1)
	}
	s.all()
	s.rangeOp(nil, []byte{0, 10}, true, false)
	s.del([]byte{0, 5})
	s.all()
	return s.b
}

func seedDeleteToEmpty() []byte {
	var s seedBuf
	s.put([]byte{1, 2}, 1)
	s.put([]byte{1, 3}, 2)
	s.all()
	s.del([]byte{1, 2})
	s.del([]byte{1, 3})
	s.all()
	s.get([]byte{1, 2})
	return s.b
}

func seedRangeBounds() []byte {
	var s seedBuf
	for i := byte(0); i < 8; i++ {
		s.put([]byte{i}, i)
	}
	s.rangeOp(nil, nil, true, true)
	s.rangeOp([]byte{2}, []byte{6}, false, false)
	s.rangeOp(nil, []byte{4}, true, false)
	s.rangeOp([]byte{3}, nil, false, true)
	s.rangeOp([]byte{5}, []byte{5}, false, false)
	s.rangeOp([]byte{6}, []byte{2}, false, false)
	return s.b
}
