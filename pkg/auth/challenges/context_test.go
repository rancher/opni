package challenges_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/auth/challenges"
	"google.golang.org/grpc/metadata"
)

var _ = Describe("Cluster package", func() {
	Describe("ClientMetadataFromIncomingContext", func() {
		It("should extract asserted ID and client random from incoming metadata", func() {
			md := metadata.Pairs(
				challenges.ClientIdAssertionMetadataKey, "cluster-123",
				challenges.ClientRandomMetadataKey, "bm90aGluZy11cC1teS1zbGVldmU",
			)
			ctx := metadata.NewIncomingContext(context.Background(), md)
			cm, err := challenges.ClientMetadataFromIncomingContext(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.IdAssertion).To(Equal("cluster-123"))
			Expect(string(cm.Random)).To(Equal("nothing-up-my-sleeve"))
		})

		It("should return false when the asserted ID is not found in metadata", func() {
			ctx := context.Background()
			_, err := challenges.ClientMetadataFromIncomingContext(ctx)
			Expect(err).To(HaveOccurred())
		})

		It("should return false when the metadata contains an empty slice", func() {
			md := metadata.MD{string(challenges.ClientIdAssertionMetadataKey): []string{}}
			ctx := metadata.NewIncomingContext(context.Background(), md)
			_, err := challenges.ClientMetadataFromIncomingContext(ctx)
			Expect(err).To(HaveOccurred())
		})
	})
})
