package machinery

import (
	"context"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/imageresolver"
	"github.com/rancher/opni/pkg/imageresolver/kubernetes"
)

func ConfigureImageResolver(_ context.Context, cfg *v1beta1.ImageResolverSpec) (*imageresolver.ImageResolver, error) {
	switch {
	case cfg.ImageRepoOverride != "" && cfg.ImageTagOverride != "":
		return imageresolver.NewImageResolver(
			imageresolver.NewDefaultResolveImageDriver(cfg.ImageRepoOverride, cfg.ImageTagOverride),
		), nil
	case cfg.Type == v1beta1.ImageResolverTypeKubernetes:
		driver, err := kubernetes.NewKubernetesResolveImageDriver(
			cfg.Kubernetes.ControlNamespace,
			cfg.ImageTagOverride,
		)
		if err != nil {
			return nil, err
		}
		return imageresolver.NewImageResolver(driver), nil
	default:
		return imageresolver.NewImageResolver(
			imageresolver.NewDefaultResolveImageDriver(cfg.ImageRepoOverride, cfg.ImageTagOverride),
		), nil
	}
}
