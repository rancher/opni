package stream

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/kralicky/totem"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func NewAgentPlugin(p StreamAPIExtension) plugin.Plugin {
	pc, _, _, ok := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)
	name := "unknown"
	if ok {
		fnName := fn.Name()
		name = fnName[strings.LastIndex(fnName, "plugins/")+len("plugins/") : strings.LastIndex(fnName, ".")]
	}

	ext := &agentStreamExtensionServerImpl{
		name:             name,
		logger:           logger.NewPluginLogger().Named(name).Named("stream"),
		streamClientCond: sync.NewCond(&sync.Mutex{}),
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
		if clientDisconnectHandler, ok := p.(StreamClientDisconnectHandler); ok {
			ext.clientDisconnectHandler = clientDisconnectHandler
		}
	}
	return &streamApiExtensionPlugin[*agentStreamExtensionServerImpl]{
		extensionSrv: ext,
	}
}

type agentStreamExtensionServerImpl struct {
	streamv1.UnsafeStreamServer
	apiextensions.UnimplementedStreamAPIExtensionServer

	name                    string
	servers                 []*richServer
	clientHandler           StreamClientHandler
	clientDisconnectHandler StreamClientDisconnectHandler
	logger                  *zap.SugaredLogger

	streamClientCond *sync.Cond
	streamClient     grpc.ClientConnInterface
}

// Implements streamv1.StreamServer
func (e *agentStreamExtensionServerImpl) Connect(stream streamv1.Stream_ConnectServer) error {
	e.logger.Debug("stream connected")

	opts := []totem.ServerOption{
		totem.WithName("plugin_" + e.name),
	}
	ts, err := totem.NewServer(stream, opts...)

	if err != nil {
		e.logger.With(
			zap.Error(err),
		).Error("failed to create stream server")
		return err
	}
	for _, srv := range e.servers {
		ts.RegisterService(srv.Desc, srv.Impl)
	}

	cc, errC := ts.Serve()

	e.logger.Debug("stream server started")

	select {
	case err := <-errC:
		if errors.Is(err, io.EOF) {
			e.logger.Debug("stream disconnected")
		} else {
			e.logger.With(
				zap.Error(err),
			).Warn("stream disconnected with error")
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
		if e.clientDisconnectHandler != nil {
			e.logger.Debug("calling disconnect handler")
			e.clientDisconnectHandler.StreamDisconnected()
		}
		e.logger.Debug("stream client is no longer available")
		e.streamClient = nil
		e.streamClientCond.Broadcast()
		e.streamClientCond.L.Unlock()
	}()

	return <-errC
}

func (e *agentStreamExtensionServerImpl) Notify(ctx context.Context, event *streamv1.StreamEvent) (*emptypb.Empty, error) {
	e.logger.With(
		"type", event.Type.String(),
	).Debugf("received notify event for '%s'", e.name)
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

	if event.Type == streamv1.EventType_DiscoveryComplete {
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
		if e.clientHandler != nil {
			e.logger.Debug("calling client handler")
			go e.clientHandler.UseStreamClient(e.streamClient)
		} else {
			e.logger.Warn("bug: no client handler or stream client")
		}
		e.streamClientCond.L.Unlock()
	}
	return &emptypb.Empty{}, nil
}
