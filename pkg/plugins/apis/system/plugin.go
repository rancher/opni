package system

import (
	"context"
	"sync"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SystemPluginClient interface {
	UseManagementAPI(management.ManagementClient)
	UseKeyValueStore(KVStoreClient)
}

type SystemPluginServer interface {
	ServeManagementAPI(management.ManagementServer)
	ServeKeyValueStore(storage.KeyValueStore)
}

const SystemPluginID = "system"

type systemPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	client SystemPluginClient
}

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
	})
	return nil
}

type systemPluginClientImpl struct {
	UnimplementedSystemServer
	broker *plugin.GRPCBroker
	server *grpc.Server
	client SystemPluginClient
}

func (c *systemPluginClientImpl) UseManagementAPI(ctx context.Context, in *BrokerID) (*emptypb.Empty, error) {
	cc, err := c.broker.Dial(in.Id)
	if err != nil {
		return nil, err
	}
	defer cc.Close()
	client := management.NewManagementClient(cc)
	c.client.UseManagementAPI(client)
	return &emptypb.Empty{}, nil
}

func (c *systemPluginClientImpl) UseKeyValueStore(ctx context.Context, in *BrokerID) (*emptypb.Empty, error) {
	cc, err := c.broker.Dial(in.Id)
	if err != nil {
		return nil, err
	}
	defer cc.Close()
	c.client.UseKeyValueStore(&kvStoreClientImpl{
		ctx:    ctx,
		client: NewKeyValueStoreClient(cc),
	})
	return &emptypb.Empty{}, nil
}

// Gateway side

func (p *systemPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
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

func (s *systemPluginHandler) ServeManagementAPI(api management.ManagementServer) {
	s.serveSystemApi(
		func(srv *grpc.Server) {
			management.RegisterManagementServer(srv, api)
		},
		func(id uint32) {
			s.client.UseManagementAPI(s.ctx, &BrokerID{
				Id: id,
			})
		},
	)
}

func (s *systemPluginHandler) ServeKeyValueStore(store storage.KeyValueStore) {
	kvStoreSrv := &kvStoreServer{
		store: store,
	}
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

func init() {
	plugins.Scheme.Add(SystemPluginID, NewPlugin(nil))
}

func (s *systemPluginHandler) serveSystemApi(regCallback func(*grpc.Server), useCallback func(uint32)) {
	id := s.broker.NextId()
	var srv *grpc.Server
	once := sync.Once{}
	go s.broker.AcceptAndServe(id, func(so []grpc.ServerOption) *grpc.Server {
		srv = grpc.NewServer(so...)
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
	if srv != nil {
		once.Do(srv.Stop)
	}
}
