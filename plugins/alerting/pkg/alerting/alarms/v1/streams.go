package alarms

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/fingerprint"
	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func fallbackInterval(evalInterval time.Duration) *timestamppb.Timestamp {
	return timestamppb.New(time.Now().Add(-evalInterval))
}

var (
	DisconnectStreamEvaluateInterval = time.Second * 10
	CapabilityStreamEvaluateInterval = time.Second * 10
	CortexStreamEvaluateInterval     = time.Second * 30
)

func NewAgentStream() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      shared.AgentClusterHealthStatusStream,
		Subjects:  []string{shared.AgentClusterHealthStatusSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  1 * 1024 * 50, // 50KB
	}
}

func NewAgentDurableReplayConsumer(clusterId string) *nats.ConsumerConfig {
	return &nats.ConsumerConfig{
		Durable:        NewDurableAgentReplaySubject(clusterId),
		DeliverSubject: NewDurableAgentReplaySubject(clusterId),
		DeliverPolicy:  nats.DeliverNewPolicy,
		FilterSubject:  NewAgentStreamSubject(clusterId),
		AckPolicy:      nats.AckExplicitPolicy,
		ReplayPolicy:   nats.ReplayInstantPolicy,
	}
}

func NewDurableAgentReplaySubject(clusterId string) string {
	return fmt.Sprintf("%s-%s", shared.AgentClusterHealthStatusDurableReplay, clusterId)
}

func NewAgentStreamSubject(clusterId string) string {
	return fmt.Sprintf("%s.%s", shared.AgentClusterHealthStatusStream, clusterId)
}

func NewCortexStatusStream() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      shared.CortexStatusStream,
		Subjects:  []string{shared.CortexStatusStreamSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  1 * 1024 * 50, // 50KB
	}
}

func NewCortexStatusSubject() string {
	return fmt.Sprintf("%s.%s", shared.CortexStatusStream, "cortex")
}

func (p *AlarmServerComponent) onSystemConditionCreate(conditionId, conditionName, namespace string, condition *alertingv1.AlertCondition) error {
	lg := logger.PluginLoggerFromContext(p.ctx).With("onSystemConditionCreate", conditionId)
	ctx := logger.WithPluginLogger(p.ctx, lg)
	lg.Debug(fmt.Sprintf("received condition update: %v", condition))
	disconnect := condition.GetAlertType().GetSystem()
	jsCtx, cancel := context.WithCancel(p.ctx)
	lg.Debug(fmt.Sprintf("Creating agent disconnect with timeout %s", disconnect.GetTimeout().AsDuration()))
	agentId := condition.GetClusterId().Id
	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			clusterId:          agentId,
			alertmanagerlabels: map[string]string{},
		},
		&internalConditionContext{
			parentCtx:        ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: DisconnectStreamEvaluateInterval,
			cancelEvaluation: cancel,
			evaluateDuration: disconnect.GetTimeout().AsDuration(),
		},
		&internalConditionStorage{
			js:              p.js.Get(),
			durableConsumer: NewAgentDurableReplayConsumer(agentId),
			streamSubject:   NewAgentStreamSubject(agentId),
			stateStorage:    p.stateStorage.Get(),
			incidentStorage: p.incidentStorage.Get(),
			msgCh:           make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*corev1.ClusterHealthStatus]{
			healthOnMessage: func(h *corev1.ClusterHealthStatus) (healthy bool, md map[string]string, ts *timestamppb.Timestamp) {
				if h == nil {
					return false, map[string]string{}, fallbackInterval(disconnect.GetTimeout().AsDuration())
				}
				if h.HealthStatus == nil {
					return false, map[string]string{}, fallbackInterval(disconnect.GetTimeout().AsDuration())
				}
				if h.HealthStatus.Status == nil {
					return false, map[string]string{}, fallbackInterval(disconnect.GetTimeout().AsDuration())
				}
				lg.Debug(fmt.Sprintf("received agent health update connected %v : %s", h.HealthStatus.Status.Connected, h.HealthStatus.Status.Timestamp.String()))
				return h.HealthStatus.Status.Connected, map[string]string{}, h.HealthStatus.Status.Timestamp
			},
			triggerHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				p.notifications.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				_, _ = p.notifications.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
		},
	)
	// handles re-entrant conditions
	evaluator.CalculateInitialState()
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.SubscriberLoop()
	}()
	// spawn a watcher for the incidents
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.EvaluateLoop()
	}()
	p.runner.AddSystemConfigListener(conditionId, &EvaluatorContext{
		Ctx:     evaluator.evaluationCtx,
		Cancel:  evaluator.cancelEvaluation,
		running: &atomic.Bool{},
	})
	return nil
}

