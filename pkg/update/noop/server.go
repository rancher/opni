package noop

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/urn"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type syncServer struct {
	SyncServerOptions
}

var _ update.UpdateTypeHandler = (*syncServer)(nil)

type SyncServerOptions struct {
	allowedTypes []urn.UpdateType
}

type SyncServerOption func(*SyncServerOptions)

func (o *SyncServerOptions) apply(opts ...SyncServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithAllowedTypes(allowedTypes ...urn.UpdateType) SyncServerOption {
	return func(o *SyncServerOptions) {
		o.allowedTypes = allowedTypes
	}
}
func NewSyncServer(opts ...SyncServerOption) update.UpdateTypeHandler {
	options := SyncServerOptions{
		allowedTypes: urn.AllUpdateTypes(),
	}
	options.apply(opts...)

	return &syncServer{
		SyncServerOptions: options,
	}
}

func (s *syncServer) CalculateExpectedManifest(ctx context.Context, updateType urn.UpdateType) (*controlv1.UpdateManifest, error) {
	if !slices.Contains(s.allowedTypes, updateType) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid update type: %q", updateType)
	}
	return &controlv1.UpdateManifest{
		Items: []*controlv1.UpdateManifestEntry{
			{
				Package: urn.NewOpniURN(updateType, updateStrategy, "unmanaged").String(),
				Path:    "unmanaged",
				Digest:  emptyDigest,
			},
		},
	}, nil
}

func (s *syncServer) CalculateUpdate(ctx context.Context, manifest *controlv1.UpdateManifest) (*controlv1.PatchList, error) {
	updateType, err := update.GetType(manifest.GetItems())
	if err != nil {
		return nil, err
	}
	if !slices.Contains(s.allowedTypes, updateType) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid update type: %q", updateType)
	}
	return &controlv1.PatchList{
		Items: []*controlv1.PatchSpec{
			{
				Op:        controlv1.PatchOp_None,
				Package:   urn.NewOpniURN(updateType, updateStrategy, "unmanaged").String(),
				Path:      "unmanaged",
				OldDigest: emptyDigest,
				NewDigest: emptyDigest,
			},
		},
	}, nil
}

func (*syncServer) Collectors() []prometheus.Collector {
	return nil
}

func (*syncServer) Strategy() string {
	return updateStrategy
}
