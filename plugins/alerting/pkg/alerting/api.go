package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/backend"

	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
)

// --- Trigger ---

func (p *Plugin) TriggerAlerts(ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest) (*alertingv1alpha.TriggerAlertsResponse, error) {
	lg := p.Logger.With("Handler", "TriggerAlerts")
	lg.Debugf("Received request to trigger alerts  on condition %s", req.GetConditionId())
	lg.Debugf("Received alert annotations : %s", req.Annotations)
	// get the condition ID details
	a, err := p.GetAlertCondition(ctx, req.ConditionId)
	if err != nil {
		return nil, err
	}
	sendNotif := a.GetAttachedEndpoints()

	if sendNotif != nil {
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
		// FIXME: submitting this during a reload can lead to a context.Cancel
		// on this post operation, however its unclear if this would lead to actual
		// problems in this function
		apiNode := backend.NewAlertManagerPostAlertClient(
			availableEndpoint,
			ctx,
			backend.WithLogger(lg),
			backend.WithExpectClosure(backend.NewExpectStatusOk()),
		)
		err = apiNode.DoRequest()
		if err != nil {
			return nil, err
		}
	}
	return &alertingv1alpha.TriggerAlertsResponse{}, nil
}
