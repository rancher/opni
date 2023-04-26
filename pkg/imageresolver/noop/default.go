package noop

import (
	"context"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/imageresolver"
)

type defaultImageResolverDriver struct {
	image string
	tag   string
}

func NewDefaultResolveImageDriver(image string, tag string) (imageresolver.ResolveImageDriver, error) {
	return &defaultImageResolverDriver{
		image: image,
		tag:   tag,
	}, nil
}

func (d *defaultImageResolverDriver) GetImageRepository(_ context.Context) string {
	if d.image == "" {
		return "example.io/opni"
	}
	return d.image
}

func (d *defaultImageResolverDriver) GetImageTag(_ context.Context) string {
	if d.tag == "" {
		return "latest"
	}
	return d.tag
}

func (d *defaultImageResolverDriver) UseMinimalImage() bool {
	return true
}

func init() {
	imageresolver.RegisterImageResolverBuilder(v1beta1.ImageResolverTypeDefault,
		func(args ...any) (imageresolver.ResolveImageDriver, error) {
			image := args[0].(string)
			tag := args[1].(string)

			return NewDefaultResolveImageDriver(image, tag)
		},
	)
}
