package machinery

import (
	"context"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/imageresolver"
)

func ConfigureImageResolver(_ context.Context, cfg *v1beta1.ImageResolverSpec) (*imageresolver.ImageResolver, error) {
	switch {
	case cfg.ImageRepoOverride != "" && cfg.ImageTagOverride != "":
		builder := imageresolver.GetImageResolverBuilder(v1beta1.ImageResolverTypeDefault)
		driver, err := builder(cfg.ImageRepoOverride, cfg.ImageTagOverride)
		if err != nil {
			return nil, err
		}
		return imageresolver.NewImageResolver(
			driver,
		), nil
	case cfg.Type == v1beta1.ImageResolverTypeKubernetes:
		builder := imageresolver.GetImageResolverBuilder(v1beta1.ImageResolverTypeKubernetes)
		driver, err := builder(cfg.Kubernetes.ControlNamespace, cfg.ImageTagOverride)
		if err != nil {
			return nil, err
		}
		return imageresolver.NewImageResolver(
			driver,
		), nil
	default:
		builder := imageresolver.GetImageResolverBuilder(v1beta1.ImageResolverTypeDefault)
		driver, err := builder(cfg.ImageRepoOverride, cfg.ImageTagOverride)
		if err != nil {
			return nil, err
		}
		return imageresolver.NewImageResolver(
			driver,
		), nil
	}
}
