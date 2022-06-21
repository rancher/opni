package clients

import (
	"context"

	"emperror.dev/errors"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ManagementClientOptions struct {
	address     string
	dialOptions []grpc.DialOption
}

type ManagementClientOption func(*ManagementClientOptions)

func (o *ManagementClientOptions) Apply(opts ...ManagementClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithAddress(addr string) ManagementClientOption {
	return func(o *ManagementClientOptions) {
		o.address = addr
	}
}

func WithDialOptions(options ...grpc.DialOption) ManagementClientOption {
	return func(o *ManagementClientOptions) {
		o.dialOptions = append(o.dialOptions, options...)
	}
}

func NewManagementClient(ctx context.Context, opts ...ManagementClientOption) (managementv1.ManagementClient, error) {
	options := ManagementClientOptions{
		address: managementv1.DefaultManagementSocket(),
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		},
	}
	options.Apply(opts...)
	cc, err := grpc.DialContext(ctx, options.address, options.dialOptions...)
	if err != nil {
		return nil, err
	}
	return managementv1.NewManagementClient(cc), nil
}

func FromExtension[T any](
	ctx context.Context,
	client managementv1.ManagementClient,
	serviceName string,
	constructor func(grpc.ClientConnInterface) T,
) (T, error) {
	list, err := client.APIExtensions(ctx, &emptypb.Empty{})
	if err != nil {
		return lo.Empty[T](), err
	}
	for _, ext := range list.Items {
		if ext.ServiceDesc.GetName() == serviceName {
			return constructor(managementv1.UnderlyingConn(client)), nil
		}
	}
	return lo.Empty[T](), errors.New("Service is not installed: " + serviceName)
}
