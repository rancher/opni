package identserver

import (
	"context"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/ident"
	"google.golang.org/protobuf/types/known/emptypb"
)

type identServer struct {
	controlv1.UnsafeIdentityServer
	provider ident.Provider
}

func NewFromProvider(provider ident.Provider) controlv1.IdentityServer {
	return &identServer{
		provider: provider,
	}
}

func (s *identServer) Whoami(ctx context.Context, _ *emptypb.Empty) (*corev1.Reference, error) {
	id, err := s.provider.UniqueIdentifier(ctx)
	if err != nil {
		return nil, err
	}
	return &corev1.Reference{
		Id: id,
	}, nil
}
