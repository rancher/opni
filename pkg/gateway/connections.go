package gateway

import (
	"context"
	"encoding/base64"
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

type activeTrackedInstance struct {
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

	listenersMu       sync.Mutex
	connListeners     []TrackedConnectionListener
	instanceListeners []TrackedInstanceListener

	mu                sync.RWMutex
	activeConnections map[string]*activeTrackedConnection
	activeInstances   map[string]*activeTrackedInstance

	whoami string

	leaderTracker *ConnectionTrackerLeader
}

func NewConnectionTracker(
	rootContext context.Context,
	localInstanceInfo *corev1.InstanceInfo,
	connectionsKv, trackerKv storage.KeyValueStore,
	connectionsLm, trackerLm storage.LockManager,
	lg *slog.Logger,
) *ConnectionTracker {
	ct := &ConnectionTracker{
		localInstanceInfo: localInstanceInfo,
		rootContext:       rootContext,
		connectionsKv:     connectionsKv,
		connectionsLm:     connectionsLm,
		logger:            lg,
		activeConnections: make(map[string]*activeTrackedConnection),
		activeInstances:   make(map[string]*activeTrackedInstance),
		whoami:            uuid.New().String(),
	}
	le := newConnectionTrackerLeader(
		trackerLm,
		trackerKv,
		ct.localInstanceInfo,
		ct.whoami,
		ct.logger,
	)
	ct.leaderTracker = le
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
		activeInstances:   make(map[string]*activeTrackedInstance),
		leaderTracker:     nil,
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

func (ct *ConnectionTracker) AddTrackedInstanceListener(listener TrackedInstanceListener) {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	ct.instanceListeners = append(ct.instanceListeners, listener)
	for _, instance := range ct.activeInstances {
		listener.HandleTrackedInstance(instance.trackingContext, instance.instanceInfo)
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

	if ct.localInstanceInfo != nil {
		eg.Go(func() error {
			return ct.leaderTracker.lockInstance(ctx)
		})

	}
	connectionsWatcher, err := ct.connectionsKv.Watch(ctx, "", storage.WithPrefix(), storage.WithRevision(0))
	if err != nil {
		return err
	}

	trackerWatcher, err := ct.leaderTracker.leaderInfo.Watch(ctx, "", storage.WithPrefix(), storage.WithRevision(0))
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

	eg.Go(func() error {
		for event := range trackerWatcher {
			ct.mu.Lock()
			var key string
			switch event.EventType {
			case storage.WatchEventPut:
				key = event.Current.Key()
			case storage.WatchEventDelete:
				key = event.Previous.Key()
			}
			if strings.HasPrefix(key, instancesKey) {
				ct.handleInstanceEvent(event)
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

func (ct *ConnectionTracker) handleInstanceEvent(event storage.WatchEvent[storage.KeyRevision[[]byte]]) {
	switch event.EventType {
	case storage.WatchEventPut:
		leaseId := event.Current.Key()[len(instancesKey)+1:]
		instanceInfo := &corev1.InstanceInfo{}
		if err := proto.Unmarshal(event.Current.Value(), instanceInfo); err != nil {
			ct.logger.With(logger.Err(err)).Error("failed to unmarshal instance info")
			return
		}
		if _, ok := ct.activeInstances[leaseId]; ok {
			return
		}
		ct.logger.With("leaseId", leaseId).Debug("tracking new instance")
		trackingCtx, cancel := context.WithCancel(ct.rootContext)
		instance := &activeTrackedInstance{
			instanceInfo:          instanceInfo,
			trackingContext:       trackingCtx,
			cancelTrackingContext: cancel,
		}
		ct.activeInstances[leaseId] = instance
		for _, listener := range ct.instanceListeners {
			listener.HandleTrackedInstance(instance.trackingContext, instance.instanceInfo)
		}
	case storage.WatchEventDelete:
		leaseId := event.Previous.Key()[len(instancesKey)+1:]
		instanceInfo := &corev1.InstanceInfo{}
		if err := proto.Unmarshal(event.Previous.Value(), instanceInfo); err != nil {
			ct.logger.With(logger.Err(err)).Error("failed to unmarshal instance info")
			return
		}
		if instance, ok := ct.activeInstances[leaseId]; ok {
			ct.logger.With("leaseId", leaseId).Debug("tracked instance deleted")
			instance.cancelTrackingContext()
			delete(ct.activeInstances, leaseId)
		} else {
			ct.logger.With("leaseId", leaseId).Debug("ignoring untracked instance deletion event")
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

// holds the remote storage backend mechanisms to determine
// which instance should virtually report what the tracked instances are
type ConnectionTrackerLeader struct {
	lm         storage.LockManager
	leaderInfo storage.KeyValueStore
	lg         *slog.Logger

	localInstanceInfo *corev1.InstanceInfo

	whoami string
}

func newConnectionTrackerLeader(
	lm storage.LockManager,
	leaderInfo storage.KeyValueStore,
	instanceInfo *corev1.InstanceInfo,
	whoami string,
	lg *slog.Logger,
) *ConnectionTrackerLeader {
	return &ConnectionTrackerLeader{
		lm:                lm,
		leaderInfo:        leaderInfo,
		localInstanceInfo: instanceInfo,
		whoami:            whoami,
		lg:                lg,
	}
}

// repeatedly attempts to acquire the connection lock for the unique key
// "$instances". This key can be used to track the set of all instances
// that are currently running.
func (ct *ConnectionTrackerLeader) lockInstance(ctx context.Context) error {
	instanceInfo := &corev1.InstanceInfo{
		RelayAddress:      ct.localInstanceInfo.GetRelayAddress(),
		ManagementAddress: ct.localInstanceInfo.GetManagementAddress(),
		GatewayAddress:    ct.localInstanceInfo.GetGatewayAddress(),
		WebAddress:        ct.localInstanceInfo.GetWebAddress(),
	}
	instanceVal := base64.StdEncoding.EncodeToString(util.Must(proto.Marshal(instanceInfo)))
	lock := ct.lm.NewLock(instancesKey)
RETRY:
	for ctx.Err() == nil {
		// lock.WithInitialValue(base64.StdEncoding.EncodeToString(util.Must(proto.Marshal(instanceInfo)))))
		expired, err := lock.Lock(ctx)
		if err != nil {
			ct.lg.With(logger.Err(err)).Warn("failed to lock leader info")
			goto RETRY
		}

		if err := ct.putLeader(ctx, expired, []byte(instanceVal)); err != nil {
			ct.lg.With(logger.Err(err)).Error("failed to put leader info after leader was acquired...")
			lock.Unlock()
			goto RETRY
		}
		select {
		case <-expired:
			lock.Unlock()
			goto RETRY
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}

func (ct *ConnectionTrackerLeader) putLeader(
	ctx context.Context,
	expired chan struct{},
	instanceVal []byte,
) error {
	for ctx.Err() == nil {
		select {
		case <-expired:
			return fmt.Errorf("lock expired")
		default:
			if err := ct.leaderInfo.Put(ctx, ct.leaderKey(), instanceVal); err == nil {
				return nil
			} else {
				ct.lg.Warn(fmt.Sprintf("failed to put leader info in KV : %s", err))
			}
		}
	}
	return ctx.Err()
}

func (ct *ConnectionTrackerLeader) leaderKey() string {
	return path.Join(instancesKey, ct.whoami)
}
