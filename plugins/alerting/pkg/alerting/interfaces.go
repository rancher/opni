package alerting

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type EvaluatorOptions struct {
	js          nats.JetStreamContext
	storageNode *alertstorage.StorageNode
}

type EvaluatorOption func(*EvaluatorOptions)

func WithStorageNode(storageNode *alertstorage.StorageNode) EvaluatorOption {
	return func(o *EvaluatorOptions) {
		o.storageNode = storageNode
	}
}

func WithJetStream(js nats.JetStreamContext) EvaluatorOption {
	return func(o *EvaluatorOptions) {
		o.js = js
	}
}

func (o *EvaluatorOptions) apply(opts ...EvaluatorOption) {
	for _, opt := range opts {
		opt(o)
	}
}

type ConditionEvaluator interface {
	Create(conditionId string, cond *alertingv1.AlertCondition)
	Delete(conditionId string, cond *alertingv1.AlertCondition)
	// the returned cancel func should not be a derivative of parentCtx
	Evaluate(parentCtx, ctxWithCancel context.Context, opts ...EvaluatorOptions)
	Ingester(parentCtx, ctxWithCancel context.Context, opts ...EvaluatorOptions)
}

type InternalConditionWatcher interface {
	WatchEvents()
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

type EvaluationContext interface {
	Spawn(...InternalConditionWatcher)
	ReEntrantIndexing(evals ...ConditionEvaluator)
}

var AlertingBackends = map[alertingv1.AlertType]AlertTypeBackend{}

var mu sync.Mutex
var datasourceToAlertingCapability map[string]AlertingStore = make(map[string]AlertingStore)

func RegisterDatasource(datasource string, store AlertingStore) {
	defer mu.Unlock()
	mu.Lock()
	datasourceToAlertingCapability[datasource] = store
}

func UnregisterDatasource(datasource string) {
	defer mu.Unlock()
	mu.Lock()
	delete(datasourceToAlertingCapability, datasource)
}

type AlertingStore interface {
}

type AlertTypeBackend interface {
	// ListTemplates List templates available for this alert type
	ListTemplates() []TemplateInfo
	// ListChoices List choices for values to be filled in
	ListChoices() error
	Create(p *Plugin, req alertingv1.AlertCondition) (*corev1.Reference, error)
	Update(p *Plugin, req alertingv1.AlertConditionWithId) (*emptypb.Empty, error)
	Delete(p *Plugin, req *corev1.Reference) (*emptypb.Empty, error)

	WithCurrentRequest(req proto.Message, ctx context.Context) AlertTypeBackend
}

type TemplateInfo struct {
	Name        string
	Description string
}

type RequestBase struct {
	req proto.Message
	p   *Plugin
	ctx context.Context
	lg  *zap.SugaredLogger
}

type AlertingMonitoring struct {
	RequestBase
}

func NewAlertingMonitoringStore(p *Plugin, lg *zap.SugaredLogger) *AlertingMonitoring {
	return &AlertingMonitoring{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.Background(),
			lg:  lg,
		},
	}
}

func NewAlertTypeSystemBackend(p *Plugin, lg *zap.SugaredLogger) AlertTypeBackend {
	return &AlertTypeSystemBackend{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.Background(),
			lg:  lg,
		},
	}
}

func (a *AlertTypeSystemBackend) WithCurrentRequest(req proto.Message, ctx context.Context) AlertTypeBackend {
	a.req = req
	a.ctx = ctx
	return a
}

func NewAlertTypeKubeMetricsBackend(p *Plugin, lg *zap.SugaredLogger) AlertTypeBackend {
	return &AlertTypeKubeMetricsBackend{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.Background(),
			lg:  lg,
		},
	}
}

func (a *AlertTypeKubeMetricsBackend) WithCurrentRequest(req proto.Message, ctx context.Context) AlertTypeBackend {
	a.req = req
	a.ctx = ctx
	return a
}
