package openid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/lestrrat-go/jwx/jwt/openid"
	"github.com/mitchellh/mapstructure"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/rbac"
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
	keyRefresher    *jwk.AutoRefresh
	conf            *OpenidConfig
	wellKnownConfig *WellKnownConfiguration
}

var _ auth.Middleware = (*OpenidMiddleware)(nil)

func New(config v1beta1.AuthProviderSpec) (auth.Middleware, error) {
	m := &OpenidMiddleware{
		keyRefresher: jwk.NewAutoRefresh(context.Background()),
		conf:         &OpenidConfig{},
	}
	if err := mapstructure.Decode(config.Options, m.conf); err != nil {
		return nil, err
	}
	issuerURL, err := url.Parse(m.conf.Issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse openid issuer URL: %v", err)
	}
	if issuerURL.Scheme != "https" {
		return nil, fmt.Errorf("openid issuer URL must have https scheme")
	}
	if !strings.HasSuffix(issuerURL.Path, ".well-known/openid-configuration") {
		issuerURL.Path = path.Join(issuerURL.Path, ".well-known/openid-configuration")
	}

	wellKnownCfg, err := fetchWellKnownConfig(issuerURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch openid configuration: %v", err)
	}
	if wellKnownCfg.Issuer != m.conf.Issuer {
		return nil, fmt.Errorf("openid issuer URL mismatch (server returned %q, expected %q)",
			wellKnownCfg.Issuer, m.conf.Issuer)
	}
	m.wellKnownConfig = wellKnownCfg
	m.keyRefresher.Configure(wellKnownCfg.JwksUri)
	return m, nil
}

func (m *OpenidMiddleware) Description() string {
	return "OpenID Connect"
}

func (m *OpenidMiddleware) Handle(c *fiber.Ctx) error {
	lg := c.Context().Logger()
	set, err := m.keyRefresher.Fetch(context.Background(), m.wellKnownConfig.JwksUri)
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

func fetchWellKnownConfig(url string) (*WellKnownConfiguration, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var cfg WellKnownConfiguration
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
