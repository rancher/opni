package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/health"
	natsutil "github.com/rancher/opni/pkg/util/nats"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ WatcherHooks[any] = &ManagementWatcherHooks[any]{}

type ManagementWatcherHooks[T any] struct {
	parentCtx context.Context
	cases     []lo.Tuple2[func(T) bool, []func(context.Context, T) error]
}

func (c *ManagementWatcherHooks[T]) RegisterEvent(eventFilter func(T) bool, hook ...func(context.Context, T) error) {
	c.cases = append(c.cases, lo.Tuple2[func(T) bool, []func(context.Context, T) error]{A: eventFilter, B: hook})
}

func (c *ManagementWatcherHooks[T]) HandleEvent(event T) {
	for _, cs := range c.cases {
		if cs.A(event) {
			for _, hook := range cs.B {
				_ = hook(c.parentCtx, event)
			}
		}
	}
}

var _ ReplayJetStreamHooks[any] = &AlertingStreamHooks[any]{}

type AlertingStreamHooks[T any] struct {
	lg            *zap.SugaredLogger
	js            nats.JetStreamContext
	parentCtx     context.Context
	msgCh         chan *nats.Msg
	sub           *nats.Subscription
	ingressStream *nats.StreamConfig
	// allows ingress stream to be replayed for new subscribers
	durableOrderedConsumer *nats.ConsumerConfig
	derivedStreams         []lo.Tuple2[func(T) bool, []func(context.Context, T) error]
}

func NewAlertingStreamHooks[T any](
	js nats.JetStreamContext,
	parentCtx context.Context,
	lg *zap.SugaredLogger,
	ingressStream *nats.StreamConfig,
	durableOrderedConsumer *nats.ConsumerConfig,
) *AlertingStreamHooks[T] {
	res := &AlertingStreamHooks[T]{
		js:            js,
		lg:            lg,
		parentCtx:     parentCtx,
		ingressStream: ingressStream,
	}
	streamErr := natsutil.NewPersistentStream(js, ingressStream)
	if streamErr != nil {
		panic(streamErr)
	}
	consumerErr := res.SetDurableOrderedPushConsumer(durableOrderedConsumer)
	if consumerErr != nil {
		panic(consumerErr)
	}
	//sub, err := r.js.Subscribe
	msgCh := make(chan *nats.Msg, 32)
	sub, err := js.ChanSubscribe(ingressStream.Subjects[0], msgCh, nats.Bind(ingressStream.Name, durableOrderedConsumer.Durable))
	if err != nil {
		panic(err)
	}
	res.sub = sub
	res.msgCh = msgCh
	return res
}

func (r *AlertingStreamHooks[T]) GetIngressStream() string {
	return r.ingressStream.Name
}

func (r *AlertingStreamHooks[T]) SetDurableOrderedPushConsumer(durableConsumer *nats.ConsumerConfig) error {
	r.durableOrderedConsumer = durableConsumer
	err := natsutil.NewDurableReplayConsumer(r.js, r.ingressStream.Name, durableConsumer)
	return err
}

func (r *AlertingStreamHooks[T]) PublishLoop() {
	t := time.NewTicker(time.Second * 10)
	defer func(sub *nats.Subscription, t *time.Ticker) {
		t.Stop()
		err := sub.Unsubscribe()
		if err != nil {
			r.lg.Error(err)
		}
		r.lg.Debugf("exiting from publish loop for %s/%s", r.ingressStream.Name, r.durableOrderedConsumer.Name)
	}(r.sub, t)
	for {
		select {
		case <-r.parentCtx.Done():
			return
		case msg := <-r.msgCh:
			var event T
			err := json.Unmarshal(msg.Data, &event)
			if err != nil {
				r.lg.Errorf("failed to unmarshal event: %s", err)
			}
			r.HandleEvent(event)
		case <-t.C:
			info, _ := r.sub.ConsumerInfo()
			r.lg.Debugf("max stream sequence delivered: %d\n", info.Delivered.Stream)
			r.lg.Debugf("max consumer sequence delivered: %d\n", info.Delivered.Consumer)
			r.lg.Debugf("num ack pending: %d\n", info.NumAckPending)
			r.lg.Debugf("num redelivered: %d\n", info.NumRedelivered)
		}
	}
}

