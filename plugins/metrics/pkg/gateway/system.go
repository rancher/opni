package gateway

import (
	"context"
	"os"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"

	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"
)

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
	p.mgmtClientC <- client
	cfg, err := client.GetConfig(context.Background(), &emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		p.logger.With(
			zap.Error(err),
		).Error("failed to get config")
		os.Exit(1)
	}
	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.logger.With(
			zap.Error(err),
		).Error("failed to load config")
		os.Exit(1)
	}
	machinery.LoadAuthProviders(p.ctx, objectList)
	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		backend, err := machinery.ConfigureStorageBackend(p.ctx, &config.Spec.Storage)
		if err != nil {
			p.logger.With(
				zap.Error(err),
			).Error("failed to configure storage backend")
			os.Exit(1)
		}
		p.storageBackendC <- backend
		p.gatewayConfigC <- config
	})

	p.authMiddlewaresC <- machinery.LoadAuthProviders(p.ctx, objectList)
	<-p.ctx.Done()
}

func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	p.keyValueStoreClientC <- client
	<-p.ctx.Done()
}
