package update_test

import (
	"context"
	"fmt"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/urn"
	"github.com/rancher/opni/pkg/util/streams"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("update server", Ordered, Label("unit"), func() {
	var (
		server = update.NewUpdateServer(testlog.Log)
		mock   = newMockHandler()
	)
	BeforeAll(func() {
		server.RegisterUpdateHandler(mock.Strategy(), mock)
	})

	When("manifest has mixed strategies", func() {
		It("should return an error", func() {
			urn1 := urn.NewOpniURN(urn.Plugin, "test1", "bar")
			urn2 := urn.NewOpniURN(urn.Plugin, "test2", "bar")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: urn1.String(),
						Path:    "foo",
						Digest:  "abc",
					},
					{
						Package: urn2.String(),
						Path:    "bar",
						Digest:  "abc",
					},
				},
			}
			_, err := server.SyncManifest(context.Background(), manifest)
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})
	})
	When("manifest has an unregistered strategy", func() {
		It("should return an error", func() {
			urn1 := urn.NewOpniURN(urn.Plugin, "test1", "bar")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: urn1.String(),
						Path:    "foo",
						Digest:  "abc",
					},
				},
			}
			_, err := server.SyncManifest(context.Background(), manifest)
			Expect(status.Code(err)).To(Equal(codes.Unimplemented))
		})
	})
	When("manifest is valid", func() {
		It("should return a patch list and desired state", func() {
			urn1 := urn.NewOpniURN(urn.Plugin, "noop", "bar")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: urn1.String(),
						Path:    "foo",
						Digest:  "abc",
					},
				},
			}
			syncResults, err := server.SyncManifest(context.Background(), manifest)
			Expect(err).ToNot(HaveOccurred())
			Expect(syncResults.RequiredPatches).ToNot(BeNil())
		})
	})

	Context("stream interceptor", Ordered, func() {
		var (
			grpcSrv *grpc.Server
			client  testgrpc.StreamServiceClient
			conn    *grpc.ClientConn
			lis     *bufconn.Listener
		)
		BeforeAll(func() {
			lis = bufconn.Listen(1024 * 1024)
			grpcSrv = grpc.NewServer(
				grpc.ChainStreamInterceptor(
					func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						sc := &streams.ServerStreamWithContext{
							Ctx:    context.WithValue(ss.Context(), cluster.ClusterIDKey, "cluster-1"),
							Stream: ss,
						}
						return handler(srv, sc)
					},
					server.StreamServerInterceptor(),
				),
				grpc.Creds(insecure.NewCredentials()),
			)

			testgrpc.RegisterStreamServiceServer(grpcSrv, &testgrpc.StreamServer{
				ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
					return stream.Send(&testgrpc.StreamResponse{
						Response: fmt.Sprintf("handler called %d times", mock.streamHandlerCalls),
					})
				},
			})
			go grpcSrv.Serve(lis)

			var err error
			conn, err = grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
			Expect(err).NotTo(HaveOccurred())

			client = testgrpc.NewStreamServiceClient(conn)
		})
		AfterAll(func() {
			conn.Close()
			lis.Close()
		})
		When("no strategy in metadata", func() {
			It("should pass through the handler", func() {
				stream, err := client.Stream(context.Background(), grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				resp, err := stream.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.GetResponse()).To(Equal("handler called 0 times"))
			})
		})
		When("no server interceptor in handler", func() {
			It("should pass through the handler", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKey, "noop",
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				resp, err := stream.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.GetResponse()).To(Equal("handler called 0 times"))
			})
		})
		When("there is a server interceptor in handler", func() {
			It("should call the handler", func() {
				interceptor := mock.mockInterceptor()
				server.RegisterUpdateHandler("mock", interceptor)
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKey, "mock",
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				resp, err := stream.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.GetResponse()).To(Equal("handler called 1 times"))
			})
		})
		When("there are multiple server interceptors in handler", func() {
			It("should call the handlers", func() {
				mock.streamHandlerCalls = 0
				interceptor := mock.mockInterceptor()
				server.RegisterUpdateHandler("mock2", interceptor)
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKey, "mock",
				))
				ctx = metadata.AppendToOutgoingContext(ctx, controlv1.UpdateStrategyKey, "mock2")
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				resp, err := stream.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.GetResponse()).To(Equal("handler called 2 times"))
			})
		})
	})
})
