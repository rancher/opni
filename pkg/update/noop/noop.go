package noop

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/update"
)

const (
	updateStrategy = "noop"
)

type noopUpdate struct{}

func NewNoopUpdate() *noopUpdate {
	return &noopUpdate{}
}

func (n *noopUpdate) Strategy() string {
	return updateStrategy
}

func (n *noopUpdate) Collectors() []prometheus.Collector {
	return []prometheus.Collector{}
}

func (n *noopUpdate) CalculateUpdate(
	_ context.Context,
	manifest *controlv1.UpdateManifest,
) (*controlv1.PatchList, *controlv1.UpdateManifest, error) {
	patches := []*controlv1.PatchSpec{}
	for _, item := range manifest.GetItems() {
		patches = append(patches, &controlv1.PatchSpec{
			Op:        controlv1.PatchOp_None,
			Package:   item.Package,
			Path:      item.Path,
			OldDigest: item.Digest,
			NewDigest: item.Digest,
		})
	}
	return &controlv1.PatchList{
		Items: patches,
	}, manifest, nil
}

func (n *noopUpdate) GetCurrentManifest(_ context.Context) (*controlv1.UpdateManifest, error) {
	return &controlv1.UpdateManifest{
		Items: nil,
	}, nil
}

func (n *noopUpdate) HandleSyncResults(_ context.Context, _ *controlv1.SyncResults) error {
	return nil
}

func init() {
	update.RegisterAgentSyncHandlerBuilder(updateStrategy, func(args ...any) (update.SyncHandler, error) {
		return NewNoopUpdate(), nil
	})
	update.RegisterPluginSyncHandlerBuilder(updateStrategy, func(args ...any) (update.SyncHandler, error) {
		return NewNoopUpdate(), nil
	})
}