func (p *AlarmServerComponent) onDownstreamCapabilityConditionCreate(conditionId, conditionName, namespace string, condition *alertingv1.AlertCondition) error {
	lg := logger.PluginLoggerFromContext(p.ctx).With("onCapabilityStatusCreate", conditionId)
	ctx := logger.WithPluginLogger(p.ctx, lg)
	capability := condition.GetAlertType().GetDownstreamCapability()
	lg.Debug(fmt.Sprintf("received condition update: %v", condition))
	jsCtx, cancel := context.WithCancel(p.ctx)
	lg.Debug(fmt.Sprintf("Creating agent capability unhealthy with timeout %s", capability.GetFor().AsDuration()))
	agentId := condition.GetClusterId().Id
	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			clusterId:          agentId,
			alertmanagerlabels: map[string]string{},
		},
		&internalConditionContext{
			parentCtx:        ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: CapabilityStreamEvaluateInterval,
			cancelEvaluation: cancel,
			evaluateDuration: capability.GetFor().AsDuration(),
		},
		&internalConditionStorage{
			js:              p.js.Get(),
			durableConsumer: NewAgentDurableReplayConsumer(agentId),
			streamSubject:   NewAgentStreamSubject(agentId),
			stateStorage:    p.stateStorage.Get(),
			incidentStorage: p.incidentStorage.Get(),
			msgCh:           make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*corev1.ClusterHealthStatus]{
			healthOnMessage: func(h *corev1.ClusterHealthStatus) (healthy bool, md map[string]string, ts *timestamppb.Timestamp) {
				healthy = true
				if h == nil {
					return false, map[string]string{}, fallbackInterval(capability.GetFor().AsDuration())
				}
				if h.HealthStatus == nil {
					return false, map[string]string{}, fallbackInterval(capability.GetFor().AsDuration())
				}
				if h.HealthStatus.Health == nil {
					return false, map[string]string{}, fallbackInterval(capability.GetFor().AsDuration())
				}
				lg.Debug(fmt.Sprintf("found health conditions %v", h.HealthStatus.Health.Conditions))
				md = map[string]string{}
				for _, s := range h.HealthStatus.Health.Conditions {
					for _, badState := range capability.GetCapabilityState() {
						if strings.Contains(s, badState) {
							healthy = false
							strArr := strings.Split(s, " ")
							if len(strArr) == 1 {
								md[strArr[0]] = "evaluate health details unavailable"
							} else {
								md[strArr[0]] = strings.Join(strArr[1:], ",")
							}
						}
					}
				}
				return healthy, md, h.HealthStatus.GetStatus().GetTimestamp()
			},
			triggerHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				_, _ = p.notifications.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				_, _ = p.notifications.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
		},
	)
	// handles re-entrant conditions
	evaluator.CalculateInitialState()
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.SubscriberLoop()
	}()
	// spawn a watcher for the incidents
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.EvaluateLoop()
	}()
	p.runner.AddSystemConfigListener(conditionId, &EvaluatorContext{
		Ctx:     evaluator.evaluationCtx,
		Cancel:  evaluator.cancelEvaluation,
		running: &atomic.Bool{},
	})
	return nil
}

