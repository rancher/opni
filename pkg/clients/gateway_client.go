package clients

import (
	"context"
	"crypto/tls"
	"fmt"

	"emperror.dev/errors"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/kralicky/totem"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/b2mac"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
)

type GatewayClient interface {
	grpc.ServiceRegistrar
	credentials.PerRPCCredentials
	// Connect returns a ClientConnInterface connected to the streaming server
	Connect(context.Context) (grpc.ClientConnInterface, future.Future[error])
	// Dial returns a standard ClientConnInterface for Unary connections
	Dial(context.Context) (grpc.ClientConnInterface, error)
}

func NewGatewayClient(
	address string,
	ip ident.Provider,
	kr keyring.Keyring,
	trustStrategy trust.Strategy,
) (GatewayClient, error) {
	client := &gatewayClient{
		grpcAddress: address,
	}
	id, err := ip.UniqueIdentifier(context.Background())
	if err != nil {
		return nil, err
	}
	var sharedKeys *keyring.SharedKeys
	kr.Try(func(sk *keyring.SharedKeys) {
		if sharedKeys != nil {
			err = errors.New("keyring contains multiple shared key sets")
			return
		}
		sharedKeys = sk
	})
	if err != nil {
		return nil, err
	}
	if sharedKeys == nil {
		return nil, errors.New("keyring is missing shared keys")
	}

	tlsConfig, err := trustStrategy.TLSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS config: %w", err)
	}

	client.id = id
	client.sharedKeys = sharedKeys
	client.tlsConfig = tlsConfig
	return client, nil
}

type gatewayClient struct {
	grpcAddress string
	id          string
	sharedKeys  *keyring.SharedKeys
	tlsConfig   *tls.Config

	services []util.ServicePack[any]
}

func (gc *gatewayClient) RegisterService(desc *grpc.ServiceDesc, impl any) {
	gc.services = append(gc.services, util.PackService(desc, impl))
}

func (gc *gatewayClient) Connect(ctx context.Context) (grpc.ClientConnInterface, future.Future[error]) {
	gcc, err := grpc.DialContext(ctx, gc.grpcAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(gc.tlsConfig)),
		grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor(), gc.streamClientInterceptor),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, future.Instant(fmt.Errorf("failed to dial gateway: %w", err))
	}

	streamClient := streamv1.NewStreamClient(gcc)
	stream, err := streamClient.Connect(ctx)
	if err != nil {
		return nil, future.Instant(fmt.Errorf("failed to connect to gateway: %w", err))
	}

	ts := totem.NewServer(stream)
	for _, sp := range gc.services {
		ts.RegisterService(sp.Unpack())
	}

	cc, errC := ts.Serve()
	f := future.New[error]()
	go func() {
		select {
		case <-ctx.Done():
			f.Set(ctx.Err())
		case err := <-errC:
			f.Set(err)
		}
	}()
	return cc, f
}

func (gc *gatewayClient) Dial(ctx context.Context) (grpc.ClientConnInterface, error) {
	return grpc.DialContext(ctx, gc.grpcAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(gc.tlsConfig)),
		grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor(), gc.unaryClientInterceptor),
		grpc.WithBlock(),
	)
}

func (gc *gatewayClient) streamClientInterceptor(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	nonce, mac, err := b2mac.New512([]byte(gc.id), []byte(method), gc.sharedKeys.ClientKey)
	if err != nil {
		return nil, err
	}
	authHeader, err := b2mac.EncodeAuthHeader([]byte(gc.id), nonce, mac)
	if err != nil {
		return nil, err
	}

	ctx = metadata.AppendToOutgoingContext(ctx, auth.AuthorizationKey, authHeader)
	return streamer(ctx, desc, cc, method, opts...)
}

func (gc *gatewayClient) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	authMap := make(map[string]string, 0)
	info, ok := credentials.RequestInfoFromContext(ctx)
	if !ok {
		return authMap, errors.New("no request info in context")
	}

	nonce, mac, err := b2mac.New512([]byte(gc.id), []byte(info.Method), gc.sharedKeys.ClientKey)
	if err != nil {
		return authMap, err
	}
	authHeader, err := b2mac.EncodeAuthHeader([]byte(gc.id), nonce, mac)
	if err != nil {
		return authMap, err
	}
	authMap[auth.AuthorizationKey] = authHeader
	return authMap, nil
}

func (gc *gatewayClient) RequireTransportSecurity() bool {
	return true
}

func (gc *gatewayClient) unaryClientInterceptor(
	ctx context.Context,
	method string,
	req interface{},
	reply interface{},
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	authHeader, err := b2mac.NewEncodedHeader([]byte(gc.id), []byte(method), gc.sharedKeys.ClientKey)
	if err != nil {
		return err
	}

	ctx = metadata.AppendToOutgoingContext(ctx, auth.AuthorizationKey, authHeader)
	return invoker(ctx, method, req, reply, cc, opts...)
}
