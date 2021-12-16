package main

import (
	"log"

	"github.com/kralicky/opni-gateway/pkg/cli"
)

func main() {
	if err := cli.BuildRootCmd().Execute(); err != nil {
		log.Fatal(err)
	}
}
