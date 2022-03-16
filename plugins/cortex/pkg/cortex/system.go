package cortex

import (
	"context"
	"os"

	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/machinery"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) UseManagementAPI(client management.ManagementClient) {
	p.mgmtApi.Set(client)
	cfg, err := client.GetConfig(context.Background(), &emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to get config")
		os.Exit(1)
	}
	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to load config")
		os.Exit(1)
	}
	machinery.LoadAuthProviders(p.ctx, objectList)
	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		backend, err := machinery.ConfigureStorageBackend(p.ctx, &config.Spec.Storage)
		if err != nil {
			p.logger.With(
				"err", err,
			).Error("failed to configure storage backend")
			os.Exit(1)
		}
		p.storageBackend.Set(backend)
		p.config.Set(config)
		p.configureDistributorClient(p.loadCortexCerts())
	})
	<-p.ctx.Done()
}

func (p *Plugin) UseKeyValueStore(system.KVStoreClient) {}
