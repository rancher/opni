package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/proto"
)

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

// --- Trigger ---

func (p *Plugin) TriggerAlerts(ctx context.Context, req *alertingv1alpha.TriggerAlertsRequest) (*alertingv1alpha.TriggerAlertsResponse, error) {
	// persist with alert log api

	// dispatch with alert condition id to alert endpoint id

	return nil, shared.AlertingErrNotImplemented
}
