package openid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/lestrrat-go/jwx/jwt/openid"
	"github.com/mitchellh/mapstructure"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"go.uber.org/zap"
)

var ErrNoSigningKeyFound = fmt.Errorf("no signing key found in the JWK set")

const (
	TokenKey = "token"
)

type WellKnownConfiguration struct {
	Issuer           string `json:"issuer"`
	AuthEndpoint     string `json:"authorization_endpoint"`
	TokenEndpoint    string `json:"token_endpoint"`
	UserinfoEndpoint string `json:"userinfo_endpoint"`
	JwksUri          string `json:"jwks_uri"`
}

type OpenidConfig struct {
	// The OpenID issuer identifier URL. The provider's OpenID configuration
	// will be fetched from <Issuer>/.well-known/openid-configuration.
	// The issuer URL must exactly match the issuer in the well-known
	// configuration, including any trailing slashes.
	Issuer string `mapstructure:"issuer"`
}

type OpenidMiddleware struct {
	keyRefresher *jwk.AutoRefresh
	conf         *OpenidConfig
	logger       *zap.SugaredLogger
	ctx          context.Context

	wellKnownConfig *WellKnownConfiguration
	lock            sync.Mutex
}

var _ auth.Middleware = (*OpenidMiddleware)(nil)

func New(ctx context.Context, config v1beta1.AuthProviderSpec) (auth.Middleware, error) {
	m := &OpenidMiddleware{
		keyRefresher: jwk.NewAutoRefresh(ctx),
		conf:         &OpenidConfig{},
		logger:       logger.New().Named("openid"),
		ctx:          ctx,
	}
	if err := mapstructure.Decode(config.Options, m.conf); err != nil {
		return nil, err
	}
	issuerURL, err := url.Parse(m.conf.Issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse openid issuer URL: %w", err)
	}
	if !strings.HasSuffix(issuerURL.Path, ".well-known/openid-configuration") {
		issuerURL.Path = path.Join(issuerURL.Path, ".well-known/openid-configuration")
	}

	waitctx.AddOne(ctx)
	go func() {
		m.connectToAuthProvider(issuerURL)
		waitctx.Done(ctx)
	}()
	return m, nil
}

func (m *OpenidMiddleware) Description() string {
	return "OpenID Connect"
}

func (m *OpenidMiddleware) Handle(c *fiber.Ctx) error {
	m.lock.Lock()
	if m.wellKnownConfig == nil {
		c.Status(http.StatusServiceUnavailable)
		return c.SendString("auth provider is not ready")
	}
	lg := c.Context().Logger()
	set, err := m.keyRefresher.Fetch(m.ctx, m.wellKnownConfig.JwksUri)
	if err != nil {
		lg.Printf("failed to fetch JWK set: %v", err)
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		lg.Printf("no authorization header in request")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	value := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	token, err := jwt.ParseString(value,
		jwt.WithKeySet(set),
		jwt.WithValidate(true),
		jwt.WithToken(openid.New()),
	)
	if err != nil {
		lg.Printf("failed to parse JWT: %v", err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	userID := token.(openid.Token).Email()
	c.Request().Header.Del("Authorization")
	c.Locals(rbac.UserIDKey, userID)
	return c.Next()
}

func (m *OpenidMiddleware) connectToAuthProvider(issuerURL *url.URL) {
	lg := m.logger
	for {
		if m.ctx.Err() != nil {
			return
		}
		wellKnownCfg, err := fetchWellKnownConfig(issuerURL.String())
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to fetch openid configuration (will retry)")
			select {
			case <-time.After(time.Second * 2):
			case <-m.ctx.Done():
				return
			}
			continue
		}
		if wellKnownCfg.Issuer != m.conf.Issuer {
			lg.With(
				zap.String("response", wellKnownCfg.Issuer),
				zap.String("expected", m.conf.Issuer),
			).Error("issuer mismatch")
			return
		}
		m.lock.Lock()
		defer m.lock.Unlock()
		m.wellKnownConfig = wellKnownCfg
		m.keyRefresher.Configure(wellKnownCfg.JwksUri)
		break
	}
}

func fetchWellKnownConfig(url string) (*WellKnownConfiguration, error) {
	client := http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}
	defer resp.Body.Close()
	var cfg WellKnownConfiguration
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
