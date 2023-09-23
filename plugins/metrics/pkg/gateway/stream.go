package gateway

import (
	"context"

	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/metrics"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"github.com/rancher/opni/plugins/metrics/pkg/backend"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc:              &remotewrite.RemoteWrite_ServiceDesc,
			Impl:              &p.cortexRemoteWrite,
			RequireCapability: wellknown.CapabilityMetrics,
		},
		{
			Desc:              &remoteread.RemoteReadGateway_ServiceDesc,
			Impl:              &p.metrics,
			RequireCapability: wellknown.CapabilityMetrics,
		},
		{
			Desc:              &node.NodeMetricsCapability_ServiceDesc,
			Impl:              p.metrics.NodeBackend,
			RequireCapability: wellknown.CapabilityMetrics,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	type clientset struct {
		agent.ClientSet
		remoteread.RemoteReadAgentClient
	}
	p.delegate.Set(streamext.NewDelegate(cc, func(cci grpc.ClientConnInterface) backend.MetricsAgentClientSet {
		return &clientset{
			ClientSet:             agent.NewClientSet(cci),
			RemoteReadAgentClient: remoteread.NewRemoteReadAgentClient(cci),
		}
	}))
}

func (p *Plugin) labelsForStreamMetrics(ctx context.Context) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Key(metrics.LabelImpersonateAs).String(cluster.StreamAuthorizedID(ctx)),
		attribute.Key("handler").String("plugin_metrics"),
	}
}
