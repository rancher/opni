package cluster_test

import (
	"context"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/util/streams"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

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
	Context("PerRPCCredentials", func() {
		It("should allow using cluster ID as PerRPCCredentials", func() {
			server := grpc.NewServer(
				grpc.Creds(insecure.NewCredentials()),
				grpc.ChainStreamInterceptor(
					func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						stream := streams.NewServerStreamWithContext(ss)
						stream.Ctx = cluster.ClusterIDKey.FromIncomingCredentials(stream.Ctx)
						return handler(srv, stream)
					},
				),
			)

			id := make(chan any, 1)
			testgrpc.RegisterStreamServiceServer(server, &testgrpc.StreamServer{
				ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
					id <- stream.Context().Value(cluster.ClusterIDKey)
					return nil
				},
			})
			listener := bufconn.Listen(1024 * 1024)
			go server.Serve(listener)
			DeferCleanup(listener.Close)

			cc, _ := grpc.Dial("bufconn",
				grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithPerRPCCredentials(cluster.ClusterIDKey),
			)

			client := testgrpc.NewStreamServiceClient(cc)
			{
				ctx := context.WithValue(context.Background(), cluster.ClusterIDKey, "foo")

				_, err := client.Stream(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(<-id).To(BeEquivalentTo("foo"))
			}
			{
				_, err := client.Stream(context.Background())
				Expect(err).NotTo(HaveOccurred())

				Expect(<-id).To(BeNil())
			}
		})
		Specify("misc error checking", func() {
			ctx := cluster.ClusterIDKey.FromIncomingCredentials(context.Background())
			Expect(ctx).To(Equal(context.Background()))
		})
	})
})
