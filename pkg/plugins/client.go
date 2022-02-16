package plugins

import (
	"os"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func ClientConfig(md meta.PluginMeta, scheme meta.Scheme) *plugin.ClientConfig {
	return &plugin.ClientConfig{
		Plugins:          scheme.PluginMap(),
		HandshakeConfig:  Handshake,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Managed:          true,
		Cmd:              exec.Command(md.BinaryPath),
		Logger:           logger.NewHCLogger(logger.New()).Named("plugin"),
		SyncStdout:       os.Stdout,
		SyncStderr:       os.Stderr,
	}
}

func ServeConfig(scheme meta.Scheme) *plugin.ServeConfig {
	return &plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         scheme.PluginMap(),
		GRPCServer:      plugin.DefaultGRPCServer,
		Logger:          logger.NewForPlugin(),
	}
}

func Serve(scheme meta.Scheme) {
	plugin.Serve(ServeConfig(scheme))
}

type ActivePlugin struct {
	Metadata meta.PluginMeta
	Client   *grpc.ClientConn
	Raw      interface{}
}

type PluginLoader struct {
	ActivePlugins map[string][]ActivePlugin
	Logger        *zap.SugaredLogger
}

func NewPluginLoader() *PluginLoader {
	return &PluginLoader{
		ActivePlugins: map[string][]ActivePlugin{},
		Logger:        logger.New().Named("pluginloader"),
	}
}

func (pl *PluginLoader) Load(md meta.PluginMeta, cc *plugin.ClientConfig) {
	lg := pl.Logger
	client := plugin.NewClient(cc)
	rpcClient, err := client.Client()
	if err != nil {
		lg.With(
			zap.Error(err),
			"plugin", md.Module,
		).Error("failed to load plugin")
		return
	}
	lg.With(
		"plugin", md.Module,
	).Debug("checking if plugin implements any interfaces in the scheme")
	for id := range cc.Plugins {
		raw, err := rpcClient.Dispense(id)
		if err != nil {
			lg.With(
				zap.Error(err),
				"plugin", md.Module,
				"id", id,
			).Debug("no implementation found")
			continue
		}
		lg.With(
			"plugin", md.Module,
			"id", id,
		).Debug("implementation found")
		pl.ActivePlugins[id] = append(pl.ActivePlugins[id], ActivePlugin{
			Metadata: md,
			Client:   rpcClient.(*plugin.GRPCClient).Conn,
			Raw:      raw,
		})
	}
}

func (pl *PluginLoader) DispenseAll(id string) []ActivePlugin {
	return pl.ActivePlugins[id]
}
