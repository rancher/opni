package gateway

import (
	"context"
	"strings"
	sync "sync"

	"github.com/rancher/opni/pkg/agent"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
)

type TrackedConnectionListener interface {
	// Called when a new agent connection to any gateway instance is tracked.
	// The provided context will be canceled when the tracked connection is deleted.
	// Implementations of this method MUST NOT block.
	HandleTrackedConnection(ctx context.Context, agentId string, prefix string, instanceInfo *corev1.InstanceInfo)
}

type activeTrackedConnection struct {
	agentId               string
	prefix                string
	instanceInfo          *corev1.InstanceInfo
	trackingContext       context.Context
	cancelTrackingContext context.CancelFunc
}

// ConnectionTracker is a locally replicated view into the current state of
// all live gateway/agent connections. It can be used to determine which
// agents are currently connected, and to which gateway instance.
// All gateway instances track the same state and update in real time at the
// speed of the underlying storage backend.
type ConnectionTracker struct {
	localInstanceInfo *corev1.InstanceInfo
	rootContext       context.Context
	kv                storage.KeyValueStore
	lm                storage.LockManager
	logger            *zap.SugaredLogger

	listenersMu sync.Mutex
	listeners   []TrackedConnectionListener

	mu                sync.RWMutex
	activeConnections map[string]*activeTrackedConnection
}

func NewConnectionTracker(
	rootContext context.Context,
	localInstanceInfo *corev1.InstanceInfo,
	kv storage.KeyValueStore,
	lm storage.LockManager,
	lg *zap.SugaredLogger,
) *ConnectionTracker {
	return &ConnectionTracker{
		localInstanceInfo: localInstanceInfo,
		rootContext:       rootContext,
		kv:                kv,
		lm:                lm,
		logger:            lg,
		activeConnections: make(map[string]*activeTrackedConnection),
	}
}

type AgentInstanceInfo struct {
	Instance *corev1.InstanceInfo
	IsLocal  bool
}

func (ct *ConnectionTracker) createTrackedConn(agentId string, prefix string, instanceInfo *corev1.InstanceInfo) *activeTrackedConnection {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	ctx, cancel := context.WithCancel(ct.rootContext)
	conn := &activeTrackedConnection{
		agentId:               agentId,
		prefix:                prefix,
		instanceInfo:          instanceInfo,
		trackingContext:       ctx,
		cancelTrackingContext: cancel,
	}
	for _, listener := range ct.listeners {
		listener.HandleTrackedConnection(ctx, agentId, prefix, instanceInfo)
	}
	return conn
}

func (ct *ConnectionTracker) Lookup(agentId string) (AgentInstanceInfo, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if conn, ok := ct.activeConnections[agentId]; ok {
		return AgentInstanceInfo{}, false
	} else {
		info := util.ProtoClone(conn.instanceInfo)
		return AgentInstanceInfo{
			Instance: info,
			IsLocal:  ct.isLocal(info),
		}, true
	}
}

func (ct *ConnectionTracker) AddTrackedConnectionListener(listener TrackedConnectionListener) {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	ct.listeners = append(ct.listeners, listener)
	for _, conn := range ct.activeConnections {
		// if !ct.isLocal(conn.instanceInfo) {
		listener.HandleTrackedConnection(conn.trackingContext, conn.agentId, conn.prefix, conn.instanceInfo)
		// }
	}
}

func (ct *ConnectionTracker) RemoveRemoteConnectionListener(listener TrackedConnectionListener) {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	for i, l := range ct.listeners {
		if l == listener {
			ct.listeners = append(ct.listeners[:i], ct.listeners[i+1:]...)
			return
		}
	}
}

func (ct *ConnectionTracker) LocalInstanceInfo() *corev1.InstanceInfo {
	return util.ProtoClone(ct.localInstanceInfo)
}

