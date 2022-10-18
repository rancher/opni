package alerting

import (
	"context"
	"os"
	"time"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"

	lru "github.com/hashicorp/golang-lru"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
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
	<-p.Ctx.Done()
}

// UseKeyValueStore Alerting Condition & Alert Endpoints are stored in K,V stores
func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	var (
		nc  *nats.Conn
		err error
	)
	p.inMemCache, err = lru.New(AlertingLogCacheSize)
	if err != nil {
		p.inMemCache, _ = lru.New(AlertingLogCacheSize / 2)
	}
	p.storageNode = alertstorage.NewStorageNode(
		alertstorage.WithStorage(&alertstorage.StorageAPIs{
			Conditions: system.NewKVStoreClient[*alertingv1alpha.AlertCondition](client),
			Endpoints:  system.NewKVStoreClient[*alertingv1alpha.AlertEndpoint](client),
		}),
	)

	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(0),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(1*time.Minute),
		backoffv2.WithMultiplier(1.1),
	)
	b := retrier.Start(p.Ctx)
	for backoffv2.Continue(b) {
		nc, err = p.newNatsConnection()
		if err == nil {
			break
		}
		p.Logger.Error("failed to connect to nats, retrying")
	}
	p.natsConn.Set(nc)
	<-p.Ctx.Done()
}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {
	go func() {
		for {
			breakOut := false
			ccCortexAdmin, err := intf.GetClientConn(p.Ctx, "CortexAdmin")
			if err != nil {
				p.Logger.Errorf("alerting failed to get cortex admin client conn %s", err)
				p.adminClient = future.New[cortexadmin.CortexAdminClient]()
				UnregisterDatasource(shared.MonitoringDatasource)
			} else {
				adminClient := cortexadmin.NewCortexAdminClient(ccCortexAdmin)
				p.adminClient.Set(adminClient)
				RegisterDatasource(shared.MonitoringDatasource, NewAlertingMonitoringStore(p, p.Logger))
			}

			ccCortexOps, err := intf.GetClientConn(p.Ctx, "CortexOps")
			if err != nil {
				p.Logger.Errorf("alerting failed to get cortex ops client conn %s", err)
				p.cortexOpsClient = future.New[cortexops.CortexOpsClient]()
			} else {
				opsClient := cortexops.NewCortexOpsClient(ccCortexOps)
				p.cortexOpsClient.Set(opsClient)
			}

			select {
			case <-p.Ctx.Done():
				breakOut = true
			default:
				continue
			}
			if breakOut {
				break
			}
			time.Sleep(ApiExtensionBackoff)
		}
	}()
}
