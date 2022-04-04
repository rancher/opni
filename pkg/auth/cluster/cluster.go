package cluster

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/b2mac"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/ecdh"
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
	keyringStoreBroker storage.KeyringStoreBroker
	fakeKeyringStore   storage.KeyringStore
	headerKey          string
	logger             *zap.SugaredLogger
}

var _ auth.Middleware = (*ClusterMiddleware)(nil)

func New(keyringStore storage.KeyringStoreBroker, headerKey string) (*ClusterMiddleware, error) {
	fakeKeyringStore, err := initFakeKeyring(keyringStore)
	if err != nil {
		return nil, fmt.Errorf("failed to set up keyring store: %v", err)
	}

	return &ClusterMiddleware{
		keyringStoreBroker: keyringStore,
		fakeKeyringStore:   fakeKeyringStore,
		headerKey:          headerKey,
		logger: logger.New(
			logger.WithSampling(&zap.SamplingConfig{
				Initial:    1,
				Thereafter: 0,
			}),
		).Named("auth").Named("cluster"),
	}, nil
}

var fakeKeyring keyring.Keyring

func initFakeKeyring(broker storage.KeyringStoreBroker) (storage.KeyringStore, error) {
	store, err := broker.KeyringStore(context.Background(), "gateway-internal", &core.Reference{
		Id: "fake",
	})
	if err != nil {
		return nil, err
	}

	kp1 := ecdh.NewEphemeralKeyPair()
	kp2 := ecdh.NewEphemeralKeyPair()
	sec, err := ecdh.DeriveSharedSecret(kp1, ecdh.PeerPublicKey{
		PublicKey: kp2.PublicKey,
		PeerType:  ecdh.PeerTypeClient,
	})
	if err != nil {
		return nil, err
	}
	fakeKeyring = keyring.New(keyring.NewSharedKeys(sec))
	store.Put(context.Background(), fakeKeyring)
	return store, nil
}

func (m *ClusterMiddleware) doFakeKeyringVerify(mac []byte, id []byte, nonce uuid.UUID, payload []byte) {
	fakeKeyring, err := m.fakeKeyringStore.Get(context.Background())
	if err != nil {
		m.logger.Errorf("failed to get fake keyring: %v", err)
		return
	}
	fakeKeyring.Try(func(shared *keyring.SharedKeys) {
		b2mac.Verify(mac, id, nonce, payload, shared.ClientKey)
	})
}

func (m *ClusterMiddleware) Handle(c *fiber.Ctx) error {
	lg := m.logger
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		lg.Debug("unauthorized: authorization header required")
		return c.Status(fiber.StatusBadRequest).SendString("authorization header required")
	}

	clusterID, nonce, mac, err := b2mac.DecodeAuthHeader(authHeader)
	if err != nil {
		lg.Debug("unauthorized: malformed MAC in auth header: " + authHeader)
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}

	ks, err := m.keyringStoreBroker.KeyringStore(context.Background(), "gateway", &core.Reference{
		Id: string(clusterID),
	})
	if err != nil {
		lg.Debugf("unauthorized: error looking up keyring store for cluster %s: %v", clusterID, err)
		m.doFakeKeyringVerify(mac, clusterID, nonce, c.Body())
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	kr, err := ks.Get(context.Background())
	if err != nil {
		lg.Debugf("unauthorized: error looking up keyring for cluster %s: %v", clusterID, err)
		m.doFakeKeyringVerify(mac, clusterID, nonce, c.Body())
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	authorized := false
	var sharedKeys *keyring.SharedKeys
	if ok := kr.Try(func(shared *keyring.SharedKeys) {
		if err := b2mac.Verify(mac, clusterID, nonce, c.Body(), shared.ClientKey); err == nil {
			authorized = true
			sharedKeys = shared
		}
	}); !ok {
		lg.Errorf("unauthorized: invalid or corrupted keyring for cluster %s: %v", clusterID, err)
		return c.Status(fiber.StatusInternalServerError).SendString("invalid or corrupted keyring")
	}
	if !authorized {
		lg.Debugf("unauthorized: invalid mac for cluster %s", clusterID)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	c.Request().Header.Add(m.headerKey, string(clusterID))
	c.Locals(SharedKeysKey, sharedKeys)
	c.Locals(ClusterIDKey, string(clusterID))
	return c.Next()
}

func AuthorizedKeys(c *fiber.Ctx) *keyring.SharedKeys {
	return c.Locals(SharedKeysKey).(*keyring.SharedKeys)
}

func AuthorizedID(c *fiber.Ctx) string {
	return c.Locals(ClusterIDKey).(string)
}
