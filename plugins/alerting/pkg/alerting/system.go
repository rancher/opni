package alerting

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alarms/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/node_backend"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"

	"github.com/nats-io/nats.go"
	alertingClient "github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingStorage "github.com/rancher/opni/pkg/alerting/storage"

	"github.com/rancher/opni/pkg/alerting/server"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/storage/kvutil"
)

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
	lg := logger.PluginLoggerFromContext(p.ctx)

	opt := &shared.AlertingClusterOptions{}
	p.mgmtClient.Set(client)
	cfg, err := client.GetConfig(context.Background(),
		&emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		lg.With(
			"err", err,
		).Error("Failed to get mgmnt config")
		os.Exit(1)
	}
	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		lg.With(
			"err", err,
		).Error("failed to load config")
		os.Exit(1)
	}
	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		p.gatewayConfig.Set(config)
		backend, err := machinery.ConfigureStorageBackend(p.ctx, &config.Spec.Storage)
		if err != nil {
			lg.With(logger.Err(err)).Error("failed to configure storage backend")
			os.Exit(1)
		}
		p.storageBackend.Set(backend)
		opt = &shared.AlertingClusterOptions{
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

	})
	tlsConfig := p.loadCerts()
	p.configureDriver(
		p.ctx,
		driverutil.NewOption("alertingOptions", opt),
		driverutil.NewOption("logger", lg.WithGroup("alerting-manager")),
		driverutil.NewOption("subscribers", []chan alertingClient.AlertingClient{p.clusterNotifier}),
		driverutil.NewOption("tlsConfig", tlsConfig),
	)
	p.alertingTLSConfig.Set(tlsConfig)
	go p.handleDriverNotifications()
	go p.runSync()
	p.useWatchers(client)
	<-p.ctx.Done()
}

// UseKeyValueStore Alerting Condition & Alert Endpoints are stored in K,V stores
func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	lg := logger.PluginLoggerFromContext(p.ctx)

	p.capabilitySpecStore.Set(node_backend.CapabilitySpecKV{
		DefaultCapabilitySpec: kvutil.WithKey(system.NewKVStoreClient[*node.AlertingCapabilitySpec](client), "/alerting/config/capability/default"),
		NodeCapabilitySpecs:   kvutil.WithPrefix(system.NewKVStoreClient[*node.AlertingCapabilitySpec](client), "/alerting/config/capability/nodes"),
	})

	var (
		nc  *nats.Conn
		err error
	)

	cfg := p.gatewayConfig.Get().Spec.Storage.JetStream
	natsURL := os.Getenv("NATS_SERVER_URL")
	natsSeedPath := os.Getenv("NKEY_SEED_FILENAME")
	if cfg == nil {
		cfg = &v1beta1.JetStreamStorageSpec{}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = natsURL
	}
	if cfg.NkeySeedPath == "" {
		cfg.NkeySeedPath = natsSeedPath
	}
	nc, err = natsutil.AcquireNATSConnection(
		p.ctx,
		cfg,
		natsutil.WithLogger(lg),
		natsutil.WithNatsOptions([]nats.Option{
			nats.ErrorHandler(func(nc *nats.Conn, s *nats.Subscription, err error) {
				if s != nil {
					lg.Error("nats : async error in %q/%q: %v", s.Subject, s.Queue, err)
				} else {
					lg.Warn("nats : async error outside subscription")
				}
			}),
		}),
	)
	if err != nil {
		lg.With(logger.Err(err)).Error("fatal error connecting to NATs")
	}
	p.natsConn.Set(nc)
	mgr, err := p.natsConn.Get().JetStream()
	if err != nil {
		panic(err)
	}
	p.js.Set(mgr)
	b := alertingStorage.NewDefaultAlertingBroker(mgr)
	p.storageClientSet.Set(b.NewClientSet())
	// spawn a reindexing task
	go func() {
		err := p.storageClientSet.Get().ForceSync(p.ctx)
		if err != nil {
			panic(err)
		}
		clStatus, err := p.GetClusterStatus(p.ctx, &emptypb.Empty{})
		if err != nil {
			lg.With(logger.Err(err)).Error("failed to get cluster status")
			return
		}
		if clStatus.State == alertops.InstallState_Installed || clStatus.State == alertops.InstallState_InstallUpdating {
			syncInfo, err := p.getSyncInfo(p.ctx)
			if err != nil {
				lg.With(logger.Err(err)).Error("failed to get sync info")
			} else {
				for _, comp := range p.Components() {
					comp.Sync(p.ctx, syncInfo)
				}
			}
			conf, err := p.GetClusterConfiguration(p.ctx, &emptypb.Empty{})
			if err != nil {
				lg.With(logger.Err(err)).Error("failed to get cluster configuration")
				return
			}
			peers := listPeers(int(conf.GetNumReplicas()))
			lg.Info(fmt.Sprintf("reindexing known alerting peers to : %v", peers))
			ctxca, ca := context.WithTimeout(context.Background(), 5*time.Second)
			defer ca()
			alertingClient, err := p.alertingClient.GetContext(ctxca)
			if err != nil {
				lg.Error(err.Error())
				return
			}

			alertingClient.MemberlistClient().SetKnownPeers(peers)
			for _, comp := range p.Components() {
				comp.SetConfig(server.Config{
					Client: alertingClient,
				})
			}
		}
	}()
	<-p.ctx.Done()
}

func UseCachingProvider(c caching.CachingProvider[proto.Message]) {
	c.SetCache(caching.NewInMemoryGrpcTtlCache(50*1024*1024, time.Minute))
}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {
	lg := logger.PluginLoggerFromContext(p.ctx)

	services := []string{"CortexAdmin", "CortexOps"}
	cc, err := intf.GetClientConn(p.ctx, services...)
	if err != nil {
		lg.With(logger.Err(err)).Error(fmt.Sprintf("failed to get required clients for alerting : %s", strings.Join(services, ",")))
		if p.ctx.Err() != nil {
			// Plugin is shutting down, don't exit
			return
		}
		os.Exit(1)
	}
	p.adminClient.Set(cortexadmin.NewCortexAdminClient(cc))
	p.cortexOpsClient.Set(cortexops.NewCortexOpsClient(cc))
}

func (p *Plugin) handleDriverNotifications() {
	lg := logger.PluginLoggerFromContext(p.ctx)

	for {
		select {
		case <-p.ctx.Done():
			lg.Info("shutting down cluster driver update handler")
			return
		case client := <-p.clusterNotifier:
			lg.Info("updating alerting client based on cluster status")
			serverCfg := server.Config{
				Client: client.Clone(),
			}
			for _, comp := range p.Components() {
				comp.SetConfig(serverCfg)
			}
		}
	}
}

func (p *Plugin) useWatchers(client managementv1.ManagementClient) {
	cw := p.newClusterWatcherHooks(p.ctx, alarms.NewAgentStream())
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
			ApiAddress:      fmt.Sprintf("%s-%d.%s:9093", shared.AlertmanagerService, i, shared.AlertmanagerService),
			EmbeddedAddress: fmt.Sprintf("%s-%d.%s:3000", shared.AlertmanagerService, i, shared.AlertmanagerService),
		})
	}
	return peers
}
