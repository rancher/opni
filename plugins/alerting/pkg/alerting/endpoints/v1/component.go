package endpoints

import (
	"context"
	"sync"

	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	notifications "github.com/rancher/opni/plugins/alerting/pkg/alerting/notifications/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/ops"
	"go.uber.org/zap"
)

type EndpointServerComponent struct {
	alertingv1.UnsafeAlertEndpointsServer

	ctx context.Context
	util.Initializer

	mu                   sync.Mutex
	clusterConfiguration struct{}

	notifications *notifications.NotificationServerComponent

	logger *zap.SugaredLogger

	endpointStorage  future.Future[storage.EndpointStorage]
	conditionStorage future.Future[storage.ConditionStorage]
	routerStorage    future.Future[storage.RouterStorage]
	opsNode          future.Future[*ops.AlertingOpsNode]
}

func NewEndpointServerComponent(
	ctx context.Context,
	logger *zap.SugaredLogger,
	notifications *notifications.NotificationServerComponent,
) *EndpointServerComponent {
	return &EndpointServerComponent{
		ctx:              ctx,
		logger:           logger,
		notifications:    notifications,
		endpointStorage:  future.New[storage.EndpointStorage](),
		conditionStorage: future.New[storage.ConditionStorage](),
		routerStorage:    future.New[storage.RouterStorage](),
		opsNode:          future.New[*ops.AlertingOpsNode](),
	}
}

type EndpointServerConfiguration struct {
	storage.EndpointStorage
	storage.ConditionStorage
	storage.RouterStorage
	OpsNode *ops.AlertingOpsNode
}

func (e *EndpointServerComponent) Status() struct{} {
	return struct{}{}
}

func (e *EndpointServerComponent) SetConfig(conf struct{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clusterConfiguration = conf
}

func (e *EndpointServerComponent) Sync(_ bool) error {
	return nil
}

func (e *EndpointServerComponent) Initialize(conf EndpointServerConfiguration) {
	e.InitOnce(func() {
		e.endpointStorage.Set(conf.EndpointStorage)
		e.conditionStorage.Set(conf.ConditionStorage)
		e.routerStorage.Set(conf.RouterStorage)
		e.opsNode.Set(conf.OpsNode)
		// e.notifications.Set(conf.notifications)
	})
}