func (r *AlertingStreamHooks[T]) RegisterEvent(eventFilter func(T) bool, hooks ...func(context.Context, T) error) {
	r.derivedStreams = append(r.derivedStreams, lo.Tuple2[func(T) bool, []func(context.Context, T) error]{A: eventFilter, B: hooks})
}

func (r *AlertingStreamHooks[T]) HandleEvent(event T) {
	for _, stream := range r.derivedStreams {
		for _, hook := range stream.B {
			_ = hook(r.parentCtx, event)
		}
	}
}

func isClusterStatusWellFormed(status *corev1.ClusterHealthStatus) bool {
	if status.HealthStatus == nil ||
		status.HealthStatus.Status.Timestamp == nil ||
		status.Cluster.Id == "" {
		return false
	}
	return true
}

func (p *Plugin) agentDisconnectPublishHook(ctx context.Context, status *corev1.ClusterHealthStatus) error {
	msg := &health.StatusUpdate{
		ID:     status.Cluster.Id,
		Status: status.HealthStatus.Status,
	}
	agentDisconnectData, err := json.Marshal(msg)
	if err != nil {
		p.Logger.Errorf("failed to marshal cluster health status update : %s", err)
	}
	_, err = p.js.Get().PublishAsync(shared.NewAgentDisconnectSubject(status.Cluster.GetId()), agentDisconnectData)
	if err != nil {
		p.Logger.Errorf("failed to publish cluster health status update : %s", err)
	}
	return nil
}

func (p *Plugin) downstreamCapabilityPublishHook(ctx context.Context, status *corev1.ClusterHealthStatus) error {
	healthStatusData, err := json.Marshal(status)
	if err != nil {
		p.Logger.Errorf("failed to marshal cluster health status update : %s", err)
	}
	_, err = p.js.Get().PublishAsync(shared.NewHealthStatusSubject(status.Cluster.GetId()), healthStatusData)
	return err
}

// cluster watcher hooks
func NewDefaultClusterWatcherHooks[T any](parentCtx context.Context) *ManagementWatcherHooks[T] {
	return &ManagementWatcherHooks[T]{
		parentCtx: parentCtx,
		cases:     []lo.Tuple2[func(T) bool, []func(context.Context, T) error]{},
	}
}

func (p *Plugin) newClusterStatusWatcherHooks(
	ctx context.Context,
	streamConfig *nats.StreamConfig,
	durableConsumer *nats.ConsumerConfig,
) *AlertingStreamHooks[*corev1.ClusterHealthStatus] {
	csw := NewAlertingStreamHooks[*corev1.ClusterHealthStatus](
		p.js.Get(),
		ctx,
		p.Logger.With("watcher", "global-status-health"),
		streamConfig,
		durableConsumer,
	)
	// register agent disconnect / downstream capability hook for durable consumer
	csw.RegisterEvent(
		isClusterStatusWellFormed,
		func(ctx context.Context, event *corev1.ClusterHealthStatus) error {
			return p.agentDisconnectPublishHook(ctx, event)
		},
		func(ctx context.Context, event *corev1.ClusterHealthStatus) error {
			return p.downstreamCapabilityPublishHook(ctx, event)
		},
	)
	return csw
}

// opni-alerting implementation of cluster management hooks
func (p *Plugin) newClusterWatcherHooks(ctx context.Context) *ManagementWatcherHooks[*managementv1.WatchEvent] {
	cw := NewDefaultClusterWatcherHooks[*managementv1.WatchEvent](ctx)
	cw.RegisterEvent(
		createClusterEvent,
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			return p.createDefaultDisconnect(ctx, event.Cluster.Id)
		},
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			return p.createDefaultCapabilityHealth(ctx, event.Cluster.Id)
		},
	)
	cw.RegisterEvent(
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
	return event.Type == managementv1.WatchEventType_Created
}

