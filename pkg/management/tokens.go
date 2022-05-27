package management

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *Server) CreateBootstrapToken(
	ctx context.Context,
	req *managementv1.CreateBootstrapTokenRequest,
) (*corev1.BootstrapToken, error) {
	if err := validation.Validate(req); err != nil {
		return nil, err
	}
	token, err := m.coreDataSource.StorageBackend().CreateToken(ctx, req.Ttl.AsDuration(),
		storage.WithLabels(req.GetLabels()),
		storage.WithCapabilities(req.GetCapabilities()),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return token, nil
}

func (m *Server) RevokeBootstrapToken(
	ctx context.Context,
	ref *corev1.Reference,
) (*emptypb.Empty, error) {
	if err := validation.Validate(ref); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, m.coreDataSource.StorageBackend().DeleteToken(ctx, ref)
}

func (m *Server) ListBootstrapTokens(
	ctx context.Context,
	_ *emptypb.Empty,
) (*corev1.BootstrapTokenList, error) {
	tokens, err := m.coreDataSource.StorageBackend().ListTokens(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	tokenList := &corev1.BootstrapTokenList{}
	tokenList.Items = append(tokenList.Items, tokens...)
	return tokenList, nil
}

func (m *Server) GetBootstrapToken(
	ctx context.Context,
	ref *corev1.Reference,
) (*corev1.BootstrapToken, error) {
	if err := validation.Validate(ref); err != nil {
		return nil, err
	}
	token, err := m.coreDataSource.StorageBackend().GetToken(ctx, ref)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return token, nil
}
