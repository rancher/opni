package management

import (
	"context"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *Server) CreateLocalPassword(ctx context.Context, _ *emptypb.Empty) (*managementv1.LocalPasswordResponse, error) {
	password, err := m.localAuth.GenerateAdminPassword(ctx)
	if err != nil {
		return nil, err
	}
	return &managementv1.LocalPasswordResponse{
		Password: password,
	}, nil
}
