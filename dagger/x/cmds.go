package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dagger.io/dagger"
	"golang.org/x/sync/errgroup"
)

type Binary struct {
	Name        string `json:"name"`
	SourceImage string `json:"sourceImage"`
	Version     string `json:"version"`
	Path        string `json:"path"`
}

type TestBinOptions struct {
	Binaries []Binary `json:"binaries"`
}

func TestBin(ctx context.Context, client *dagger.Client, opts TestBinOptions) error {
	targets := map[string][]Binary{}
	for _, b := range opts.Binaries {
		if b.Version == "" {
			b.Version = "latest"
		}
		img := fmt.Sprintf("%s:%s", b.SourceImage, b.Version)
		targets[img] = append(targets[img], b)
	}

	eg, ctx := errgroup.WithContext(ctx)
	for img, binaries := range targets {
		img := img
		for _, b := range binaries {
			b := b
			eg.Go(func() error {
				ctr := client.Container().From(img)
				location := b.Path
				check := []string{"/", "/usr/bin", "/usr/local/bin"}
				for _, c := range check {
					abs := filepath.Join(c, b.Name)
					if _, err := ctr.File(abs).Size(ctx); err == nil {
						location = abs
						break
					}
				}
				if location == "" {
					out, err := ctr.
						WithExec([]string{"which", b.Name}, dagger.ContainerWithExecOpts{SkipEntrypoint: true}).
						Stdout(ctx)
					if err != nil {
						return err
					}
					location = strings.TrimSpace(out)
				}
				if location == "" {
					return fmt.Errorf("could not find binary %q in image %q", b.Name, img)
				}
				fmt.Printf("Found binary %q at %q\n", b.Name, location)

				if _, err := ctr.File(location).Export(ctx, filepath.Join("testbin/bin", b.Name)); err != nil {
					return err
				}
				return nil
			})
		}
	}

	return eg.Wait()
}

func main() {
	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "testbin":
		opts := TestBinOptions{}
		if err := json.Unmarshal([]byte(os.Args[2]), &opts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := TestBin(ctx, client, opts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