// Starts the connection tracker. This will block until the context is canceled
// and the underlying kv store watcher is closed.
func (ct *ConnectionTracker) Run(ctx context.Context) error {
	watcher, err := ct.kv.Watch(ctx, "", storage.WithPrefix())
	if err != nil {
		return err
	}
	for event := range watcher {
		ct.mu.Lock()
		ct.handleEventLocked(event)
		ct.mu.Unlock()
	}
	return nil
}

func (ct *ConnectionTracker) HandleAgentConnection(ctx context.Context, clientSet agent.ClientSet) {
	agentId := cluster.StreamAuthorizedID(ctx)
	locker := ct.lm.Locker(agentId, lock.WithAcquireContext(ctx),
		lock.WithInitialValue(string(util.Must(protojson.Marshal(ct.localInstanceInfo)))))
	if err := locker.Lock(); err != nil {
		ct.logger.With(
			zap.Error(err),
			"agentId", agentId,
		).Error("failed to acquire lock on agent connection")
		return
	}
	defer func() {
		if err := locker.Unlock(); err != nil {
			ct.logger.With(
				zap.Error(err),
				"agentId", agentId,
			).Error("failed to release lock on agent connection")
		}
	}()

	ct.logger.Debug("acquired connection lock for agent: ", agentId)
	<-ctx.Done()
	ct.logger.Debug("releasing connection lock for agent: ", agentId)
}

func (ct *ConnectionTracker) handleEventLocked(event storage.WatchEvent[storage.KeyRevision[[]byte]]) {
	decodeKey := func(key string) (agentId string, leaseId string, ok bool) {
		parts := strings.Split(key, "/")

		if len(parts) != 2 {
			// only handle keys of the form <agentId>/<leaseId>
			return "", "", false
		}

		agentId = parts[0]
		leaseId = parts[1]
		ok = true
		return
	}

	switch event.EventType {
	case storage.WatchEventPut:
		agentId, _, ok := decodeKey(event.Current.Key())
		if !ok {
			return
		}
		lg := ct.logger.With("agentId", agentId)
		instanceInfo := &corev1.InstanceInfo{}
		if err := protojson.Unmarshal(event.Current.Value(), instanceInfo); err != nil {
			lg.With(
				zap.Error(err),
				"key", event.Current.Key(),
			).Error("malformed instance info was written by controlling gateway, ignoring")
			return
		}
		if ct.isLocal(instanceInfo) {
			ct.logger.Debug("tracking new connection (local): ", agentId)
		} else {
			ct.logger.Debug("tracking new connection: ", agentId)
		}
		ct.activeConnections[agentId] = ct.createTrackedConn(agentId, event.Current.Key(), instanceInfo)
	case storage.WatchEventDelete:
		agentId, _, ok := decodeKey(event.Previous.Key())
		if !ok {
			return
		}
		lg := ct.logger.With("agentId", agentId)
		prev := &corev1.InstanceInfo{}
		if err := protojson.Unmarshal(event.Previous.Value(), prev); err == nil {
			if ct.isLocal(prev) {
				lg.Debug("tracked connection deleted (local)")
			} else {
				lg.With(
					"prevAddress", prev.GetRelayAddress(),
				).Debug("tracked connection deleted")
			}
		} else {
			lg.With(
				"prevAddress", "(unknown)",
			).Debug("tracked connection deleted")
		}
		if conn, ok := ct.activeConnections[agentId]; ok {
			conn.cancelTrackingContext()
		}
		delete(ct.activeConnections, agentId)
	}
}

func (ct *ConnectionTracker) isLocal(instanceInfo *corev1.InstanceInfo) bool {
	return instanceInfo.GetRelayAddress() == ct.localInstanceInfo.GetRelayAddress()
}

func (ct *ConnectionTracker) IsLocallyTracked(agentId string) bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	info, ok := ct.activeConnections[agentId]
	if ok && ct.isLocal(info.instanceInfo) {
		return true
	}
	return false
}

func (ct *ConnectionTracker) IsRemotelyTracked(agentId string) bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	info, ok := ct.activeConnections[agentId]
	if ok && !ct.isLocal(info.instanceInfo) {
		return true
	}
	return false
}
