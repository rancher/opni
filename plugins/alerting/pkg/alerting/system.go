package alerting

import (
	"context"
	"fmt"
	"os"
	"time"

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
		if os.Getenv(shared.LocalBackendEnvToggle) != "" {
			opt.Endpoints = []string{fmt.Sprintf("http://localhost:%d", p.endpointBackend.Get().Port())}
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
	if os.Getenv(shared.LocalBackendEnvToggle) != "" {
		b := &LocalEndpointBackend{
			configFilePath: shared.LocalAlertManagerPath,
			p:              p,
		}
		go func() {
			// FIXME: management url is not correct
			err := shared.BackendDefaultFile("http://localhost:5001")
			if err != nil {
				panic(err)
			}
			b.Start()
			p.endpointBackend.Set(b)
			options := p.alertingOptions.Get()
			options.Endpoints = []string{fmt.Sprintf("http://localhost:%d", b.Port())}
			p.alertingOptions = future.New[AlertingOptions]()
			p.alertingOptions.Set(options)
		}()
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
			breakOut := false
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
