package alerting

import (
	"context"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/condition"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/endpoint"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/log"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/trigger"
	"google.golang.org/protobuf/types/known/emptypb"
)

var alertingMutex = &sync.Mutex{}

// Provider alerting interface to be injected into the gateway
//
// Should at least encapsulate all alerting plugin implementations
type Provider interface {
	endpoint.AlertEndpointsClient
	condition.AlertConditionsClient
	log.AlertLogsClient
	trigger.AlertingClient
}

func IsNil(p *Provider) bool {
	alertingMutex.Lock()
	defer alertingMutex.Unlock()
	return p == nil
}

func DoCreate(
	p Provider,
	ctx context.Context,
	req *alertingv1alpha.AlertCondition,
) (*corev1.Reference, error) {
	alertingMutex.Lock()
	defer alertingMutex.Unlock()
	return p.CreateAlertCondition(ctx, req)
}

func DoDelete(
	p Provider,
	ctx context.Context,
	req *corev1.Reference,
) (*emptypb.Empty, error) {
	alertingMutex.Lock()
	defer alertingMutex.Unlock()
	return p.DeleteAlertEndpoint(ctx, req)
}

func DoTrigger(
	p Provider,
	ctx context.Context,
	req *alertingv1alpha.TriggerAlertsRequest,
) (*alertingv1alpha.TriggerAlertsResponse, error) {
	alertingMutex.Lock()
	defer alertingMutex.Unlock()
	return p.TriggerAlerts(ctx, req)
}
