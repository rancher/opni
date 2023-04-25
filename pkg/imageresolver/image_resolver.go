package imageresolver

import (
	"context"
	"fmt"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ResolveImageDriver interface {
	GetImageRepository(context.Context) string
	GetImageTag(context.Context) string
	UseMinimalImage() bool
}

type ImageResolver struct {
	controlv1.UnsafeImageUpgradeServer
	ResolveImageDriver
}

func NewImageResolver(driver ResolveImageDriver) *ImageResolver {
	return &ImageResolver{
		ResolveImageDriver: driver,
	}
}

func (r *ImageResolver) GetAgentImages(ctx context.Context, _ *emptypb.Empty) (*controlv1.AgentImages, error) {
	repo := r.GetImageRepository(ctx)
	tag := r.GetImageTag(ctx)
	return &controlv1.AgentImages{
		AgentImage: func() string {
			if r.UseMinimalImage() {
				return fmt.Sprintf("%s:%s-minimal", repo, tag)
			}
			return fmt.Sprintf("%s:%s", repo, tag)
		}(),
		ControllerImage: fmt.Sprintf("%s:%s", repo, tag),
	}, nil
}

type defaultImageResolverDriver struct {
	image string
	tag   string
}

func NewDefaultResolveImageDriver(image string, tag string) ResolveImageDriver {
	return &defaultImageResolverDriver{
		image: image,
		tag:   tag,
	}
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
