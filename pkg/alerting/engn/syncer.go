package engn

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util/buffer"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var DefaultSyncTimeout = 5 * time.Second

type SyncConstructor interface {
	Construct() (SyncPayload, error)
	GetHash() (string, error)
}

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

type SyncManager interface {
	alertops.ConfigReconcilerServer
	Targets() []string
	Push(payload SyncPayload)
	PushTarget(id string, payload SyncPayload)
	Info() map[string]RemoteInfo
}

type SyncPayload struct {
	SyncId    string `json:"syncId"`
	ConfigKey string `json:"configKey"`
	Data      []byte `json:"data"`
}

type RemoteInfo struct {
	WhoAmI        string
	LastApplied   *timestamppb.Timestamp
	LastSyncState alertops.SyncState
	LastSyncId    string
}

type syncManager struct {
	alertops.UnsafeConfigReconcilerServer

	ctx context.Context
	lg  *zap.SugaredLogger

	remoteMu   sync.Mutex
	remoteInfo map[string]RemoteInfo

	syncConstructor SyncConstructor
	syncMu          sync.RWMutex
	inputSyncChan   chan *alertops.SyncRequest
	syncPushers     map[string]*buffer.RingBuffer[*alertops.SyncRequest]
}

func NewSyncManager(
	ctx context.Context,
	constructor SyncConstructor,
) SyncManager {
	return &syncManager{
		ctx:             ctx,
		lg:              logger.NewPluginLogger().Named("alerting-syncer"),
		syncConstructor: constructor,
		remoteInfo:      make(map[string]RemoteInfo),
		inputSyncChan:   make(chan *alertops.SyncRequest, 1),
		syncPushers:     make(map[string]*buffer.RingBuffer[*alertops.SyncRequest]),
	}
}

var _ SyncManager = (*syncManager)(nil)

func (s *syncManager) addSyncPusher(id string, pusher chan *alertops.SyncRequest) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	b := buffer.NewRingBuffer(s.inputSyncChan, pusher)
	s.syncPushers[id] = b
	go b.Run()

}

func (s *syncManager) removeSyncPusher(id string) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	delete(s.syncPushers, id)
}

func (s *syncManager) addInfo(id string, info RemoteInfo) {
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	s.remoteInfo[id] = info
}

func (s *syncManager) removeInfo(id string) {
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	delete(s.remoteInfo, id)
}

func (s *syncManager) SyncConfig(server alertops.ConfigReconciler_SyncConfigServer) error {
	s.lg.Debug("connecting client")
	assignedLifecycleUuid := uuid.New().String()
	syncChan := make(chan *alertops.SyncRequest, 1)
	defer func() {
		close(syncChan)
		s.removeSyncPusher(assignedLifecycleUuid)
		s.removeInfo(assignedLifecycleUuid)
	}()
	s.addSyncPusher(assignedLifecycleUuid, syncChan)
	s.lg.Debug("assigned ids")
	s.lg.Debug(len(s.syncPushers))
	payload, err := s.syncConstructor.Construct()
	if err != nil {
		s.lg.Error(err)
		return err
	}

	if err := server.Send(&alertops.SyncRequest{
		LifecycleId: assignedLifecycleUuid,
		SyncId:      payload.SyncId,
		Items: []*alertingv1.PutConfigRequest{
			{
				Key:    payload.ConfigKey,
				Config: payload.Data,
			},
		},
	}); err != nil {
		s.lg.Error(err)
		return err
	}
	s.lg.Debug("sent a sync request")

	connErr := lo.Async(func() error {
		for {
			info, err := server.Recv()
			if err == io.EOF {
				s.lg.Info("remote server gracefully closed connection")
				return err
			}
			if err != nil {
				s.lg.Errorf("unexpected error received from server %s", err)
				return err
			}
			s.lg.Debug("adding remote info")
			s.addInfo(assignedLifecycleUuid, RemoteInfo{
				WhoAmI:        info.Whoami,
				LastApplied:   timestamppb.Now(),
				LastSyncState: info.State,
				LastSyncId:    info.SyncId,
			})
		}
	})
	var mu sync.Mutex //FIXME: might not be necessary
	for {
		select {
		case <-s.ctx.Done():
			s.lg.Info("exiting syncer engine loop, parent context is exiting...")
			return s.ctx.Err()
		case <-server.Context().Done():
			s.lg.Info("exiting syncer engine loop, server context is exiting...")
			return server.Context().Err()
		case err := <-connErr:
			return err
		case syncReq := <-syncChan:
			if syncReq.LifecycleId == assignedLifecycleUuid {
				mu.Lock()
				s.lg.Debugf("sending sync request over stream for %s", assignedLifecycleUuid)
				if err := server.Send(syncReq); err != nil {
					return err
				}
				mu.Unlock()
			}
		}
	}
}

func (s *syncManager) Targets() []string {
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	return lo.Keys(s.syncPushers)
}

func (s *syncManager) Push(payload SyncPayload) {
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	for id := range s.syncPushers {
		s.inputSyncChan <- &alertops.SyncRequest{
			SyncId:      payload.SyncId,
			LifecycleId: id,
			Items: []*alertingv1.PutConfigRequest{
				{
					Key:    payload.ConfigKey,
					Config: payload.Data,
				},
			},
		}
	}
}

func (s *syncManager) PushTarget(target string, payload SyncPayload) {
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	s.lg.Debug("pushing sync to target?")
	for id := range s.syncPushers {
		if id == target {
			s.lg.Debugf("pushing sync to target %s", target)
			s.inputSyncChan <- &alertops.SyncRequest{
				SyncId:      payload.SyncId,
				LifecycleId: id,
				Items: []*alertingv1.PutConfigRequest{
					{
						Key:    payload.ConfigKey,
						Config: payload.Data,
					},
				},
			}
			s.lg.Debug("pushed sync to target")
		}
	}
}

func (s *syncManager) Info() map[string]RemoteInfo {
	res := make(map[string]RemoteInfo)
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	for id, info := range s.remoteInfo {
		res[id] = RemoteInfo{
			WhoAmI:        info.WhoAmI,
			LastApplied:   info.LastApplied,
			LastSyncState: info.LastSyncState,
			LastSyncId:    info.LastSyncId,
		}
	}
	return res
}
