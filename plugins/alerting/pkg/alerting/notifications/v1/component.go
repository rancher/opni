package notifications

import (
	"sync"

	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/ops"
	"go.uber.org/zap"
)

type NotificationServerComponent struct {
	alertingv1.UnsafeAlertNotificationsServer

	util.Initializer

	mu                   sync.Mutex
	clusterConfiguration struct{}

	logger *zap.SugaredLogger

	opsNode          future.Future[*ops.AlertingOpsNode]
	conditionStorage future.Future[storage.ConditionStorage]
}

func NewNotificationServerComponent(
	logger *zap.SugaredLogger,
) *NotificationServerComponent {
	return &NotificationServerComponent{
		logger:           logger,
		opsNode:          future.New[*ops.AlertingOpsNode](),
		conditionStorage: future.New[storage.ConditionStorage](),
	}
}

type NotificationServerConfiguration struct {
	storage.ConditionStorage
	OpsNode *ops.AlertingOpsNode
}

func (n *NotificationServerComponent) Status() struct{} {
	return struct{}{}
}

func (e *NotificationServerComponent) SetConfig(conf struct{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clusterConfiguration = conf
}

func (n *NotificationServerComponent) Sync(_ bool) error {
	return nil
}

func (n *NotificationServerComponent) Initialize(conf NotificationServerConfiguration) {
	n.InitOnce(func() {
		n.conditionStorage.Set(conf.ConditionStorage)
		n.opsNode.Set(conf.OpsNode)
	})
}
