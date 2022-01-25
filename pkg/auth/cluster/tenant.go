package cluster

import (
	"context"
	"crypto/ed25519"

	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/opni-monitoring/pkg/auth"
	"github.com/kralicky/opni-monitoring/pkg/b2bmac"
	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/keyring"
	"github.com/kralicky/opni-monitoring/pkg/storage"
)

type ClusterMiddleware struct {
	clusterStore storage.ClusterStore
}

var _ auth.Middleware = (*ClusterMiddleware)(nil)

func New(clusterStore storage.ClusterStore) *ClusterMiddleware {
	return &ClusterMiddleware{
		clusterStore: clusterStore,
	}
}

func (m *ClusterMiddleware) Description() string {
	return "Cluster Authentication"
}

func (m *ClusterMiddleware) Handle(c *fiber.Ctx) error {
	lg := c.Context().Logger()
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).SendString("Authorization header required")
	}

	clusterID, nonce, mac, err := b2bmac.DecodeAuthHeader(authHeader)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString(err.Error())
	}

	ks, err := m.clusterStore.KeyringStore(context.Background(), &core.Reference{
		Id: clusterID,
	})
	if err != nil {
		lg.Printf("unauthorized: no keyring store found for cluster %s: %v", clusterID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	kr, err := ks.Get(context.Background())
	if err != nil {
		lg.Printf("unauthorized: no keyring found for cluster %s: %v", clusterID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var clientKey ed25519.PrivateKey
	if ok := kr.Try(func(shared *keyring.SharedKeys) {
		clientKey = shared.ClientKey
	}); !ok {
		lg.Printf("unauthorized: invalid keyring for cluster %s: %v", clusterID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if err := b2bmac.Verify(mac, clusterID, nonce, c.Body(), clientKey); err != nil {
		lg.Printf("unauthorized: invalid mac for cluster %s: %v", clusterID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	c.Request().Header.Add("X-Scope-OrgID", clusterID)
	return c.Next()
}
