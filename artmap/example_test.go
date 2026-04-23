package artmap_test

import (
	"fmt"

	"github.com/KentBeck/AdaptiveRadixTree2/artmap"
)

func ExampleOrdered_string() {
	m := artmap.New[string, int]()
	m.Put("banana", 2)
	m.Put("apple", 1)
	m.Put("cherry", 3)

	v, _ := m.Get("apple")
	fmt.Println(v)
	for k, v := range m.All() {
		fmt.Println(k, v)
	}
	for k := range m.Range("b", "d") {
		fmt.Println("range:", k)
	}
	// Output:
	// 1
	// apple 1
	// banana 2
	// cherry 3
	// range: banana
	// range: cherry
}

func ExampleOrdered_int64() {
	m := artmap.New[int64, string]()
	m.Put(-100, "neg")
	m.Put(0, "zero")
	m.Put(50, "fifty")

	v, _ := m.Get(-100)
	fmt.Println(v)
	for k, v := range m.All() {
		fmt.Println(k, v)
	}
	for k := range m.Range(-50, 100) {
		fmt.Println("range:", k)
	}
	// Output:
	// neg
	// -100 neg
	// 0 zero
	// 50 fifty
	// range: 0
	// range: 50
}

func ExampleOrdered_uint32() {
	m := artmap.New[uint32, int]()
	m.Put(100, 1)
	m.Put(0, 0)
	m.Put(4294967295, 2)

	v, _ := m.Get(100)
	fmt.Println(v)
	for k := range m.All() {
		fmt.Println(k)
	}
	for k := range m.Range(50, 200) {
		fmt.Println("range:", k)
	}
	// Output:
	// 1
	// 0
	// 100
	// 4294967295
	// range: 100
}

func ExampleOrdered_int32() {
	m := artmap.New[int32, int]()
	m.Put(-1, 0)
	m.Put(0, 1)
	m.Put(1, 2)

	for k := range m.All() {
		fmt.Println(k)
	}
	// Output:
	// -1
	// 0
	// 1
}

func ExampleOrdered_float64() {
	m := artmap.New[float64, int]()
	m.Put(-1.5, 0)
	m.Put(0.0, 1)
	m.Put(3.14, 2)

	v, _ := m.Get(3.14)
	fmt.Println(v)
	for k := range m.All() {
		fmt.Println(k)
	}
	for k := range m.Range(-1.0, 3.14) {
		fmt.Println("range:", k)
	}
	// Output:
	// 2
	// -1.5
	// 0
	// 3.14
	// range: 0
}

func ExampleOrdered_float32() {
	m := artmap.New[float32, int]()
	m.Put(-0.5, 0)
	m.Put(0.0, 1)
	m.Put(2.5, 2)
	for k := range m.All() {
		fmt.Println(k)
	}
	// Output:
	// -0.5
	// 0
	// 2.5
}

func ExampleOrdered_uint8() {
	m := artmap.New[uint8, string]()
	m.Put(255, "hi")
	m.Put(0, "lo")
	for k, v := range m.All() {
		fmt.Println(k, v)
	}
	// Output:
	// 0 lo
	// 255 hi
}

func ExampleOrdered_int8() {
	m := artmap.New[int8, int]()
	m.Put(-128, 0)
	m.Put(0, 1)
	m.Put(127, 2)
	for k := range m.All() {
		fmt.Println(k)
	}
	// Output:
	// -128
	// 0
	// 127
}
