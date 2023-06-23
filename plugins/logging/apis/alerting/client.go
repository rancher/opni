package alerting

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AlertingClientOptions struct {
	listenAddr  string
	dialOptions []grpc.DialOption
}

type AlertingClientOption func(*AlertingClientOptions)

func (o *AlertingClientOptions) apply(opts ...AlertingClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithListenAddress(addr string) AlertingClientOption {
	return func(o *AlertingClientOptions) {
		o.listenAddr = addr
	}
}

func WithDialOptions(options ...grpc.DialOption) AlertingClientOption {
	return func(o *AlertingClientOptions) {
		o.dialOptions = append(o.dialOptions, options...)
	}
}

func NewMonitorClient(ctx waitctx.PermissiveContext, opts ...AlertingClientOption) (MonitorManagementClient, error) {
	options := AlertingClientOptions{
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
	return NewMonitorManagementClient(cc), nil
}

func NewNotificationClient(ctx waitctx.PermissiveContext, opts ...AlertingClientOption) (NotificationManagementClient, error) {
	options := AlertingClientOptions{
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
	return NewNotificationManagementClient(cc), nil
}

func NewAlertClient(ctx waitctx.PermissiveContext, opts ...AlertingClientOption) (AlertManagementClient, error) {
	options := AlertingClientOptions{
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
	return NewAlertManagementClient(cc), nil
}
