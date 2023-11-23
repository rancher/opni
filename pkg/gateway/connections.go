package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	sync "sync"
	"time"

	"github.com/google/uuid"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/streams"
	"golang.org/x/sync/errgroup"
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

type TrackedInstanceListener interface {
	// Called when a new gateway instance is tracked.
	// The provided context will be canceled when the tracked instance is deleted.
	// Implementations of this method MUST NOT block.
	HandleTrackedInstance(ctx context.Context, instanceInfo *corev1.InstanceInfo)
}

const (
	instancesKey = "__instances"
)

type activeTrackedConnection struct {
	agentId               string
	holder                string
	revision              int64
	timestamp             time.Time
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
	connectionsKv     storage.KeyValueStore
	connectionsLm     storage.LockManager
	logger            *slog.Logger

	listenersMu   sync.Mutex
	connListeners []TrackedConnectionListener

	mu                sync.RWMutex
	activeConnections map[string]*activeTrackedConnection

	whoami string
}

func NewConnectionTracker(
	rootContext context.Context,
	localInstanceInfo *corev1.InstanceInfo,
	connectionsKv storage.KeyValueStore,
	connectionsLm storage.LockManager,
	lg *slog.Logger,
) *ConnectionTracker {
	ct := &ConnectionTracker{
		localInstanceInfo: localInstanceInfo,
		rootContext:       rootContext,
		connectionsKv:     connectionsKv,
		connectionsLm:     connectionsLm,
		logger:            lg,
		activeConnections: make(map[string]*activeTrackedConnection),
		whoami:            uuid.New().String(),
	}
	return ct
}

func NewReadOnlyConnectionTracker(
	rootContext context.Context,
	connectionsKv storage.KeyValueStore,
) *ConnectionTracker {
	return &ConnectionTracker{
		localInstanceInfo: nil,
		rootContext:       rootContext,
		connectionsKv:     connectionsKv,
		logger:            slog.New(slog.NewJSONHandler(io.Discard, nil)),
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

	ct.connListeners = append(ct.connListeners, listener)
	for _, conn := range ct.activeConnections {
		listener.HandleTrackedConnection(conn.trackingContext, conn.agentId, conn.holder, conn.instanceInfo)
	}
}

func (ct *ConnectionTracker) LocalInstanceInfo() *corev1.InstanceInfo {
	return util.ProtoClone(ct.localInstanceInfo)
}

// Starts the connection tracker. This will block until the context is canceled
// and the underlying kv store watcher is closed.
func (ct *ConnectionTracker) Run(ctx context.Context) error {
	// var wg sync.WaitGroup
	eg, ctx := errgroup.WithContext(ctx)

	connectionsWatcher, err := ct.connectionsKv.Watch(ctx, "", storage.WithPrefix(), storage.WithRevision(0))
	if err != nil {
		return err
	}

	eg.Go(func() error {
		for event := range connectionsWatcher {
			ct.mu.Lock()
			var key string
			switch event.EventType {
			case storage.WatchEventPut:
				key = event.Current.Key()
			case storage.WatchEventDelete:
				key = event.Previous.Key()
			}
			if !strings.HasPrefix(key, instancesKey) {
				ct.handleConnEvent(event)
			}
			ct.mu.Unlock()
		}
		return nil
	})

	return eg.Wait()
}

type instanceInfoKeyType struct{}

var instanceInfoKey = instanceInfoKeyType{}

func (ct *ConnectionTracker) connKey(agentId string) string {
	return path.Join(agentId, ct.whoami)
}

func (ct *ConnectionTracker) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		agentId := cluster.StreamAuthorizedID(ss.Context())
		instanceInfo := &corev1.InstanceInfo{
			RelayAddress:      ct.localInstanceInfo.GetRelayAddress(),
			ManagementAddress: ct.localInstanceInfo.GetManagementAddress(),
			GatewayAddress:    ct.localInstanceInfo.GetGatewayAddress(),
			WebAddress:        ct.localInstanceInfo.GetWebAddress(),
		}
		key := agentId
		lock := ct.connectionsLm.NewLock(
			key,
		)
		ct.logger.With("agentId", agentId).Debug("attempting to acquire connection lock")
		if acquired, _, err := lock.TryLock(ss.Context()); !acquired {
			ct.logger.With(
				logger.Err(err),
				"agentId", agentId,
			).Error("failed to acquire lock on agent connection")
			if err == nil {
				return status.Errorf(codes.FailedPrecondition, "already connected to a different gateway instance")
			}
			return status.Errorf(codes.Internal, "failed to acquire lock on agent connection: %v", err)
		}
		ct.logger.With("agentId", agentId).Debug("acquired agent connection lock")

		defer func() {
			ct.logger.With("agentId", agentId).Debug("releasing agent connection lock")
			if err := lock.Unlock(); err != nil {
				ct.logger.With(
					logger.Err(err),
					"agentId", agentId,
				).Error("failed to release lock on agent connection")
			}
		}()

		instanceInfo.Acquired = true
		if err := ct.connectionsKv.Put(ss.Context(), ct.connKey(agentId), util.Must(proto.Marshal(instanceInfo))); err != nil {
			ct.logger.Warn("failed to persist instance info in the connections KV")
		}

		stream := streams.NewServerStreamWithContext(ss)
		stream.Ctx = context.WithValue(stream.Ctx, instanceInfoKey, util.ProtoClone(instanceInfo))
		return handler(srv, stream)
	}
}

