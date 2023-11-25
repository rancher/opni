package gateway

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	sync "sync"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/config/reactive"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/streams"
	"github.com/rancher/opni/pkg/versions"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protopath"
)

type TrackedConnectionListener interface {
	// Called when a new agent connection to any gateway instance is tracked.
	// The provided context will be canceled when the tracked connection is deleted.
	// If a tracked connection is updated, this method will be called again with
	// the same context, agentId, and leaseId, but updated instanceInfo.
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
	instancesKey = "$instances"
)

type activeTrackedConnection struct {
	agentId               string
	leaseId               string
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
	rootContext context.Context
	kv          storage.KeyValueStore
	lm          storage.LockManager
	mgr         *configv1.GatewayConfigManager
	logger      *slog.Logger

	listenersMu       sync.Mutex
	connListeners     []TrackedConnectionListener
	instanceListeners []TrackedInstanceListener

	mu                sync.RWMutex
	activeConnections map[string]*activeTrackedConnection
	activeInstances   map[string]*activeTrackedInstance

	localInstanceInfoMu sync.Mutex
	localInstanceInfo   *corev1.InstanceInfo
	// relayConf           reactive.Reactive[*configv1.RelayServerSpec]
	// managementConf      reactive.Reactive[*configv1.ManagementServerSpec]
	// dashboardConf       reactive.Reactive[*configv1.DashboardServerSpec]
}

func NewConnectionTracker(
	rootContext context.Context,
	mgr *configv1.GatewayConfigManager,
	kv storage.KeyValueStore,
	lm storage.LockManager,
	lg *slog.Logger,
) *ConnectionTracker {
	hostname, _ := os.Hostname()
	ct := &ConnectionTracker{
		localInstanceInfo: &corev1.InstanceInfo{
			Annotations: map[string]string{
				"hostname": hostname,
				"pid":      fmt.Sprint(os.Getpid()),
				"version":  versions.Version,
			},
		},
		rootContext:       rootContext,
		mgr:               mgr,
		kv:                kv,
		lm:                lm,
		logger:            lg,
		activeConnections: make(map[string]*activeTrackedConnection),
		activeInstances:   make(map[string]*activeTrackedInstance),
	}
	return ct
}

func NewReadOnlyConnectionTracker(
	rootContext context.Context,
	kv storage.KeyValueStore,
) *ConnectionTracker {
	return &ConnectionTracker{
		localInstanceInfo: nil,
		rootContext:       rootContext,
		kv:                kv,
		logger:            slog.New(slog.NewJSONHandler(io.Discard, nil)),
		activeConnections: make(map[string]*activeTrackedConnection),
		activeInstances:   make(map[string]*activeTrackedInstance),
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
		listener.HandleTrackedConnection(conn.trackingContext, conn.agentId, conn.leaseId, conn.instanceInfo)
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
	ct.localInstanceInfoMu.Lock()
	defer ct.localInstanceInfoMu.Unlock()
	return util.ProtoClone(ct.localInstanceInfo)
}

// Starts the connection tracker. This will block until the context is canceled
// and the underlying kv store watcher is closed.
func (ct *ConnectionTracker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	if ct.LocalInstanceInfo() != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ct.lockInstance(ctx)
		}()
	}
	watcher, err := ct.kv.Watch(ctx, "", storage.WithPrefix(), storage.WithRevision(0))
	if err != nil {
		return err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range watcher {
			ct.mu.Lock()
			var key string
			switch event.EventType {
			case storage.WatchEventPut:
				key = event.Current.Key()
			case storage.WatchEventDelete:
				key = event.Previous.Key()
			}
			if strings.HasPrefix(key, instancesKey) {
				ct.handleInstanceEventLocked(event)
			} else {
				ct.handleConnectionEventLocked(event)
			}
			ct.mu.Unlock()
		}
	}()
	wg.Wait()
	return nil
}

