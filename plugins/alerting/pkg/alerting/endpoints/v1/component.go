package endpoints

import (
	"context"
	"sync"

	"github.com/rancher/opni/pkg/alerting/server"
	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	notifications "github.com/rancher/opni/plugins/alerting/pkg/alerting/notifications/v1"
	"go.uber.org/zap"
)

type EndpointServerComponent struct {
	alertingv1.UnsafeAlertEndpointsServer

	ctx context.Context
	util.Initializer

	mu sync.Mutex
	server.Config

	notifications *notifications.NotificationServerComponent
	ManualSync    func(ctx context.Context, routerKeys []string, routers spec.RouterStorage)

	logger *zap.SugaredLogger

	endpointStorage  future.Future[spec.EndpointStorage]
	conditionStorage future.Future[spec.ConditionStorage]
	routerStorage    future.Future[spec.RouterStorage]
}

var _ server.ServerComponent = (*EndpointServerComponent)(nil)

func NewEndpointServerComponent(
	ctx context.Context,
	logger *zap.SugaredLogger,
	notifications *notifications.NotificationServerComponent,
) *EndpointServerComponent {
	return &EndpointServerComponent{
		ctx:              ctx,
		logger:           logger,
		notifications:    notifications,
		endpointStorage:  future.New[spec.EndpointStorage](),
		conditionStorage: future.New[spec.ConditionStorage](),
		routerStorage:    future.New[spec.RouterStorage](),
	}
}

type EndpointServerConfiguration struct {
	spec.EndpointStorage
	spec.ConditionStorage
	spec.RouterStorage

	ManualSync func(ctx context.Context, routerKeys []string, routers spec.RouterStorage)
}

func (e *EndpointServerComponent) Name() string {
	return "endpoint"
}

func (e *EndpointServerComponent) Status() server.Status {
	return server.Status{
		Running: e.Initialized(),
	}
}

func (e *EndpointServerComponent) Ready() bool {
	return e.Initialized()
}

func (e *EndpointServerComponent) Healthy() bool {
	return e.Initialized()
}

func (e *EndpointServerComponent) SetConfig(conf server.Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Config = conf
}

func (e *EndpointServerComponent) Sync(_ context.Context, _ alertingSync.SyncInfo) error {
	return nil
}

func (e *EndpointServerComponent) Initialize(conf EndpointServerConfiguration) {
	e.InitOnce(func() {
		e.endpointStorage.Set(conf.EndpointStorage)
		e.conditionStorage.Set(conf.ConditionStorage)
		e.routerStorage.Set(conf.RouterStorage)
		e.ManualSync = conf.ManualSync
	})
}
