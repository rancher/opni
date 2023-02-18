package slo

import (
	"context"
	"os"
	"time"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
	for retries := 10; retries > 0; retries-- {
		apiExtensions, err := client.APIExtensions(context.Background(), &emptypb.Empty{})
		if err != nil {
			p.logger.Debug("failed to get API extensions, retrying in 1s", "error", err)
			time.Sleep(time.Second)
			continue
		}
		found := false
		for _, ext := range apiExtensions.Items {
			if ext.ServiceDesc.GetName() == "CortexAdmin" {
				found = true
				break
			}
		}
		if !found {
			p.logger.Debug("cortex API not available yet, retrying in 1s")
			time.Sleep(time.Second)
			continue
		}
		p.logger.Debug("cortex API available")
		p.mgmtClient.Set(client)
		<-p.ctx.Done()
	}
	p.logger.Error("cortex api not available, stopping plugin")
	os.Exit(1)
}

func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	p.storage.Set(StorageAPIs{
		SLOs:     system.NewKVStoreClient[*sloapi.SLOData](client),
		Services: system.NewKVStoreClient[*sloapi.Service](client),
		Metrics:  system.NewKVStoreClient[*sloapi.Metric](client),
	})
	<-p.ctx.Done()
}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {
	cc, err := intf.GetClientConn(p.ctx, "CortexAdmin")
	if err != nil {
		p.logger.Error("failed to get cortex admin client", "error", err)
		os.Exit(1)
	}
	adminClient := cortexadmin.NewCortexAdminClient(cc)
	alertingEndpointClient := alertingv1.NewAlertEndpointsClient(cc)

	p.adminClient.Set(adminClient)
	p.alertEndpointClient.Set(alertingEndpointClient)
	RegisterDatasource(
		shared.MonitoringDatasource,
		NewSLOMonitoringStore(p, p.logger),
		NewMonitoringServiceBackend(p, p.logger),
	)
}
