package alerting_manager

import (
	"github.com/rancher/opni/pkg/alerting/shared"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AlertingDriverOptions struct {
	Logger             *zap.SugaredLogger                        `option:"logger"`
	K8sClient          client.Client                             `option:"k8sClient"`
	GatewayRef         types.NamespacedName                      `option:"gatewayRef"`
	ConfigKey          string                                    `option:"configKey"`
	InternalRoutingKey string                                    `option:"internalRoutingKey"`
	AlertingOptions    *shared.AlertingClusterOptions            `option:"alertingOptions"`
	Subscribers        []chan shared.AlertingClusterNotification `option:"subscribers"`
}
