package machinery

import (
	"github.com/rancher/opni/pkg/agent/upgrader"
	"github.com/rancher/opni/pkg/agentmanifest"
	"github.com/rancher/opni/pkg/config/v1beta1"
)

func ConfigureAgentManifestResolver(cfg *v1beta1.AgentManifestResolverSpec) (*agentmanifest.AgentManifestResolver, error) {
	switch {
	case cfg.Type == v1beta1.AgentManifestResolverTypeKubernetes:
		builder := agentmanifest.GetAgentManifestBuilder(v1beta1.AgentManifestResolverTypeKubernetes)
		driver, err := builder(cfg.Kubernetes.ControlNamespace)
		if err != nil {
			return nil, err
		}
		return agentmanifest.NewAgentManifestResolver(
			driver,
			agentmanifest.WithPackages(cfg.Packages...),
		), nil
	default:
		builder := agentmanifest.GetAgentManifestBuilder(v1beta1.AgentManifestResolverTypeNoop)
		driver, err := builder()
		if err != nil {
			return nil, err
		}
		return agentmanifest.NewAgentManifestResolver(
			driver,
			agentmanifest.WithPackages(cfg.Packages...),
		), nil
	}
}

func ConfigureAgentUpgrader(cfg *v1beta1.AgentUpgradeSpec) (upgrader.AgentUpgrader, error) {
	switch {
	case cfg.Type == v1beta1.AgentUpgradeKubernetes:
		builder := upgrader.GetUpgraderBuilder(cfg.Type)
		if cfg.Kubernetes != nil {
			return builder(cfg.Kubernetes.Namespace, cfg.RepoOverride)
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
