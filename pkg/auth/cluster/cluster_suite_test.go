package cluster_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/b2mac"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/keyring"
)

var (
	testServerKey    ed25519.PrivateKey
	testClientKey    ed25519.PrivateKey
	invalidKey       ed25519.PrivateKey
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
	sharedKeys := keyring.NewSharedKeys(sec)
	testServerKey = sharedKeys.ServerKey
	testClientKey = sharedKeys.ClientKey
	testSharedSecret = sec

	_, invalidKey, err = ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
})

func validAuthHeader[T, U string | []byte](id T, payload U) string {
	str, err := b2mac.NewEncodedHeader([]byte(id), []byte(payload), testClientKey)
	if err != nil {
		panic(err)
	}
	return str
}

func invalidAuthHeader[T, U string | []byte](id T, payload U) string {
	str, err := b2mac.NewEncodedHeader([]byte(id), []byte(payload), invalidKey)
	if err != nil {
		panic(err)
	}
	return str
}
