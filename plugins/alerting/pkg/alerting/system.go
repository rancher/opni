package alerting

import (
	"context"
	"os"

	alertapi "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

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
		p.alertingEndpoints.Set(config.Spec.Alerting.Endpoints)
	})
}

// UseKeyValueStore Alerting Condition & Alert Endpoints are stored in K,V stores
func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	p.storage.Set(StorageAPIs{
		Conditions:    system.NewKVStoreClient[*alertapi.AlertCondition](client),
		AlertEndpoint: system.NewKVStoreClient[*alertapi.AlertEndpoint](client),
	})
}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {

}
