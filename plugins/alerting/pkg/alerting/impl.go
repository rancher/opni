package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

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

var _ WatcherHooks[proto.Message] = &ManagementWatcherHooks[proto.Message]{}

type ManagementWatcherHooks[T proto.Message] struct {
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

func NewDefaultClusterWatcherHooks[T proto.Message](parentCtx context.Context) *ManagementWatcherHooks[T] {
	return &ManagementWatcherHooks[T]{
		parentCtx: parentCtx,
		cases:     []lo.Tuple2[func(T) bool, []func(context.Context, T) error]{},
	}
}

func (p *Plugin) newClusterWatcherHooks(ctx context.Context, ingressStream *nats.StreamConfig) *ManagementWatcherHooks[*managementv1.WatchEvent] {
	err := natsutil.NewPersistentStream(p.js.Get(), ingressStream)
	if err != nil {
		panic(err)
	}
	cw := NewDefaultClusterWatcherHooks[*managementv1.WatchEvent](ctx)
	cw.RegisterEvent(
		createClusterEvent,
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			err := natsutil.NewDurableReplayConsumer(p.js.Get(), ingressStream.Name, NewAgentDurableReplayConsumer(event.Cluster.Id))
			p.Logger.Info("added durable ordered push consumer for cluster %s", event.Cluster.Id)
			if err != nil {
				panic(err)
			}
			return nil

		},
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
	js              nats.JetStreamContext
	streamSubject   string
	durableConsumer *nats.ConsumerConfig
	storageNode     *alertstorage.StorageNode
	msgCh           chan *nats.Msg
}

type internalConditionState struct {
	inMemoryFiring bool
	stateLock      sync.Mutex
	firingLock     sync.RWMutex
}

type internalConditionHooks[T proto.Message] struct {
	healthOnMessage    func(h T) (healthy bool, ts *timestamppb.Timestamp)
	triggerHook        func(ctx context.Context, conditionId string, annotations map[string]string)
	resolveHook        func(ctx context.Context, conditionId string, annotations map[string]string)
	finalizerOnMessage func(msg T) bool
	finalizerHook      func(ctx context.Context, msg T, conditionId string, annotations map[string]string)
}

func NewInternalConditionEvaluator[T proto.Message](
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
type InternalConditionEvaluator[T proto.Message] struct {
	*internalConditionMetadata
	*internalConditionContext
	*internalConditionStorage
	*internalConditionState
	*internalConditionHooks[T]
}

// infinite & blocking : must be run in a goroutine
func (c *InternalConditionEvaluator[T]) SubscriberLoop() {
	defer c.cancelEvaluation()
	//replay consumer if it exists
	t := time.NewTicker(c.evaluateInterval)
	defer t.Stop()
	for {
		shouldExit := false
		select {
		case <-c.evaluationCtx.Done():
			return
		case <-t.C:
			subStream, err := c.js.ChanSubscribe(c.streamSubject, c.msgCh)
			if err != nil {
				c.lg.Warn("failed to subscribe to stream %s", err)
				continue
			}
			defer subStream.Unsubscribe()
			if err != nil {
				continue
			}
			shouldExit = true
		}
		if shouldExit {
			break
		}
	}
	t.Stop()
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
			msg.Ack()
			if c.finalizerOnMessage(status) {
				c.finalizerHook(c.evaluationCtx, status, c.conditionId, c.alertmanagerlabels)
				c.cancelEvaluation()
			}
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
		if err != nil {
			c.lg.Error(err)
			c.cancelEvaluation()
			return
		}
	} else if getErr != nil {
		c.lg.Error(getErr)
	}
	if st, getErr := c.storageNode.GetConditionStatusTracker(c.evaluationCtx, c.conditionId); errors.Is(nats.ErrKeyNotFound, getErr) {
		if err := c.storageNode.CreateConditionStatusTracker(c.evaluationCtx, c.conditionId, incomingState); err != nil {
			c.cancelEvaluation()
			return
		}

	} else if getErr == nil {
		incomingState = st
	}
	if incomingState.Firing { // need to update this in memory value
		c.SetFiring(true)
	}
	_ = c.UpdateState(c.evaluationCtx, incomingState)
}
