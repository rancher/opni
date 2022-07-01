package ecdh_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/keyring"
)

var _ = Describe("ECDH", Label("unit"), func() {
	It("should generate a key pair", func() {
		ekp := ecdh.NewEphemeralKeyPair()
		Expect(ekp.PrivateKey).NotTo(BeNil())
		Expect(ekp.PublicKey).NotTo(BeNil())
	})

	It("should compute equal shared secrets", func() {
		ekpA := ecdh.NewEphemeralKeyPair()
		Expect(ekpA.PrivateKey).NotTo(BeNil())
		Expect(ekpA.PublicKey).NotTo(BeNil())
		ekpB := ecdh.NewEphemeralKeyPair()
		Expect(ekpB.PrivateKey).NotTo(BeNil())
		Expect(ekpB.PublicKey).NotTo(BeNil())

		Expect(ekpA.PrivateKey).NotTo(Equal(ekpB.PrivateKey))
		Expect(ekpA.PublicKey).NotTo(Equal(ekpB.PublicKey))

		secretA, err := ecdh.DeriveSharedSecret(ekpA, ecdh.PeerPublicKey{
			PublicKey: ekpB.PublicKey,
			PeerType:  ecdh.PeerTypeServer,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(secretA).NotTo(BeNil())

		secretB, err := ecdh.DeriveSharedSecret(ekpB, ecdh.PeerPublicKey{
			PublicKey: ekpA.PublicKey,
			PeerType:  ecdh.PeerTypeClient,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(secretB).NotTo(BeNil())

		Expect(secretA).To(Equal(secretB))
	})
	It("should generate equal client keys", func() {
		ekpA := ecdh.NewEphemeralKeyPair()
		ekpB := ecdh.NewEphemeralKeyPair()

		secretA, err := ecdh.DeriveSharedSecret(ekpA, ecdh.PeerPublicKey{
			PublicKey: ekpB.PublicKey,
			PeerType:  ecdh.PeerTypeServer,
		})
		Expect(err).NotTo(HaveOccurred())

		secretB, err := ecdh.DeriveSharedSecret(ekpB, ecdh.PeerPublicKey{
			PublicKey: ekpA.PublicKey,
			PeerType:  ecdh.PeerTypeClient,
		})
		Expect(err).NotTo(HaveOccurred())

		kr1 := keyring.NewSharedKeys(secretA)
		kr2 := keyring.NewSharedKeys(secretB)

		Expect(kr1.ClientKey).To(Equal(kr2.ClientKey))
		Expect(kr1.ServerKey).To(Equal(kr2.ServerKey))
	})

	It("should handle errors", func() {
		ekpA := ecdh.NewEphemeralKeyPair()
		ekpB := ecdh.NewEphemeralKeyPair()

		By("using a scalar of incorrect length")
		_, err := ecdh.DeriveSharedSecret(ecdh.EphemeralKeyPair{
			PrivateKey: make([]byte, 31),
		}, ecdh.PeerPublicKey{
			PublicKey: ekpB.PublicKey,
			PeerType:  ecdh.PeerTypeClient,
		})
		Expect(err).To(MatchError("bad scalar length: 31, expected 32"))

		By("using a point of incorrect length")
		_, err = ecdh.DeriveSharedSecret(ekpA, ecdh.PeerPublicKey{
			PublicKey: make([]byte, 31),
		})
		Expect(err).To(MatchError("bad point length: 31, expected 32"))

		By("specifying an invalid peer type")
		_, err = ecdh.DeriveSharedSecret(ekpA, ecdh.PeerPublicKey{
			PublicKey: ekpB.PublicKey,
			PeerType:  ecdh.PeerType(99),
		})
		Expect(err).To(MatchError(ecdh.ErrInvalidPeerType))

		By("using a low order point")
		_, err = ecdh.DeriveSharedSecret(ekpA, ecdh.PeerPublicKey{
			PublicKey: []byte{
				0x5f, 0x9c, 0x95, 0xbc, 0xa3, 0x50, 0x8c, 0x24, 0xb1, 0xd0, 0xb1,
				0x55, 0x9c, 0x83, 0xef, 0x5b, 0x04, 0x44, 0x5c, 0xc4, 0x58, 0x1c,
				0x8e, 0x86, 0xd8, 0x22, 0x4e, 0xdd, 0xd0, 0x9f, 0x11, 0x57,
			},
		})
		Expect(err).To(MatchError("bad input point: low order point"))
	})
})
