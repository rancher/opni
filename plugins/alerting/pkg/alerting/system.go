package alerting

import (
	"context"
	"github.com/rancher/opni/pkg/management"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage/broker_init"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
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
		opt := shared.AlertingClusterOptions{
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
		p.configureAlertManagerConfiguration(
			p.Ctx,
			drivers.WithAlertingRuntimeOptions(&opt),
			drivers.WithLogger(p.Logger.Named("alerting-manager")),
			drivers.WithSubscribers([]chan shared.AlertingClusterNotification{
				p.clusterNotifier,
			}),
		)
	})
	go func() {
		for {
			select {
			case <-p.Ctx.Done():
				return
			case incomingState := <-p.clusterNotifier:
				if !p.enabledBool.Load() && incomingState.A {
					err := p.reindex(p.Ctx)
					if err != nil {
						p.Logger.With("err", err).Error("failed to reindex")
					}
				} else if !incomingState.A {
					err := p.teardown(p.Ctx)
					if err != nil {
						p.Logger.With("err", err).Error("failed to teardown")
					}
				}
				p.enabledBool.Store(incomingState.A)
			}
		}
	}()
	p.UseWatchers(client)
	<-p.Ctx.Done()
}

func (p *Plugin) UseWatchers(client managementv1.ManagementClient) {
	cw := p.newClusterWatcherHooks(p.Ctx, NewAgentStream())
	clusterCrud, clusterHealthStatus, cortexBackendStatus :=
		func() { p.watchGlobalCluster(client, cw) },
		func() { p.watchGlobalClusterHealthStatus(client, NewAgentStream()) },
		func() { p.watchCortexClusterStatus() }

	p.globalWatchers = management.NewConditionWatcher(
		clusterCrud,
		clusterHealthStatus,
		cortexBackendStatus,
	)
	p.globalWatchers.WatchEvents()
}

// UseKeyValueStore Alerting Condition & Alert Endpoints are stored in K,V stores
func (p *Plugin) UseKeyValueStore(_ system.KeyValueStoreClient) {
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
	b := broker_init.NewDefaultAlertingBroker(mgr)
	p.storageClientSet.Set(b.NewClientSet())
	// spawn a reindexing task
	go func() {
		err := p.storageClientSet.Get().ForceSync(p.Ctx)
		if err != nil {
			panic(err)
		}
		status, err := p.opsNode.GetClusterStatus(p.Ctx, &emptypb.Empty{})
		if err != nil {
			p.Logger.With("err", err).Error("failed to get cluster status")
			return
		}
		if status.State == alertops.InstallState_Installed {
			p.reindex(p.Ctx)
		}
	}()
	<-p.Ctx.Done()
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
