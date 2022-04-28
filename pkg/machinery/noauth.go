package machinery

import (
	"context"

	"github.com/rancher/opni/pkg/auth"
	authnoauth "github.com/rancher/opni/pkg/auth/noauth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
)

func SetupNoauthServer(
	ctx context.Context,
	lg logger.ExtendedSugaredLogger,
	ap *v1beta1.AuthProvider,
) {
	if ap.Name == "noauth" {
		mw, err := auth.GetMiddleware("noauth")
		if err != nil {
			panic(err)
		}
		srvCfg := auth.NamedMiddlewareAs[*authnoauth.NoauthMiddleware](mw).ServerConfig()
		srv := noauth.NewServer(srvCfg)
		waitctx.Go(ctx, func() {
			if err := srv.Run(ctx); err != nil {
				lg.With(
					zap.Error(err),
				).Warn("noauth server stopped")
			}
		})
	}
}
