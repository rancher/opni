package machinery

import (
	"errors"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/oci"
	"github.com/rancher/opni/pkg/update"
	"go.uber.org/zap"
)

func ConfigurePluginUpgrader(cfg v1beta1.PluginUpgradeSpec, pluginDir string, lg *zap.SugaredLogger) (update.SyncHandler, error) {
	switch cfg.Type {
	case v1beta1.PluginUpgradeBinary:
		builder := update.GetPluginSyncHandlerBuilder(cfg.Type)
		if builder == nil {
			return nil, errors.New("plugin provider not found")
		}
		return builder(pluginDir, lg)
	default:
		builder := update.GetPluginSyncHandlerBuilder("noop")
		return builder()
	}
}

func ConfigureAgentUpgrader(cfg *v1beta1.AgentUpgradeSpec, lg *zap.SugaredLogger) (update.SyncHandler, error) {
	switch {
	case cfg.Type == v1beta1.AgentUpgradeKubernetes:
		builder := update.GetAgentSyncHandlerBuilder(cfg.Type)
		if cfg.Kubernetes != nil {
			return builder(lg, cfg.Kubernetes.Namespace, cfg.Kubernetes.RepoOverride)
		}
		return builder(lg)
	case cfg.Type == v1beta1.AgentUpgradeNoop:
		builder := update.GetAgentSyncHandlerBuilder(cfg.Type)
		return builder()
	default:
		builder := update.GetAgentSyncHandlerBuilder("noop")
		return builder()
	}
}

func ConfigureOCIFetcher(providerType string, args ...any) (oci.Fetcher, error) {
	if providerType == "" {
		providerType = "noop"
	}
	builder := oci.GetFetcherBuilder(providerType)
	if builder == nil {
		return nil, errors.New("oci provider not found")
	}
	return builder(args...)
}
