package tenant

import (
	"context"
	"crypto/ed25519"

	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/opni-monitoring/pkg/auth"
	"github.com/kralicky/opni-monitoring/pkg/b2bmac"
	"github.com/kralicky/opni-monitoring/pkg/keyring"
	"github.com/kralicky/opni-monitoring/pkg/storage"
)

type TenantMiddleware struct {
	tenantStore storage.TenantStore
}

var _ auth.Middleware = (*TenantMiddleware)(nil)

func New(tenantStore storage.TenantStore) *TenantMiddleware {
	return &TenantMiddleware{
		tenantStore: tenantStore,
	}
}

func (m *TenantMiddleware) Description() string {
	return "Tenant Authentication"
}

func (m *TenantMiddleware) Handle(c *fiber.Ctx) error {
	lg := c.Context().Logger()
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).SendString("Authorization header required")
	}

	tenantID, nonce, mac, err := b2bmac.DecodeAuthHeader(authHeader)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString(err.Error())
	}

	ks, err := m.tenantStore.KeyringStore(context.Background(), tenantID)
	if err != nil {
		lg.Printf("unauthorized: no keyring store found for tenant %s: %v", tenantID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	kr, err := ks.Get(context.Background())
	if err != nil {
		lg.Printf("unauthorized: no keyring found for tenant %s: %v", tenantID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var clientKey ed25519.PrivateKey
	if ok := kr.Try(func(shared *keyring.SharedKeys) {
		clientKey = shared.ClientKey
	}); !ok {
		lg.Printf("unauthorized: invalid keyring for tenant %s: %v", tenantID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if err := b2bmac.Verify(mac, tenantID, nonce, c.Body(), clientKey); err != nil {
		lg.Printf("unauthorized: invalid mac for tenant %s: %v", tenantID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	c.Request().Header.Add("X-Scope-OrgID", tenantID)
	return c.Next()
}
