package noauth

import (
	"github.com/gofiber/fiber/v2"
	"github.com/mitchellh/mapstructure"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/auth/openid"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/noauth"
	"go.uber.org/zap"
)

type NoauthMiddleware struct {
	openidMiddleware auth.Middleware
	noauthConfig     *noauth.ServerConfig
	logger           *zap.SugaredLogger
}

var _ auth.Middleware = (*NoauthMiddleware)(nil)

func New(config v1beta1.AuthProviderSpec) (auth.Middleware, error) {
	lg := logger.New().Named("noauth")
	openidMw, err := openid.New(config)
	if err != nil {
		return nil, err
	}
	m := &NoauthMiddleware{
		openidMiddleware: openidMw,
		noauthConfig:     &noauth.ServerConfig{},
		logger:           lg,
	}
	if err := mapstructure.Decode(config.Options, m.noauthConfig); err != nil {
		return nil, err
	}
	m.noauthConfig.Logger = lg

	srv := noauth.NewServer(m.noauthConfig)
	go func() {
		if err := srv.Run(); err != nil {
			lg.Error(err)
		}
	}()

	return m, nil
}

func (m *NoauthMiddleware) Description() string {
	return "noauth"
}

func (m *NoauthMiddleware) Handle(c *fiber.Ctx) error {
	return m.openidMiddleware.Handle(c)
}
