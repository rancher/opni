package noauth

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/auth/openid"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/noauth"
	"github.com/rancher/opni-monitoring/pkg/util"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"go.uber.org/zap"
)

type NoauthMiddleware struct {
	openidMiddleware auth.Middleware
	noauthConfig     *noauth.ServerConfig
	logger           *zap.SugaredLogger
}

var _ auth.Middleware = (*NoauthMiddleware)(nil)

// todo: move noauth server start somewhere else
func New(ctx context.Context, config v1beta1.AuthProviderSpec) (auth.Middleware, error) {
	lg := logger.New().Named("noauth")
	openidMw, err := openid.New(ctx, config)
	if err != nil {
		return nil, err
	}
	conf, err := util.DecodeStruct[noauth.ServerConfig](config.Options)
	if err != nil {
		return nil, err
	}
	m := &NoauthMiddleware{
		openidMiddleware: openidMw,
		noauthConfig:     conf,
		logger:           lg,
	}
	m.noauthConfig.Logger = lg

	srv := noauth.NewServer(m.noauthConfig)
	waitctx.Go(ctx, func() {
		if err := srv.Run(ctx); err != nil {
			lg.With(
				zap.Error(err),
			).Warn("noauth server stopped")
		}
	})

	return m, nil
}

func (m *NoauthMiddleware) Description() string {
	return "noauth"
}

func (m *NoauthMiddleware) Handle(c *fiber.Ctx) error {
	return m.openidMiddleware.Handle(c)
}
