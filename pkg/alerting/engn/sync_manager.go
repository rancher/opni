package engn

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var DefaultSyncTimeout = 5 * time.Second

type SyncConstructor interface {
	Construct() (SyncPayload, error)
	GetHash() (string, error)
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

type RemoteInfoBroadcaster struct {
	*sync.Cond
	info RemoteInfo
}

type syncManager struct {
	alertops.UnsafeConfigReconcilerServer

	ctx context.Context
	lg  *zap.SugaredLogger

	// anything that updates the sync state should use this
	syncMu          sync.RWMutex
	syncConstructor SyncConstructor

	payloadMu        sync.RWMutex
	connectedSyncers map[string]*RemoteInfoBroadcaster
	curSyncPayload   *SyncPayload
	// inputSyncChan   chan *alertops.SyncRequest
	// syncPushers     map[string]*buffer.RingBuffer[*alertops.SyncRequest]
}

func NewSyncManager(
	ctx context.Context,
	constructor SyncConstructor,
) SyncManager {
	return &syncManager{
		ctx:              ctx,
		lg:               logger.NewPluginLogger().Named("alerting-syncer"),
		syncConstructor:  constructor,
		connectedSyncers: make(map[string]*RemoteInfoBroadcaster),
	}
}

var _ SyncManager = (*syncManager)(nil)

func (s *syncManager) setSyncPayload(payload SyncPayload) {
	s.payloadMu.Lock()
	defer s.payloadMu.Unlock()
	s.curSyncPayload = &payload
}

func (s *syncManager) getSyncPayload() *SyncPayload {
	s.payloadMu.RLock()
	defer s.payloadMu.RUnlock()
	return s.curSyncPayload
}

func (s *syncManager) addConnectedSyncer(id string, cond *sync.Cond) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.connectedSyncers[id] = &RemoteInfoBroadcaster{
		Cond: cond,
	}
}

func (s *syncManager) removeConnectedSyncer(id string) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if cond, ok := s.connectedSyncers[id]; ok {
		cond.Signal()
	}
	delete(s.connectedSyncers, id)
}

func (s *syncManager) setInfo(id string, info RemoteInfo) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if bi, ok := s.connectedSyncers[id]; ok {
		bi.info = info
	}
}

func (s *syncManager) getRemoteInfo(id string) (RemoteInfo, bool) {
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	ret, ok := s.connectedSyncers[id]
	if !ok {
		s.lg.Debugf("COULD NOT FIND ID %s", id)
		return ret.info, ok
	}
	return RemoteInfo{
		WhoAmI:        ret.info.WhoAmI,
		LastApplied:   ret.info.LastApplied,
		LastSyncState: ret.info.LastSyncState,
		LastSyncId:    ret.info.LastSyncId,
	}, true
}

func (s *syncManager) SyncConfig(server alertops.ConfigReconciler_SyncConfigServer) error {
	assignedLifecycleUuid := uuid.New().String()
	var serverMu sync.Mutex //FIXME: might not be necessary
	cond := sync.NewCond(&serverMu)
	lg := s.lg.With("assignedId", assignedLifecycleUuid)
	s.addConnectedSyncer(assignedLifecycleUuid, cond)
	lg.Debug("connecting client ...")
	defer func() {
		s.removeConnectedSyncer(assignedLifecycleUuid)
	}()
	lg.With("connected clients", len(s.connectedSyncers)).Debug("connected client")
	payload, err := s.syncConstructor.Construct()
	if err != nil {
		s.lg.Error(err)
		return err
	}
	lg.Debug("sending initial sync request")
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
	s.lg.Debug("sent an initial sync request")

	connErr := lo.Async(func() error {
		for {
			info, err := server.Recv()

			if err != nil {
				cond.L.Lock()
				cond.Signal()
				cond.L.Unlock()
				lg.Errorf("unexpected error received from server %s", err)
				return err
			}
			lg.Debugf("updating remote info %s, %s, %s", info.Whoami, info.SyncId, info.State.String())
			s.setInfo(assignedLifecycleUuid, RemoteInfo{
				WhoAmI:        info.Whoami,
				LastApplied:   timestamppb.Now(),
				LastSyncState: info.State,
				LastSyncId:    info.SyncId,
			})
			lg.Debugf("updated info %s", s.connectedSyncers[assignedLifecycleUuid].info)
		}
	})

	returnErr := lo.Async(
		func() error {
			for {
			WAITFORSYNC:
				lg.Debug("waiting")
				cond.L.Lock()
				cond.Wait()
				lg.Debug("waking")
				cond.L.Unlock()
				payload := s.getSyncPayload()
				select {
				case <-s.ctx.Done():
					lg.Info("exiting syncer engine loop, parent context is exiting...")
					return s.ctx.Err()
				case <-server.Context().Done():
					lg.Info("exiting syncer engine loop, server context is exiting...")
					return server.Context().Err()
				case err := <-connErr:
					lg.Errorf("exiting syncer engine loop, serving stream error: %s", err)
					return err
				default:
					lg.Debug("getting remote info")
					remoteInfo, ok := s.getRemoteInfo(assignedLifecycleUuid)
					lg.Debug("got remote info")
					if !ok {
						s.lg.Debug("remote info not found, assume no sync has been applieddd")
						server.Send(&alertops.SyncRequest{
							LifecycleId: assignedLifecycleUuid,
							SyncId:      payload.SyncId,
							Items: []*alertingv1.PutConfigRequest{
								{
									Key:    payload.ConfigKey,
									Config: payload.Data,
								},
							},
						})
						goto WAITFORSYNC
					}
					if remoteInfo.LastSyncId != payload.SyncId {
						lg.Debug("out of date")
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
							return err
						}
						goto WAITFORSYNC
					}
					if remoteInfo.LastSyncState != alertops.SyncState_Synced {
						lg.Debug("sync error, send again")
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
							return err
						}
						goto WAITFORSYNC
					}
				}
			}
		},
	)
	return <-returnErr
}

func (s *syncManager) Targets() []string {
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	return lo.Keys(s.connectedSyncers)
}

func (s *syncManager) Push(payload SyncPayload) {
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	s.lg.With("syncId", payload.SyncId, "connectedClients", len(s.connectedSyncers)).Debug("broadcasting sync...")
	s.setSyncPayload(payload)
	for id, cond := range s.connectedSyncers {
		s.lg.With("id", id, "syncId", payload.SyncId).Debug("signaling sync")
		cond.L.Lock()
		cond.Signal()
		cond.L.Unlock()
	}
	s.lg.With("syncId", payload.SyncId, "connectedClients", len(s.connectedSyncers)).Debug("broadcasted sync")
}

func (s *syncManager) PushTarget(target string, payload SyncPayload) {
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	s.setSyncPayload(payload)
	for id, cond := range s.connectedSyncers {
		if id == target {
			s.lg.With("syncId", payload.SyncId, "target", target).Debug("pushing sync to target")
			cond.Broadcast()
		}
	}
}

func (s *syncManager) Info() map[string]RemoteInfo {
	res := make(map[string]RemoteInfo)
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	for id, conn := range s.connectedSyncers {
		s.lg.Debug("get %s %s", id, conn.info)
		res[id] = RemoteInfo{
			WhoAmI:        conn.info.WhoAmI,
			LastApplied:   conn.info.LastApplied,
			LastSyncState: conn.info.LastSyncState,
			LastSyncId:    conn.info.LastSyncId,
		}
	}
	return res
}
