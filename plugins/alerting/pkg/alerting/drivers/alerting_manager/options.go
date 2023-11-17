package alerting_manager

import (
	"context"
	"crypto/tls"

	alertingClient "github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/shared"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingDriverOptions struct {
	Context            context.Context                      `option:"context"`
	K8sClient          client.Client                        `option:"k8sClient"`
	GatewayRef         types.NamespacedName                 `option:"gatewayRef"`
	ConfigKey          string                               `option:"configKey"`
	InternalRoutingKey string                               `option:"internalRoutingKey"`
	AlertingOptions    *shared.AlertingClusterOptions       `option:"alertingOptions"`
	Subscribers        []chan alertingClient.AlertingClient `option:"subscribers"`
	TlsConfig          *tls.Config                          `option:"tlsConfig"`
}
