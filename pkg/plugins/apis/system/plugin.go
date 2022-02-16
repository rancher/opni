package system

import (
	"context"
	"os"
	"os/signal"
	"sync"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"google.golang.org/grpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type SystemPluginClient interface {
	UseManagementAPI(api management.ManagementClient)
}

type SystemPluginServer interface {
	ServeManagementAPI(api management.ManagementServer)
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

	mgmtClient management.ManagementClient
}

func (c *systemPluginClientImpl) UseManagementAPI(ctx context.Context, in *BrokerID) (*emptypb.Empty, error) {
	if c.mgmtClient == nil {
		cc, err := c.broker.Dial(in.Id)
		if err != nil {
			return nil, err
		}
		c.mgmtClient = management.NewManagementClient(cc)
	}
	c.client.UseManagementAPI(c.mgmtClient)
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
	id := s.broker.NextId()
	var srv *grpc.Server
	once := sync.Once{}
	go s.broker.AcceptAndServe(id, func(so []grpc.ServerOption) *grpc.Server {
		srv = grpc.NewServer(so...)
		go func() {
			<-s.ctx.Done()
			once.Do(srv.Stop)
		}()
		management.RegisterManagementServer(srv, api)
		return srv
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.client.UseManagementAPI(s.ctx, &BrokerID{
			Id: id,
		})
	}()

	select {
	case <-s.ctx.Done():
	case <-done:
	}
	if srv != nil {
		once.Do(srv.Stop)
	}
}

func init() {
	plugins.Scheme.Add(SystemPluginID, NewPlugin(nil))

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		plugin.CleanupClients()
		os.Exit(0)
	}()
}
