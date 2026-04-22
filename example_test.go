package art_test

import (
	"fmt"

	art "github.com/KentBeck/AdaptiveRadixTree2"
)

func ExampleTree_Put() {
	t := art.New[int]()
	t.Put([]byte("apple"), 1)
	v, _ := t.Get([]byte("apple"))
	fmt.Println(v)
	// Output:
	// 1
}

func ExampleTree_Get() {
	t := art.New[int]()
	t.Put([]byte("apple"), 1)
	hit, ok := t.Get([]byte("apple"))
	fmt.Println(hit, ok)
	miss, ok := t.Get([]byte("banana"))
	fmt.Println(miss, ok)
	// Output:
	// 1 true
	// 0 false
}

func ExampleTree_Delete() {
	t := art.New[int]()
	t.Put([]byte("apple"), 1)
	fmt.Println(t.Delete([]byte("apple")))
	fmt.Println(t.Delete([]byte("apple")))
	// Output:
	// true
	// false
}

func ExampleTree_Len() {
	t := art.New[int]()
	fmt.Println(t.Len())
	t.Put([]byte("apple"), 1)
	t.Put([]byte("apricot"), 2)
	fmt.Println(t.Len())
	t.Delete([]byte("apple"))
	fmt.Println(t.Len())
	// Output:
	// 0
	// 2
	// 1
}

func ExampleTree_All() {
	t := art.New[int]()
	t.Put([]byte("banana"), 3)
	t.Put([]byte("apple"), 1)
	t.Put([]byte("apricot"), 2)
	for k, v := range t.All() {
		fmt.Printf("%s=%d\n", k, v)
	}
	// Output:
	// apple=1
	// apricot=2
	// banana=3
}

func ExampleTree_Range() {
	t := art.New[int]()
	t.Put([]byte("apple"), 1)
	t.Put([]byte("apricot"), 2)
	t.Put([]byte("banana"), 3)
	for k, v := range t.Range([]byte("ap"), []byte("b")) {
		fmt.Printf("%s=%d\n", k, v)
	}
	// Output:
	// apple=1
	// apricot=2
}

func ExampleTree_Min() {
	t := art.New[int]()
	t.Put([]byte("banana"), 3)
	t.Put([]byte("apple"), 1)
	t.Put([]byte("apricot"), 2)
	k, v, ok := t.Min()
	fmt.Printf("%s=%d ok=%v\n", k, v, ok)
	// Output:
	// apple=1 ok=true
}

func ExampleTree_Max() {
	t := art.New[int]()
	t.Put([]byte("banana"), 3)
	t.Put([]byte("apple"), 1)
	t.Put([]byte("apricot"), 2)
	k, v, ok := t.Max()
	fmt.Printf("%s=%d ok=%v\n", k, v, ok)
	// Output:
	// banana=3 ok=true
}

func ExampleTree_Ceiling() {
	t := art.New[int]()
	t.Put([]byte("apple"), 1)
	t.Put([]byte("apricot"), 2)
	t.Put([]byte("banana"), 3)
	k, v, ok := t.Ceiling([]byte("apq"))
	fmt.Printf("%s=%d ok=%v\n", k, v, ok)
	_, _, ok = t.Ceiling([]byte("z"))
	fmt.Println(ok)
	// Output:
	// apricot=2 ok=true
	// false
}

func ExampleTree_Floor() {
	t := art.New[int]()
	t.Put([]byte("apple"), 1)
	t.Put([]byte("apricot"), 2)
	t.Put([]byte("banana"), 3)
	k, v, ok := t.Floor([]byte("apq"))
	fmt.Printf("%s=%d ok=%v\n", k, v, ok)
	_, _, ok = t.Floor([]byte(""))
	fmt.Println(ok)
	// Output:
	// apple=1 ok=true
	// false
}

func ExampleTree_Clone() {
	t := art.New[int]()
	t.Put([]byte("apple"), 1)
	cp := t.Clone()
	cp.Put([]byte("banana"), 2)
	fmt.Println(t.Len(), cp.Len())
	// Output:
	// 1 2
}

func ExampleTree_Clear() {
	t := art.New[int]()
	t.Put([]byte("apple"), 1)
	t.Put([]byte("banana"), 2)
	t.Clear()
	fmt.Println(t.Len())
	// Output:
	// 0
}
