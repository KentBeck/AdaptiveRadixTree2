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
