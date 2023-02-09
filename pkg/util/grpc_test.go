package util_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	defaultTtl              = time.Second * 5
	defaultEvictionInterval = time.Second * 1
)

var _ = BuildCachingInterceptorSuite(
	"default grpc middleware",
	func() util.GrpcCachingInterceptor {
		return util.NewClientGrpcEntityCacher(
			caching.NewInMemoryEntityCache("50Mi", defaultEvictionInterval),
		)
	},
	func(clientCacher util.GrpcCachingInterceptor) (testgrpc.CachedServiceServer, testgrpc.CachedServiceClient) {
		cachedServer := testgrpc.NewCachedServer(defaultTtl)

		listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", util.Must(freeport.GetFreePort())))
		Expect(err).To(Succeed())

		server := grpc.NewServer(
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             15 * time.Second,
				PermitWithoutStream: true,
			}),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    15 * time.Second,
				Timeout: 5 * time.Second,
			}),
			grpc.ChainUnaryInterceptor(
				clientCacher.UnaryServerInterceptor(),
			),
		)

		testgrpc.RegisterCachedServiceServer(server, cachedServer)

		_ = lo.Async(func() error {
			return server.Serve(listener)
		})
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainUnaryInterceptor(
				clientCacher.UnaryClientInterceptor(),
			),
		}
		conn, err := grpc.Dial(listener.Addr().String(), opts...)
		Expect(err).To(Succeed())
		cachedClient := testgrpc.NewCachedServiceClient(conn)

		DeferCleanup(func() {
			conn.Close()
			server.Stop()
		})
		return cachedServer, cachedClient
	},
)

