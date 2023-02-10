package alerting

import (
	"context"
	"sync"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
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
	ctx context.Context,
	p Provider,
	req *alertingv1.AlertCondition,
) (*corev1.Reference, error) {
	alertingMutex.Lock()
	defer alertingMutex.Unlock()
	return p.CreateAlertCondition(ctx, req)
}

func DoDelete(
	ctx context.Context,
	p Provider,
	req *corev1.Reference,
) (*emptypb.Empty, error) {
	alertingMutex.Lock()
	defer alertingMutex.Unlock()
	return p.DeleteAlertCondition(ctx, req)
}

func DoTrigger(
	ctx context.Context,
	p Provider,
	req *alertingv1.TriggerAlertsRequest,
) (*alertingv1.TriggerAlertsResponse, error) {
	alertingMutex.Lock()
	defer alertingMutex.Unlock()
	return p.TriggerAlerts(ctx, req)
}
