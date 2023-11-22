package services

import (
	"fmt"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"google.golang.org/protobuf/types/known/emptypb"
)

func RequestNodeSync(sctx types.ServiceContext, target *corev1.Reference) error {
	if target == nil || target.Id == "" {
		panic("bug: target must be non-nil and have a non-empty ID. this logic was recently changed - please update the caller")
	}
	_, err := sctx.Delegate().
		WithTarget(target).
		SyncNow(sctx, &capabilityv1.Filter{CapabilityNames: []string{wellknown.CapabilityMetrics}})
	return err
}

func BroadcastNodeSync(sctx types.ServiceContext) error {
	lock := system.NewLock(sctx, sctx.KeyValueStoreClient(), "locks/broadcast-sync")
	return lock.Try(
		func() { // acquired
			broadcastNodeSyncLocked(sctx)
		},
		func() { // not acquired
			sctx.Logger().Debug("skipping broadcast sync")
		},
	)
}

func broadcastNodeSyncLocked(sctx types.ServiceContext) {
	sctx.Delegate().
		WithBroadcastSelector(&streamv1.TargetSelector{}, func(target *corev1.Reference, _ *emptypb.Empty, err error) {
			if err != nil {
				sctx.Logger().With(
					"agent", target.GetId(),
					logger.Err(err),
				).Warn("agent responded with error to sync request")
			}
		}).
		SyncNow(sctx, &capabilityv1.Filter{
			CapabilityNames: []string{wellknown.CapabilityMetrics},
		})
}

func StartActiveSyncWatcher[T any](ctx types.ServiceContext, activeStore storage.KeyValueStoreT[T]) error {
	lg := ctx.Logger().WithGroup("active-sync")
	activeEvents, err := activeStore.Watch(ctx, "", storage.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to watch active config: %w", err)
	}
	go func() {
		for {
			e, ok := <-activeEvents
			if !ok {
				return
			}
			var id string
			switch e.EventType {
			case storage.WatchEventPut:
				id = e.Current.Key()
			case storage.WatchEventDelete:
				id = e.Previous.Key()
			}
			if id == "" {
				continue
			}
			if err := RequestNodeSync(ctx, &corev1.Reference{Id: id}); err != nil {
				lg.Warn("failed to request node sync", logger.Err(err))
			} else {
				lg.Info("requested node sync", "id", id)
			}
		}
	}()
	return nil
}

func StartDefaultSyncWatcher[T any](ctx types.ServiceContext, defaultStore storage.ValueStoreT[T]) error {
	lg := ctx.Logger().WithGroup("default-sync")
	defaultEvents, err := defaultStore.Watch(ctx)
	if err != nil {
		return fmt.Errorf("failed to watch default config: %w", err)
	}
	go func() {
		for {
			_, ok := <-defaultEvents
			if !ok {
				return
			}
			if err := BroadcastNodeSync(ctx); err != nil {
				lg.Warn("failed to broadcast node sync", logger.Err(err))
			} else {
				lg.Info("broadcasted node sync")
			}
		}
	}()
	return nil
}
