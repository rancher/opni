package management

import (
	"context"

	core "github.com/rancher/opni-monitoring/pkg/core"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *Server) CreateBootstrapToken(
	ctx context.Context,
	req *CreateBootstrapTokenRequest,
) (*core.BootstrapToken, error) {
	ttl := DefaultTokenTTL
	if req.GetTtl() != nil {
		ttl = req.GetTtl().AsDuration()
	}
	token, err := m.tokenStore.CreateToken(ctx, ttl)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return token.ToBootstrapToken(), nil
}

func (m *Server) RevokeBootstrapToken(
	ctx context.Context,
	ref *core.Reference,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, grpcError(m.tokenStore.DeleteToken(ctx, ref))
}

func (m *Server) ListBootstrapTokens(
	ctx context.Context,
	_ *emptypb.Empty,
) (*core.BootstrapTokenList, error) {
	tokens, err := m.tokenStore.ListTokens(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	tokenList := &core.BootstrapTokenList{
		Items: make([]*core.BootstrapToken, len(tokens)),
	}
	for i, token := range tokens {
		tokenList.Items[i] = token.ToBootstrapToken()
	}
	return tokenList, nil
}
