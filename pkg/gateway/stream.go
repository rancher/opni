package gateway

import (
	"context"
	"errors"

	"github.com/kralicky/totem"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	agentv1 "github.com/rancher/opni/pkg/agent"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
)

type remote struct {
	cc *grpc.ClientConn
}

type StreamServer struct {
	streamv1.UnimplementedStreamServer
	logger       *zap.SugaredLogger
	handler      ConnectionHandler
	clusterStore storage.ClusterStore
	services     []util.ServicePack[any]
	remotes      []remote
	interceptor  grpc.UnaryServerInterceptor
}

func NewStreamServer(
	handler ConnectionHandler,
	clusterStore storage.ClusterStore,
	interceptor grpc.UnaryServerInterceptor,
	lg *zap.SugaredLogger,
) *StreamServer {
	return &StreamServer{
		logger:       lg.Named("grpc"),
		handler:      handler,
		clusterStore: clusterStore,
		interceptor:  interceptor,
	}
}

func (s *StreamServer) Connect(stream streamv1.Stream_ConnectServer) error {
	s.logger.Debug("handling new stream connection")
	ts, err := totem.NewServer(stream,
		totem.WithName("gateway-server"),
		// totem.WithUnaryServerInterceptor(s.interceptor),
	)
	if err != nil {
		return err
	}
	for _, service := range s.services {
		ts.RegisterService(service.Unpack())
	}

	ctx := stream.Context()
	id := cluster.StreamAuthorizedID(ctx)

	c, err := s.clusterStore.GetCluster(ctx, &corev1.Reference{
		Id: id,
	})
	if err != nil {
		return err
	}

	// find any remote streams that match the cluster's capabilities, or that
	// don't require any capabilities. If the cluster loses any required
	// capabilities once it is connected, the stream will be closed and the
	// cluster will need to reconnect.
	requiredCapabilities := []*corev1.ClusterCapability{}
	for _, r := range s.remotes {
		// var services []*descriptorpb.ServiceDescriptorProto
		// for _, service := range r.services {
		// 	// check if the service requires a specific capability
		// 	if service.Options.RequireCapability != "" {
		// 		rc := capabilities.Cluster(service.Options.RequireCapability)
		// 		// if the cluster doesn't have the capability, skip the service
		// 		if !capabilities.Has(cluster, rc) {
		// 			continue
		// 		} else {
		// 			requiredCapabilities = append(requiredCapabilities, rc)
		// 		}
		// 	}
		// 	services = append(services, service.ServiceDescriptor)
		// }
		// if len(services) == 0 {
		// 	continue
		// }

		streamClient := streamv1.NewStreamClient(r.cc)
		ctx := cluster.AuthorizedOutgoingContext(ctx)
		splicedStream, err := streamClient.Connect(ctx, grpc.WaitForReady(true))
		if err != nil {
			return err
		}
		if err := ts.Splice(splicedStream); err != nil {
			return err
		}
	}

	// set up a context which will be canceled when the cluster loses any
	// required capabilities

	eventC, err := s.clusterStore.WatchCluster(ctx, c)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	var ca context.CancelFunc
	ctx, ca = capabilities.NewContext(ctx, eventC, requiredCapabilities...)
	defer ca()

	cc, errC := ts.Serve()

	go s.handler.HandleAgentConnection(ctx, agentv1.NewClientSet(cc))

	select {
	case err = <-errC:
		if err != nil {
			s.logger.With(
				zap.Error(err),
			).Warn("agent stream disconnected")
		}
		return status.Error(codes.Unavailable, err.Error())
	case <-ctx.Done():
		s.logger.With(
			zap.Error(ctx.Err()),
		).Info("agent stream closing")
		err := ctx.Err()
		if errors.Is(err, capabilities.ErrObjectDeleted) ||
			errors.Is(err, capabilities.ErrCapabilityNotFound) {
			return status.Error(codes.FailedPrecondition, err.Error())
		}
		return status.Error(codes.Unavailable, err.Error())
	}
}

func (s *StreamServer) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.logger.With(
		zap.String("service", desc.ServiceName),
	).Debug("registering service")
	if len(desc.Streams) > 0 {
		s.logger.With(
			zap.String("service", desc.ServiceName),
		).Fatal("failed to register service: nested streams are currently not supported")
	}
	s.services = append(s.services, util.PackService(desc, impl))
}

func (s *StreamServer) AddRemote(cc *grpc.ClientConn) error {
	s.logger.With(
		zap.String("address", cc.Target()),
	).Debug("adding remote connection")
	s.remotes = append(s.remotes, remote{
		cc: cc,
	})
	return nil
}
