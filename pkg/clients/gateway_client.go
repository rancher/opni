package clients

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"time"

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
	// credentials.PerRPCCredentials
	// Connect returns a ClientConnInterface connected to the streaming server.
	// The connection remains active until the provided context is canceled.
	Connect(context.Context) (grpc.ClientConnInterface, future.Future[error])
	RegisterSplicedStream(cc grpc.ClientConnInterface, name string)
	ClientConn() grpc.ClientConnInterface
}

func NewGatewayClient(
	ctx context.Context,
	address string,
	ip ident.Provider,
	kr keyring.Keyring,
	trustStrategy trust.Strategy,
) (GatewayClient, error) {
	id, err := ip.UniqueIdentifier(ctx)
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

	cc, err := dial(ctx, address, id, sharedKeys, tlsConfig)
	if err != nil {
		return nil, err
	}

	client := &gatewayClient{
		cc: cc,
		id: id,
	}

	return client, nil
}

type splicedConn struct {
	name string
	cc   grpc.ClientConnInterface
}

type gatewayClient struct {
	cc *grpc.ClientConn
	id string

	services []util.ServicePack[any]
	spliced  []*splicedConn
}

func (gc *gatewayClient) RegisterService(desc *grpc.ServiceDesc, impl any) {
	gc.services = append(gc.services, util.PackService(desc, impl))
}

func (gc *gatewayClient) RegisterSplicedStream(cc grpc.ClientConnInterface, name string) {
	gc.spliced = append(gc.spliced, &splicedConn{
		name: name,
		cc:   cc,
	})
}

func dial(ctx context.Context, address, id string, sharedKeys *keyring.SharedKeys, tlsConfig *tls.Config) (*grpc.ClientConn, error) {
	streamInt := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		nonce, mac, err := b2mac.New512([]byte(id), []byte(method), sharedKeys.ClientKey)
		if err != nil {
			return nil, err
		}
		authHeader, err := b2mac.EncodeAuthHeader([]byte(id), nonce, mac)
		if err != nil {
			return nil, err
		}

		ctx = metadata.AppendToOutgoingContext(ctx, auth.AuthorizationKey, authHeader)
		return streamer(ctx, desc, cc, method, opts...)
	}

	unaryInt := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		nonce, mac, err := b2mac.New512([]byte(id), []byte(method), sharedKeys.ClientKey)
		if err != nil {
			return err
		}
		authHeader, err := b2mac.EncodeAuthHeader([]byte(id), nonce, mac)
		if err != nil {
			return err
		}

		ctx = metadata.AppendToOutgoingContext(ctx, auth.AuthorizationKey, authHeader)
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	return grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor(), streamInt),
		grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor(), unaryInt),
		grpc.WithDefaultCallOptions(
			grpc.WaitForReady(true),
			grpc.MaxCallSendMsgSize(math.MaxInt32),
			grpc.MaxCallRecvMsgSize(math.MaxInt32),
		),
	)
}

func (gc *gatewayClient) Connect(ctx context.Context) (grpc.ClientConnInterface, future.Future[error]) {
	streamClient := streamv1.NewStreamClient(gc.cc)
	stream, err := streamClient.Connect(ctx)
	if err != nil {
		return nil, future.Instant(fmt.Errorf("failed to connect to gateway: %w", err))
	}

	shortId := gc.id
	if len(shortId) > 8 {
		shortId = shortId[:8]
	}
	ts, err := totem.NewServer(stream, totem.WithName("gateway-client-"+shortId))
	if err != nil {
		return nil, future.Instant(fmt.Errorf("failed to create totem server: %w", err))
	}
	for _, sp := range gc.services {
		ts.RegisterService(sp.Unpack())
	}
	cleanup := []func() error{}
	for _, sc := range gc.spliced {
		streamClient := streamv1.NewStreamClient(sc.cc)
		splicedStream, err := streamClient.Connect(ctx)
		if err != nil {
			return nil, future.Instant(fmt.Errorf("failed to connect to spliced server: %w", err))
		}
		cleanup = append(cleanup, splicedStream.CloseSend)

		if err := ts.Splice(splicedStream, totem.WithStreamName(sc.name)); err != nil {
			return nil, future.Instant(fmt.Errorf("failed to splice stream: %w", err))
		}

		defer func() {
			ctx, ca := context.WithTimeout(ctx, 2*time.Second)
			defer ca()
			streamClient.Notify(ctx, &streamv1.StreamEvent{
				Type: streamv1.EventType_DiscoveryComplete,
			})
		}()
	}

	cc, errC := ts.Serve()
	f := future.NewFromChannel(errC)

	go func() {
		<-f.C()
		stream.CloseSend()
		for _, c := range cleanup {
			c()
		}
	}()
	return cc, f
}

func (gc *gatewayClient) ClientConn() grpc.ClientConnInterface {
	return gc.cc
}

// func (gc *gatewayClient) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
// 	authMap := make(map[string]string, 0)
// 	info, ok := credentials.RequestInfoFromContext(ctx)
// 	if !ok {
// 		return authMap, errors.New("no request info in context")
// 	}

// 	nonce, mac, err := b2mac.New512([]byte(gc.id), []byte(info.Method), gc.sharedKeys.ClientKey)
// 	if err != nil {
// 		return authMap, err
// 	}
// 	authHeader, err := b2mac.EncodeAuthHeader([]byte(gc.id), nonce, mac)
// 	if err != nil {
// 		return authMap, err
// 	}
// 	authMap[auth.AuthorizationKey] = authHeader
// 	return authMap, nil
// }

// func (gc *gatewayClient) RequireTransportSecurity() bool {
// 	return true
// }

// func (gc *gatewayClient) unaryClientInterceptor(
// 	ctx context.Context,
// 	method string,
// 	req interface{},
// 	reply interface{},
// 	cc *grpc.ClientConn,
// 	invoker grpc.UnaryInvoker,
// 	opts ...grpc.CallOption,
// ) error {
// 	authHeader, err := b2mac.NewEncodedHeader([]byte(gc.id), []byte(method), gc.sharedKeys.ClientKey)
// 	if err != nil {
// 		return err
// 	}

// 	ctx = metadata.AppendToOutgoingContext(ctx, auth.AuthorizationKey, authHeader)
// 	return invoker(ctx, method, req, reply, cc, opts...)
// }