func reduceCortexAdminStates(componentsToTrack []string, cStatus *cortexadmin.CortexStatus) (healthy bool, md map[string]string, ts *timestamppb.Timestamp) {
	if cStatus == nil {
		return false, map[string]string{}, timestamppb.Now()
	}
	ts = cStatus.GetTimestamp()
	// helps track status errors to particular components, like having 3 expected replicas, but only 1-2 are running
	memberReports := map[string]bool{}
	for _, cmp := range componentsToTrack {
		switch cmp {
		case shared.CortexDistributor:
			if cStatus.Distributor == nil {
				return false, map[string]string{
					shared.CortexDistributor: "status unavailable",
				}, ts
			}
			services := cStatus.GetDistributor().GetServices().GetServices()
			if len(services) == 0 {
				return false, map[string]string{
					shared.CortexDistributor: "no services",
				}, ts
			}
			for _, svc := range services {
				memberReports[svc.GetName()] = true
				if strings.Contains(svc.GetName(), shared.CortexDistributor) && svc.GetStatus() != "Running" {
					return false, map[string]string{
						shared.CortexDistributor: fmt.Sprintf("%s not running", svc.GetName()),
					}, ts
				}
			}
		case shared.CortexIngester:
			if cStatus.Ingester == nil {
				return false,
					map[string]string{
						shared.CortexIngester: "status unavailable",
					}, ts
			}
			members := cStatus.GetIngester().GetMemberlist().GetMembers().GetItems()
			for _, member := range members {
				if _, ok := memberReports[member.Name]; !ok {
					memberReports[member.Name] = true
				}
			}
			services := cStatus.GetIngester().GetServices().GetServices()
			if len(services) == 0 {
				return false,
					map[string]string{
						shared.CortexIngester: "no services",
					}, ts
			}
			for _, svc := range services {
				memberReports[svc.GetName()] = true
				if strings.Contains(svc.GetName(), shared.CortexIngester) && svc.GetStatus() != "Running" {
					return false,
						map[string]string{
							shared.CortexIngester: fmt.Sprintf("%s not running", svc.GetName()),
						}, ts
				}
			}
		case shared.CortexRuler:
			if cStatus.Ruler == nil {
				return false,
					map[string]string{
						shared.CortexRuler: "status unavailable",
					}, ts
			}
			members := cStatus.GetRuler().GetMemberlist().GetMembers().GetItems()
			for _, member := range members {
				if _, ok := memberReports[member.Name]; !ok {
					memberReports[member.Name] = true
				}
			}
			services := cStatus.GetRuler().GetServices().GetServices()
			if len(services) == 0 {
				return false,
					map[string]string{
						shared.CortexRuler: "no services",
					}, ts
			}
			for _, svc := range services {
				memberReports[svc.GetName()] = true
				if strings.Contains(svc.GetName(), shared.CortexRuler) && svc.GetStatus() != "Running" {
					return false,
						map[string]string{
							shared.CortexRuler: fmt.Sprintf("%s not running", svc.GetName()),
						}, ts
				}
			}
		case shared.CortexPurger:
			if cStatus.Purger == nil {
				return false,
					map[string]string{
						shared.CortexPurger: "status unavailable",
					}, ts
			}
			services := cStatus.GetPurger().GetServices().GetServices()
			if len(services) == 0 {
				return false,
					map[string]string{
						shared.CortexPurger: "no services",
					}, ts
			}
			for _, svc := range services {
				memberReports[svc.GetName()] = true
				if strings.Contains(svc.GetName(), shared.CortexPurger) && svc.GetStatus() != "Running" {
					return false,
						map[string]string{
							shared.CortexPurger: fmt.Sprintf("%s not running", svc.GetName()),
						}, ts
				}
			}
		case shared.CortexCompactor:
			if cStatus.Compactor == nil {
				return false,
					map[string]string{
						shared.CortexCompactor: "status unavailable",
					}, ts
			}
			members := cStatus.GetCompactor().GetMemberlist().GetMembers().GetItems()
			for _, member := range members {
				if _, ok := memberReports[member.Name]; !ok {
					memberReports[member.Name] = true
				}
			}
			services := cStatus.GetCompactor().GetServices().GetServices()
			if len(services) == 0 {
				return false,
					map[string]string{
						shared.CortexCompactor: "no services",
					}, ts
			}
			for _, svc := range services {
				memberReports[svc.GetName()] = true
				if strings.Contains(svc.GetName(), shared.CortexCompactor) && svc.GetStatus() != "Running" {
					return false,
						map[string]string{
							shared.CortexCompactor: fmt.Sprintf("%s not running", svc.GetName()),
						}, ts
				}
			}
		case shared.CortexStoreGateway:
			if cStatus.StoreGateway == nil {
				return false,
					map[string]string{
						shared.CortexStoreGateway: "status unavailable",
					}, ts
			}
			services := cStatus.GetStoreGateway().GetServices().GetServices()
			if len(services) == 0 {
				return false,
					map[string]string{
						shared.CortexStoreGateway: "no services",
					}, ts
			}
			for _, svc := range services {
				memberReports[svc.GetName()] = true
				if strings.Contains(svc.GetName(), shared.CortexStoreGateway) && svc.GetStatus() != "Running" {
					return false,
						map[string]string{
							shared.CortexStoreGateway: fmt.Sprintf("%s not running", svc.GetName()),
						}, ts
				}
			}
		case shared.CortexQueryFrontend:
			if cStatus.QueryFrontend == nil {
				return false,
					map[string]string{
						shared.CortexQueryFrontend: "status unavailable",
					}, ts
			}
			services := cStatus.GetQueryFrontend().GetServices().GetServices()
			if len(services) == 0 {
				return false,
					map[string]string{
						shared.CortexQueryFrontend: "no services",
					}, ts
			}
			for _, svc := range services {
				memberReports[svc.GetName()] = true
				if strings.Contains(svc.GetName(), shared.CortexQueryFrontend) && svc.GetStatus() != "Running" {
					return false,
						map[string]string{
							shared.CortexQueryFrontend: fmt.Sprintf("%s not running", svc.GetName()),
						}, ts
				}
			}
		case shared.CortexQuerier:
			if cStatus.Querier == nil {
				return false,
					map[string]string{
						shared.CortexQuerier: "status unavailable",
					}, ts
			}
			services := cStatus.GetQuerier().GetServices().GetServices()
			if len(services) == 0 {
				return false,
					map[string]string{
						shared.CortexQuerier: "no services",
					}, ts
			}
			for _, svc := range services {
				memberReports[svc.GetName()] = true
				if strings.Contains(svc.GetName(), shared.CortexQuerier) && svc.GetStatus() != "Running" {
					return false,
						map[string]string{
							shared.CortexQuerier: fmt.Sprintf("%s not running", svc.GetName()),
						}, ts
				}
			}
		}
	}
	// on cortex-status error, if a specific component is not reported, we assume it is unhealthy
	for _, component := range componentsToTrack {
		for member, reportedOn := range memberReports {
			if strings.Contains(member, component) && !reportedOn {
				return false, map[string]string{
					component: fmt.Sprintf("Component %s's status was never reported", component),
				}, ts
			}
		}
	}
	return true, map[string]string{}, ts
}

