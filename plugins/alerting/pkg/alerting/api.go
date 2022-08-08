package alerting

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"time"

	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const postableAlertRoute = "/api/v1/alerts"

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

// --- Trigger ---

func (p *Plugin) TriggerAlerts(ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest) (*alertingv1alpha.TriggerAlertsResponse, error) {
	// get the condition ID details
	a, err := p.GetAlertCondition(ctx, req.ConditionId)
	if err != nil {
		return nil, err
	}
	notifId := a.NotificationId

	// persist with alert log api
	_, err = p.CreateAlertLog(ctx, &corev1.AlertLog{
		ConditionId: req.ConditionId,
		Timestamp: &timestamppb.Timestamp{
			Seconds: time.Now().Unix(),
		},
		Metadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"Info":     structpb.NewStringValue(a.Description),
				"Severity": structpb.NewStringValue("Severe"),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if notifId != nil {
		// send notification
		var alertsArr []*PostableAlert
		alert := &PostableAlert{}
		alert.WithCondition(req.ConditionId.Id)
		resp, err := PostAlert(ctx, p.alertingOptions.Get().Endpoints[0], alertsArr)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to send trigger alert in alertmanager: %d", resp.StatusCode)
		}
	}
	// dispatch with alert condition id to alert endpoint id

	return &alertingv1alpha.TriggerAlertsResponse{}, nil
}
