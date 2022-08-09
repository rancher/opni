package gateway

import (
	"context"
	"errors"

	"github.com/kralicky/totem"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/rancher/opni/pkg/agent"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
)

type remote struct {
	services []*apiextensions.ServiceDescriptor
	cc       *grpc.ClientConn
}

type StreamServer struct {
	streamv1.UnsafeStreamServer
	logger       *zap.SugaredLogger
	handler      ConnectionHandler
	clusterStore storage.ClusterStore
	services     []util.ServicePack[any]
	remotes      []remote
}

func NewStreamServer(handler ConnectionHandler, clusterStore storage.ClusterStore, lg *zap.SugaredLogger) *StreamServer {
	return &StreamServer{
		logger:       lg.Named("grpc"),
		handler:      handler,
		clusterStore: clusterStore,
	}
}

func (s *StreamServer) Connect(stream streamv1.Stream_ConnectServer) error {
	s.logger.Debug("handling new stream connection")
	ts := totem.NewServer(stream)
	for _, service := range s.services {
		ts.RegisterService(service.Unpack())
	}
	id := cluster.StreamAuthorizedID(stream.Context())

	ctx := stream.Context()
	cluster, err := s.clusterStore.GetCluster(ctx, &corev1.Reference{
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
		var services []*descriptorpb.ServiceDescriptorProto
		for _, service := range r.services {
			// check if the service requires a specific capability
			if service.Options.RequireCapability != "" {
				rc := capabilities.Cluster(service.Options.RequireCapability)
				// if the cluster doesn't have the capability, skip the service
				if !capabilities.Has(cluster, rc) {
					continue
				} else {
					requiredCapabilities = append(requiredCapabilities, rc)
				}
			}
			services = append(services, service.ServiceDescriptor)
		}
		if len(services) == 0 {
			continue
		}

		streamClient := streamv1.NewStreamClient(r.cc)
		splicedStream, err := streamClient.Connect(ctx)
		if err != nil {
			return err
		}
		ts.Splice(splicedStream, services...)
	}

	// set up a context which will be canceled when the cluster loses any
	// required capabilities

	eventC, err := s.clusterStore.WatchCluster(stream.Context(), cluster)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	var ca context.CancelFunc
	ctx, ca = capabilities.NewContext(ctx, eventC, requiredCapabilities...)
	defer ca()

	cc, errC := ts.Serve()

	go s.handler.HandleAgentConnection(ctx, agent.NewClientSet(cc))

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
	if len(desc.Streams) > 0 {
		s.logger.With(
			zap.String("service", desc.ServiceName),
		).Fatal("failed to register service: nested streams are currently not supported")
	}
	s.services = append(s.services, util.PackService(desc, impl))
}

func (s *StreamServer) AddRemote(cc *grpc.ClientConn, services *apiextensions.ServiceDescriptorList) error {
	s.remotes = append(s.remotes, remote{
		services: services.Descriptors,
		cc:       cc,
	})
	return nil
}
