package openid

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/opni-gateway/pkg/auth"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/lestrrat-go/jwx/jwt/openid"
	"github.com/mitchellh/mapstructure"
)

var ErrNoSigningKeyFound = fmt.Errorf("no signing key found in the JWK set")

const (
	TokenKey = "token"
)

type OpenidConfig struct {
	JwkUrl string `mapstructure:"jwkUrl"`
}

type OpenidMiddleware struct {
	keyRefresher *jwk.AutoRefresh
	conf         *OpenidConfig
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
	m.keyRefresher.Configure(m.conf.JwkUrl)
	return m, nil
}

func (m *OpenidMiddleware) Description() string {
	return "OpenID Connect"
}

func (m *OpenidMiddleware) Handle(c *fiber.Ctx) error {
	set, err := m.keyRefresher.Fetch(context.Background(), m.conf.JwkUrl)
	if err != nil {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		log.Println("no authorization header")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	value := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	token, err := jwt.ParseString(value,
		jwt.WithKeySet(set),
		jwt.WithValidate(true),
		jwt.WithToken(openid.New()),
	)
	if err != nil {
		log.Println(err.Error())
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	AddTokenToContext(c, token.(openid.Token))
	return c.Next()
}

func TokenFromContext(c *fiber.Ctx) (openid.Token, bool) {
	token, ok := c.Locals(TokenKey).(openid.Token)
	return token, ok
}

func AddTokenToContext(c *fiber.Ctx, token openid.Token) {
	c.Locals(TokenKey, token)
}
