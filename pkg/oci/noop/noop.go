package noop

import (
	"context"

	"github.com/rancher/opni/pkg/oci"
)

type noopOCIFetcher struct{}

func NewNoopOCIFetcher() (oci.Fetcher, error) {
	return &noopOCIFetcher{}, nil
}

func (d *noopOCIFetcher) GetImage(_ context.Context, _ oci.ImageType) (oci.Image, error) {
	return oci.Image{
		Registry:   "example.io",
		Repository: "opni-noop",
		Digest:     "sha256:123456789",
	}, nil
}

func init() {
	oci.RegisterFetcherBuilder("noop",
		func(args ...any) (oci.Fetcher, error) {
			return NewNoopOCIFetcher()
		},
	)
}
