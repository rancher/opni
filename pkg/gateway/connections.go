package gateway

import (
	"context"
	"encoding/base64"
	"strings"
	sync "sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/streams"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type TrackedConnectionListener interface {
	// Called when a new agent connection to any gateway instance is tracked.
	// The provided context will be canceled when the tracked connection is deleted.
	// Implementations of this method MUST NOT block.
	HandleTrackedConnection(ctx context.Context, agentId string, leaseId string, instanceInfo *corev1.InstanceInfo)
}

type activeTrackedConnection struct {
	agentId               string
	leaseId               string
	revision              int64
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

func (ct *ConnectionTracker) Lookup(agentId string) *corev1.InstanceInfo {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if conn, ok := ct.activeConnections[agentId]; ok {
		return conn.instanceInfo
	}
	return nil
}

func (ct *ConnectionTracker) ListActiveConnections() []string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	agents := make([]string, 0, len(ct.activeConnections))
	for agentId := range ct.activeConnections {
		agents = append(agents, agentId)
	}
	return agents
}

func (ct *ConnectionTracker) AddTrackedConnectionListener(listener TrackedConnectionListener) {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	ct.listeners = append(ct.listeners, listener)
	for _, conn := range ct.activeConnections {
		listener.HandleTrackedConnection(conn.trackingContext, conn.agentId, conn.leaseId, conn.instanceInfo)
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
	watcher, err := ct.kv.Watch(ctx, "", storage.WithPrefix(), storage.WithRevision(0))
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

type instanceInfoKeyType struct{}

var instanceInfoKey = instanceInfoKeyType{}

func (ct *ConnectionTracker) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		agentId := cluster.StreamAuthorizedID(ss.Context())
		instanceInfo := &corev1.InstanceInfo{
			RelayAddress:      ct.localInstanceInfo.GetRelayAddress(),
			ManagementAddress: ct.localInstanceInfo.GetManagementAddress(),
		}
		locker := ct.lm.Locker(agentId,
			lock.WithAcquireContext(ss.Context()),
			lock.WithInitialValue(base64.StdEncoding.EncodeToString(util.Must(proto.Marshal(instanceInfo)))),
		)
		ct.logger.Debug("attempting to acquire connection lock for agent: ", agentId)
		if acquired, err := locker.TryLock(); !acquired {
			ct.logger.With(
				zap.Error(err),
				"agentId", agentId,
			).Error("failed to acquire lock on agent connection")
			if err == nil {
				return status.Errorf(codes.FailedPrecondition, "already connected to a different gateway instance")
			}
			return status.Errorf(codes.Internal, "failed to acquire lock on agent connection: %v", err)
		}
		ct.logger.Debug("acquired connection lock for agent: ", agentId)

		defer func() {
			ct.logger.Debug("releasing connection lock for agent: ", agentId)
			if err := locker.Unlock(); err != nil {
				ct.logger.With(
					zap.Error(err),
					"agentId", agentId,
				).Error("failed to release lock on agent connection")
			}
		}()

		instanceInfo.Acquired = true
		ct.kv.Put(ss.Context(), locker.Key(), util.Must(proto.Marshal(instanceInfo)))

		stream := streams.NewServerStreamWithContext(ss)
		stream.Ctx = context.WithValue(stream.Ctx, instanceInfoKey, util.ProtoClone(instanceInfo))
		return handler(srv, stream)
	}
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
		agentId, leaseId, ok := decodeKey(event.Current.Key())
		if !ok {
			return
		}
		ct.listenersMu.Lock()
		defer ct.listenersMu.Unlock()
		instanceInfo := sync.OnceValues(func() (*corev1.InstanceInfo, error) {
			var info corev1.InstanceInfo
			if err := proto.Unmarshal(event.Current.Value(), &info); err != nil {
				return nil, err
			}
			return &info, nil
		})
		lg := ct.logger.With(
			"agentId", agentId,
			"key", event.Current.Key(),
		)
		if conn, ok := ct.activeConnections[agentId]; ok {
			if conn.leaseId == leaseId {
				// key was updated, not a new connection
				lg.Debug("tracked connection updated")
				return
			}
			info, err := instanceInfo()
			if err != nil {
				lg.With(zap.Error(err)).Error("failed to unmarshal instance info")
				return
			}
			if !info.GetAcquired() {
				// a different instance is only attempting to acquire the lock,
				// ignore the event
				ct.logger.Debugf("observed lock attempt for agent %s from instance %s", agentId, info.GetRelayAddress())
				return
			}
			// a different instance has acquired the lock, invalidate
			// the current tracked connection
			ct.logger.Debug("tracked connection invalidated: ", agentId)
			conn.cancelTrackingContext()
			delete(ct.activeConnections, agentId)
		}
		info, err := instanceInfo()
		if err != nil {
			lg.With(zap.Error(err)).Error("failed to unmarshal instance info")
			return
		}
		if !info.GetAcquired() {
			return // ignore unacquired connections
		}
		if ct.IsLocalInstance(info) {
			ct.logger.Debug("tracking new connection (local): ", agentId)
		} else {
			ct.logger.Debug("tracking new connection: ", agentId)
		}
		ctx, cancel := context.WithCancel(ct.rootContext)
		conn := &activeTrackedConnection{
			agentId:               agentId,
			leaseId:               leaseId,
			revision:              event.Current.Revision(),
			instanceInfo:          info,
			trackingContext:       ctx,
			cancelTrackingContext: cancel,
		}
		ct.activeConnections[agentId] = conn
		for _, listener := range ct.listeners {
			listener.HandleTrackedConnection(ctx, agentId, leaseId, info)
		}
	case storage.WatchEventDelete:
		agentId, leaseId, ok := decodeKey(event.Previous.Key())
		if !ok {
			return
		}
		// make sure the previous revision of the deleted key is the same as the
		// revision of the tracked connection.
		lg := ct.logger.With("key", event.Previous.Key(), "rev", event.Previous.Revision())
		if conn, ok := ct.activeConnections[agentId]; ok {
			if conn.leaseId != leaseId {
				// likely an expired lock attempt, ignore

				return
			}
			prev := &corev1.InstanceInfo{}
			if err := proto.Unmarshal(event.Previous.Value(), prev); err == nil {
				if ct.IsLocalInstance(prev) {
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
			conn.cancelTrackingContext()

			delete(ct.activeConnections, agentId)
		} else {
			lg.Debug("ignoring untracked key deletion event: ", event.Previous.Key())
		}
	}
}

func (ct *ConnectionTracker) IsLocalInstance(instanceInfo *corev1.InstanceInfo) bool {
	// Note: this assumes that if the relay address is the same, then the
	// management address is also the same. If we ever decide to allow
	// standalone management servers, this will need to be updated.
	return instanceInfo.GetRelayAddress() == ct.localInstanceInfo.GetRelayAddress()
}

func (ct *ConnectionTracker) IsTrackedLocal(agentId string) bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	info, ok := ct.activeConnections[agentId]
	if ok && ct.IsLocalInstance(info.instanceInfo) {
		return true
	}
	return false
}

func (ct *ConnectionTracker) IsTrackedRemote(agentId string) bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	info, ok := ct.activeConnections[agentId]
	if ok && !ct.IsLocalInstance(info.instanceInfo) {
		return true
	}
	return false
}
