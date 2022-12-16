//go:build v1

package main

import "fmt"

func main() {
	// print the first 10 rows of Pascal's triangle
	for i := 0; i < 10; i++ {
		for j := 0; j <= i; j++ {
			fmt.Printf("%d ", binomial(i, j))
		}
		fmt.Println()
	}
}

func binomial(n, k int) int {
	if k == 0 || k == n {
		return 1
	}
	return binomial(n-1, k-1) + binomial(n-1, k)
}
