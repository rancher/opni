package gateway

import (
	"context"

	"github.com/kralicky/totem"
	"github.com/rancher/opni/pkg/agent"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/descriptorpb"
)

type remote struct {
	services []*descriptorpb.ServiceDescriptorProto
	cc       *grpc.ClientConn
}

type StreamServer struct {
	streamv1.UnsafeStreamServer
	logger   *zap.SugaredLogger
	handler  ConnectionHandler
	services []util.ServicePack[any]
	remotes  []remote
}

func NewStreamServer(handler ConnectionHandler, lg *zap.SugaredLogger) *StreamServer {
	return &StreamServer{
		logger:  lg.Named("grpc"),
		handler: handler,
	}
}

func (s *StreamServer) Connect(stream streamv1.Stream_ConnectServer) error {
	s.logger.Debug("handling new stream connection")
	ts := totem.NewServer(stream)
	for _, service := range s.services {
		ts.RegisterService(service.Unpack())
	}
	for _, r := range s.remotes {
		streamClient := streamv1.NewStreamClient(r.cc)
		splicedStream, err := streamClient.Connect(context.Background())
		if err != nil {
			return err
		}
		ts.Splice(splicedStream, r.services...)
	}
	cc, errC := ts.Serve()

	ctx, ca := context.WithCancel(stream.Context())
	defer ca()
	go s.handler.HandleAgentConnection(ctx, agent.NewClientSet(cc))

	err := <-errC
	if err != nil {
		s.logger.With(
			zap.Error(err),
		).Info("agent stream disconnected")
	}
	return err
}

func (s *StreamServer) RegisterService(desc *grpc.ServiceDesc, impl any) {
	if len(desc.Streams) > 0 {
		s.logger.With(
			zap.String("service", desc.ServiceName),
		).Fatal("failed to register service: nested streams are currently not supported")
	}
	s.services = append(s.services, util.PackService(desc, impl))
}

func (s *StreamServer) AddRemote(cc *grpc.ClientConn, services []*descriptorpb.ServiceDescriptorProto) error {
	s.remotes = append(s.remotes, remote{
		services: services,
		cc:       cc,
	})
	return nil
}
