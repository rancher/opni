package plugins

import (
	"os"
	"os/exec"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/plugins/meta"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
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
		Logger: hclog.New(&hclog.LoggerOptions{
			Level: hclog.Error,
		}),
		GRPCDialOptions: []grpc.DialOption{
			grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		},
		SyncStdout: os.Stdout,
		SyncStderr: os.Stderr,
		Reattach:   rc,
	}
}

func ServeConfig(scheme meta.Scheme) *plugin.ServeConfig {
	return &plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         scheme.PluginMap(),
		GRPCServer: func(opts []grpc.ServerOption) *grpc.Server {
			opts = append(opts,
				grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
				grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
			)
			return grpc.NewServer(opts...)
		},
		Logger: hclog.New(&hclog.LoggerOptions{
			Level: hclog.Error,
		}),
	}
}

func Serve(scheme meta.Scheme) {
	plugin.Serve(ServeConfig(scheme))
}
