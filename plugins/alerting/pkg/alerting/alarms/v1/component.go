package alarms

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/metrics"
	notifications "github.com/rancher/opni/plugins/alerting/pkg/alerting/notifications/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/server"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

var _ rules.RuleSyncServer = (*AlarmServerComponent)(nil)

type AlarmServerComponent struct {
	alertingv1.UnsafeAlertConditionsServer
	rules.UnsafeRuleSyncServer

	util.Initializer
	ctx context.Context

	mu sync.Mutex
	server.Config

	logger *zap.SugaredLogger

	runner        *Runner
	notifications *notifications.NotificationServerComponent

	conditionStorage future.Future[storage.ConditionStorage]
	incidentStorage  future.Future[storage.IncidentStorage]
	stateStorage     future.Future[storage.StateStorage]

	routerStorage future.Future[storage.RouterStorage]

	js future.Future[nats.JetStreamContext]

	mgmtClient      future.Future[managementv1.ManagementClient]
	adminClient     future.Future[cortexadmin.CortexAdminClient]
	cortexOpsClient future.Future[cortexops.CortexOpsClient]

	metricExporter *AlarmMetricsExporter
}

func NewAlarmServerComponent(
	ctx context.Context,
	logger *zap.SugaredLogger,
	notifications *notifications.NotificationServerComponent,
) *AlarmServerComponent {
	comp := &AlarmServerComponent{
		ctx:              ctx,
		logger:           logger,
		runner:           NewRunner(),
		notifications:    notifications,
		conditionStorage: future.New[storage.ConditionStorage](),
		incidentStorage:  future.New[storage.IncidentStorage](),
		stateStorage:     future.New[storage.StateStorage](),
		routerStorage:    future.New[storage.RouterStorage](),
		js:               future.New[nats.JetStreamContext](),
		mgmtClient:       future.New[managementv1.ManagementClient](),
		adminClient:      future.New[cortexadmin.CortexAdminClient](),
		cortexOpsClient:  future.New[cortexops.CortexOpsClient](),
		metricExporter:   NewAlarmMetricsExporter(),
	}
	return comp
}

type AlarmServerConfiguration struct {
	storage.ConditionStorage
	storage.IncidentStorage
	storage.StateStorage
	storage.RouterStorage
	Js              nats.JetStreamContext
	MgmtClient      managementv1.ManagementClient
	AdminClient     cortexadmin.CortexAdminClient
	CortexOpsClient cortexops.CortexOpsClient
}

var _ server.ServerComponent = (*AlarmServerComponent)(nil)

func (a *AlarmServerComponent) Name() string {
	return "alarm"
}

func (a *AlarmServerComponent) Status() server.Status {
	return server.Status{
		Running: a.Initialized(),
	}
}

func (a *AlarmServerComponent) Ready() bool {
	return a.Initialized()
}

func (a *AlarmServerComponent) Healthy() bool {
	return a.Initialized()
}

func (a *AlarmServerComponent) SetConfig(conf server.Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Config = conf
}

func (a *AlarmServerComponent) Sync(ctx context.Context, shouldSync bool) error {
	conditionStorage, err := a.conditionStorage.GetContext(ctx)
	if err != nil {
		return err
	}
	conds, err := conditionStorage.List(ctx)
	if err != nil {
		return err
	}
	eg := &util.MultiErrGroup{}
	for _, cond := range conds {
		cond := cond
		if shouldSync {
			eg.Go(func() error {
				activationAttrs := []attribute.KeyValue{
					{
						Key:   "cluster_id",
						Value: attribute.StringValue(cond.GetClusterId().Id),
					},
					{
						Key:   "name",
						Value: attribute.StringValue(cond.GetName()),
					},
					{
						Key:   "datasource",
						Value: attribute.StringValue(cond.DatasourceName()),
					},
					{
						Key:   "id",
						Value: attribute.StringValue(cond.GetId()),
					},
				}
				_, err := a.applyAlarm(ctx, cond, cond.Id)
				if err != nil {
					activationAttrs = append(activationAttrs, attribute.KeyValue{
						Key:   "status",
						Value: attribute.StringValue("failed"),
					})
				} else {
					activationAttrs = append(activationAttrs, attribute.KeyValue{
						Key:   "status",
						Value: attribute.StringValue("success"),
					})
				}
				a.metricExporter.activationStatusCounter.Add(ctx, 1, metric.WithAttributes(
					activationAttrs...,
				))
				return err
			})
		} else {
			eg.Go(func() error {
				return a.teardownCondition(ctx, cond, cond.Id, false)
			})
		}
	}
	eg.Wait()
	if err := eg.Error(); err != nil {
		return err
	}
	return nil
}

func (a *AlarmServerComponent) Initialize(conf AlarmServerConfiguration) {
	a.InitOnce(func() {
		a.mgmtClient.Set(conf.MgmtClient)
		a.adminClient.Set(conf.AdminClient)
		a.js.Set(conf.Js)
		a.cortexOpsClient.Set(conf.CortexOpsClient)
		a.conditionStorage.Set(conf.ConditionStorage)
		a.incidentStorage.Set(conf.IncidentStorage)
		a.stateStorage.Set(conf.StateStorage)
		a.routerStorage.Set(conf.RouterStorage)
	})
}

type AlarmMetricsExporter struct {
	activationStatusCounter metric.Int64Counter
}

func NewAlarmMetricsExporter() *AlarmMetricsExporter {
	return &AlarmMetricsExporter{
		activationStatusCounter: metrics.ActivationStatusCounter,
	}
}
