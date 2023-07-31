package util_test

import (
	"math/big"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("uuid to big ints", Label("unit"), func() {
	When("converting uuids to big ints", func() {
		It("should uniquely convert them", func() {
			uuid1 := uuid.New()
			uuid2 := uuid.New()

			bigInt, err := util.UUIDToBigInt(uuid1)
			Expect(err).NotTo(HaveOccurred())
			bigInt2, err := util.UUIDToBigInt(uuid2)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the big ints are not equal")
			cmpRes := bigInt.Cmp(bigInt2)
			Expect(cmpRes).NotTo(Equal(0))

		})
		It("should map the same uuid to the same big int", func() {
			uuid1 := uuid.New()
			bigInt, err := util.UUIDToBigInt(uuid1)
			Expect(err).NotTo(HaveOccurred())
			bigInt2, err := util.UUIDToBigInt(uuid1)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the big ints are equal")
			cmpRes := bigInt.Cmp(bigInt2)
			Expect(cmpRes).To(Equal(0))
		})
	})

	When("using the bit manipulation helpers", func() {
		It("should XOR big ints", func() {
			b, err := util.UUIDToBigInt(uuid.New())
			Expect(err).NotTo(HaveOccurred())
			xor1 := util.XorBigInt(b, b)
			By("veryfying the reflexive property of XOR")
			Expect(xor1.Cmp(big.NewInt(0))).To(Equal(0))

			By("veryfing yhe uniqueness property of XOR")
			b2, err := util.UUIDToBigInt(uuid.New())
			Expect(err).NotTo(HaveOccurred())
			xorUniq := util.XorBigInt(big.NewInt(5000), b2)
			intermediateXor := xorUniq
			Expect(xorUniq.Cmp(big.NewInt(5000))).NotTo(Equal(0))
			Expect(xorUniq.Cmp(b2)).NotTo(Equal(0))
			xorUniq = util.XorBigInt(xorUniq, b)
			Expect(xorUniq.Cmp(intermediateXor)).NotTo(Equal(0))
			Expect(xorUniq.Cmp(b)).NotTo(Equal(0))
			Expect(xorUniq.Cmp(b2)).NotTo(Equal(0))

			xorUniq = util.XorBigInt(xorUniq, b)
			Expect(xorUniq.Cmp(intermediateXor)).To(Equal(0))
			Expect(xorUniq.Cmp(b)).NotTo(Equal(0))
			Expect(xorUniq.Cmp(b2)).NotTo(Equal(0))
			Expect(xorUniq.Cmp(big.NewInt(5000))).NotTo(Equal(0))

			xorUniq = util.XorBigInt(xorUniq, big.NewInt(5000))
			Expect(xorUniq.Cmp(intermediateXor)).NotTo(Equal(0))
			Expect(xorUniq.Cmp(b2)).To(Equal(0))

			xorUniq = util.XorBigInt(xorUniq, b2)
			Expect(xorUniq.Cmp(big.NewInt(0))).To(Equal(0))
		})
	})
})
