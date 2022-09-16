package alerting

import (
	"context"
	"os"
	"time"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/util"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"

	lru "github.com/hashicorp/golang-lru"
	alertapi "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
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
		p.AlertingOptions.Set(opt)
	})
	<-p.Ctx.Done()
}

// UseKeyValueStore Alerting Condition & Alert Endpoints are stored in K,V stores
func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	var err error
	p.inMemCache, err = lru.New(AlertingLogCacheSize)
	if err != nil {
		p.inMemCache, _ = lru.New(AlertingLogCacheSize / 2)
	}
	if os.Getenv(shared.LocalBackendEnvToggle) != "" { // test mode
		b := &backend.LocalEndpointBackend{
			ConfigFilePath: shared.LocalAlertManagerPath,
		}
		go func() {
			// FIXME: management url is not correct
			err := shared.BackendDefaultFile("http://localhost:5001")
			if err != nil {
				panic(err)
			}
			b.Start(p.Ctx, p.Logger)
			peb := b.Port()
			opt := p.AlertingOptions.Get()
			opt.WorkerNodesService = "http://localhost"
			opt.ControllerNodeService = "http://localhost"
			opt.WorkerNodePort = peb
			opt.ControllerNodePort = peb
			opt.ControllerClusterPort = peb
			p.endpointBackend.Set(b)

			p.AlertingOptions = future.New[shared.NewAlertingOptions]()
			p.AlertingOptions.Set(opt)
		}()
	} else { // production mode
		client, err := util.NewK8sClient(util.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			// causes integration test to crash
			// panic(err)
		}
		p.endpointBackend.Set(&backend.K8sEndpointBackend{
			Client: client,
		})
	}

	p.storage.Set(StorageAPIs{
		Conditions:    system.NewKVStoreClient[*alertapi.AlertCondition](client),
		AlertEndpoint: system.NewKVStoreClient[*alertapi.AlertEndpoint](client),
	})
	<-p.Ctx.Done()
}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {
	go func() {
		for {
			breakOut := false
			cc, err := intf.GetClientConn(p.Ctx, "CortexAdmin")
			if err != nil {
				p.adminClient = future.New[cortexadmin.CortexAdminClient]()
				UnregisterDatasource(shared.MonitoringDatasource)
			} else {
				adminClient := cortexadmin.NewCortexAdminClient(cc)
				p.adminClient.Set(adminClient)
				RegisterDatasource(shared.MonitoringDatasource, NewAlertingMonitoringStore(p, p.Logger))
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
