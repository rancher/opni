package notifications

import (
	"context"
	"sync"

	"github.com/rancher/opni/pkg/alerting/server"
	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"log/slog"
)

type NotificationServerComponent struct {
	alertingv1.UnsafeAlertNotificationsServer

	util.Initializer

	mu sync.Mutex
	server.Config

	logger *slog.Logger

	conditionStorage future.Future[spec.ConditionStorage]
	endpointStorage  future.Future[spec.EndpointStorage]
}

var _ server.ServerComponent = (*NotificationServerComponent)(nil)

func NewNotificationServerComponent(
	logger *slog.Logger,
) *NotificationServerComponent {
	return &NotificationServerComponent{
		logger:           logger,
		conditionStorage: future.New[spec.ConditionStorage](),
		endpointStorage:  future.New[spec.EndpointStorage](),
	}
}

type NotificationServerConfiguration struct {
	spec.ConditionStorage
	spec.EndpointStorage
}

func (n *NotificationServerComponent) Name() string {
	return "notification"
}

func (n *NotificationServerComponent) Status() server.Status {
	return server.Status{
		Running: n.Initialized(),
	}
}

func (n *NotificationServerComponent) Ready() bool {
	return n.Initialized()
}

func (n *NotificationServerComponent) Healthy() bool {
	return n.Initialized()
}

func (n *NotificationServerComponent) SetConfig(conf server.Config) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Config = conf
}

func (n *NotificationServerComponent) Sync(_ context.Context, _ alertingSync.SyncInfo) error {
	return nil
}

func (n *NotificationServerComponent) Initialize(conf NotificationServerConfiguration) {
	n.InitOnce(func() {
		n.conditionStorage.Set(conf.ConditionStorage)
		n.endpointStorage.Set(conf.EndpointStorage)
	})
}
