package alerting

import (
	"context"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync"
)

var alertingMutex = &sync.Mutex{}

// Provider alerting interface to be injected into the gateway
//
// Should at least encapsulate all alerting plugin implementations
type Provider interface {
	alertingv1alpha.AlertingClient
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
