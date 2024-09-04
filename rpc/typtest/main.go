package main

import (
	"fmt"
	"time"
)

type C struct {
	Name string    `json:"name"`
	Age  int       `json:"age"`
	Date time.Time `json:"date"`
}

type Blah[A, B any] struct {
	a A
	b B
	c C
}

func (b Blah[A, B]) String() string {
	return fmt.Sprintf("%+v, %+v, %+v", b.a, b.b, b.c)
}

func main() {
	var b any = Blah[int, string]{1, "hello", C{"name", 1, time.Now()}}
	y, ok := b.(fmt.Stringer)
	if !ok {
		fmt.Println("not a stringer")
		return
	}
	fmt.Println(y.String())
}
