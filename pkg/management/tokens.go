package management

import (
	"context"

	core "github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	return newBootstrapToken(token), nil
}

func (m *Server) RevokeBootstrapToken(
	ctx context.Context,
	ref *core.Reference,
) error {
	return grpcError(m.tokenStore.DeleteToken(ctx, ref))
}

func (m *Server) ListBootstrapTokens(
	ctx context.Context,
) (*core.BootstrapTokenList, error) {
	tokens, err := m.tokenStore.ListTokens(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	tokenList := &core.BootstrapTokenList{
		Items: make([]*core.BootstrapToken, len(tokens)),
	}
	for i, token := range tokens {
		tokenList.Items[i] = newBootstrapToken(token)
	}
	return tokenList, nil
}

func newBootstrapToken(token *tokens.Token) *core.BootstrapToken {
	t := &core.BootstrapToken{
		TokenID: make([]byte, len(token.ID)),
		Secret:  make([]byte, len(token.Secret)),
		LeaseID: token.Metadata.LeaseID,
		Ttl:     token.Metadata.TTL,
	}
	copy(t.TokenID, token.ID)
	copy(t.Secret, token.Secret)
	return t
}
