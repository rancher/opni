package plugins

import (
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/plugins/meta"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var pluginLog = logger.New().Named("plugin")

func ClientConfig(md meta.PluginMeta, scheme meta.Scheme) *plugin.ClientConfig {
	return &plugin.ClientConfig{
		Plugins:          scheme.PluginMap(),
		HandshakeConfig:  Handshake,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Managed:          true,
		Cmd:              exec.Command(md.Path),
	}
}

func ServeConfig(scheme meta.Scheme) *plugin.ServeConfig {
	return &plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         scheme.PluginMap(),
		GRPCServer:      plugin.DefaultGRPCServer,
	}
}

func Serve(scheme meta.Scheme) {
	plugin.Serve(ServeConfig(scheme))
}

type ActivePlugin struct {
	Client *grpc.ClientConn
	Raw    interface{}
}

var (
	activePlugins = map[string][]ActivePlugin{}
)

func Load(cc *plugin.ClientConfig) {
	client := plugin.NewClient(cc)
	rpcClient, err := client.Client()
	if err != nil {
		pluginLog.With(
			zap.Error(err),
			"plugin", cc.Cmd.Path,
		).Error("failed to load plugin")
		return
	}
	pluginLog.With(
		"plugin", cc.Cmd.Path,
	).Debug("checking if plugin implements any interfaces in the scheme")
	for id := range cc.Plugins {
		raw, err := rpcClient.Dispense(id)
		if err != nil {
			pluginLog.With(
				zap.Error(err),
				"plugin", cc.Cmd.Path,
				"id", id,
			).Debug("no implementation found")
			continue
		}
		pluginLog.With(
			"plugin", cc.Cmd.Path,
			"id", id,
		).Debug("implementation found")
		activePlugins[id] = append(activePlugins[id], ActivePlugin{
			Client: rpcClient.(*plugin.GRPCClient).Conn,
			Raw:    raw,
		})
	}
}

func DispenseAll(id string) []ActivePlugin {
	return activePlugins[id]
}
