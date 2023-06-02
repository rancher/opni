package alarms

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	notifications "github.com/rancher/opni/plugins/alerting/pkg/alerting/notifications/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/server"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"go.uber.org/zap"
)

type AlarmServerComponent struct {
	alertingv1.UnsafeAlertConditionsServer

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
}

func NewAlarmServerComponent(
	ctx context.Context,
	logger *zap.SugaredLogger,
	notifications *notifications.NotificationServerComponent,
) *AlarmServerComponent {
	return &AlarmServerComponent{
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
	}
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

func (a *AlarmServerComponent) Collectors() []prometheus.Collector {
	return []prometheus.Collector{}
}

func (a *AlarmServerComponent) Sync(ctx context.Context, shouldSync bool) error {
	conds, err := a.conditionStorage.Get().List(a.ctx)
	if err != nil {
		return err
	}
	iErrGroup := &util.MultiErrGroup{}
	iErrGroup.Add(len(conds))
	for _, cond := range conds {
		cond := cond
		go func() {
			defer iErrGroup.Done()
			if shouldSync {
				if _, err := a.setupCondition(ctx, a.logger, cond, cond.Id); err != nil {
					iErrGroup.AddError(err)
				}
			} else {
				if err := a.deleteCondition(ctx, a.logger, cond, cond.Id); err != nil {
					iErrGroup.AddError(err)
				}
			}
		}()
	}
	iErrGroup.Wait()
	if err := iErrGroup.Error(); err != nil {
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
