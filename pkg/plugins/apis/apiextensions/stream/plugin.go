package stream

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/kralicky/totem"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
)

const (
	StreamAPIExtensionPluginID = "opni.apiextensions.StreamAPIExtension"
	ServiceID                  = "apiextensions.StreamAPIExtension"
)

type StreamAPIExtension interface {
	StreamServers() []Server
}

// A plugin can optionally implement StreamClientHandler to obtain a
// grpc.ClientConnInterface to the plugin's side of the spliced stream.
type StreamClientHandler interface {
	UseStreamClient(client grpc.ClientConnInterface)
}

// A plugin can optionally implement StreamClientDisconnectHandler to
// be notified when the stream disconnects
type StreamClientDisconnectHandler interface {
	StreamClientHandler
	StreamDisconnected()
}

type Server struct {
	Desc              *grpc.ServiceDesc
	Impl              interface{}
	RequireCapability string
}

type richServer struct {
	Server
	richDesc *desc.ServiceDescriptor
}

type streamApiExtensionPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	extensionSrv *streamExtensionServerImpl
}

var _ plugin.GRPCPlugin = (*streamApiExtensionPlugin)(nil)

func (p *streamApiExtensionPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	apiextensions.RegisterStreamAPIExtensionServer(s, p.extensionSrv)
	streamv1.RegisterStreamServer(s, p.extensionSrv)
	return nil
}

func (p *streamApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	// TODO: need to check for stream service availability, otherwise we get 'unknown service stream.Stream' errors
	// which are not very helpful in debugging
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return apiextensions.NewStreamAPIExtensionClient(c), nil
}

func NewPlugin(p StreamAPIExtension) plugin.Plugin {
	pc, _, _, ok := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)
	name := "unknown"
	if ok {
		fnName := fn.Name()
		name = fnName[strings.LastIndex(fnName, "plugins/")+len("plugins/") : strings.LastIndex(fnName, ".")]
	}

	ext := &streamExtensionServerImpl{
		name:             name,
		logger:           logger.NewPluginLogger().Named(name).Named("stream"),
		streamClientCond: sync.NewCond(&sync.Mutex{}),
		// connectSem:       make(chan struct{}, 1),
	}
	if p != nil {
		servers := p.StreamServers()
		for _, srv := range servers {
			descriptor, err := grpcreflect.LoadServiceDescriptor(srv.Desc)
			if err != nil {
				panic(err)
			}
			ext.servers = append(ext.servers, &richServer{
				Server:   srv,
				richDesc: descriptor,
			})
		}
		if clientHandler, ok := p.(StreamClientHandler); ok {
			ext.clientHandler = clientHandler
		}
	}
	return &streamApiExtensionPlugin{
		extensionSrv: ext,
	}
}

type streamExtensionServerImpl struct {
	streamv1.UnsafeStreamServer
	name string
	apiextensions.UnimplementedStreamAPIExtensionServer
	servers       []*richServer
	clientHandler StreamClientHandler
	logger        *zap.SugaredLogger

	streamClientCond *sync.Cond
	streamClient     grpc.ClientConnInterface

	// connectSem chan struct{}
}

// Implements streamv1.StreamServer
func (e *streamExtensionServerImpl) Connect(stream streamv1.Stream_ConnectServer) error {
	// select {
	// case e.connectSem <- struct{}{}:
	// 	defer func() { <-e.connectSem }()
	// default:
	// 	return status.Error(codes.FailedPrecondition, "another connection is already active")
	// }

	e.logger.Debug("stream connected")
	ts, err := totem.NewServer(stream, totem.WithName("plugin_"+e.name))
	if err != nil {
		e.logger.With(
			zap.Error(err),
		).Error("failed to create totem server")
		return err
	}
	for _, srv := range e.servers {
		ts.RegisterService(srv.Desc, srv.Impl)
	}

	cc, errC := ts.Serve()

	e.logger.Debug("totem server started")

	select {
	case err := <-errC:
		if errors.Is(err, io.EOF) {
			e.logger.Debug("stream disconnected")
		} else {
			e.logger.With(
				zap.Error(err),
			).Warn("stream disconnected with error")
		}
		if e.clientHandler != nil {
			if d, ok := e.clientHandler.(StreamClientDisconnectHandler); ok {
				e.logger.Debug("notifying agent of disconnect")
				d.StreamDisconnected()
			}
		}
		return err
	default:
	}

	e.streamClientCond.L.Lock()
	e.logger.Debug("stream client is now available")
	e.streamClient = cc
	e.streamClientCond.Broadcast()
	e.streamClientCond.L.Unlock()

	defer func() {
		e.streamClientCond.L.Lock()
		e.logger.Debug("stream client is no longer available")
		e.streamClient = nil
		e.streamClientCond.Broadcast()
		e.streamClientCond.L.Unlock()
	}()

	return <-errC
}

func (e *streamExtensionServerImpl) Notify(ctx context.Context, event *streamv1.StreamEvent) (*emptypb.Empty, error) {
	e.logger.With(
		"type", event.Type.String(),
	).Debug("received notify event")
	returned := make(chan struct{})
	defer close(returned)
	go func() {
		select {
		case <-ctx.Done():
			e.streamClientCond.L.Lock()
			e.streamClientCond.Broadcast()
			e.streamClientCond.L.Unlock()
		case <-returned:
		}
	}()

	switch event.Type {
	case streamv1.EventType_DiscoveryComplete:
		e.logger.Debug("processing discovery complete event")
		e.streamClientCond.L.Lock()
		for e.streamClient == nil {
			if ctx.Err() != nil {
				e.streamClientCond.L.Unlock()
				e.logger.Debug("context cancelled while waiting for stream client")
				return nil, ctx.Err()
			}
			e.logger.Debug("waiting for stream client to become available")
			e.streamClientCond.Wait()
		}
		e.streamClientCond.L.Unlock()

		if e.clientHandler != nil && e.streamClient != nil {
			e.logger.Debug("calling client handler")
			go e.clientHandler.UseStreamClient(e.streamClient)
		} else {
			e.logger.Warn("bug: no client handler or stream client")
		}
	}
	return &emptypb.Empty{}, nil
}

// func (e *mgmtExtensionServerImpl) Services(context.Context, *emptypb.Empty) (*apiextensions.ServiceDescriptorList, error) {
// 	list := []*apiextensions.ServiceDescriptor{}
// 	for _, srv := range e.servers {
// 		fqn := srv.richDesc.GetFullyQualifiedName()
// 		sd := srv.richDesc.AsServiceDescriptorProto()
// 		sd.Name = &fqn
// 		list = append(list, &apiextensions.ServiceDescriptor{
// 			ServiceDescriptor: sd,
// 			Options: &apiextensions.ServiceOptions{
// 				RequireCapability: srv.RequireCapability,
// 			},
// 		})
// 	}
// 	return &apiextensions.ServiceDescriptorList{
// 		Items: list,
// 	}, nil
// }

var _ apiextensions.StreamAPIExtensionServer = (*streamExtensionServerImpl)(nil)

func init() {
	plugins.GatewayScheme.Add(StreamAPIExtensionPluginID, NewPlugin(nil))
	plugins.AgentScheme.Add(StreamAPIExtensionPluginID, NewPlugin(nil))
}