func (p *AlarmServerComponent) onCortexClusterStatusCreate(conditionId, conditionName, namespace string, condition *alertingv1.AlertCondition) error {
	lg := logger.PluginLoggerFromContext(p.ctx).With("onCortexClusterStatusCreate", conditionId)
	ctx := logger.WithPluginLogger(p.ctx, lg)
	cortex := condition.GetAlertType().GetMonitoringBackend()
	lg.Debug(fmt.Sprintf("received condition update: %v", condition))
	jsCtx, cancel := context.WithCancel(p.ctx)
	lg.Debug(fmt.Sprintf("Creating cortex status with timeout %s", cortex.GetFor().AsDuration()))

	evaluator := NewInternalConditionEvaluator(
		&internalConditionMetadata{
			conditionId:        conditionId,
			conditionName:      conditionName,
			clusterId:          "", // unused here
			alertmanagerlabels: map[string]string{},
		},
		&internalConditionContext{
			parentCtx:        ctx,
			evaluationCtx:    jsCtx,
			evaluateInterval: CortexStreamEvaluateInterval,
			cancelEvaluation: cancel,
			evaluateDuration: cortex.GetFor().AsDuration(),
		},
		&internalConditionStorage{
			js:              p.js.Get(),
			durableConsumer: nil,
			streamSubject:   NewCortexStatusSubject(),
			stateStorage:    p.stateStorage.Get(),
			incidentStorage: p.incidentStorage.Get(),
			msgCh:           make(chan *nats.Msg, 32),
		},
		&internalConditionState{},
		&internalConditionHooks[*cortexadmin.CortexStatus]{
			healthOnMessage: func(h *cortexadmin.CortexStatus) (healthy bool, md map[string]string, ts *timestamppb.Timestamp) {
				if h == nil {
					return false, map[string]string{}, fallbackInterval(cortex.GetFor().AsDuration())
				}
				return reduceCortexAdminStates(cortex.GetBackendComponents(), h)
			},
			triggerHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				_, _ = p.notifications.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
			resolveHook: func(ctx context.Context, conditionId string, labels, annotations map[string]string) {
				lg.Debug("resolve cortex status condition")
				_, _ = p.notifications.ResolveAlerts(ctx, &alertingv1.ResolveAlertsRequest{
					ConditionId:   &corev1.Reference{Id: conditionId},
					ConditionName: conditionName,
					Namespace:     namespace,
					Labels:        lo.Assign(condition.GetRoutingLabels(), labels),
					Annotations:   lo.Assign(condition.GetRoutingAnnotations(), annotations),
				})
			},
		},
	)
	// handles re-entrant conditions
	evaluator.CalculateInitialState()
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.SubscriberLoop()
	}()
	// spawn a watcher for the incidents
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		evaluator.EvaluateLoop()
	}()
	p.runner.AddSystemConfigListener(conditionId, &EvaluatorContext{
		Ctx:     evaluator.evaluationCtx,
		Cancel:  evaluator.cancelEvaluation,
		running: &atomic.Bool{},
	})
	return nil
}

