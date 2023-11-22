package types

import (
	"context"
	"log/slog"

	"github.com/rancher/opni/pkg/agent"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"golang.org/x/tools/pkg/memoize"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type PluginContext interface {
	context.Context
	Logger() *slog.Logger
	Metrics() *Metrics
	Memoize(key any, fn memoize.Function) *memoize.Promise

	ManagementClient() managementv1.ManagementClient
	KeyValueStoreClient() system.KeyValueStoreClient
	StreamClient() grpc.ClientConnInterface
	ClusterDriver() drivers.ClusterDriver
	GatewayConfig() *v1beta1.GatewayConfig
	AuthMiddlewares() map[string]auth.Middleware
	ExtensionClient() system.ExtensionClientInterface
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
	SetServingStatus(serviceName string, status healthpb.HealthCheckResponse_ServingStatus)
}

type StreamServiceContext interface {
	ServiceContext
}