// repeatedly attempts to acquire the connection lock for the unique key
// "$instances". This key can be used to track the set of all instances
// that are currently running.
func (ct *ConnectionTracker) lockInstance(ctx context.Context) {
	ctx, ca := context.WithCancel(ctx)
	defer ca()

	locker := ct.lm.Locker(instancesKey, lock.WithAcquireContext(ctx),
		lock.WithInitialValue(base64.StdEncoding.EncodeToString(util.Must(proto.Marshal(ct.LocalInstanceInfo())))))

	updateInstanceInfo, cancel := lo.NewDebounce(100*time.Millisecond, func() {
		ct.kv.Put(ctx, locker.Key(), util.Must(proto.Marshal(ct.LocalInstanceInfo())))
	})
	defer cancel()

	listenOnce := sync.OnceFunc(func() {
		relay := reactive.Message[*configv1.RelayServerSpec](ct.mgr.Reactive(protopath.Path(configv1.ProtoPath().Relay())))
		mgmt := reactive.Message[*configv1.ManagementServerSpec](ct.mgr.Reactive(protopath.Path(configv1.ProtoPath().Management())))
		server := reactive.Message[*configv1.ServerSpec](ct.mgr.Reactive(protopath.Path(configv1.ProtoPath().Server())))
		dashboard := reactive.Message[*configv1.DashboardServerSpec](ct.mgr.Reactive(protopath.Path(configv1.ProtoPath().Dashboard())))

		relay.WatchFunc(ctx, func(msg *configv1.RelayServerSpec) {
			ct.localInstanceInfoMu.Lock()
			defer ct.localInstanceInfoMu.Unlock()
			if addr := msg.GetAdvertiseAddress(); addr != "" {
				ct.localInstanceInfo.RelayAddress = addr
			} else {
				ct.logger.Warn("relay advertise address not set; will advertise the listen address")
				ct.localInstanceInfo.RelayAddress = msg.GetGrpcListenAddress()
			}
			updateInstanceInfo()
		})

		mgmt.WatchFunc(ctx, func(msg *configv1.ManagementServerSpec) {
			ct.localInstanceInfoMu.Lock()
			defer ct.localInstanceInfoMu.Unlock()
			if addr := msg.GetAdvertiseAddress(); addr != "" {
				ct.localInstanceInfo.ManagementAddress = addr
			} else {
				ct.logger.Warn("management advertise address not set; will advertise the listen address")
				ct.localInstanceInfo.ManagementAddress = msg.GetGrpcListenAddress()
			}
			updateInstanceInfo()
		})

		server.WatchFunc(ctx, func(msg *configv1.ServerSpec) {
			ct.localInstanceInfoMu.Lock()
			defer ct.localInstanceInfoMu.Unlock()
			if addr := msg.GetAdvertiseAddress(); addr != "" {
				ct.localInstanceInfo.GatewayAddress = addr
			} else {
				ct.logger.Warn("gateway advertise address not set; will advertise the listen address")
				ct.localInstanceInfo.GatewayAddress = msg.GetGrpcListenAddress()
			}
			updateInstanceInfo()
		})

		dashboard.WatchFunc(ctx, func(msg *configv1.DashboardServerSpec) {
			ct.localInstanceInfoMu.Lock()
			defer ct.localInstanceInfoMu.Unlock()
			if addr := msg.GetAdvertiseAddress(); addr != "" {
				ct.localInstanceInfo.WebAddress = addr
			} else {
				ct.logger.Warn("web advertise address not set; will advertise the listen address")
				ct.localInstanceInfo.WebAddress = msg.GetHttpListenAddress()
			}
			updateInstanceInfo()
		})
	})
	for ctx.Err() == nil {
		locker := ct.lm.Locker(instancesKey, lock.WithAcquireContext(ctx),
			lock.WithInitialValue(base64.StdEncoding.EncodeToString(util.Must(proto.Marshal(ct.LocalInstanceInfo())))))
		locker.Lock()
		listenOnce()
	}
}

type instanceInfoKeyType struct{}

var instanceInfoKey = instanceInfoKeyType{}

func (ct *ConnectionTracker) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		agentId := cluster.StreamAuthorizedID(ss.Context())
		instanceInfo := ct.LocalInstanceInfo()
		locker := ct.lm.Locker(agentId,
			lock.WithAcquireContext(ss.Context()),
			lock.WithInitialValue(base64.StdEncoding.EncodeToString(util.Must(proto.Marshal(instanceInfo)))),
		)
		ct.logger.With("agentId", agentId).Debug("attempting to acquire connection lock")
		if acquired, err := locker.TryLock(); !acquired {
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
			if err := locker.Unlock(); err != nil {
				ct.logger.With(
					logger.Err(err),
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

func (ct *ConnectionTracker) handleConnectionEventLocked(event storage.WatchEvent[storage.KeyRevision[[]byte]]) {
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
		info, err := instanceInfo()
		if err != nil {
			lg.With(logger.Err(err)).Error("failed to unmarshal instance info")
			return
		}
		isUpdate := false
		if conn, ok := ct.activeConnections[agentId]; ok {
			if !info.GetAcquired() {
				// a different instance is only attempting to acquire the lock,
				// ignore the event
				ct.logger.With("agent", agentId, "instance", info.GetRelayAddress()).Debug("observed lock attempt from another instance")
				return
			}
			if conn.leaseId == leaseId {
				// key was updated, not a new connection
				isUpdate = true
			} else {
				// a different instance has acquired the lock, invalidate
				// the current tracked connection
				ct.logger.With("agentId", agentId).Debug("tracked connection invalidated")
				conn.cancelTrackingContext()
				delete(ct.activeConnections, agentId)
			}
		}
		if !info.GetAcquired() {
			return // ignore unacquired connections
		}
		if ct.IsLocalInstance(info) {
			ct.logger.With("agentId", agentId).Debug("tracking new connection (local)")
		} else if isUpdate {
			ct.logger.With("agentId", agentId).Debug("tracked connection updated")
		} else {
			ct.logger.With("agentId", agentId).Debug("tracking new connection")
		}

		var trackingContext context.Context
		if !isUpdate {
			ctx, cancel := context.WithCancel(ct.rootContext)
			trackingContext = ctx
			conn := &activeTrackedConnection{
				agentId:               agentId,
				leaseId:               leaseId,
				revision:              event.Current.Revision(),
				instanceInfo:          info,
				trackingContext:       ctx,
				cancelTrackingContext: cancel,
			}
			ct.activeConnections[agentId] = conn
		} else {
			conn := ct.activeConnections[agentId]
			conn.revision = event.Current.Revision()
			conn.instanceInfo = info
			trackingContext = conn.trackingContext
		}
		for _, listener := range ct.connListeners {
			listener.HandleTrackedConnection(trackingContext, agentId, leaseId, info)
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
			lg.With("key", event.Previous.Key()).Debug("ignoring untracked key deletion event")
		}
	}
}

func (ct *ConnectionTracker) handleInstanceEventLocked(event storage.WatchEvent[storage.KeyRevision[[]byte]]) {
	switch event.EventType {
	case storage.WatchEventPut:
		leaseId := event.Current.Key()[len(instancesKey)+1:]
		instanceInfo := &corev1.InstanceInfo{}
		if err := proto.Unmarshal(event.Current.Value(), instanceInfo); err != nil {
			ct.logger.With(logger.Err(err)).Error("failed to unmarshal instance info")
			return
		}
		if existing, ok := ct.activeInstances[leaseId]; ok {
			ct.logger.With("leaseId", leaseId).Debug("tracked instance updated")
			existing.instanceInfo = instanceInfo
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
