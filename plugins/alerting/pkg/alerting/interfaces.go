package alerting

import (
	"context"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync"
)

var AlertingBackends = map[alertingv1alpha.AlertType]AlertTypeBackend{}

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
	Create(p *Plugin, req alertingv1alpha.AlertCondition) (*corev1.Reference, error)
	Update(p *Plugin, req alertingv1alpha.AlertConditionWithId) (*emptypb.Empty, error)
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
