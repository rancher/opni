package alerting_manager

import (
	alertingClient "github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/shared"
	"k8s.io/apimachinery/pkg/types"
	"log/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingDriverOptions struct {
	Logger             *slog.Logger                         `option:"logger"`
	K8sClient          client.Client                        `option:"k8sClient"`
	GatewayRef         types.NamespacedName                 `option:"gatewayRef"`
	ConfigKey          string                               `option:"configKey"`
	InternalRoutingKey string                               `option:"internalRoutingKey"`
	AlertingOptions    *shared.AlertingClusterOptions       `option:"alertingOptions"`
	Subscribers        []chan alertingClient.AlertingClient `option:"subscribers"`
}
