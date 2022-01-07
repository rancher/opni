package tenant

import (
	"crypto/ed25519"
	"log"

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
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).SendString("Authorization header required")
	}

	tenantID, nonce, mac, err := b2bmac.DecodeAuthHeader(authHeader)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString(err.Error())
	}

	ks, err := m.tenantStore.KeyringStore(c.Context(), tenantID)
	if err != nil {
		log.Println("Failed to get keyring store for tenant:", err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	kr, err := ks.Get(c.Context())
	if err != nil {
		log.Println("[ERROR] Failed to get keyring for tenant:", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	var clientKey ed25519.PrivateKey
	if ok := kr.Try(func(shared *keyring.SharedKeys) {
		clientKey = shared.ClientKey
	}); !ok {
		log.Fatal("[ERROR] Keyring does not contain shared keys")
	}

	if err := b2bmac.Verify(mac, tenantID, nonce, c.Body(), clientKey); err != nil {
		log.Println("Tenant authorization failed:", err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	c.Request().Header.Add("X-Scope-OrgID", tenantID)
	return c.Next()
}
