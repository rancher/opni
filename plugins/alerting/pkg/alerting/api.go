package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

// --- Trigger ---

func (p *Plugin) TriggerAlerts(ctx context.Context, req *alertingv1.TriggerAlertsRequest) (*alertingv1.TriggerAlertsResponse, error) {
	lg := p.Logger.With("Handler", "TriggerAlerts")
	lg.Debugf("Received request to trigger alerts  on condition %s", req.GetConditionId())
	lg.Debugf("Received alert annotations : %s", req.Annotations)

	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		lg.Errorf("Failed to fetch plugin options within timeout : %s", err)
		return nil, err
	}
	// dispatch with alert condition id to alert endpoint id, by obeying rate limiting from AM
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, options)
	if err != nil {
		return nil, err
	}
	apiNode := backend.NewAlertManagerPostAlertClient(
		ctx,
		availableEndpoint,
		backend.WithLogger(lg),
		backend.WithExpectClosure(backend.NewExpectStatusOk()),
		backend.WithPostAlertBody(req.ConditionId.Id, req.Annotations),
	)
	err = apiNode.DoRequest()
	if err != nil {
		return nil, err
	}
	return &alertingv1.TriggerAlertsResponse{}, nil
}

func (p *Plugin) ResolveAlerts(ctx context.Context, req *alertingv1.ResolveAlertsRequest) (*alertingv1.ResolveAlertsResponse, error) {
	lg := p.Logger.With("Handler", "ResolveAlerts")
	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		lg.Errorf("Failed to fetch plugin options within timeout : %s", err)
		return nil, err
	}
	// dispatch with alert condition id to alert endpoint id, by obeying rate limiting from AM
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, options)
	if err != nil {
		return nil, err
	}
	apiNode := backend.NewAlertManagerPostAlertClient(
		ctx,
		availableEndpoint,
		backend.WithLogger(lg),
		backend.WithExpectClosure(backend.NewExpectStatusOk()),
		backend.WithPostResolveAlertBody(req.ConditionId.Id, req.Annotations),
	)
	err = apiNode.DoRequest()
	if err != nil {
		return nil, err
	}
	return &alertingv1.ResolveAlertsResponse{}, nil
}
