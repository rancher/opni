package machinery

import (
	"errors"
	"log/slog"

	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/oci"
	"github.com/rancher/opni/pkg/update"
)

func ConfigurePluginUpgrader(cfg *configv1.PluginUpgradesSpec, pluginDir string, lg *slog.Logger) (update.SyncHandler, error) {
	switch cfg.GetDriver() {
	case configv1.PluginUpgradesSpec_Binary:
		builder := update.GetPluginSyncHandlerBuilder(cfg.GetDriver().String())
		if builder == nil {
			return nil, errors.New("plugin provider not found")
		}
		return builder(pluginDir, lg)
	default:
		builder := update.GetPluginSyncHandlerBuilder(configv1.PluginUpgradesSpec_Noop.String())
		return builder()
	}
}

func ConfigureAgentUpgrader(cfg *configv1.AgentUpgradesSpec, lg *slog.Logger) (update.SyncHandler, error) {
	switch cfg.GetDriver() {
	case configv1.AgentUpgradesSpec_Kubernetes:
		builder := update.GetAgentSyncHandlerBuilder(cfg.GetDriver().String())
		if cfg.Kubernetes != nil {
			return builder(lg, cfg.GetKubernetes().GetNamespace(), cfg.Kubernetes.GetRepoOverride())
		}
		return builder(lg)
	case configv1.AgentUpgradesSpec_Noop:
		builder := update.GetAgentSyncHandlerBuilder(cfg.GetDriver().String())
		return builder()
	default:
		builder := update.GetAgentSyncHandlerBuilder(configv1.AgentUpgradesSpec_Kubernetes.String())
		return builder()
	}
}

func ConfigureOCIFetcher(providerType configv1.KubernetesAgentUpgradeSpec_ImageResolver, args ...any) (oci.Fetcher, error) {
	builder := oci.GetFetcherBuilder(providerType.String())
	if builder == nil {
		return nil, errors.New("oci provider not found")
	}
	return builder(args...)
}
