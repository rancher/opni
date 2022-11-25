package alerting

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ WatcherHooks[any] = &ClusterWatcherHooks[any]{}

type ClusterWatcherHooks[T any] struct {
	parentCtx context.Context
	cases     []lo.Tuple2[func(T) bool, []func(context.Context, T) error]
}

func NewDefaultClusterWatcherHooks[T any](parentCtx context.Context) *ClusterWatcherHooks[T] {
	return &ClusterWatcherHooks[T]{
		parentCtx: parentCtx,
		cases:     []lo.Tuple2[func(T) bool, []func(context.Context, T) error]{},
	}
}

func (p *Plugin) newClusterWatcherHooks(ctx context.Context) *ClusterWatcherHooks[*managementv1.WatchEvent] {
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

func (c *ClusterWatcherHooks[T]) RegisterEvent(eventType func(T) bool, hook ...func(context.Context, T) error) {
	c.cases = append(c.cases, lo.Tuple2[func(T) bool, []func(context.Context, T) error]{A: eventType, B: hook})
}

func (c *ClusterWatcherHooks[T]) HandleEvent(event T) {
	for _, cs := range c.cases {
		if cs.A(event) {
			for _, hook := range cs.B {
				_ = hook(c.parentCtx, event)
			}
		}
	}
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

type InternalConditionEvaluator[T any] struct {
	// metadata
	lg                 *zap.SugaredLogger
	conditionName      string
	conditionId        string
	clusterId          string
	alertmanagerlabels map[string]string
	// contexts
	parentCtx        context.Context
	cancelEvaluation context.CancelFunc
	evaluateDuration time.Duration
	evaluationCtx    context.Context

	inMemoryFiring bool
	stateLock      sync.Mutex
	firingLock     sync.RWMutex

	healthOnMessage func(h T) (healthy bool, ts *timestamppb.Timestamp)
	triggerHook     func(ctx context.Context, conditionId string, annotations map[string]string)
	storageNode     *alertstorage.StorageNode
	msgCh           chan *nats.Msg
}

// infinite & blocking : must be run in a goroutine
func (c *InternalConditionEvaluator[T]) SubscriberLoop() {
	defer c.cancelEvaluation()
	if c.msgCh == nil { // TODO : mark this condition as manually invalid if this happens
		c.lg.Errorf("msgCh is not initialized for condition %s", c.conditionName)
		return
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
	ticker := time.NewTicker(time.Second * 10)
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
	if st, getErr := c.storageNode.GetConditionStatusTracker(c.evaluationCtx, c.conditionId); getErr != nil {
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
