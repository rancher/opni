package noauth

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/util"
	"log/slog"
)

type NoauthMiddleware struct {
	openidMiddleware auth.HTTPMiddleware
	noauthConfig     *noauth.ServerConfig
	logger           *slog.Logger
}

var _ auth.Middleware = (*NoauthMiddleware)(nil)

func New(ctx context.Context, config v1beta1.AuthProviderSpec) (*NoauthMiddleware, error) {
	lg := logger.New().WithGroup("noauth")
	conf, err := util.DecodeStruct[noauth.ServerConfig](config.Options)
	if err != nil {
		return nil, err
	}
	openidConf, err := util.DecodeStruct[map[string]any](conf.OpenID)
	if err != nil {
		return nil, err
	}
	openidMw, err := openid.New(ctx, v1beta1.AuthProviderSpec{
		Type:    "openid",
		Options: *openidConf,
	})
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

func (m *NoauthMiddleware) Handle(c *gin.Context) {
	m.openidMiddleware.Handle(c)
}

func (m *NoauthMiddleware) ServerConfig() *noauth.ServerConfig {
	return m.noauthConfig
}