func deleteClusterEvent(event *managementv1.WatchEvent) bool {
	return event.Type == managementv1.WatchEventType_Deleted
}

var _ InternalConditionWatcher = &SimpleInternalConditionWatcher{}

type SimpleInternalConditionWatcher struct {
	closures []func()
}

func NewSimpleInternalConditionWatcher(cl ...func()) *SimpleInternalConditionWatcher {
	return &SimpleInternalConditionWatcher{
		closures: cl,
	}
}

func (s *SimpleInternalConditionWatcher) WatchEvents() {
	for _, f := range s.closures {
		f := f
		go func() {
			f()
		}()
	}
}

type internalConditionMetadata struct {
	lg                 *zap.SugaredLogger
	conditionName      string
	conditionId        string
	clusterId          string
	alertmanagerlabels map[string]string
}

type internalConditionContext struct {
	parentCtx        context.Context
	cancelEvaluation context.CancelFunc
	evaluateDuration time.Duration
	evaluationCtx    context.Context
	evaluateInterval time.Duration
}

type internalConditionStorage struct {
	js            nats.JetStreamContext
	stream        *nats.StreamConfig
	streamSubject string
	storageNode   *alertstorage.StorageNode
	msgCh         chan *nats.Msg
}

type internalConditionState struct {
	inMemoryFiring bool
	stateLock      sync.Mutex
	firingLock     sync.RWMutex
}

type internalConditionHooks[T any] struct {
	healthOnMessage func(h T) (healthy bool, ts *timestamppb.Timestamp)
	triggerHook     func(ctx context.Context, conditionId string, annotations map[string]string)
	resolveHook     func(ctx context.Context, conditionId string, annotations map[string]string)
}

func NewInternalConditionEvaluator[T any](
	metadata *internalConditionMetadata,
	context *internalConditionContext,
	storage *internalConditionStorage,
	state *internalConditionState,
	hooks *internalConditionHooks[T],
) *InternalConditionEvaluator[T] {
	return &InternalConditionEvaluator[T]{
		metadata,
		context,
		storage,
		state,
		hooks,
	}
}

// --------------------------------
type InternalConditionEvaluator[T any] struct {
	*internalConditionMetadata
	*internalConditionContext
	*internalConditionStorage
	*internalConditionState
	*internalConditionHooks[T]
}

// infinite & blocking : must be run in a goroutine
func (c *InternalConditionEvaluator[T]) SubscriberLoop() {
	defer c.cancelEvaluation()
	for {
		err := natsutil.NewPersistentStream(c.js, c.stream)
		if err != nil {
			c.lg.Errorf("alerting disconnect stream does not exist and cannot be created %s", err)
			continue
		}
		msgCh := make(chan *nats.Msg, 32)
		sub, err := c.js.ChanSubscribe(c.streamSubject, msgCh)
		defer sub.Unsubscribe()
		if err != nil {
			c.lg.Errorf("failed  to subscribe to %s : %s", c.streamSubject, err)
			continue
		}
		break
	}
	for {
		select {
		case <-c.parentCtx.Done():
			return
		case <-c.evaluationCtx.Done():
			return
		case msg := <-c.msgCh:
			var status T
			err := json.Unmarshal(msg.Data, &status)
			if err != nil {
				c.lg.Error(err)
			}
			healthy, ts := c.healthOnMessage(status)
			incomingState := alertstorage.State{
				Healthy:   healthy,
				Firing:    c.IsFiring(),
				Timestamp: ts,
			}
			c.UpdateState(c.evaluationCtx, &incomingState)
		}
	}
}

