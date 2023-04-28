package challenges_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/keyring/ephemeral"
	_ "github.com/rancher/opni/pkg/test/setup"
)

var (
	testSharedKeys  *keyring.SharedKeys
	testKeyring     keyring.Keyring
	sessionAttrKey1 *keyring.EphemeralKey
	sessionAttrKey2 *keyring.EphemeralKey
	ctrl            *gomock.Controller
)

func TestChallenges(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Challenges Suite")
}

var _ = BeforeSuite(func() {
	ctrl = gomock.NewController(GinkgoT())
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

	sessionAttrKey1 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
		session.AttributeLabelKey: "example-session-attribute-1",
	})
	sessionAttrKey2 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
		session.AttributeLabelKey: "example-session-attribute-2",
	})

	testKeyring = keyring.New(testSharedKeys, sessionAttrKey1, sessionAttrKey2)
})
