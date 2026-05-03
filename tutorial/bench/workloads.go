// Package bench provides shared key/value workloads and size-reporting
// helpers for every tutorial stage. Each stage's bench_test.go imports
// this package, so the chapter-by-chapter comparison numbers come from
// exactly the same key sets and only the tree implementation differs.
//
// Three workload shapes:
//
//   - Dense: contiguous 8-byte big-endian integers. Maximum prefix
//     sharing -- almost every adjacent pair differs only in the last
//     byte or two. Friendly to a trie.
//   - Sparse: random 16-byte keys. No common prefixes. Hostile to a
//     naive trie because each key forces 16 levels of inner nodes.
//   - URL: a corpus of realistic-looking URLs. Heavy prefix sharing
//     up to the path component, then divergent. Mid-shape.
//
// All generators are deterministic (seeded RNG) so benchmark numbers
// are reproducible across runs and machines.
package bench

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
)

// Workload bundles the keys and values used by a single benchmark
// run. Vals[i] is the value stored at Keys[i].
type Workload struct {
	Name string
	Keys [][]byte
	Vals []int
}

// Dense returns n contiguous 8-byte big-endian integer keys, starting
// at zero. Every adjacent pair of keys shares all but the trailing
// few bytes -- the highest possible prefix sharing for a sorted-map
// workload.
func Dense(n int) Workload {
	keys := make([][]byte, n)
	vals := make([]int, n)
	for i := 0; i < n; i++ {
		k := make([]byte, 8)
		binary.BigEndian.PutUint64(k, uint64(i))
		keys[i] = k
		vals[i] = i
	}
	return Workload{Name: fmt.Sprintf("Dense/%d", n), Keys: keys, Vals: vals}
}

// Sparse returns n pseudorandom 16-byte keys. Adjacent keys share no
// bytes on average -- the worst case for a node256-only trie because
// every key forces all 16 levels to allocate.
func Sparse(n int) Workload {
	r := rand.New(rand.NewPCG(0xa11ce, 0xb0b))
	keys := make([][]byte, n)
	vals := make([]int, n)
	for i := 0; i < n; i++ {
		k := make([]byte, 16)
		for j := range k {
			k[j] = byte(r.UintN(256))
		}
		keys[i] = k
		vals[i] = i
	}
	return Workload{Name: fmt.Sprintf("Sparse/%d", n), Keys: keys, Vals: vals}
}

// URL returns n URL-shaped keys built from a small set of hosts and
// paths. Heavy prefix sharing up to the host boundary, then diverges.
// Lengths vary roughly between 25 and 80 bytes.
func URL(n int) Workload {
	hosts := []string{
		"https://api.example.com/",
		"https://www.example.com/",
		"https://docs.example.com/",
		"https://example.org/",
		"https://example.net/",
		"https://cdn.example.io/",
	}
	paths := []string{
		"v1/users/", "v1/orders/", "v2/users/", "v2/orders/",
		"static/img/", "static/css/", "static/js/",
		"blog/posts/", "blog/tags/", "search?q=",
	}
	r := rand.New(rand.NewPCG(0xc0ffee, 0xdeadbeef))
	keys := make([][]byte, n)
	vals := make([]int, n)
	for i := 0; i < n; i++ {
		host := hosts[r.UintN(uint(len(hosts)))]
		path := paths[r.UintN(uint(len(paths)))]
		// 8 hex chars of randomness so most keys are unique.
		tail := fmt.Sprintf("%08x", r.Uint32())
		keys[i] = []byte(host + path + tail)
		vals[i] = i
	}
	return Workload{Name: fmt.Sprintf("URL/%d", n), Keys: keys, Vals: vals}
}

// AvgKeyLen reports the mean key length across w. Useful for the
// "bytes per key including key bytes" denominator when comparing
// stages, since dense / sparse / URL have very different key sizes.
func (w Workload) AvgKeyLen() float64 {
	if len(w.Keys) == 0 {
		return 0
	}
	total := 0
	for _, k := range w.Keys {
		total += len(k)
	}
	return float64(total) / float64(len(w.Keys))
}
