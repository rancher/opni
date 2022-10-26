package cluster_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/b2mac"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/keyring"
)

var (
	testSharedKeys   *keyring.SharedKeys
	testServerKey    ed25519.PrivateKey
	testClientKey    ed25519.PrivateKey
	invalidKey       ed25519.PrivateKey
	testKeyring      keyring.Keyring
	testSharedSecret []byte
)

func TestClusterAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster Suite")
}

var _ = BeforeSuite(func() {
	kp1 := ecdh.NewEphemeralKeyPair()
	kp2 := ecdh.NewEphemeralKeyPair()
	sec, err := ecdh.DeriveSharedSecret(kp1, ecdh.PeerPublicKey{
		PublicKey: kp2.PublicKey,
		PeerType:  ecdh.PeerTypeClient,
	})
	if err != nil {
		panic(err)
	}
	testSharedKeys = keyring.NewSharedKeys(sec)
	testServerKey = testSharedKeys.ServerKey
	testClientKey = testSharedKeys.ClientKey
	testSharedSecret = sec

	testKeyring = keyring.New(testSharedKeys)
	_, invalidKey, err = ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
})

func validAuthHeader[T, U string | []byte](id T, payload U) (uuid.UUID, string) {
	nonce := uuid.New()
	str, err := b2mac.NewEncodedHeader([]byte(id), nonce, []byte(payload), testClientKey)
	if err != nil {
		panic(err)
	}
	return nonce, str
}

func invalidAuthHeader[T, U string | []byte](id T, payload U) (uuid.UUID, string) {
	nonce := uuid.New()
	str, err := b2mac.NewEncodedHeader([]byte(id), nonce, []byte(payload), invalidKey)
	if err != nil {
		panic(err)
	}
	return nonce, str
}
