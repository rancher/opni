package plugins

import (
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
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

type PluginLoader struct {
	ActivePlugins map[string][]ActivePlugin
}

var DefaultPluginLoader = NewPluginLoader()

func NewPluginLoader() *PluginLoader {
	return &PluginLoader{
		ActivePlugins: map[string][]ActivePlugin{},
	}
}

func pluginLogName(cc *plugin.ClientConfig) string {
	if cc.Cmd != nil {
		return cc.Cmd.String()
	}
	return cc.Reattach.Addr.String()
}

func (pl *PluginLoader) Load(cc *plugin.ClientConfig) {
	client := plugin.NewClient(cc)
	rpcClient, err := client.Client()
	if err != nil {
		pluginLog.With(
			zap.Error(err),
			"plugin", pluginLogName(cc),
		).Error("failed to load plugin")
		return
	}
	pluginLog.With(
		"plugin", pluginLogName(cc),
	).Debug("checking if plugin implements any interfaces in the scheme")
	for id := range cc.Plugins {
		raw, err := rpcClient.Dispense(id)
		if err != nil {
			pluginLog.With(
				zap.Error(err),
				"plugin", pluginLogName(cc),
				"id", id,
			).Debug("no implementation found")
			continue
		}
		pluginLog.With(
			"plugin", pluginLogName(cc),
			"id", id,
		).Debug("implementation found")
		pl.ActivePlugins[id] = append(pl.ActivePlugins[id], ActivePlugin{
			Client: rpcClient.(*plugin.GRPCClient).Conn,
			Raw:    raw,
		})
	}
}

func (pl *PluginLoader) DispenseAll(id string) []ActivePlugin {
	return pl.ActivePlugins[id]
}

func Load(cc *plugin.ClientConfig) {
	DefaultPluginLoader.Load(cc)
}

func DispenseAll(id string) []ActivePlugin {
	return DefaultPluginLoader.DispenseAll(id)
}
