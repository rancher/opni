package openid

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lestrrat-go/backoff/v2"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
)

var ErrNoSigningKeyFound = fmt.Errorf("no signing key found in the JWK set")

const (
	TokenKey = "token"
)

type OpenidMiddleware struct {
	keyRefresher *jwk.AutoRefresh
	conf         *OpenidConfig
	logger       *zap.SugaredLogger

	wellKnownConfig *WellKnownConfiguration
	lock            sync.Mutex

	cache *UserInfoCache
}

var _ auth.Middleware = (*OpenidMiddleware)(nil)

func New(ctx context.Context, config v1beta1.AuthProviderSpec) (*OpenidMiddleware, error) {
	conf, err := util.DecodeStruct[OpenidConfig](config.Options)
	if err != nil {
		return nil, err
	}
	m := &OpenidMiddleware{
		keyRefresher: jwk.NewAutoRefresh(ctx),
		conf:         conf,
		logger:       logger.New().Named("openid"),
	}
	if m.conf.IdentifyingClaim == "" {
		m.conf.IdentifyingClaim = "sub"
	}

	waitctx.Go(ctx, func() {
		m.tryConfigureKeyRefresher(ctx)
	})
	return m, nil
}

func (m *OpenidMiddleware) Handle(c *fiber.Ctx) error {
	m.lock.Lock()
	if m.wellKnownConfig == nil {
		c.Status(http.StatusServiceUnavailable)
		m.lock.Unlock()
		return c.SendString("auth provider is not ready")
	}
	m.lock.Unlock()

	lg := c.Context().Logger()
	lg.Printf("handling auth request")
	// Some providers serve their JWKS URI at `/.well-known/jwks.json`, which is
	// not a registered well-known URI. openid-configuration is, however.
	ctx, ca := context.WithTimeout(c.Context(), time.Second*5)
	defer ca()
	set, err := m.keyRefresher.Fetch(ctx, m.wellKnownConfig.JwksUri)
	if err != nil {
		lg.Printf("failed to fetch JWK set: %v", err)
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		lg.Printf("no authorization header in request")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	bearerToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	var userID string
	switch GetTokenType(bearerToken) {
	case IDToken:
		idt, err := ValidateIDToken(bearerToken, set)
		if err != nil {
			lg.Printf("failed to validate ID token: %v", err)
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		claim, ok := idt.Get(m.conf.IdentifyingClaim)
		if !ok {
			lg.Printf("identifying claim %q not found in ID token", m.conf.IdentifyingClaim)
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		userID = fmt.Sprint(claim)
	case Opaque:
		userInfo, err := m.cache.Get(bearerToken)
		if err != nil {
			lg.Printf("failed to get user info: %v", err)
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		uid, err := userInfo.UserID()
		if err != nil {
			lg.Printf("failed to get user id: %v", err)
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		userID = uid
	}
	c.Request().Header.Del("Authorization")
	c.Locals(rbac.UserIDKey, userID)
	return c.Next()
}

func (m *OpenidMiddleware) tryConfigureKeyRefresher(ctx context.Context) {
	lg := m.logger
	p := backoff.Exponential(
		backoff.WithMinInterval(50*time.Millisecond),
		backoff.WithMaxInterval(time.Minute),
		backoff.WithMultiplier(2),
		backoff.WithJitterFactor(0.05),
	)
	b := p.Start(ctx)
	for backoff.Continue(b) {
		wellKnownCfg, err := m.conf.GetWellKnownConfiguration()
		if err != nil {
			if isDiscoveryErrFatal(err) {
				lg.With(
					zap.Error(err),
				).Fatal("fatal error fetching openid configuration")
			} else {
				lg.With(
					zap.Error(err),
				).Warn("failed to fetch openid configuration (will retry)")
			}
			continue
		}
		lg.With(
			"issuer", wellKnownCfg.Issuer,
		).Info("successfully fetched openid configuration")
		m.lock.Lock()
		defer m.lock.Unlock()
		m.wellKnownConfig = wellKnownCfg
		m.keyRefresher.Configure(wellKnownCfg.JwksUri)
		m.cache, err = NewUserInfoCache(m.conf, m.logger)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Fatal("failed to create user info cache")
		}
		break
	}
}
