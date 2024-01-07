package main

import (
	"context"
	"fmt"
)

type blah struct{}

// func (b *blah) Validate(ctx context.Context) {
// 	fmt.Println("hi")
// }

// func (b *blah) Validate() {
// 	fmt.Println("bye")
// }

func main() {
	var b *blah

	switch v := any(b).(type) {
	case interface{ Validate(context.Context) }:
		fmt.Println("context")
		v.Validate(context.Background())
	case interface{ Validate() }:
		fmt.Println("no context")
		v.Validate()
	}

}
