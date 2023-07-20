package update

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/urn"
	"github.com/rancher/opni/pkg/util/streams"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var _ controlv1.UpdateSyncServer = (*UpdateServer)(nil)

type UpdateServer struct {
	controlv1.UnsafeUpdateSyncServer
	logger         *zap.SugaredLogger
	updateHandlers map[string]UpdateTypeHandler
	handlerMu      sync.RWMutex
}

func NewUpdateServer(lg *zap.SugaredLogger) *UpdateServer {
	return &UpdateServer{
		logger:         lg.Named("update-server"),
		updateHandlers: make(map[string]UpdateTypeHandler),
	}
}

func (s *UpdateServer) RegisterUpdateHandler(strategy string, handler UpdateTypeHandler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.logger.Infof("registering update handler for strategy %q", strategy)
	s.updateHandlers[strategy] = handler
}

// SyncManifest implements UpdateSync.  It expects a manifest with a single
// type and a single strategy.  It will return an error if either of these
// conditions are not met.  The package URN must be in the following format
// urn:opni:<type>:<strategy>:<name>
func (s *UpdateServer) SyncManifest(ctx context.Context, manifest *controlv1.UpdateManifest) (*controlv1.SyncResults, error) {
	lg := s.logger
	strategy, err := getStrategy(manifest.GetItems())
	if err != nil {
		lg.With(
			zap.Error(err),
		).Warn("could not sync agent manifest")
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	s.logger.With(
		"strategy", strategy,
	).Info("syncing agent manifest")

	s.handlerMu.RLock()
	handler, ok := s.updateHandlers[strategy]
	if !ok {
		return nil, status.Errorf(codes.Unimplemented, "no handler for update strategy: %q", strategy)
	}
	s.handlerMu.RUnlock()

	patchList, err := handler.CalculateUpdate(ctx, manifest)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Error("error calculating updates")
		return nil, err
	}

	lg.With(
		"patches", len(patchList.GetItems()),
	).Info("computed updates")

	return &controlv1.SyncResults{
		RequiredPatches: patchList,
	}, nil
}

type manifestMetadataKeyType struct{}

var manifestMetadataKey = manifestMetadataKeyType{}

func ManifestMetadataFromContext(ctx context.Context) (*controlv1.UpdateManifest, bool) {
	v, ok := ctx.Value(manifestMetadataKey).(*controlv1.UpdateManifest)
	return v, ok
}

type checkedManifest struct {
	manifest   *controlv1.UpdateManifest
	updateType string
	upToDate   bool
}

func (s *UpdateServer) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, ok := metadata.FromIncomingContext(stream.Context())
		if !ok {
			return status.Errorf(codes.InvalidArgument, "request metadata not found")
		}

		checked := []checkedManifest{}
		for _, updateType := range urn.AllUpdateTypes() {
			strategy := md.Get(controlv1.UpdateStrategyKeyForType(updateType))
			if len(strategy) != 1 {
				return status.Errorf(codes.InvalidArgument, "update strategy for type %q missing or invalid", updateType)
			}
			handler, ok := s.updateHandlers[strategy[0]]
			if !ok {
				return status.Errorf(codes.Unimplemented, "no handler for update strategy: %q", strategy[0])
			}

			expected, err := handler.CalculateExpectedManifest(stream.Context(), updateType)
			if err != nil {
				return err
			}

			actual := md.Get(controlv1.ManifestDigestKeyForType(updateType))
			if len(actual) != 1 {
				return status.Errorf(codes.InvalidArgument, "manifest digest for type %q missing or invalid", updateType)
			}
			expectedDigest := expected.Digest()
			checked = append(checked, checkedManifest{
				manifest:   expected,
				updateType: string(updateType),
				upToDate:   actual[0] == expectedDigest,
			})
		}
		typesOutOfDate := []string{}
		for _, c := range checked {
			if !c.upToDate {
				typesOutOfDate = append(typesOutOfDate, c.updateType)
			}
		}
		if len(typesOutOfDate) > 0 {
			sort.Strings(typesOutOfDate)
			return status.Errorf(codes.FailedPrecondition, "%s resources out of date", strings.Join(typesOutOfDate, ", "))
		}

		combined := &controlv1.UpdateManifest{}
		for _, c := range checked {
			combined.Items = append(combined.Items, c.manifest.Items...)
		}
		combined.Sort()

		return handler(srv, &streams.ServerStreamWithContext{
			Stream: stream,
			Ctx:    context.WithValue(stream.Context(), manifestMetadataKey, combined),
		})
	}
}

func (s *UpdateServer) Collectors() []prometheus.Collector {
	var collectors []prometheus.Collector
	s.handlerMu.RLock()
	defer s.handlerMu.RUnlock()
	for _, handler := range s.updateHandlers {
		collectors = append(collectors, handler.Collectors()...)
	}
	return collectors
}
