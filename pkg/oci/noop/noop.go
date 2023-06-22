package noop

import (
	"context"

	"github.com/opencontainers/go-digest"
	"github.com/rancher/opni/pkg/oci"
)

const (
	imageDigest = "sha256:15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225"
)

type noopOCIFetcher struct{}

func NewNoopOCIFetcher() (oci.Fetcher, error) {
	return &noopOCIFetcher{}, nil
}

func (d *noopOCIFetcher) GetImage(_ context.Context, _ oci.ImageType) (*oci.Image, error) {
	return &oci.Image{
		Registry:   "example.io",
		Repository: "opni-noop",
		Digest:     digest.Digest(imageDigest),
	}, nil
}

func init() {
	oci.RegisterFetcherBuilder("noop",
		func(args ...any) (oci.Fetcher, error) {
			return NewNoopOCIFetcher()
		},
	)
}
