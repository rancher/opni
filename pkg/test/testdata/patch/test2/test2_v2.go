//go:build v2

package main

import "fmt"

func main() {
	// print the first 50 Fibonacci numbers
	var a, b int = 0, 1
	for i := 0; i < 50; i++ {
		fmt.Println(a)
		a, b = b, a+b
	}
	// then print the first 50 Pell numbers
	fmt.Println()
	a, b = 0, 1
	for i := 0; i < 50; i++ {
		fmt.Println(a)
		a, b = b, 2*b+a
	}
}
