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
	"go.uber.org/zap"
)

type NoauthMiddleware struct {
	openidMiddleware auth.Middleware
	noauthConfig     *noauth.ServerConfig
	logger           *zap.SugaredLogger
}

var _ auth.Middleware = (*NoauthMiddleware)(nil)

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

	return m, nil
}

func (m *NoauthMiddleware) Description() string {
	return "noauth"
}

func (m *NoauthMiddleware) Handle(c *fiber.Ctx) error {
	return m.openidMiddleware.Handle(c)
}

func (m *NoauthMiddleware) ServerConfig() *noauth.ServerConfig {
	return m.noauthConfig
}
