package cmds

import (
	"fmt"

	"dagger.io/dagger"
)

type Binary struct {
	Name        string `json:"name"`
	SourceImage string `json:"sourceImage"`
	Version     string `json:"version"`
	Path        string `json:"path"`
}

type TestBinOptions struct {
	Binaries []Binary `json:"binaries"`

	// If true, will mount files into the container instead of copying them.
	MountOnly bool `json:"-"`
}

func TestBin(client *dagger.Client, ctr *dagger.Container, opts TestBinOptions) *dagger.Container {
	ctr = ctr.Pipeline("Download Test Binaries")
	targets := map[string][]Binary{}
	for _, b := range opts.Binaries {
		if b.Version == "" {
			b.Version = "latest"
		}
		img := fmt.Sprintf("%s:%s", b.SourceImage, b.Version)
		targets[img] = append(targets[img], b)
	}

	for img, binaries := range targets {
		img := img
		binaryCtr := client.Container().From(img)
		for _, b := range binaries {
			path, file := fmt.Sprintf("/src/testbin/bin/%s", b.Name), binaryCtr.File(b.Path)
			if opts.MountOnly {
				ctr = ctr.WithMountedFile(path, file)
			} else {
				ctr = ctr.WithFile(path, file)
			}
		}
	}

	return ctr
}
