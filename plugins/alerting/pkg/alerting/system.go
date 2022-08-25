package alerting

import (
	"context"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"os"
	"time"

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
		p.logger.With(
			"err", err,
		).Error("Failed to get mgmnt config")
		os.Exit(1)
	}
	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to load config")
		os.Exit(1)
	}
	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		opt := AlertingOptions{
			Endpoints:         config.Spec.Alerting.Endpoints,
			ConfigMap:         config.Spec.Alerting.ConfigMapName,
			Namespace:         config.Spec.Alerting.Namespace,
			StatefulSet:       config.Spec.Alerting.StatefulSetName,
			CortexHookHandler: config.Spec.Alerting.ManagementHookHandlerName,
		}
		p.alertingOptions.Set(opt)
	})
	<-p.ctx.Done()
}

// UseKeyValueStore Alerting Condition & Alert Endpoints are stored in K,V stores
func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	var err error
	p.inMemCache, err = lru.New(AlertingLogCacheSize)
	if err != nil {
		p.inMemCache, _ = lru.New(AlertingLogCacheSize / 2)
	}
	if os.Getenv(LocalBackendEnvToggle) != "" {
		p.endpointBackend.Set(&LocalEndpointBackend{
			configFilePath: LocalAlertManagerPath,
		})
	} else {
		p.endpointBackend.Set(&K8sEndpointBackend{})
	}

	p.storage.Set(StorageAPIs{
		Conditions:    system.NewKVStoreClient[*alertapi.AlertCondition](client),
		AlertEndpoint: system.NewKVStoreClient[*alertapi.AlertEndpoint](client),
	})
	<-p.ctx.Done()
}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {
	go func() {
		for {
			cc, err := intf.GetClientConn(p.ctx, "CortexAdmin")
			if err != nil {
				p.adminClient = future.New[cortexadmin.CortexAdminClient]()
				UnregisterDatasource(shared.MonitoringDatasource)
			} else {
				adminClient := cortexadmin.NewCortexAdminClient(cc)
				p.adminClient.Set(adminClient)
				RegisterDatasource(shared.MonitoringDatasource, NewAlertingMonitoringStore(p, p.logger))
			}
			select {
			case <-p.ctx.Done():
				break
			default:
				continue
			}
			time.Sleep(ApiExtensionBackoff)
		}
	}()
}
