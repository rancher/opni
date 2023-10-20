package gateway

import (
	"context"
	"os"

	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/task"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"
)

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
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
	})
	<-p.ctx.Done()
}

func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	p.kv.Set(client)
	ctrl, err := task.NewController(p.ctx, "uninstall", system.NewKVStoreClient[*opnicorev1.TaskStatus](client), &UninstallTaskRunner{
		storageNamespace:  p.storageNamespace,
		opensearchManager: p.opensearchManager,
		backendDriver:     p.backendDriver,
		storageBackend:    p.storageBackend,
		logger:            p.logger.WithGroup("uninstaller"),
	})
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to create task controller")
		os.Exit(1)
	}

	p.uninstallController.Set(ctrl)
	<-p.ctx.Done()
}
