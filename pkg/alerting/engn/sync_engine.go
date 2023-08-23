package engn

import (
	"context"

	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

type SyncEngine interface {
	Sync(ctx context.Context, info alertingSync.SyncInfo) error
}

func NewSyncEngine(
	ctx context.Context,
	constructor SyncConstructor,
	driver drivers.ClusterDriver,
	syncManager SyncManager,
) SyncEngine {
	return &syncEngine{
		ctx:             ctx,
		lg:              logger.NewPluginLogger().Named("alerting-syncer"),
		syncConstructor: constructor,
		driver:          driver,
		syncManager:     syncManager,
	}
}

type syncEngine struct {
	ctx context.Context
	lg  *zap.SugaredLogger

	syncConstructor SyncConstructor
	syncManager     SyncManager
	driver          drivers.ClusterDriver
	lastSyncId      string
}

func (s *syncEngine) Sync(ctx context.Context, info alertingSync.SyncInfo) error {
	if !info.ShouldSync {
		return nil
	}

	syncId, err := s.syncConstructor.GetHash()
	if err != nil {
		return err
	}
	if syncId != s.lastSyncId {
		s.lastSyncId = syncId
		s.lg.Infof("broadcasting sync with syncId %s", syncId)
		payload, err := s.syncConstructor.Construct()
		if err != nil {
			s.lg.Errorf("failed to construct sync payload %s", err)
			return err
		}
		s.syncManager.Push(payload)
		return nil
	}

	remoteInfo := s.syncManager.Info()
	queued := []string{}
	for id, info := range remoteInfo {
		if info.LastSyncId != syncId {
			queued = append(queued, id)
		}
		if info.LastSyncState != alertops.SyncState_Synced {
			queued = append(queued, id)
		}
	}
	queued = lo.Uniq(queued)
	if len(queued) > 0 {
		payload, err := s.syncConstructor.Construct()
		if err != nil {
			s.lg.Errorf("failed to construct sync payload %s", err)
			return err
		}
		for _, id := range queued {
			s.syncManager.PushTarget(id, payload)
		}
	}
	return nil
}