func BuildCachingInterceptorSuite(
	name string,
	clientSideCacherConstructor func() util.GrpcCachingInterceptor,
	serverAndClientConstructor func(
		client util.GrpcCachingInterceptor,
	) (testgrpc.CachedServiceServer, testgrpc.CachedServiceClient),
) bool {
	return Describe(fmt.Sprintf("GRPC caching interceptor for %s", name), Ordered, Label("unit"), func() {
		var testServer testgrpc.CachedServiceServer
		var testClient testgrpc.CachedServiceClient
		var clientEntityCacher util.GrpcCachingInterceptor
		var ctx context.Context
		BeforeAll(func() {
			ctx = context.Background()
			clientEntityCacher = clientSideCacherConstructor()
			testServer, testClient = serverAndClientConstructor(clientEntityCacher)

		})

		var aggregate int64 = 1

		When("using the client-side caching interceptor", func() {
			Specify("the client should be able request caching using cache-control headers", func() {
				_, err := testClient.Increment(ctx, &testgrpc.IncrementRequest{
					Value: 1,
				})
				Expect(err).To(Succeed())
				ctxMetadata := util.WithGrpcClientCaching(ctx, 5*time.Second)

				md, ok := metadata.FromOutgoingContext(ctxMetadata)
				Expect(ok).To(BeTrue())
				Expect(md.Get(util.GrpcCacheControlHeader(util.CacheTypeClient))).NotTo(HaveLen(0))

				value, err := testClient.GetValue(
					ctxMetadata,
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(aggregate)))

				var wg sync.WaitGroup
				for i := 0; i < 5; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := testClient.Increment(ctx, &testgrpc.IncrementRequest{
							Value: 1,
						})
						Expect(err).To(Succeed())
					}()
					aggregate++
				}
				wg.Wait()
				By("verifying the value in the cache hasn't expired")
				value, err = testClient.GetValue(
					ctx,
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(1)))

				By("verifying we can tell the client to bypass any caching")
				value, err = testClient.GetValue(
					util.WithBypassCache(ctx),
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(aggregate)))

				for i := 0; i < 5; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := testClient.Increment(ctx, &testgrpc.IncrementRequest{
							Value: 1,
						})
						Expect(err).To(Succeed())
					}()
					aggregate++
				}
				wg.Wait()

				By("verifying the value in the cache hasn't expired (2)")
				value, err = testClient.GetValue(
					ctx,
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(1)))

				By("letting the cache expire and getting the server's actual value")
				Eventually(func() int64 {
					return util.Must(testServer.GetValue(ctx, &emptypb.Empty{})).Value
				}, defaultTtl*2, defaultEvictionInterval).Should(Equal(int64(aggregate)))
			})

			Specify("the server should be able to force the client to cache the response", func() {
				Eventually(func() int64 {
					return util.Must(testServer.GetValue(ctx, &emptypb.Empty{})).Value
				}, defaultTtl*2, defaultEvictionInterval).Should(Equal(int64(aggregate)))

				_, err := testClient.Increment(ctx, &testgrpc.IncrementRequest{
					Value: 1,
				})
				Expect(err).To(Succeed())
				aggregate++

				value, err := testClient.GetValueWithForcedClientCaching(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(aggregate)))
				var wg sync.WaitGroup
				for i := 0; i < 10; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := testClient.Increment(ctx, &testgrpc.IncrementRequest{
							Value: 1,
						})
						Expect(err).To(Succeed())
					}()
					aggregate++
				}
				wg.Wait()

				By("verifying the value exists in the cache, despite the client not opting into it")
				value, err = testClient.GetValueWithForcedClientCaching(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(aggregate - 10))

				By("veryfing the client can still bypass the cache")
				value, err = testClient.GetValueWithForcedClientCaching(
					util.WithBypassCache(ctx),
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(aggregate)))

				By("verifying the value in the cache will eventually expire")
				Eventually(func() int64 {
					return util.Must(testServer.GetValue(ctx, &emptypb.Empty{})).Value
				}, defaultTtl*2, defaultEvictionInterval).Should(Equal(int64(aggregate)))
			})

			Specify("proto messages can implement their own cache keys", func() {
				var _ caching.CacheKeyer = (*testgrpc.ObjectReference)(nil)

				id1, id2 := uuid.New().String(), uuid.New().String()
				_, err := testClient.IncrementObject(ctx, &testgrpc.IncrementObjectRequest{
					Id: &testgrpc.ObjectReference{
						Id: id1,
					},
					Value: 1,
				})
				Expect(err).To(Succeed())

				_, err = testClient.IncrementObject(ctx, &testgrpc.IncrementObjectRequest{
					Id: &testgrpc.ObjectReference{
						Id: id2,
					},
					Value: 1,
				})
				Expect(err).To(Succeed())

				By("sanity checking ObjectReference's use of cache key")
				value, err := testClient.GetObjectValue(
					util.WithGrpcClientCaching(ctx, 5*time.Second),
					&testgrpc.ObjectReference{
						Id: id1,
					})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(1)))

				_, err = testClient.IncrementObject(ctx, &testgrpc.IncrementObjectRequest{
					Id: &testgrpc.ObjectReference{
						Id: id2,
					},
					Value: 1,
				})
				Expect(err).To(Succeed())

				// no cache requested for object
				value, err = testClient.GetObjectValue(
					util.WithGrpcClientCaching(ctx, 5*time.Second),
					&testgrpc.ObjectReference{
						Id: id2,
					})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(2)))

				By("verifying that the client can opt out of server's cache keys")
				ctxMetadata := util.WithGrpcClientCaching(util.WithIgnoreServerCacheKeys(ctx), 5*time.Second)
				_, err = testClient.IncrementObject(
					ctxMetadata,
					&testgrpc.IncrementObjectRequest{
						Id: &testgrpc.ObjectReference{
							Id: id1,
						},
						Value: 2,
					},
				)
				Expect(err).To(Succeed())
				// client-side requests this object without server-set cache keys
				value, err = testClient.GetObjectValue(util.WithIgnoreServerCacheKeys(ctx), &testgrpc.ObjectReference{
					Id: id1,
				})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(3)))

				value, err = testClient.GetObjectValue(ctx, &testgrpc.ObjectReference{
					Id: id1,
				})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(1)))
			})
		})
	})
}
