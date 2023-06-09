package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"dagger.io/dagger"
	"github.com/rancher/opni/dagger/x/cmds"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dagger/x <command> <args>")
		os.Exit(2)
	}

	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "testbin":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: go run ./dagger/x testbin 'TestBinOptions JSON'")
			os.Exit(2)
		}
		opts := cmds.TestBinOptions{}
		if err := json.Unmarshal([]byte(os.Args[2]), &opts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		ctr := cmds.TestBin(client, client.Container(), opts)

		mounts, err := ctr.Mounts(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var wg sync.WaitGroup
		for _, m := range mounts {
			m := m
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctr.File(m).Export(ctx, filepath.Join("./testbin/bin", m))
			}()
		}
		wg.Wait()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %s\n", os.Args[1])
	}
}
