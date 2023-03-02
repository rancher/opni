package gateway

import (
	"context"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/import/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/import/pkg/backend"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
	"go.uber.org/zap"
)

type Plugin struct {
	system.UnimplementedSystemPluginClient

	ctx    context.Context
	logger *zap.SugaredLogger

	importBackend     backend.ImportBackend
	cortexRemoteWrite cortex.RemoteWriteForwarder

	config          future.Future[*v1beta1.GatewayConfig]
	cortexClientSet future.Future[cortex.ClientSet]
	delegate        future.Future[streamext.StreamDelegate[remoteread.RemoteReadAgentClient]]
}

func NewPlugin(ctx context.Context) *Plugin {
	p := &Plugin{
		ctx:    ctx,
		logger: logger.NewPluginLogger().Named("import"),

		config:          future.New[*v1beta1.GatewayConfig](),
		cortexClientSet: future.New[cortex.ClientSet](),
		delegate:        future.New[streamext.StreamDelegate[remoteread.RemoteReadAgentClient]](),
	}

	future.Wait2(p.cortexClientSet, p.config,
		func(cortexClientSet cortex.ClientSet, config *v1beta1.GatewayConfig) {
			p.cortexRemoteWrite.Initialize(cortex.RemoteWriteForwarderConfig{
				CortexClientSet: cortexClientSet,
				Config:          &config.Spec,
				Logger:          p.logger.Named("cortex-rw"),
			})
		})

	future.Wait1(p.delegate,
		func(delegate streamext.StreamDelegate[remoteread.RemoteReadAgentClient]) {
			p.importBackend.Initialize(backend.ImportBackendConfig{
				Logger:   p.logger.Named("import-backend"),
				Delegate: delegate,
			})
		})

	return p
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))
	p := NewPlugin(ctx)

	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(
		util.PackService(&remoteread.RemoteReadGateway_ServiceDesc, &p.importBackend)),
	)

	return scheme
}
