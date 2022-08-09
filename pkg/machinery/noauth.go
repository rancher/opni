package machinery

import (
	"context"
	"fmt"

	authnoauth "github.com/rancher/opni/pkg/auth/noauth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
)

func NewNoauthServer(
	ctx context.Context,
	ap *v1beta1.AuthProvider,
) *noauth.Server {
	mw, err := authnoauth.New(ctx, ap.Spec)
	if err != nil {
		panic(fmt.Errorf("failed to create noauth auth provider: %w", err))
	}
	return noauth.NewServer(mw.ServerConfig())
}
