package system

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/go-plugin"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SystemPluginClient interface {
	UseManagementAPI(managementv1.ManagementClient)
	UseCachingProvider(caching.CachingProvider[proto.Message])
	UseKeyValueStore(KeyValueStoreClient)
	UseAPIExtensions(ExtensionClientInterface)
	mustEmbedUnimplementedSystemPluginClient()
}

// UnimplementedSystemPluginClient must be embedded to have forward compatible implementations.
type UnimplementedSystemPluginClient struct{}

func (UnimplementedSystemPluginClient) UseManagementAPI(managementv1.ManagementClient)            {}
func (UnimplementedSystemPluginClient) UseKeyValueStore(KeyValueStoreClient)                      {}
func (UnimplementedSystemPluginClient) UseAPIExtensions(ExtensionClientInterface)                 {}
func (UnimplementedSystemPluginClient) UseCachingProvider(caching.CachingProvider[proto.Message]) {}
func (UnimplementedSystemPluginClient) mustEmbedUnimplementedSystemPluginClient()                 {}

type SystemPluginServer interface {
	ServeManagementAPI(server managementv1.ManagementServer)
	ServeKeyValueStore(namespace string, backend storage.Backend)
	ServeAPIExtensions(dialAddress string) error
	ServeCachingProvider()
}

const (
	SystemPluginID  = "opni.System"
	KVServiceID     = "system.KeyValueStore"
	SystemServiceID = "system.System"
)

type systemPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	client SystemPluginClient
}

var _ plugin.Plugin = (*systemPlugin)(nil)

func NewPlugin(client SystemPluginClient) plugin.Plugin {
	return &systemPlugin{
		client: client,
	}
}

// Plugin side

func (p *systemPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	RegisterSystemServer(s, &systemPluginClientImpl{
		broker: broker,
		server: s,
		client: p.client,
		cache:  caching.NewClientGrpcTtlCacher(),
	})
	return nil
}

type systemPluginClientImpl struct {
	UnsafeSystemServer
	broker *plugin.GRPCBroker
	server *grpc.Server
	client SystemPluginClient
	cache  caching.GrpcCachingInterceptor
}

func (c *systemPluginClientImpl) UseManagementAPI(_ context.Context, in *BrokerID) (*emptypb.Empty, error) {
	cc, err := c.broker.Dial(
		in.Id,
		grpc.WithChainUnaryInterceptor(
			c.cache.UnaryClientInterceptor(),
		),
	)
	if err != nil {
		return nil, err
	}
	defer cc.Close()
	client := managementv1.NewManagementClient(cc)
	c.client.UseManagementAPI(client)
	return &emptypb.Empty{}, nil
}

func (c *systemPluginClientImpl) UseKeyValueStore(_ context.Context, in *BrokerID) (*emptypb.Empty, error) {
	cc, err := c.broker.Dial(in.Id)
	if err != nil {
		return nil, err
	}
	defer cc.Close()
	c.client.UseKeyValueStore(NewKeyValueStoreClient(cc))
	return &emptypb.Empty{}, nil
}

func (c *systemPluginClientImpl) UseAPIExtensions(ctx context.Context, addr *DialAddress) (*emptypb.Empty, error) {
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithContextDialer(util.DialProtocol),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  1 * time.Millisecond,
				Multiplier: 2,
				Jitter:     0.2,
				MaxDelay:   1 * time.Second,
			},
		}),
		grpc.WithChainUnaryInterceptor(
			c.cache.UnaryClientInterceptor(),
		),
	}
	cc, err := grpc.DialContext(ctx, addr.Value,
		dialOpts...,
	)
	if err != nil {
		return nil, err
	}
	defer cc.Close()
	c.client.UseAPIExtensions(&apiExtensionInterfaceImpl{
		managementClientConn: cc,
	})
	return &emptypb.Empty{}, nil
}

func (c *systemPluginClientImpl) UseCachingProvider(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// no external caching provider served
	c.client.UseCachingProvider(c.cache)
	return &emptypb.Empty{}, nil
}

// Gateway side

func (p *systemPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	systemErr := plugins.CheckAvailability(ctx, c, SystemServiceID)
	kvErr := plugins.CheckAvailability(ctx, c, KVServiceID)
	if systemErr != nil && kvErr != nil {
		return nil, systemErr
	}

	return &systemPluginHandler{
		ctx:    ctx,
		broker: broker,
		client: NewSystemClient(c),
	}, nil
}

type systemPluginHandler struct {
	ctx    context.Context
	broker *plugin.GRPCBroker
	client SystemClient
}

func (s *systemPluginHandler) ServeManagementAPI(api managementv1.ManagementServer) {
	s.serveSystemApi(
		func(srv *grpc.Server) {
			managementv1.RegisterManagementServer(srv, api)
		},
		func(id uint32) {
			s.client.UseManagementAPI(s.ctx, &BrokerID{
				Id: id,
			})
		},
	)
}

func (s *systemPluginHandler) ServeKeyValueStore(namespace string, backend storage.Backend) {
	store := backend.KeyValueStore(namespace)
	var lockManager storage.LockManager
	if lmb, ok := backend.(storage.LockManagerBroker); ok {
		lockManager = lmb.LockManager(namespace)
	}

	kvStoreSrv := NewKVStoreServer(store, lockManager)
	s.serveSystemApi(
		func(srv *grpc.Server) {
			RegisterKeyValueStoreServer(srv, kvStoreSrv)
		},
		func(id uint32) {
			s.client.UseKeyValueStore(s.ctx, &BrokerID{
				Id: id,
			})
		},
	)
}

func (s *systemPluginHandler) ServeAPIExtensions(dialAddr string) error {
	_, err := s.client.UseAPIExtensions(s.ctx, &DialAddress{
		Value: dialAddr,
	})
	return err
}

func (s *systemPluginHandler) ServeCachingProvider() {
	s.client.UseCachingProvider(s.ctx, &emptypb.Empty{})
}

func init() {
	plugins.GatewayScheme.Add(SystemPluginID, NewPlugin(nil))
}

func (s *systemPluginHandler) serveSystemApi(regCallback func(*grpc.Server), useCallback func(uint32)) {
	id := s.broker.NextId()
	var srv *grpc.Server
	srvLock := make(chan struct{})
	once := sync.Once{}
	go s.broker.AcceptAndServe(id, func(so []grpc.ServerOption) *grpc.Server {
		so = append(so,
			grpc.ChainStreamInterceptor(otelgrpc.StreamServerInterceptor()),
			grpc.ChainUnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		)
		srv = grpc.NewServer(so...)
		close(srvLock)
		go func() {
			<-s.ctx.Done()
			once.Do(srv.Stop)
		}()
		regCallback(srv)
		return srv
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		useCallback(id)
	}()
	select {
	case <-s.ctx.Done():
	case <-done:
	}
	<-srvLock
	if srv != nil {
		once.Do(srv.Stop)
	}
}
