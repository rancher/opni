package machinery

import (
	"github.com/rancher/opni/pkg/agent/upgrader"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/imageresolver"
)

func ConfigureImageResolver(cfg *v1beta1.ImageResolverSpec) (*imageresolver.ImageResolver, error) {
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

func ConfigureAgentUpgrader(cfg *v1beta1.AgentUpgradeSpec) (upgrader.AgentUpgrader, error) {
	switch {
	case cfg.Type == v1beta1.AgentUpgradeKubernetes:
		builder := upgrader.GetUpgraderBuilder(cfg.Type)
		if cfg.Kubernetes != nil {
			return builder(cfg.Kubernetes.Namespace)
		}
		return builder()
	case cfg.Type == v1beta1.AgentUpgradeNoop:
		builder := upgrader.GetUpgraderBuilder(cfg.Type)
		return builder()
	default:
		builder := upgrader.GetUpgraderBuilder("noop")
		return builder()
	}
}