func decodeConnKey(key string) (agentId string, leaseId string, ok bool) {
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

func (ct *ConnectionTracker) handleConnUpdate(
	event storage.WatchEvent[storage.KeyRevision[[]byte]],
	agentId string,
	conn *activeTrackedConnection,
	info *corev1.InstanceInfo,
) {
	key := event.Current.Key()
	lg := ct.logger.With("key", key, "agentId", agentId)
	lg.Debug(fmt.Sprintf("found active connection for %s", agentId))
	if !info.GetAcquired() {
		// a different instance is only attempting to acquire the lock,
		// ignore the event
		ct.logger.With("agent", agentId, "instance", info.GetRelayAddress()).Debug("tracked connection is still being acquired...")
		return
	}
	// a different instance has acquired the lock, invalidate
	// the current tracked connection
	ct.logger.With("agentId", agentId).Debug("tracked connection invalidated")
	conn.cancelTrackingContext()
	delete(ct.activeConnections, agentId)
}

func (ct *ConnectionTracker) handleConnCreate(
	event storage.WatchEvent[storage.KeyRevision[[]byte]],
	agentId, holder string,
	info *corev1.InstanceInfo,
) {
	if !info.GetAcquired() {
		return // ignore unacquired connections
	}
	if ct.IsLocalInstance(info) {
		ct.logger.With("agentId", agentId).Debug("tracking new connection (local)")
	} else {
		ct.logger.With("agentId", agentId).Debug("tracking new connection")
	}
	ctx, cancel := context.WithCancel(ct.rootContext)
	conn := &activeTrackedConnection{
		agentId:               agentId,
		holder:                holder,
		revision:              event.Current.Revision(),
		instanceInfo:          info,
		trackingContext:       ctx,
		cancelTrackingContext: cancel,
	}
	ct.activeConnections[agentId] = conn
	for _, listener := range ct.connListeners {
		listener.HandleTrackedConnection(ctx, agentId, holder, info)
	}
}

func (ct *ConnectionTracker) handleConnEvent(event storage.WatchEvent[storage.KeyRevision[[]byte]]) {
	key := event.Current.Key()
	agentId, holder, ok := decodeConnKey(key)
	if !ok {
		ct.logger.With("event", key).Warn("event cannot be indexed in the form : <agentId>/<leaseId>")
		return
	}
	switch event.EventType {
	case storage.WatchEventPut:
		ct.listenersMu.Lock()
		defer ct.listenersMu.Unlock()
		lg := ct.logger.With(
			"agentId", agentId,
			"key", event.Current.Key(),
		)
		instanceInfo := sync.OnceValues(func() (*corev1.InstanceInfo, error) {
			var info corev1.InstanceInfo
			if err := proto.Unmarshal(event.Current.Value(), &info); err != nil {
				return nil, err
			}
			return &info, nil
		})
		if conn, ok := ct.activeConnections[agentId]; ok {
			if conn.holder == holder {
				// key was updated, not a new connection
				lg.Debug("tracked connection updated")
				return
			}
			info, err := instanceInfo()
			if err != nil {
				lg.With(logger.Err(err)).Error("failed to unmarshal instance info")
				return
			}
			ct.handleConnUpdate(
				event,
				agentId,
				conn,
				info,
			)
		}
		info, err := instanceInfo()
		if err != nil {
			lg.With(logger.Err(err)).Error("failed to unmarshal instance info")
			return
		}
		ct.handleConnCreate(
			event,
			agentId,
			holder,
			info,
		)

	case storage.WatchEventDelete:
		// make sure the previous revision of the deleted key is the same as the
		// revision of the tracked connection.
		lg := ct.logger.With("key", event.Previous.Key(), "rev", event.Previous.Revision())
		if conn, ok := ct.activeConnections[agentId]; ok {
			if conn.holder != holder {
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
			lg.With("key", event.Previous.Key()).Debug("ignoring untracked key deletion event")
		}
	}
}

func (ct *ConnectionTracker) IsLocalInstance(instanceInfo *corev1.InstanceInfo) bool {
	if ct.localInstanceInfo == nil {
		return false
	}

	// Note: this assumes that if the relay address is the same, then the
	// management address is also the same. If we ever decide to allow
	// standalone management servers, this will need to be updated.
	return instanceInfo.GetRelayAddress() == ct.localInstanceInfo.GetRelayAddress()
}

func (ct *ConnectionTracker) IsTrackedLocal(agentId string) bool {
	if ct.localInstanceInfo == nil {
		return false
	}

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
