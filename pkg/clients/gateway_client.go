package clients

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"

	"log/slog"

	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/auth/cluster"
	authv2 "github.com/rancher/opni/pkg/auth/cluster/v2"
	"github.com/rancher/opni/pkg/auth/session"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/kralicky/totem"

	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
)

type GatewayClient interface {
	grpc.ServiceRegistrar
	ConnStatsQuerier
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

	tlsConfig, err := trustStrategy.TLSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS config: %w", err)
	}

	lg := logger.New().WithGroup("gateway-client")
	ncc, cc, err := dial(ctx, address, id, kr, tlsConfig, lg)
	if err != nil {
		return nil, err
	}

	client := &gatewayClient{
		cc:     cc,
		id:     id,
		logger: lg,
	}

	go func() {
		select {
		case nc := <-ncc:
			client.ncMu.Lock()
			client.nc = nc
			client.ncMu.Unlock()
			<-ctx.Done()
		case <-ctx.Done():
		}
		cc.Close()
	}()
	return client, nil
}

type splicedConn struct {
	name string
	cc   grpc.ClientConnInterface
}

type gatewayClient struct {
	cc     *grpc.ClientConn
	ncMu   sync.Mutex
	nc     net.Conn
	id     string
	logger *slog.Logger

	mu       sync.RWMutex
	services []util.ServicePack[any]
	spliced  []*splicedConn
}

func (gc *gatewayClient) RegisterService(desc *grpc.ServiceDesc, impl any) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.services = append(gc.services, util.PackService(desc, impl))
}

func (gc *gatewayClient) RegisterSplicedStream(cc grpc.ClientConnInterface, name string) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	for _, s := range gc.spliced {
		if s.name == name {
			panic("bug: duplicate spliced stream name")
		}
	}
	gc.spliced = append(gc.spliced, &splicedConn{
		name: name,
		cc:   cc,
	})
}

func dial(ctx context.Context, address, id string, kr keyring.Keyring, tlsConfig *tls.Config, lg *slog.Logger) (<-chan net.Conn, *grpc.ClientConn, error) {
	authChallenge, err := authv2.NewClientChallenge(kr, id, lg)
	if err != nil {
		return nil, nil, err
	}

	sessionAttrChallenge, err := session.NewClientChallenge(kr)
	if err != nil {
		return nil, nil, err
	}

	challengeHandler := challenges.Chained(
		authChallenge,
		challenges.If(sessionAttrChallenge.HasAttributes).Then(sessionAttrChallenge),
	)
	ncc := make(chan net.Conn)

	cc, err := grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithChainStreamInterceptor(
			otelgrpc.StreamClientInterceptor(),
			cluster.StreamClientInterceptor(challengeHandler),
		),
		grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		grpc.WithDefaultCallOptions(
			grpc.WaitForReady(true),
			grpc.MaxCallSendMsgSize(math.MaxInt32),
			grpc.MaxCallRecvMsgSize(math.MaxInt32),
		),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			var err error
			var nc net.Conn
			if strings.Contains(addr, "://") {
				nc, err = util.DialProtocol(ctx, addr)
			} else {
				nc, err = net.Dial("tcp4", addr)
			}
			if err != nil {
				return nil, err
			}
			select {
			case ncc <- nc:
			default:
			}
			return nc, err
		}),
	)
	return ncc, cc, err
}

func (gc *gatewayClient) Connect(ctx context.Context) (_ grpc.ClientConnInterface, errf future.Future[error]) {
	streamClient := streamv1.NewStreamClient(gc.cc)
	stream, err := streamClient.Connect(ctx)
	if err != nil {
		return nil, future.Instant(fmt.Errorf("failed to connect to gateway: %w", err))
	}
	ctx = stream.Context()

	authorizedId := cluster.StreamAuthorizedID(ctx)
	attrs := session.StreamAuthorizedAttributes(ctx)
	var attrNames []string
	lg := gc.logger.With(
		"id", authorizedId,
	)
	if len(attrs) > 0 {
		for _, attr := range attrs {
			attrNames = append(attrNames, attr.Name())
		}
		lg = lg.With("attributes", attrNames)
	}
	lg.Debug("authenticated")

	cachingInterceptor := caching.NewClientGrpcTtlCacher()

	ts, err := totem.NewServer(
		stream,
		totem.WithName("agent"),
		totem.WithInterceptors(totem.InterceptorConfig{
			Incoming: cachingInterceptor.UnaryServerInterceptor(),
			Outgoing: cachingInterceptor.UnaryClientInterceptor(),
		}),
		totem.WithTracerOptions(
			resource.WithAttributes(
				semconv.ServiceNameKey.String("agent"),
				semconv.ServiceInstanceIDKey.String(authorizedId),
			),
		),
	)
	if err != nil {
		return nil, future.Instant(fmt.Errorf("failed to create totem server: %w", err))
	}
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	for _, sp := range gc.services {
		ts.RegisterService(sp.Unpack())
	}

	for _, sc := range gc.spliced {
		sc := sc
		name := fmt.Sprintf("agent|%s", sc.name)
		streamClient := streamv1.NewStreamClient(sc.cc)
		var headerMd metadata.MD
		splicedStream, err := streamClient.Connect(ctx, grpc.Header(&headerMd))
		if err != nil {
			gc.logger.With(
				"name", name,
				logger.Err(err),
			).Warn("failed to connect to spliced stream, skipping")
			continue
		}
		var correlationId string
		if values := headerMd.Get("x-correlation"); len(values) == 1 {
			correlationId = values[0]
		}
		if err := ts.Splice(splicedStream,
			totem.WithName(name),
			totem.WithTracerOptions(resource.WithAttributes(
				semconv.ServiceNameKey.String(name),
				attribute.String("agent", authorizedId),
			)),
		); err != nil {
			gc.logger.With(
				"name", name,
				logger.Err(err),
			).Warn("failed to splice remote stream, skipping")
			continue
		}

		defer func() {
			if errf.IsSet() {
				return
			}
			if _, err := streamClient.Notify(ctx, &streamv1.StreamEvent{
				Type:          streamv1.EventType_DiscoveryComplete,
				CorrelationId: correlationId,
			}); err != nil {
				gc.logger.With(
					"name", name,
					logger.Err(err),
				).Error("failed to notify remote stream")
			}
		}()
	}

	cc, errC := ts.Serve()
	f := future.NewFromChannel(errC)
	if f.IsSet() {
		gc.logger.With(
			logger.Err(f.Get()),
		).Error("failed to connect to gateway")
		// fallthrough
	}
	return cc, f
}

func (gc *gatewayClient) ClientConn() grpc.ClientConnInterface {
	return gc.cc
}
