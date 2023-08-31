package backend

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
)

type SLOStore interface {
	Create(ctx context.Context, req *slov1.CreateSLORequest) (*corev1.Reference, error)
	Update(ctx context.Context, incoming, existing *slov1.SLOData) (*slov1.SLOData, error)
	Delete(ctx context.Context, existing *slov1.SLOData) error

	Clone(ctx context.Context, clone *slov1.SLOData) (*corev1.Reference, *slov1.SLOData, error)
	MultiClusterClone(
		ctx context.Context,
		slo *slov1.SLOData,
		toClusters []*corev1.Reference,
	) ([]*corev1.Reference, []*slov1.SLOData, []error)

	Status(ctx context.Context, existing *slov1.SLOData) (*slov1.SLOStatus, error)
	Preview(ctx context.Context, s *slov1.CreateSLORequest) (*slov1.SLOPreviewResponse, error)
}
