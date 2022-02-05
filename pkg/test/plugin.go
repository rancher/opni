package test

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
	example "github.com/rancher/opni-monitoring/plugins/example/pkg"
)

func NewTestExamplePlugin(ctx context.Context) *plugin.ClientConfig {
	scheme := meta.NewScheme()
	p := &example.ExamplePlugin{}
	scheme.Add(apiextensions.ManagementAPIExtensionPluginID,
		apiextensions.NewPlugin(&example.ExampleAPIExtension_ServiceDesc, p))
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))

	cfg := plugins.ServeConfig(scheme)
	ch := make(chan *plugin.ReattachConfig)
	cfg.Test = &plugin.ServeTestConfig{
		Context:          ctx,
		ReattachConfigCh: ch,
	}
	go plugin.Serve(cfg)

	return &plugin.ClientConfig{
		HandshakeConfig: plugins.Handshake,
		Plugins:         scheme.PluginMap(),
		Reattach:        <-ch,
		Managed:         true,
	}
}
