package plugins

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/plugins/meta"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

type ClientOptions struct {
	reattach     *plugin.ReattachConfig
	secureConfig *plugin.SecureConfig
}

type ClientOption func(*ClientOptions)

func (o *ClientOptions) apply(opts ...ClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithReattachConfig(reattach *plugin.ReattachConfig) ClientOption {
	return func(o *ClientOptions) {
		o.reattach = reattach
	}
}

func WithSecureConfig(sc *plugin.SecureConfig) ClientOption {
	return func(o *ClientOptions) {
		o.secureConfig = sc
	}
}

func ClientConfig(md meta.PluginMeta, scheme meta.Scheme, opts ...ClientOption) *plugin.ClientConfig {
	options := &ClientOptions{}
	options.apply(opts...)

	cc := &plugin.ClientConfig{
		Plugins:          scheme.PluginMap(),
		HandshakeConfig:  Handshake,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Managed:          true,
		Logger: hclog.New(&hclog.LoggerOptions{
			Level: hclog.Error,
		}),
		GRPCDialOptions: []grpc.DialOption{
			grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
			grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		},
		Stderr: os.Stderr,
	}

	if options.reattach != nil {
		cc.Reattach = options.reattach
	} else {
		//#nosec G204
		cmd := exec.Command(md.BinaryPath)
		ConfigureSysProcAttr(cmd)

		switch mode := scheme.Mode(); mode {
		case meta.ModeGateway, meta.ModeAgent:
			cmd.Env = append(cmd.Environ(), fmt.Sprintf("%s=%s", meta.PluginModeEnvVar, mode))
		default:
			panic(fmt.Sprintf("unknown plugin mode: %s", mode))
		}

		cc.Cmd = cmd
		cc.AutoMTLS = true
	}
	if options.secureConfig != nil {
		cc.SecureConfig = options.secureConfig
	}

	return cc
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
