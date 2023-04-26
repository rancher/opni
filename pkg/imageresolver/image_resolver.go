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

var (
	imageResolverBuilderCache = map[string]func(...any) (ResolveImageDriver, error){}
)

func RegisterImageResolverBuilder[T ~string](name T, builder func(...any) (ResolveImageDriver, error)) {
	imageResolverBuilderCache[string(name)] = builder
}

func GetImageResolverBuilder[T ~string](name T) func(...any) (ResolveImageDriver, error) {
	return imageResolverBuilderCache[string(name)]
}
