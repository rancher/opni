package gateway

import (
	"context"

	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/metrics"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []util.ServicePackInterface {
	return p.streamServices
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	type clientset struct {
		agent.ClientSet
		remoteread.RemoteReadAgentClient
	}
	p.delegateC <- streamext.NewDelegate(cc, func(cci grpc.ClientConnInterface) types.MetricsAgentClientSet {
		return &clientset{
			ClientSet:             agent.NewClientSet(cci),
			RemoteReadAgentClient: remoteread.NewRemoteReadAgentClient(cci),
		}
	})
}

func (p *Plugin) labelsForStreamMetrics(ctx context.Context) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Key(metrics.LabelImpersonateAs).String(cluster.StreamAuthorizedID(ctx)),
		attribute.Key("handler").String("plugin_metrics"),
	}
}
