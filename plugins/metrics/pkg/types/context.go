package types

import (
	"context"

	"github.com/rancher/opni/pkg/agent"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type PluginContext interface {
	context.Context
	Logger() *zap.SugaredLogger
	Memoize(key any, function func() (any, error)) (any, error)

	ManagementClient() managementv1.ManagementClient
	KeyValueStoreClient() system.KeyValueStoreClient
	StreamClient() grpc.ClientConnInterface
	ClusterDriver() drivers.ClusterDriver
	GatewayConfig() *v1beta1.GatewayConfig
	AuthMiddlewares() map[string]auth.Middleware
}

type MetricsAgentClientSet interface {
	agent.ClientSet
	remoteread.RemoteReadAgentClient
}

type ServiceContext interface {
	PluginContext
	StorageBackend() storage.Backend
	Delegate() streamext.StreamDelegate[MetricsAgentClientSet]
}

type ManagementServiceContext interface {
	ServiceContext
	RegisterService(util.ServicePackInterface) managementext.SingleServiceController
}

type StreamServiceContext interface {
	ServiceContext
}