type internalConditionMetadata struct {
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
	incidentStorage spec.IncidentStorage
	stateStorage    spec.StateStorage

	msgCh chan *nats.Msg
}

type internalConditionState struct {
	inMemoryFiring bool
	stateLock      sync.Mutex
	firingLock     sync.RWMutex
}

type internalConditionHooks[T proto.Message] struct {
	healthOnMessage func(h T) (healthy bool, md map[string]string, ts *timestamppb.Timestamp)
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
	fingerprint fingerprint.Fingerprint
}

// infinite & blocking : must be run in a goroutine
func (c *InternalConditionEvaluator[T]) SubscriberLoop() {
	lg := logger.PluginLoggerFromContext(c.parentCtx)
	defer c.cancelEvaluation()
	// replay consumer if it exists
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
				lg.Warn("failed to subscribe to stream %s", err)
				continue
			}
			defer subStream.Unsubscribe()
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
			lg.Info("parent context is exiting, exiting evaluation loop")
			return
		case <-c.evaluationCtx.Done():
			lg.Info("evaluation context is exiting, exiting evaluation loop")
			return
		case msg := <-c.msgCh:
			var status T
			err := json.Unmarshal(msg.Data, &status)
			if err != nil {
				lg.Error("error", logger.Err(err))
			}
			healthy, md, ts := c.healthOnMessage(status)
			incomingState := alertingv1.CachedState{
				Healthy:   healthy,
				Firing:    c.IsFiring(),
				Timestamp: ts,
				Metadata:  md,
			}
			c.UpdateState(c.evaluationCtx, &incomingState)
			msg.Ack()
		}
	}
}

