package alerting

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rancher/opni/pkg/alerting/backend"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni/pkg/alerting/shared"
	"go.uber.org/zap"

	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
)

const postableAlertRoute = "/api/v1/alerts"
const amRetries = 5

func checkRateLimiting(ctx context.Context, lg *zap.SugaredLogger, conditionId string, options shared.NewAlertingOptions) (bool, error) {
	_, resp, err := backend.GetAlerts(ctx, options.GetWorkerEndpoint())
	if err != nil {
		lg.With("handler", "GetAlerts").Error(err)
		return true, err
	}
	// FIXME reimplement
	// for i := 0; i < amRetries; i++ {
	// 	resp, err = OnRetryResponse(getReq, resp)
	// 	if err != nil {
	// 		lg.With("handler", "GetAlerts").Error(err)
	// 		return true, err
	// 	}
	// 	time.Sleep(time.Second)
	// }
	return backend.IsRateLimited(conditionId, resp, lg)
}

func constructAlerts(conditionId string, annotations map[string]string) []*backend.PostableAlert {
	// marshal data into a reasonable post alert request
	var alertsArr []*backend.PostableAlert
	alert := &backend.PostableAlert{}
	alert.WithCondition(conditionId)
	for annotationName, annotationValue := range annotations {
		alert.WithRuntimeInfo(annotationName, annotationValue)
	}
	alertsArr = append(alertsArr, alert)
	return alertsArr
}

func dispatchAlert(p *Plugin, ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, alertsArr []*backend.PostableAlert) error {
	var availableEndpoint string
	status, err := p.opsNode.GetClusterConfiguration(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	if status.NumReplicas == 1 { // exactly one that is the controller
		availableEndpoint = options.GetControllerEndpoint()
	} else {
		availableEndpoint = options.GetWorkerEndpoint()
	}
	_, resp, err := backend.PostAlert(ctx, availableEndpoint, alertsArr)
	if err != nil {
		lg.With("handler", "PostAlert").Error(err)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With("handler", "PostAlert").Error(fmt.Sprintf("Unexpected response: %s", resp.Status))
		return fmt.Errorf("failed to send trigger alert in alertmanager: %d", resp.StatusCode)
	}
	return nil
}

func sendNotificationWithRateLimiting(p *Plugin, ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest) error {
	lg := p.Logger
	// check for rate limiting
	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		lg.Errorf("Failed to fetch plugin options within timeout : %s", err)
		return err
	}
	//FIXME: uncomment for now
	//shouldExitEarly, err := checkRateLimiting(ctx, lg, req.ConditionId.Id, options)
	//if err != nil {
	//	lg.Error(err)
	//	return err
	//}
	//if shouldExitEarly {
	//	return nil
	//}
	// marshal data into a reasonable post alert request
	alertsArr := constructAlerts(req.ConditionId.Id, req.Annotations)
	lg.Debug(fmt.Sprintf("Triggering alert for condition %s on endpoint %s", req.ConditionId.Id, options.GetWorkerEndpoint()))

	// send dispatch request
	return dispatchAlert(p, ctx, lg, options, alertsArr)
}

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

	// TODO : log triggers in the future

	if sendNotif != nil {
		// dispatch with alert condition id to alert endpoint id, by obeying rate limiting from AM
		err = sendNotificationWithRateLimiting(p, ctx, req)
		if err != nil {
			return nil, shared.WithInternalServerError(fmt.Sprintf("%s", err))
		}
	}
	return &alertingv1alpha.TriggerAlertsResponse{}, nil
}
