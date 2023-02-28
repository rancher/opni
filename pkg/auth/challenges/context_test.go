package challenges_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/auth/challenges"
	"google.golang.org/grpc/metadata"
)

type testParams struct {
	NewContextFunc  func(ctx context.Context, md metadata.MD) context.Context
	GetMetadataFunc func(ctx context.Context) (challenges.ClientMetadata, error)
}

func doTest(params testParams) {
	It("should extract asserted ID and client random from incoming metadata", func() {
		md := metadata.Pairs(
			challenges.ClientIdAssertionMetadataKey, "cluster-123",
			challenges.ClientRandomMetadataKey, "bm90aGluZy11cC1teS1zbGVldmU",
		)
		ctx := params.NewContextFunc(context.Background(), md)
		cm, err := params.GetMetadataFunc(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.IdAssertion).To(Equal("cluster-123"))
		Expect(string(cm.Random)).To(Equal("nothing-up-my-sleeve"))
	})

	It("should return false when the asserted ID is not found in metadata", func() {
		ctx := context.Background()
		_, err := params.GetMetadataFunc(ctx)
		Expect(err).To(HaveOccurred())
	})

	It("should return false when the asserted ID is an empty string", func() {
		md := metadata.MD{
			challenges.ClientIdAssertionMetadataKey: []string{""},
		}
		ctx := params.NewContextFunc(context.Background(), md)
		_, err := params.GetMetadataFunc(ctx)
		Expect(err).To(HaveOccurred())
	})

	It("should return false when the asserted ID metadata is an empty slice", func() {
		md := metadata.MD{
			challenges.ClientIdAssertionMetadataKey: []string{},
		}
		ctx := params.NewContextFunc(context.Background(), md)
		_, err := params.GetMetadataFunc(ctx)
		Expect(err).To(HaveOccurred())
	})

	It("should return false when the client random is not found in metadata", func() {
		md := metadata.Pairs(
			challenges.ClientIdAssertionMetadataKey, "cluster-123",
		)
		ctx := params.NewContextFunc(context.Background(), md)
		_, err := params.GetMetadataFunc(ctx)
		Expect(err).To(HaveOccurred())
	})

	It("should return false when the client random is empty", func() {
		md := metadata.Pairs(
			challenges.ClientIdAssertionMetadataKey, "cluster-123",
			challenges.ClientRandomMetadataKey, "",
		)
		ctx := params.NewContextFunc(context.Background(), md)
		_, err := params.GetMetadataFunc(ctx)
		Expect(err).To(HaveOccurred())
	})

	It("should return false when the client random is not base64 encoded", func() {
		md := metadata.Pairs(
			challenges.ClientIdAssertionMetadataKey, "cluster-123",
			challenges.ClientRandomMetadataKey, "????",
		)
		ctx := params.NewContextFunc(context.Background(), md)
		_, err := params.GetMetadataFunc(ctx)
		Expect(err).To(HaveOccurred())
	})
}

var _ = Describe("Challenge Context Utils", Label("unit"), func() {
	Describe("ClientMetadataFromIncomingContext", func() {
		doTest(testParams{
			GetMetadataFunc: challenges.ClientMetadataFromIncomingContext,
			NewContextFunc:  metadata.NewIncomingContext,
		})
	})
	Describe("ClientMetadataFromOutgoingContext", func() {
		doTest(testParams{
			GetMetadataFunc: challenges.ClientMetadataFromOutgoingContext,
			NewContextFunc:  metadata.NewOutgoingContext,
		})
	})
})
