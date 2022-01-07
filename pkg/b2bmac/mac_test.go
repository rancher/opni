package b2bmac_test

import (
	"crypto/ed25519"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kralicky/opni-monitoring/pkg/b2bmac"
)

var _ = Describe("MAC", func() {
	It("should correctly generate a MAC", func() {
		_, key, err := ed25519.GenerateKey(nil)
		Expect(err).NotTo(HaveOccurred())
		tenantID := uuid.NewString()
		payload := []byte("test")
		uuid, mac, err := b2bmac.New512(tenantID, payload, key)
		Expect(err).NotTo(HaveOccurred())
		Expect(uuid).NotTo(BeNil())
		Expect(mac).To(HaveLen(512 / 8))
	})
	It("should error if the key is the wrong length", func() {
		key := make([]byte, 65)
		tenantID := uuid.NewString()
		payload := []byte("test")
		_, _, err := b2bmac.New512(tenantID, payload, key)
		Expect(err).To(MatchError("blake2b: invalid key size"))
		err = b2bmac.Verify([]byte(""), tenantID, uuid.Nil, payload, key)
		Expect(err).To(MatchError("blake2b: invalid key size"))
	})
	It("should correctly verify MACs", func() {
		_, key, err := ed25519.GenerateKey(nil)
		Expect(err).NotTo(HaveOccurred())
		tenantID := uuid.NewString()
		payload := []byte("test")
		uuid, mac, err := b2bmac.New512(tenantID, payload, key)
		Expect(err).NotTo(HaveOccurred())
		err = b2bmac.Verify(mac, tenantID, uuid, payload, key)
		Expect(err).NotTo(HaveOccurred())
		_, wrongKey, err := ed25519.GenerateKey(nil)
		Expect(err).NotTo(HaveOccurred())
		err = b2bmac.Verify(mac, tenantID, uuid, payload, wrongKey)
		Expect(err).To(MatchError("verification failed"))
	})
})