// infinite & blocking : must be run in a goroutine
func (c *InternalConditionEvaluator[T]) EvaluateLoop() {
	defer c.cancelEvaluation() // cancel parent context, if we return (non-recoverable)
	ticker := time.NewTicker(c.evaluateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.parentCtx.Done():
			return
		case <-c.evaluationCtx.Done():
			return
		case <-ticker.C:
			lastKnownState, err := c.storageNode.GetConditionStatusTracker(c.evaluationCtx, c.conditionId)
			if err != nil {
				continue
			}
			if !lastKnownState.Healthy {
				c.lg.Debugf("condition %s is unhealthy", c.conditionName)
				interval := timestamppb.Now().AsTime().Sub(lastKnownState.Timestamp.AsTime())
				if interval > c.evaluateDuration {
					c.lg.Debugf("triggering alert for condition %s", c.conditionName)
					c.triggerHook(c.evaluationCtx, c.conditionId, metrics.MergeLabels(c.alertmanagerlabels, map[string]string{
						shared.BackendConditionIdLabel: c.conditionId,
					}))
					if err != nil {
						c.lg.Error(err)
					}
					if !c.IsFiring() {
						c.SetFiring(true)
						err = c.UpdateState(c.evaluationCtx, &alertstorage.State{
							Healthy:   lastKnownState.Healthy,
							Firing:    c.IsFiring(),
							Timestamp: timestamppb.Now(),
						})
						if err != nil {
							c.lg.Error(err)
						}
						err = c.storageNode.OpenInterval(c.evaluationCtx, c.conditionId, timestamppb.Now())
						if err != nil {
							c.lg.Error(err)
						}
					}
				} else {
					c.SetFiring(false)
				}
			} else if lastKnownState.Healthy && c.IsFiring() {
				c.lg.Debugf("condition %s is now healthy again after having fired", c.conditionName)
				c.SetFiring(false)
				err = c.storageNode.CloseInterval(c.evaluationCtx, c.conditionId, timestamppb.Now())
				if err != nil {
					c.lg.Error(err)
				}
				c.resolveHook(c.evaluationCtx, c.conditionId, metrics.MergeLabels(c.alertmanagerlabels, map[string]string{
					shared.BackendConditionIdLabel: c.conditionId,
				}))
			}
		}
	}
}

func (c *InternalConditionEvaluator[T]) SetFiring(firing bool) {
	c.firingLock.Lock()
	defer c.firingLock.Unlock()
	c.inMemoryFiring = firing
}

func (c *InternalConditionEvaluator[T]) IsFiring() bool {
	c.firingLock.RLock()
	defer c.firingLock.RUnlock()
	return c.inMemoryFiring
}

func (c *InternalConditionEvaluator[T]) UpdateState(ctx context.Context, s *alertstorage.State) error {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	return c.storageNode.UpdateConditionStatusTracker(ctx, c.conditionId, s)
}

func (c *InternalConditionEvaluator[T]) CalculateInitialState() {
	incomingState := alertstorage.DefaultState()
	if _, getErr := c.storageNode.GetIncidentTracker(c.evaluationCtx, c.conditionId); errors.Is(nats.ErrKeyNotFound, getErr) {
		err := c.storageNode.CreateIncidentTracker(c.evaluationCtx, c.conditionId)
		if err != nil { // TODO : mark this condition as manually invalid if this happens
			c.lg.Error(err)
		}
	} else if getErr != nil {
		c.lg.Error(getErr)
	}
	if st, getErr := c.storageNode.GetConditionStatusTracker(c.evaluationCtx, c.conditionId); errors.Is(nats.ErrKeyNotFound, getErr) {
		if err := c.storageNode.CreateConditionStatusTracker(c.evaluationCtx, c.conditionId, incomingState); err != nil {
			// TODO : mark this condition as manually invalid when this happens
			c.cancelEvaluation()
			return
		}

	} else {
		incomingState = st
	}
	if incomingState.Firing { // need to update this in memory value
		c.SetFiring(true)
	}
	_ = c.UpdateState(c.evaluationCtx, incomingState)
}
