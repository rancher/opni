package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"dagger.io/dagger"
	"github.com/rancher/opni/dagger/x/cmds"
	"golang.org/x/sync/errgroup"
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
		opts.MountOnly = true
		ctr := cmds.TestBin(client, client.Container(), opts)

		var eg errgroup.Group
		for _, b := range opts.Binaries {
			b := b
			eg.Go(func() error {
				_, err := ctr.File(filepath.Join("/src/testbin/bin/", b.Name)).Export(ctx, filepath.Join("./testbin/bin", b.Name))
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %s\n", os.Args[1])
	}
}
