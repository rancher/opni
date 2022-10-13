package alerting

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/rancher/opni/pkg/alerting/backend"

	"github.com/rancher/opni/pkg/alerting/shared"
	"go.uber.org/zap"

	"github.com/rancher/opni/pkg/alerting/templates"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const postableAlertRoute = "/api/v1/alerts"
const amRetries = 5

func list[T proto.Message](ctx context.Context, kvc storage.KeyValueStoreT[T], prefix string) ([]T, error) {
	keys, err := kvc.ListKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}
	items := make([]T, len(keys))
	for i, key := range keys {
		item, err := kvc.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}

func listWithKeys[T proto.Message](ctx context.Context, kvc storage.KeyValueStoreT[T], prefix string) ([]string, []T, error) {
	keys, err := kvc.ListKeys(ctx, prefix)
	if err != nil {
		return nil, nil, err
	}
	items := make([]T, len(keys))
	ids := make([]string, len(keys))
	for i, key := range keys {
		item, err := kvc.Get(ctx, key)

		if err != nil {
			return nil, nil, err
		}
		items[i] = item
		ids[i] = path.Base(key)
	}
	return ids, items, nil
}

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

func dispatchAlert(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, alertsArr []*backend.PostableAlert) error {
	_, resp, err := backend.PostAlert(ctx, options.GetWorkerEndpoint(), alertsArr)
	if err != nil {
		lg.With("handler", "PostAlert").Error(err)
		return err
	}
	// FIXME reimplement
	// for i := 0; i < amRetries; i++ {
	// 	resp, err = http.DefaultClient.Do(req)
	// 	if err != nil {
	// 		lg.With("handler", "PostAlert").Error(err)
	// 		return err
	// 	}
	// }
	if resp.StatusCode != http.StatusOK {
		lg.With("handler", "PostAlert").Error(fmt.Sprintf("Unexpected response: %s", resp.Status))
		return fmt.Errorf("failed to send trigger alert in alertmanager: %d", resp.StatusCode)
	}
	return nil
}

func sendNotificationWithRateLimiting(p *Plugin, ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest) error {
	lg := p.Logger
	// check for rate limiting
	options, err := p.AlertingOptions.GetContext(ctx)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to load alerting options in required time : %s", err))
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
	return dispatchAlert(ctx, lg, options, alertsArr)
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
	notifId := a.NotificationId

	// persist with alert log api
	//FIXME: for now ignore errors in creating alert logs
	_, _ = p.CreateAlertLog(ctx, &corev1.AlertLog{
		ConditionId: req.ConditionId,
		Timestamp: &timestamppb.Timestamp{
			Seconds: time.Now().Unix(),
		},
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"Info": structpb.NewStringValue(a.Description),
				//TODO : convert severity grpc enum to string
				"Severity": structpb.NewStringValue("Severe"),
			},
		},
	})
	if notifId != nil {
		// dispatch with alert condition id to alert endpoint id, by obeying rate limiting from AM
		err = sendNotificationWithRateLimiting(p, ctx, req)
		if err != nil {
			return nil, shared.WithInternalServerError(fmt.Sprintf("%s", err))
		}
	}
	return &alertingv1alpha.TriggerAlertsResponse{}, nil
}

func (p *Plugin) ListAvailableTemplatesForType(ctx context.Context, request *alertingv1alpha.AlertDetailChoicesRequest) (*alertingv1alpha.TemplatesResponse, error) {
	details := alertingv1alpha.EnumConditionToImplementation[alertingv1alpha.AlertType_SYSTEM]

	return &alertingv1alpha.TemplatesResponse{
		Template: templates.StrSliceAsTemplates(details.ListTemplates()),
	}, nil
}
