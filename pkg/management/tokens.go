package management

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *Server) CreateBootstrapToken(
	ctx context.Context,
	req *CreateBootstrapTokenRequest,
) (*core.BootstrapToken, error) {
	if err := validation.Validate(req); err != nil {
		return nil, err
	}
	token, err := m.storageBackend.CreateToken(ctx, req.Ttl.AsDuration(),
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
	ref *core.Reference,
) (*emptypb.Empty, error) {
	if err := validation.Validate(ref); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, m.storageBackend.DeleteToken(ctx, ref)
}

func (m *Server) ListBootstrapTokens(
	ctx context.Context,
	_ *emptypb.Empty,
) (*core.BootstrapTokenList, error) {
	tokens, err := m.storageBackend.ListTokens(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	tokenList := &core.BootstrapTokenList{}
	tokenList.Items = append(tokenList.Items, tokens...)
	return tokenList, nil
}

func (m *Server) GetBootstrapToken(
	ctx context.Context,
	ref *core.Reference,
) (*core.BootstrapToken, error) {
	if err := validation.Validate(ref); err != nil {
		return nil, err
	}
	token, err := m.storageBackend.GetToken(ctx, ref)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return token, nil
}