// infinite & blocking : must be run in a goroutine
func (c *InternalConditionEvaluator[T]) EvaluateLoop() {
	lg := logger.PluginLoggerFromContext(c.parentCtx)

	defer c.cancelEvaluation() // cancel parent context, if we return (non-recoverable)
	ticker := time.NewTicker(c.evaluateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.parentCtx.Done():
			lg.Info("parent context is exiting, exiting evaluation loop")
			return
		case <-c.evaluationCtx.Done():
			lg.Info("evaluation context is exiting, exiting evaluation loop")
			return
		case <-ticker.C:
			lastKnownState, err := c.stateStorage.Get(c.evaluationCtx, c.conditionId)
			if err != nil {
				lg.With("id", c.conditionId, "name", c.conditionName).Error(fmt.Sprintf("failed to get last internal condition state %s", err))
				continue
			}
			if !lastKnownState.Healthy {
				lg.Debug(fmt.Sprintf("condition %s is unhealthy", c.conditionName))
				interval := timestamppb.Now().AsTime().Sub(lastKnownState.Timestamp.AsTime())
				if interval > c.evaluateDuration { // then we must fire an alert
					if !c.IsFiring() {
						c.fingerprint = fingerprint.Default()
						c.SetFiring(true)
						err = c.UpdateState(c.evaluationCtx, &alertingv1.CachedState{
							Healthy:   lastKnownState.Healthy,
							Firing:    c.IsFiring(),
							Timestamp: timestamppb.Now(),
							Metadata:  lastKnownState.Metadata,
						})
						if err != nil {
							lg.Error("error", logger.Err(err))
						}
						err = c.incidentStorage.OpenInterval(c.evaluationCtx, c.conditionId, string(c.fingerprint), timestamppb.Now())
						if err != nil {
							lg.Error("error", logger.Err(err))
						}
					}
					alertLabels := map[string]string{
						message.NotificationPropertyFingerprint: string(c.fingerprint),
					}
					alertAnnotations := lo.Assign(
						alertLabels,
					)
					if lastKnownState.Metadata != nil {
						alertAnnotations = lo.Assign(
							alertAnnotations,
							lastKnownState.Metadata,
						)
					}

					lg.Debug(fmt.Sprintf("triggering alert for condition %s", c.conditionName))
					c.triggerHook(c.evaluationCtx, c.conditionId, alertLabels, alertAnnotations)
				}
			} else if lastKnownState.Healthy && c.IsFiring() &&
				// avoid potential noise from api streams & replays
				lastKnownState.Timestamp.AsTime().Add(-c.evaluateInterval).Before(time.Now()) {
				lg.Debug(fmt.Sprintf("condition %s is now healthy again after having fired", c.conditionName))
				c.SetFiring(false)
				err = c.incidentStorage.CloseInterval(c.evaluationCtx, c.conditionId, string(c.fingerprint), timestamppb.Now())
				if err != nil {
					lg.Error("error", logger.Err(err))
				}
				c.resolveHook(c.evaluationCtx, c.conditionId, map[string]string{
					message.NotificationPropertyFingerprint: string(c.fingerprint),
				}, map[string]string{
					message.NotificationPropertyFingerprint: string(c.fingerprint),
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
	if c.stateStorage.IsDiff(ctx, c.conditionId, s) {
		return c.stateStorage.Put(ctx, c.conditionId, s)
	}
	return nil
}

func (c *InternalConditionEvaluator[T]) CalculateInitialState() {
	lg := logger.PluginLoggerFromContext(c.parentCtx)

	incomingState := alertingv1.DefaultCachedState()
	if _, getErr := c.incidentStorage.Get(c.evaluationCtx, c.conditionId); getErr != nil {
		if status, ok := status.FromError(getErr); ok && status.Code() == codes.NotFound {
			err := c.incidentStorage.Put(c.evaluationCtx, c.conditionId, alertingv1.NewIncidentIntervals())
			if err != nil {
				lg.Error("error", logger.Err(err))
				c.cancelEvaluation()
				return
			}
		} else {
			c.cancelEvaluation()
			return
		}
	} else if getErr != nil {
		lg.Error("error", logger.Err(getErr))
	}
	if st, getErr := c.stateStorage.Get(c.evaluationCtx, c.conditionId); getErr != nil {
		if code, ok := status.FromError(getErr); ok && code.Code() == codes.NotFound {
			if err := c.stateStorage.Put(c.evaluationCtx, c.conditionId, incomingState); err != nil {
				c.cancelEvaluation()
				return
			}
		} else {
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
