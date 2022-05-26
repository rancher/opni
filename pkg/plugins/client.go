package plugins

import (
	"os"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/meta"
	"go.uber.org/zap"
)

func ClientConfig(md meta.PluginMeta, scheme meta.Scheme, reattach ...*plugin.ReattachConfig) *plugin.ClientConfig {
	//#nosec G204
	cmd := exec.Command(md.BinaryPath)
	ConfigureSysProcAttr(cmd)
	var rc *plugin.ReattachConfig
	if len(reattach) > 0 {
		rc = reattach[0]
		cmd = nil
	}
	return &plugin.ClientConfig{
		Plugins:          scheme.PluginMap(),
		HandshakeConfig:  Handshake,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Managed:          true,
		Cmd:              cmd,
		Logger:           logger.NewHCLogger(logger.New(logger.WithLogLevel(zap.DebugLevel))).Named("plugin"),
		SyncStdout:       os.Stdout,
		SyncStderr:       os.Stderr,
		Reattach:         rc,
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
