package management

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Server) GetClusterHealthStatus(
	ctx context.Context,
	ref *corev1.Reference,
) (*corev1.HealthStatus, error) {
	if m.healthStatusDataSource == nil {
		return nil, status.Error(codes.Unavailable, "health API not configured")
	}
	if err := validation.Validate(ref); err != nil {
		return nil, err
	}

	return m.healthStatusDataSource.ClusterHealthStatus(ref)
}
