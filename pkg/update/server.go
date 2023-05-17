package update

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var _ controlv1.UpdateSyncServer = (*UpdateServer)(nil)

type UpdateServer struct {
	controlv1.UnsafeUpdateSyncServer
	updateHandlers map[string]UpdateTypeHandler
}

func NewUpdateServer() *UpdateServer {
	return &UpdateServer{
		updateHandlers: make(map[string]UpdateTypeHandler),
	}
}

func (s *UpdateServer) RegisterUpdateHandler(strategy string, handler UpdateTypeHandler) {
	s.updateHandlers[strategy] = handler
}

// SyncManifest implements UpdateSync.  It expects a manifest with a single
// type and a single strategy.  It will return an error if either of these
// conditions are not met.  The package URN must be in the following format
// urn:opni:<type>:<strategy>:<name>
func (s *UpdateServer) SyncManifest(ctx context.Context, manifest *controlv1.UpdateManifest) (*controlv1.SyncResults, error) {
	strategy, err := getStrategy(manifest.GetItems())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	handler, ok := s.updateHandlers[strategy]
	if !ok {
		return nil, status.Error(codes.Unimplemented, "no handler for strategy")
	}

	patchList, desired, err := handler.CalculateUpdate(ctx, manifest)
	if err != nil {
		return nil, err
	}

	return &controlv1.SyncResults{
		RequiredPatches: patchList,
		DesiredState:    desired,
	}, nil
}

func (s *UpdateServer) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, ok := metadata.FromIncomingContext(stream.Context())
		if !ok {
			return handler(srv, stream)
		}

		strategy := md.Get(controlv1.UpdateStrategyKey)
		if len(strategy) != 1 {
			return handler(srv, stream)
		}

		updateHandler, ok := s.updateHandlers[strategy[0]]
		if !ok {
			return handler(srv, stream)
		}

		interceptor, ok := updateHandler.(UpdateStreamInterceptor)
		if !ok {
			return handler(srv, stream)
		}

		return interceptor.StreamServerInterceptor()(srv, stream, info, handler)
	}
}

func (s *UpdateServer) Collectors() []prometheus.Collector {
	var collectors []prometheus.Collector
	for _, handler := range s.updateHandlers {
		collectors = append(collectors, handler.Collectors()...)
	}
	return collectors
}
