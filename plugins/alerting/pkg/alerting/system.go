package alerting

import (
	"context"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

const ApiExtensionBackoff = time.Second * 5

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
	p.mgmtClient.Set(client)
	cfg, err := client.GetConfig(context.Background(),
		&emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		p.Logger.With(
			"err", err,
		).Error("Failed to get mgmnt config")
		os.Exit(1)
	}
	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.Logger.With(
			"err", err,
		).Error("failed to load config")
		os.Exit(1)
	}
	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		opt := shared.NewAlertingOptions{
			Namespace:             config.Spec.Alerting.Namespace,
			WorkerNodesService:    config.Spec.Alerting.WorkerNodeService,
			WorkerNodePort:        config.Spec.Alerting.WorkerPort,
			WorkerStatefulSet:     config.Spec.Alerting.WorkerStatefulSet,
			ControllerNodeService: config.Spec.Alerting.ControllerNodeService,
			ControllerNodePort:    config.Spec.Alerting.ControllerNodePort,
			ControllerClusterPort: config.Spec.Alerting.ControllerClusterPort,
			ConfigMap:             config.Spec.Alerting.ConfigMap,
			ManagementHookHandler: config.Spec.Alerting.ManagementHookHandler,
		}
		if os.Getenv(shared.LocalBackendEnvToggle) != "" {
			opt.WorkerNodesService = "http://localhost"
			opt.ControllerNodeService = "http://localhost"

		}

		p.configureAlertManagerConfiguration(
			p.Ctx,
			drivers.WithLogger(p.Logger.Named("alerting-manager")),
			drivers.WithAlertingOptions(&opt),
			drivers.WithManagementClient(client),
		)
	})
	p.UseWatchers(client)
	<-p.Ctx.Done()
}

func (p *Plugin) UseWatchers(client managementv1.ManagementClient) {
	cw := p.newClusterWatcherHooks(p.Ctx, NewAgentStream())
	clusterCrud, clusterHealthStatus, cortexBackendStatus :=
		func() { p.watchGlobalCluster(client, cw) },
		func() { p.watchGlobalClusterHealthStatus(client, NewAgentStream()) },
		func() { p.watchCortexClusterStatus() }

	p.globalWatchers = NewSimpleInternalConditionWatcher(
		clusterCrud,
		clusterHealthStatus,
		cortexBackendStatus,
	)
	p.globalWatchers.WatchEvents()
}

// UseKeyValueStore Alerting Condition & Alert Endpoints are stored in K,V stores
func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	var (
		nc  *nats.Conn
		err error
	)
	nc, err = natsutil.AcquireNATSConnection(
		p.Ctx,
		natsutil.WithLogger(p.Logger),
		natsutil.WithNatsOptions([]nats.Option{
			nats.ErrorHandler(func(nc *nats.Conn, s *nats.Subscription, err error) {
				if s != nil {
					p.Logger.Error("nats : async error in %q/%q: %v", s.Subject, s.Queue, err)
				} else {
					p.Logger.Warn("nats : async error outside subscription")
				}
			}),
		}),
	)
	if err != nil {
		p.Logger.With("err", err).Error("fatal error connecting to NATs")
	}
	p.natsConn.Set(nc)
	mgr, err := p.natsConn.Get().JetStream()
	if err != nil {
		panic(err)
	}
	p.js.Set(mgr)
	storageNode := alertstorage.NewStorageNode(
		alertstorage.NewStorageAPIs(mgr, time.Hour*24),
	)
	p.storageNode.Set(storageNode)
	// spawn a reindexing task
	go func() {
		p.reindexAlarms()
	}()
	<-p.Ctx.Done()
}

func (p *Plugin) reindexAlarms() {
	lg := p.Logger.With("re-indexing", "in-progress")
	conditions, err := p.storageNode.Get().Conditions.List(p.Ctx, alertstorage.WithUnredacted())
	if err != nil {
		lg.With("err", err).Error("failed to list alert conditions")
		return
	}
	for _, cond := range conditions {
		if s := cond.GetAlertType().GetSystem(); s != nil {
			// this checks that we won't crash when importing existing conditions from versions < 0.6
			if s.GetClusterId() != nil && s.GetTimeout() != nil {
				lg.Debug("re-indexing agent disconnect")
				p.onSystemConditionCreate(cond.Id, cond.Name, s)
			} else {
				// delete invalid conditions that won't do anything
				_, err := p.DeleteAlertCondition(p.Ctx, &corev1.Reference{Id: cond.Id})
				if err != nil {
					lg.With("err", err).Error("failed to delete invalid condition")
				}
			}
		}
		if s := cond.GetAlertType().GetDownstreamCapability(); s != nil {
			lg.Debug("re-indexing downstream capability")
			p.onDownstreamCapabilityConditionCreate(cond.Id, cond.Name, s)
		}
		if mc := cond.GetAlertType().GetMonitoringBackend(); mc != nil {
			lg.Debug("re-indexing monitoring backend")
			p.onCortexClusterStatusCreate(cond.Id, cond.Name, mc)
		}
	}
	lg.Info("re-indexing alarms complete")
}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {
	ccCortexAdmin, err := intf.GetClientConn(p.Ctx, "CortexAdmin")
	if err != nil {
		p.Logger.With("err", err).Error("failed to get cortex admin client")
		os.Exit(1)
	}
	p.adminClient.Set(cortexadmin.NewCortexAdminClient(ccCortexAdmin))
	ccCortexOps, err := intf.GetClientConn(p.Ctx, "CortexOps")
	if err != nil {
		p.Logger.With("err", err).Error("failed to get cortex ops client")
		os.Exit(1)
	}
	p.cortexOpsClient.Set(cortexops.NewCortexOpsClient(ccCortexOps))
}
