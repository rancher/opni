package update_test

import (
	"context"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	mock_update "github.com/rancher/opni/pkg/test/mock/update"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/urn"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/streams"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("update server", Ordered, Label("unit"), func() {
	var (
		server                 = update.NewUpdateServer(testlog.Log)
		agentUrn1              = urn.NewOpniURN(urn.Agent, "agent1", "foo")
		pluginUrn1             = urn.NewOpniURN(urn.Plugin, "test1", "bar")
		pluginUrn2             = urn.NewOpniURN(urn.Plugin, "test2", "bar")
		expectedPluginManifest *controlv1.UpdateManifest
		expectedAgentManifest  *controlv1.UpdateManifest
		patchList              *controlv1.PatchList
	)
	BeforeAll(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockHandler := mock_update.NewMockUpdateTypeHandler(ctrl)
		mockHandler.EXPECT().
			Strategy().Return("mock").AnyTimes()
		mockHandler.EXPECT().
			Collectors().Return(nil).AnyTimes()
		mockHandler.EXPECT().
			CalculateExpectedManifest(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, updateType urn.UpdateType) (*controlv1.UpdateManifest, error) {
				if updateType == urn.Agent {
					return expectedAgentManifest, nil
				}
				return expectedPluginManifest, nil
			}).AnyTimes()
		mockHandler.EXPECT().
			CalculateUpdate(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, manifest *controlv1.UpdateManifest) (*controlv1.PatchList, error) {
				return patchList, nil
			}).
			AnyTimes()
		server.RegisterUpdateHandler(mockHandler.Strategy(), mockHandler)
	})

	BeforeEach(func() {
		expectedAgentManifest = &controlv1.UpdateManifest{
			Items: []*controlv1.UpdateManifestEntry{
				{
					Package: agentUrn1.String(),
					Path:    "agent1",
					Digest:  "xyz",
				},
			},
		}
		expectedPluginManifest = &controlv1.UpdateManifest{
			Items: []*controlv1.UpdateManifestEntry{
				{
					Package: pluginUrn1.String(),
					Path:    "foo",
					Digest:  "abc",
				},
				{
					Package: pluginUrn2.String(),
					Path:    "bar",
					Digest:  "abc",
				},
			},
		}
	})

	When("manifest has mixed strategies", func() {
		It("should return an error", func() {
			invalid := util.ProtoClone(expectedPluginManifest)
			invalid.Items[1].Package = urn.NewOpniURN(urn.Agent, "test2", "bar").String()
			_, err := server.SyncManifest(context.Background(), invalid)
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
			urn1 := urn.NewOpniURN(urn.Plugin, "mock", "bar")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: urn1.String(),
						Path:    "foo",
						Digest:  "abc",
					},
				},
			}
			patchList = &controlv1.PatchList{
				Items: []*controlv1.PatchSpec{
					{
						Op:        controlv1.PatchOp_Create,
						Package:   pluginUrn2.String(),
						Path:      "bar",
						NewDigest: "abc",
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
					// if we get here, everything needs to be up to date
					manifest, ok := update.ManifestMetadataFromContext(stream.Context())
					if !ok {
						return status.Errorf(codes.Internal, "fail: no manifest metadata")
					}
					expected := &controlv1.UpdateManifest{}
					expected.Items = append(expected.Items, expectedAgentManifest.Items...)
					expected.Items = append(expected.Items, expectedPluginManifest.Items...)
					expected.Sort()

					if manifest.Digest() != expected.Digest() {
						return status.Errorf(codes.Internal, "fail: manifest digest mismatch")
					}
					stream.Send(&testgrpc.StreamResponse{
						Response: "OK",
					})
					return nil
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
			It("should return an InvalidArgument error", func() {
				stream, err := client.Stream(context.Background(), grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				_, err = stream.Recv()
				Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument))
			})
		})
		When("no handlers are available for the requested update strategies", func() {
			It("should return an Unimplemented error", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKeyForType(urn.Agent), "unimplemented",
					controlv1.UpdateStrategyKeyForType(urn.Plugin), "unimplemented",
					controlv1.ManifestDigestKeyForType(urn.Agent), "-",
					controlv1.ManifestDigestKeyForType(urn.Plugin), "-",
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				_, err = stream.Recv()
				Expect(err).To(testutil.MatchStatusCode(codes.Unimplemented))
			})
		})
		When("one handler is unimplemented but one is available", func() {
			It("should return an Unimplemented error", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKeyForType(urn.Agent), "unimplemented",
					controlv1.UpdateStrategyKeyForType(urn.Plugin), "mock",
					controlv1.ManifestDigestKeyForType(urn.Agent), "-",
					controlv1.ManifestDigestKeyForType(urn.Plugin), "-",
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				_, err = stream.Recv()
				Expect(err).To(testutil.MatchStatusCode(codes.Unimplemented))
			})
		})
		When("agent manifest is out of date", func() {
			It("should return a FailedPrecondition error", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKeyForType(urn.Agent), "mock",
					controlv1.UpdateStrategyKeyForType(urn.Plugin), "mock",
					controlv1.ManifestDigestKeyForType(urn.Agent), "old",
					controlv1.ManifestDigestKeyForType(urn.Plugin), expectedPluginManifest.Digest(),
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				_, err = stream.Recv()
				Expect(err).To(testutil.MatchStatusCode(codes.FailedPrecondition, Equal("agent resources out of date")))
			})
		})
		When("plugin manifest is out of date", func() {
			It("should return a FailedPrecondition error", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKeyForType(urn.Agent), "mock",
					controlv1.UpdateStrategyKeyForType(urn.Plugin), "mock",
					controlv1.ManifestDigestKeyForType(urn.Agent), expectedAgentManifest.Digest(),
					controlv1.ManifestDigestKeyForType(urn.Plugin), "old",
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				_, err = stream.Recv()
				Expect(err).To(testutil.MatchStatusCode(codes.FailedPrecondition, Equal("plugin resources out of date")))
			})
		})
		When("agent and plugin manifests are out of date", func() {
			It("should return a FailedPrecondition error", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKeyForType(urn.Agent), "mock",
					controlv1.UpdateStrategyKeyForType(urn.Plugin), "mock",
					controlv1.ManifestDigestKeyForType(urn.Agent), "old",
					controlv1.ManifestDigestKeyForType(urn.Plugin), "old",
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				_, err = stream.Recv()
				Expect(err).To(testutil.MatchStatusCode(codes.FailedPrecondition, Equal("agent, plugin resources out of date")))
			})
		})
		When("missing manifest digest key for agent", func() {
			It("should return an InvalidArgument error", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKeyForType(urn.Agent), "mock",
					controlv1.UpdateStrategyKeyForType(urn.Plugin), "mock",
					controlv1.ManifestDigestKeyForType(urn.Plugin), expectedPluginManifest.Digest(),
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				_, err = stream.Recv()
				Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument, Equal("manifest digest for type \"agent\" missing or invalid")))
			})
		})
		When("missing manifest digest key for plugin", func() {
			It("should return an InvalidArgument error", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKeyForType(urn.Agent), "mock",
					controlv1.UpdateStrategyKeyForType(urn.Plugin), "mock",
					controlv1.ManifestDigestKeyForType(urn.Agent), expectedAgentManifest.Digest(),
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				_, err = stream.Recv()
				Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument, Equal("manifest digest for type \"plugin\" missing or invalid")))
			})
		})
		When("all manifests are up to date", func() {
			It("should succeed", func() {
				ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
					controlv1.UpdateStrategyKeyForType(urn.Agent), "mock",
					controlv1.UpdateStrategyKeyForType(urn.Plugin), "mock",
					controlv1.ManifestDigestKeyForType(urn.Agent), expectedAgentManifest.Digest(),
					controlv1.ManifestDigestKeyForType(urn.Plugin), expectedPluginManifest.Digest(),
				))
				stream, err := client.Stream(ctx, grpc.WaitForReady(true))
				Expect(err).NotTo(HaveOccurred())
				resp, err := stream.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Response).To(Equal("OK"))
			})
		})
	})
})
