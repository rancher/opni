package alerting

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/messaging"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/ops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"go.uber.org/zap"

	lru "github.com/hashicorp/golang-lru"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/condition"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/endpoint"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/log"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/trigger"
)

const AlertingLogCacheSize = 32

type Plugin struct {
	system.UnimplementedSystemPluginClient
	condition.UnsafeAlertConditionsServer
	endpoint.UnsafeAlertEndpointsServer
	log.UnsafeAlertLogsServer
	trigger.UnsafeAlertingServer

	Ctx        context.Context
	Logger     *zap.SugaredLogger
	inMemCache *lru.Cache

	opsNode     *ops.AlertingOpsNode
	msgNode     *messaging.MessagingNode
	storageNode *alertstorage.StorageNode

	mgmtClient      future.Future[managementv1.ManagementClient]
	adminClient     future.Future[cortexadmin.CortexAdminClient]
	cortexOpsClient future.Future[cortexops.CortexOpsClient]
	natsConn        future.Future[*nats.Conn]
}

type StorageAPIs struct {
	Conditions    storage.KeyValueStoreT[*alertingv1alpha.AlertCondition]
	AlertEndpoint storage.KeyValueStoreT[*alertingv1alpha.AlertEndpoint]
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("alerting")
	clusterDriver := future.New[drivers.ClusterDriver]()
	p := &Plugin{
		Ctx:        ctx,
		Logger:     lg,
		inMemCache: nil,

		opsNode: ops.NewAlertingOpsNode(clusterDriver),
		msgNode: messaging.NewMessagingNode(future.New[*nats.Conn]()),
		storageNode: alertstorage.NewStorageNode(
			alertstorage.WithLogger(lg),
		),

		mgmtClient:      future.New[managementv1.ManagementClient](),
		adminClient:     future.New[cortexadmin.CortexAdminClient](),
		cortexOpsClient: future.New[cortexops.CortexOpsClient](),
		natsConn:        future.New[*nats.Conn](),
	}
	// TODO : restore all system conditions to load the active goroutines/channels back into memory
	return p
}

func (p *Plugin) onSystemConditionUpdate(ctx context.Context, condition *alertingv1alpha.AlertCondition) {
	lg := p.Logger.With("onSystemConditionUpdate", condition.Name)
	lg.Debugf("received condition update: %v", condition)
	if s := condition.GetAlertType().GetSystem(); s == nil {
		lg.Error("non system-alert type condition received in system condition update")
	}
	for {
		ctxCa, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel() // in case we exit early shouldn't leak the context
		nc, err := p.natsConn.GetContext(ctxCa)
		if err != nil {
			lg.Errorw("failed to get nats connection", "error", err)
			cancel()
			continue
		}
		defer nc.Close()
		// TODO : acquire jetstream stream

	}
}

var _ endpoint.AlertEndpointsServer = (*Plugin)(nil)
var _ condition.AlertConditionsServer = (*Plugin)(nil)
var _ log.AlertLogsServer = (*Plugin)(nil)
var _ trigger.AlertingServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	// We omit the trigger server here since it should only be used internally
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(
			util.PackService(
				&condition.AlertConditions_ServiceDesc,
				p,
			),
			util.PackService(
				&endpoint.AlertEndpoints_ServiceDesc,
				p,
			),
			util.PackService(
				&log.AlertLogs_ServiceDesc,
				p,
			),
			util.PackService(
				&alertops.AlertingAdmin_ServiceDesc,
				p.opsNode,
			),
		),
	)
	return scheme
}
