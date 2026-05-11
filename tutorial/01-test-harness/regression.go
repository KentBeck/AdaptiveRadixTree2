package harness

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// Scenario is one named, scripted op sequence. Bugs we've seen (or
// are obvious from API shape) become scenarios so we catch them
// every time, not just when the random seed happens to hit them.
type Scenario struct {
	Name string
	Ops  []Op
}

func emptyGetMissing() Scenario {
	return Scenario{
		Name: "empty/get-missing",
		Ops:  []Op{Length(), Get([]byte("nope")), Length()},
	}
}

func singlePutGet() Scenario {
	bs := []byte("hello")
	return Scenario{
		Name: "single-put-get",
		Ops:  []Op{Put(bs, 1), Get(bs), Length()},
	}
}

func overwrite() Scenario {
	bs := []byte("hello")
	return Scenario{
		Name: "overwrite",
		Ops:  []Op{Put(bs, 1), Put(bs, 2), Get(bs), Length()},
	}
}

func deleteMissing() Scenario {
	bs := []byte("hello")
	return Scenario{
		Name: "delete-missing",
		Ops:  []Op{Del([]byte("nope")), Length(), Put(bs, 1), Del([]byte("nope")), Length()},
	}
}

func emptyKey() Scenario {
	return Scenario{
		Name: "empty-key",
		Ops: []Op{
			Put(nil, 7), Get(nil), Length(),
			Put([]byte{}, 8), Get(nil), Get([]byte{}), Length(),
			Del(nil), Get(nil), Length(),
		},
	}
}

func prefixOf() Scenario {
	return Scenario{
		Name: "prefix-of",
		Ops: []Op{
			Put([]byte("h"), 1), Put([]byte("hi"), 2),
			Put([]byte("hello"), 3), Put([]byte("help"), 4),
			Get([]byte("h")), Get([]byte("hi")),
			Get([]byte("hello")), Get([]byte("help")),
			Del([]byte("hi")),
			Get([]byte("h")), Get([]byte("hi")),
			Get([]byte("hello")), Get([]byte("help")),
			Length(),
		},
	}
}

func boundaryBytes() Scenario {
	ops := []Op{}
	for _, b := range []byte{0x00, 0x7f, 0x80, 0xff} {
		ops = append(ops, Put([]byte{b}, int(b)))
		ops = append(ops, Put([]byte{b, b}, int(b)*2))
		ops = append(ops, Put([]byte{b, 0x00, b}, int(b)*3))
	}
	for _, b := range []byte{0x00, 0xff} {
		ops = append(ops, Get([]byte{b}))
		ops = append(ops, Get([]byte{b, b}))
	}
	ops = append(ops, Length(), Rng(nil, nil))
	return Scenario{Name: "boundary-bytes", Ops: ops}
}

func longKey() Scenario {
	k := bytes.Repeat([]byte{'x'}, 1024)
	return Scenario{
		Name: "long-key",
		Ops:  []Op{Put(k, 42), Get(k), Length(), Del(k), Get(k), Length()},
	}
}

func rangeHalfOpen() Scenario {
	return Scenario{
		Name: "range-half-open",
		Ops: []Op{
			Put([]byte("a"), 1), Put([]byte("b"), 2),
			Put([]byte("c"), 3), Put([]byte("d"), 4),
			Rng([]byte("b"), []byte("d")),
			Rng([]byte("a"), []byte("a")),
			Rng([]byte("a"), []byte("e")),
		},
	}
}

func rangeUnbounded() Scenario {
	return Scenario{
		Name: "range-unbounded",
		Ops: []Op{
			Put([]byte("z"), 26), Put([]byte("a"), 1),
			Put([]byte("m"), 13),
			Rng(nil, nil),
			Rng(nil, []byte("m")),
			Rng([]byte("m"), nil),
		},
	}
}

func rangeEmptyWindow() Scenario {
	// Both implementations agree they yield nothing. We do not require future implementations to do the same; this scenario only checks consistency between the candidate and the reference, not absolute behavior.
	return Scenario{
		Name: "range-empty-window",
		Ops: []Op{
			Put([]byte("a"), 1), Put([]byte("b"), 2), Put([]byte("c"), 3),
			Rng([]byte("z"), []byte("a")),
		},
	}
}

func deleteThenReinsert() Scenario {
	bs := []byte("hello")
	return Scenario{
		Name: "delete-then-reinsert",
		Ops: []Op{
			Put(bs, 1), Del(bs), Get(bs), Length(),
			Put(bs, 99), Get(bs), Length(),
		},
	}
}

func largeFanout() Scenario {
	ops := make([]Op, 0, 256*2+2)
	for b := 0; b < 256; b++ {
		ops = append(ops, Put([]byte{byte(b)}, b))
	}
	for b := 0; b < 256; b++ {
		ops = append(ops, Get([]byte{byte(b)}))
	}
	ops = append(ops, Length(), Rng(nil, nil))
	return Scenario{Name: "large-fanout", Ops: ops}
}

func massInsertThenDeleteAll() Scenario {
	const n = 200
	ops := make([]Op, 0, n*2+2)
	keys := make([][]byte, n)
	for i := 0; i < n; i++ {
		k := []byte(fmt.Sprintf("k%05d", i))
		keys[i] = k
		ops = append(ops, Put(k, i))
	}
	ops = append(ops, Length())
	for i := n - 1; i >= 0; i-- {
		ops = append(ops, Del(keys[i]))
	}
	ops = append(ops, Length())
	return Scenario{Name: "mass-insert-then-delete-all", Ops: ops}
}

// Scenarios returns the canonical regression set. Add a scenario by
// appending to this list -- the runner picks them up automatically.
func Scenarios() []Scenario {
	return []Scenario{
		emptyGetMissing(),
		singlePutGet(),
		overwrite(),
		deleteMissing(),
		emptyKey(),
		prefixOf(),
		boundaryBytes(),
		longKey(),
		rangeHalfOpen(),
		rangeUnbounded(),
		rangeEmptyWindow(),
		deleteThenReinsert(),
		largeFanout(),
		massInsertThenDeleteAll(),
	}
}

// RunRegression runs every scenario from Scenarios() against
// freshly-built candidate and reference, via RunDiff. Each scenario
// runs in its own t.Run sub-test so failures attribute cleanly.
func RunRegression(t *testing.T, candidate, reference Factory) {
	t.Helper()
	for _, sc := range Scenarios() {
		sc := sc
		t.Run(sanitize(sc.Name), func(t *testing.T) {
			RunDiff(t, candidate(), reference(), sc.Ops)
		})
	}
}

func sanitize(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}
