package alerting

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alarms/v1"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"

	"github.com/nats-io/nats.go"
	alertingClient "github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage/broker_init"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alarms/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/server"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

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
		opt := &shared.AlertingClusterOptions{
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
		p.configureDriver(p.Ctx,
			driverutil.NewOption("alertingOptions", opt),
			driverutil.NewOption("logger", p.Logger.Named("alerting-manager")),
			driverutil.NewOption("subscribers", []chan []alertingClient.AlertingPeer{p.clusterNotifier}),
		)
	})
	go p.handleDriverNotifications()
	p.useWatchers(client)
	<-p.Ctx.Done()
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
			for _, comp := range p.Components() {
				comp.Sync(true)
			}
			conf, err := p.opsNode.GetClusterConfiguration(p.Ctx, &emptypb.Empty{})
			if err != nil {
				p.Logger.With("err", err).Error("failed to get cluster configuration")
				return
			}
			peers := listPeers(int(conf.GetNumReplicas()))
			p.Logger.Infof("reindexing known alerting peers to : %v", peers)
			p.Client.SetKnownPeers(peers)
			for _, comp := range p.Components() {
				comp.SetConfig(server.Config{
					Client: p.Client,
				})
			}
		}
	}()
	<-p.Ctx.Done()
}

func UseCachingProvider(c caching.CachingProvider[proto.Message]) {
	c.SetCache(caching.NewInMemoryGrpcTtlCache(50*1024*1024, time.Minute))
}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {
	services := []string{"CortexAdmin", "CortexOps"}
	cc, err := intf.GetClientConn(p.Ctx, services...)
	if err != nil {
		p.Logger.With("err", err).Error("failed to get required clients for alerting : %s", strings.Join(services, ","))
		os.Exit(1)
	}
	p.adminClient.Set(cortexadmin.NewCortexAdminClient(cc))
	p.cortexOpsClient.Set(cortexops.NewCortexOpsClient(cc))
}

func (p *Plugin) handleDriverNotifications() {
	for {
		select {
		case <-p.Ctx.Done():
			p.Logger.Info("shutting down cluster driver update handler")
			return
		case knownPeers := <-p.clusterNotifier:
			p.Logger.Infof("updating known peers : %v", knownPeers)
			p.Client.SetKnownPeers(knownPeers)
			serverCfg := server.Config{
				Client: p.Client,
			}
			for _, comp := range p.Components() {
				comp.SetConfig(serverCfg)
			}
		}
	}
}

func (p *Plugin) useWatchers(client managementv1.ManagementClient) {
	cw := p.newClusterWatcherHooks(p.Ctx, alarms.NewAgentStream())
	clusterCrud, clusterHealthStatus, cortexBackendStatus := func() { p.watchGlobalCluster(client, cw) },
		func() { p.watchGlobalClusterHealthStatus(client, alarms.NewAgentStream()) },
		func() { p.watchCortexClusterStatus() }

	p.globalWatchers = management.NewConditionWatcher(
		clusterCrud,
		clusterHealthStatus,
		cortexBackendStatus,
	)
	p.globalWatchers.WatchEvents()
}

func listPeers(replicas int) []alertingClient.AlertingPeer {
	peers := []alertingClient.AlertingPeer{}
	for i := 0; i < replicas; i++ {
		peers = append(peers, alertingClient.AlertingPeer{
			ApiAddress:      fmt.Sprintf("http://%s-%d.%s:9093", shared.OperatorAlertingClusterNodeServiceName, i, shared.OperatorAlertingClusterNodeServiceName),
			EmbeddedAddress: fmt.Sprintf("http://%s-%d.%s:3000", shared.OperatorAlertingClusterNodeServiceName, i, shared.OperatorAlertingClusterNodeServiceName),
		})
	}
	return peers
}
