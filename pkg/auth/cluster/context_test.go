package cluster_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/keyring"
	"google.golang.org/grpc/metadata"

	"github.com/rancher/opni/pkg/auth/cluster"
)

var _ = Describe("Cluster Context Utils", Label("unit"), func() {
	Describe("StreamAuthorizedKeys", func() {
		It("should return shared keys from context", func() {
			keys := keyring.NewSharedKeys(make([]byte, 64))
			ctx := context.WithValue(context.Background(), cluster.SharedKeysKey, keys)
			Expect(cluster.StreamAuthorizedKeys(ctx)).To(Equal(keys))
		})
	})

	Describe("StreamAuthorizedID", func() {
		It("should return cluster ID from context", func() {
			ctx := context.WithValue(context.Background(), cluster.ClusterIDKey, "cluster-123")
			Expect(cluster.StreamAuthorizedID(ctx)).To(Equal("cluster-123"))
		})
	})

	Describe("AuthorizedOutgoingContext", func() {
		It("should append cluster ID to outgoing metadata", func() {
			ctx := context.WithValue(context.Background(), cluster.ClusterIDKey, "cluster-123")
			outgoingCtx := cluster.AuthorizedOutgoingContext(ctx)
			md, ok := metadata.FromOutgoingContext(outgoingCtx)
			Expect(ok).To(BeTrue())
			Expect(md[string(cluster.ClusterIDKey)]).To(ConsistOf("cluster-123"))
		})
	})

	Describe("AuthorizedIDFromIncomingContext", func() {
		It("should extract cluster ID from incoming metadata", func() {
			md := metadata.Pairs(string(cluster.ClusterIDKey), "cluster-123")
			ctx := metadata.NewIncomingContext(context.Background(), md)
			id, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
			Expect(ok).To(BeTrue())
			Expect(id).To(Equal("cluster-123"))
		})

		It("should return false when cluster ID is not found in metadata", func() {
			ctx := context.Background()
			_, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
			Expect(ok).To(BeFalse())
		})
		It("should return false when the metadata contains an empty slice", func() {
			md := metadata.MD{string(cluster.ClusterIDKey): []string{}}
			ctx := metadata.NewIncomingContext(context.Background(), md)
			_, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
			Expect(ok).To(BeFalse())
		})
	})
})
