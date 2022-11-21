package loggingadmin

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AdminClientOptions struct {
	listenAddr  string
	dialOptions []grpc.DialOption
}

type AdminClientOption func(*AdminClientOptions)

func (o *AdminClientOptions) apply(opts ...AdminClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithListenAddress(addr string) AdminClientOption {
	return func(o *AdminClientOptions) {
		o.listenAddr = addr
	}
}

func WithDialOptions(options ...grpc.DialOption) AdminClientOption {
	return func(o *AdminClientOptions) {
		o.dialOptions = append(o.dialOptions, options...)
	}
}

func NewClient(ctx waitctx.PermissiveContext, opts ...AdminClientOption) (LoggingAdminClient, error) {
	options := AdminClientOptions{
		listenAddr: managementv1.DefaultManagementSocket(),
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		},
	}
	options.apply(opts...)
	cc, err := grpc.DialContext(ctx, options.listenAddr, options.dialOptions...)
	if err != nil {
		return nil, err
	}
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		cc.Close()
	})
	return NewLoggingAdminClient(cc), nil
}

func NewV2Client(ctx waitctx.PermissiveContext, opts ...AdminClientOption) (LoggingAdminV2Client, error) {
	options := AdminClientOptions{
		listenAddr: managementv1.DefaultManagementSocket(),
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		},
	}
	options.apply(opts...)
	cc, err := grpc.DialContext(ctx, options.listenAddr, options.dialOptions...)
	if err != nil {
		return nil, err
	}
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		cc.Close()
	})
	return NewLoggingAdminV2Client(cc), nil
}
