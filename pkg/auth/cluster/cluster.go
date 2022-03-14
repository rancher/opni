package cluster

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/b2bmac"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"go.uber.org/zap"
)

const (
	ClusterIDKey  = "cluster_auth_cluster_id"
	SharedKeysKey = "cluster_auth_shared_keys"
)

type ClusterMiddleware struct {
	keyringStore storage.KeyringStoreBroker
	headerKey    string
	logger       *zap.SugaredLogger
}

var _ auth.Middleware = (*ClusterMiddleware)(nil)

func New(keyringStore storage.KeyringStoreBroker, headerKey string) *ClusterMiddleware {
	return &ClusterMiddleware{
		keyringStore: keyringStore,
		headerKey:    headerKey,
		logger:       logger.New().Named("auth").Named("cluster"),
	}
}

func (m *ClusterMiddleware) Description() string {
	return "Cluster Authentication"
}

func (m *ClusterMiddleware) Handle(c *fiber.Ctx) error {
	lg := m.logger
	lg.Debug("handling auth request")
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		lg.Debug("unauthorized: authorization header required")
		return c.Status(fiber.StatusUnauthorized).SendString("Authorization header required")
	}

	clusterID, nonce, mac, err := b2bmac.DecodeAuthHeader(authHeader)
	if err != nil {
		lg.Debug("unauthorized: malformed MAC in auth header")
		return c.Status(fiber.StatusUnauthorized).SendString(err.Error())
	}

	ks, err := m.keyringStore.KeyringStore(context.Background(), "gateway", &core.Reference{
		Id: string(clusterID),
	})
	if err != nil {
		lg.Debugf("unauthorized: no keyring store found for cluster %s: %v", clusterID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	kr, err := ks.Get(context.Background())
	if err != nil {
		lg.Debugf("unauthorized: no keyring found for cluster %s: %v", clusterID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	authorized := false
	var sharedKeys *keyring.SharedKeys
	if ok := kr.Try(func(shared *keyring.SharedKeys) {
		if err := b2bmac.Verify(mac, clusterID, nonce, c.Body(), shared.ClientKey); err == nil {
			authorized = true
			sharedKeys = shared
		}
	}); !ok {
		lg.Debugf("unauthorized: invalid keyring for cluster %s: %v", clusterID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	if !authorized {
		lg.Debugf("unauthorized: invalid mac for cluster %s: %v", clusterID, err)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	c.Request().Header.Add(m.headerKey, string(clusterID))
	c.Locals(SharedKeysKey, sharedKeys)
	return c.Next()
}

func AuthorizedKeys(c *fiber.Ctx) *keyring.SharedKeys {
	return c.Locals(SharedKeysKey).(*keyring.SharedKeys)
}

func AuthorizedID(c *fiber.Ctx) string {
	return c.Locals(ClusterIDKey).(string)
}
