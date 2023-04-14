package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/management"

	"google.golang.org/protobuf/proto"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (p *Plugin) newClusterWatcherHooks(ctx context.Context, ingressStream *nats.StreamConfig) *management.ManagementWatcherHooks[*managementv1.WatchEvent] {
	err := natsutil.NewPersistentStream(p.js.Get(), ingressStream)
	if err != nil {
		panic(err)
	}
	cw := management.NewManagementWatcherHooks[*managementv1.WatchEvent](ctx)
	cw.RegisterHook(
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
			return p.createDefaultDisconnect(event.Cluster.Id)
		},
		func(ctx context.Context, event *managementv1.WatchEvent) error {
			return p.createDefaultCapabilityHealth(event.Cluster.Id)
		},
	)
	cw.RegisterHook(
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
	js               nats.JetStreamContext
	streamSubject    string
	durableConsumer  *nats.ConsumerConfig
	storageClientSet storage.AlertingClientSet
	msgCh            chan *nats.Msg
}

type internalConditionState struct {
	inMemoryFiring bool
	stateLock      sync.Mutex
	firingLock     sync.RWMutex
}

type internalConditionHooks[T proto.Message] struct {
	healthOnMessage func(h T) (healthy bool, ts *timestamppb.Timestamp)
	triggerHook     func(ctx context.Context, conditionId string, labels, annotations map[string]string)
	resolveHook     func(ctx context.Context, conditionId string, labels, annotations map[string]string)
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
		"",
	}
}

// --------------------------------
type InternalConditionEvaluator[T proto.Message] struct {
	*internalConditionMetadata
	*internalConditionContext
	*internalConditionStorage
	*internalConditionState
	*internalConditionHooks[T]
	fingerprint string
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
			c.lg.Info("parent context is exiting, exiting evaluation loop")
			return
		case <-c.evaluationCtx.Done():
			c.lg.Info("evaluation context is exiting, exiting evaluation loop")
			return
		case msg := <-c.msgCh:
			var status T
			err := json.Unmarshal(msg.Data, &status)
			if err != nil {
				c.lg.Error(err)
			}
			healthy, ts := c.healthOnMessage(status)
			incomingState := alertingv1.CachedState{
				Healthy:   healthy,
				Firing:    c.IsFiring(),
				Timestamp: ts,
			}
			c.UpdateState(c.evaluationCtx, &incomingState)
			msg.Ack()
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
			c.lg.Info("parent context is exiting, exiting evaluation loop")
			return
		case <-c.evaluationCtx.Done():
			c.lg.Info("evaluation context is exiting, exiting evaluation loop")
			return
		case <-ticker.C:
			lastKnownState, err := c.storageClientSet.States().Get(c.evaluationCtx, c.conditionId)
			if err != nil {
				continue
			}
			if !lastKnownState.Healthy {
				c.lg.Debugf("condition %s is unhealthy", c.conditionName)
				interval := timestamppb.Now().AsTime().Sub(lastKnownState.Timestamp.AsTime())
				if interval > c.evaluateDuration { // then we must fire an alert
					if !c.IsFiring() {
						c.fingerprint = strconv.Itoa(int(time.Now().Unix()))
						c.SetFiring(true)
						err = c.UpdateState(c.evaluationCtx, &alertingv1.CachedState{
							Healthy:   lastKnownState.Healthy,
							Firing:    c.IsFiring(),
							Timestamp: timestamppb.Now(),
						})
						if err != nil {
							c.lg.Error(err)
						}
						err = c.storageClientSet.Incidents().OpenInterval(c.evaluationCtx, c.conditionId, c.fingerprint, timestamppb.Now())
						if err != nil {
							c.lg.Error(err)
						}
					}
					c.lg.Debugf("triggering alert for condition %s", c.conditionName)
					c.triggerHook(c.evaluationCtx, c.conditionId, map[string]string{
						alertingv1.NotificationPropertyFingerprint: c.fingerprint,
					}, map[string]string{
						alertingv1.NotificationPropertyFingerprint: c.fingerprint,
					})
				}
			} else if lastKnownState.Healthy && c.IsFiring() &&
				// avoid potential noise from api streams & replays
				lastKnownState.Timestamp.AsTime().Add(-c.evaluateInterval).Before(time.Now()) {
				c.lg.Debugf("condition %s is now healthy again after having fired", c.conditionName)
				c.SetFiring(false)
				err = c.storageClientSet.Incidents().CloseInterval(c.evaluationCtx, c.conditionId, c.fingerprint, timestamppb.Now())
				if err != nil {
					c.lg.Error(err)
				}
				c.resolveHook(c.evaluationCtx, c.conditionId, map[string]string{
					alertingv1.NotificationPropertyFingerprint: c.fingerprint,
				}, map[string]string{
					alertingv1.NotificationPropertyFingerprint: c.fingerprint,
				})
				c.fingerprint = ""
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

func (c *InternalConditionEvaluator[T]) UpdateState(ctx context.Context, s *alertingv1.CachedState) error {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	if c.storageClientSet.States().IsDiff(ctx, c.conditionId, s) {
		return c.storageClientSet.States().Put(ctx, c.conditionId, s)
	}
	return nil
}

func (c *InternalConditionEvaluator[T]) CalculateInitialState() {
	incomingState := alertingv1.DefaultCachedState()
	if _, getErr := c.storageClientSet.Incidents().Get(c.evaluationCtx, c.conditionId); errors.Is(nats.ErrKeyNotFound, getErr) {
		err := c.storageClientSet.Incidents().Put(c.evaluationCtx, c.conditionId, alertingv1.NewIncidentIntervals())
		if err != nil {
			c.lg.Error(err)
			c.cancelEvaluation()
			return
		}
	} else if getErr != nil {
		c.lg.Error(getErr)
	}
	if st, getErr := c.storageClientSet.States().Get(c.evaluationCtx, c.conditionId); errors.Is(nats.ErrKeyNotFound, getErr) {
		if err := c.storageClientSet.States().Put(c.evaluationCtx, c.conditionId, incomingState); err != nil {
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
