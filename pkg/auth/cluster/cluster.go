package cluster

import (
	"context"
	"crypto/ed25519"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/b2bmac"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/storage"
)

type ClusterMiddleware struct {
	keyringStore storage.KeyringStoreBroker
	headerKey    string
}

var _ auth.Middleware = (*ClusterMiddleware)(nil)

func New(keyringStore storage.KeyringStoreBroker, headerKey string) *ClusterMiddleware {
	return &ClusterMiddleware{
		keyringStore: keyringStore,
		headerKey:    headerKey,
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

	ks, err := m.keyringStore.KeyringStore(context.Background(), "cluster", &core.Reference{
		Id: string(clusterID),
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

	c.Request().Header.Add(m.headerKey, string(clusterID))
	return c.Next()
}
