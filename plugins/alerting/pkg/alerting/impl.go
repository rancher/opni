package alerting

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alarms/v1"

	"github.com/nats-io/nats.go"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	natsutil "github.com/rancher/opni/pkg/util/nats"
)

func (p *Plugin) newClusterWatcherHooks(ctx context.Context, ingressStream *nats.StreamConfig) *management.ManagementWatcherHooks[*managementv1.WatchEvent] {
	err := natsutil.NewPersistentStream(p.js.Get(), ingressStream)
	if err != nil {
		panic(err)
	}
	cw := management.NewManagementWatcherHooks[*managementv1.WatchEvent](ctx)
	cw.RegisterHook(
		createClusterEvent,
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			err := natsutil.NewDurableReplayConsumer(p.js.Get(), ingressStream.Name, alarms.NewAgentDurableReplayConsumer(event.Cluster.Id))
			p.logger.Info(fmt.Sprintf("added durable ordered push consumer for cluster %s", event.Cluster.Id))
			if err != nil {
				panic(err)
			}
			return nil
		},
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			return p.createDefaultDisconnect(event.Cluster.Id)
		},
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			return p.createDefaultCapabilityHealth(event.Cluster.Id)
		},
	)
	cw.RegisterHook(
		deleteClusterEvent,
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			return p.onDeleteClusterAgentDisconnectHook(ctx, event.Cluster.Id)
		},
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			return p.onDeleteClusterCapabilityHook(ctx, event.Cluster.Id)
		},
	)
	return cw
}

func createClusterEvent(event *managementv1.WatchEvent) bool {
	return event.Type == managementv1.WatchEventType_Put &&
		event.Previous == nil
}

func deleteClusterEvent(event *managementv1.WatchEvent) bool {
	return event.Type == managementv1.WatchEventType_Delete
}
