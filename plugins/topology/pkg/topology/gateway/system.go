package gateway

import (
	"context"
	"os"

	"github.com/rancher/opni/apis"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
	p.mgmtClient.Set(client)
	cfg, err := client.GetConfig(
		context.Background(),
		&emptypb.Empty{},
		grpc.WaitForReady(true),
	)
	if err != nil {
		p.logger.With("err", err).Error("failed to get config")
		os.Exit(1)
	}

	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.logger.With("err", err).Error("failed to load config")
		os.Exit(1)
	}

	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		//TODO : get whatever is necessary from here
	})

	k8sclient, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
		Scheme: apis.NewScheme(),
	})
	if err != nil {
		p.logger.With("err", err).Error("failed to create k8s client")
		os.Exit(1)
	}
	p.k8sClient.Set(k8sclient)
	<-p.ctx.Done()
}
