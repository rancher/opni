package gateway

import (
	"context"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"

	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"
)

// UseManagementAPI implements system.SystemPluginServer.
func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
	p.managementClient.C() <- client
	cfg, err := client.GetConfig(context.Background(), &emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		p.logger.With(
			logger.Err(err),
		).Error("failed to get config")
		os.Exit(1)
	}
	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.logger.With(
			logger.Err(err),
		).Error("failed to load config")
		os.Exit(1)
	}
	machinery.LoadAuthProviders(p.ctx, objectList)
	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		backend, err := machinery.ConfigureStorageBackend(p.ctx, &config.Spec.Storage)
		if err != nil {
			p.logger.With(
				logger.Err(err),
			).Error("failed to configure storage backend")
			os.Exit(1)
		}
		p.storageBackend.C() <- backend
		p.gatewayConfig.C() <- config
	})

	p.authMiddlewares.C() <- machinery.LoadAuthProviders(p.ctx, objectList)
	<-p.ctx.Done()
}

// UseManagementAPI implements system.SystemPluginServer.
func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	p.keyValueStoreClient.C() <- client
	<-p.ctx.Done()
}

// UseManagementAPI implements system.SystemPluginServer.
func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {
	p.extensionClient.C() <- intf
	<-p.ctx.Done()
}
