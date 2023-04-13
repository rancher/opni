package caching_test

import (
	"context"
	"fmt"
	"math/rand"
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
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
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

type cacheInterceptorConstructor func() caching.GrpcCachingInterceptor

var _ = BuildCachingInterceptorSuite(
	"default grpc middleware",
	func() caching.GrpcCachingInterceptor {
		return caching.NewClientGrpcTtlCacher(
			caching.NewInMemoryGrpcTtlCache(5*1024*1024, defaultEvictionInterval),
		)
	},
	func(buildCache cacheInterceptorConstructor) (
		testgrpc.SimpleServiceServer,
		testgrpc.ObjectServiceServer,
		testgrpc.AggregatorServiceServer,
		testgrpc.SimpleServiceClient,
		testgrpc.ObjectServiceClient,
		testgrpc.AggregatorServiceClient,
	) {
		testCacher := buildCache()
		aggregatorCacher := buildCache()
		testAggregatorClientCacher := buildCache()

		// ------ setup standalone servers
		simpleServer := testgrpc.NewSimpleServer(defaultTtl)
		objectServer := testgrpc.NewObjectServer(defaultTtl)

		defaultListener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", freeport.GetFreePort()))
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
				testCacher.UnaryServerInterceptor(),
			),
		)

		testgrpc.RegisterSimpleServiceServer(server, simpleServer)
		testgrpc.RegisterObjectServiceServer(server, objectServer)

		_ = lo.Async(func() error {
			return server.Serve(defaultListener)
		})
		opts1 := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainUnaryInterceptor(
				testCacher.UnaryClientInterceptor(),
			),
		}
		opts2 := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainUnaryInterceptor(
				aggregatorCacher.UnaryClientInterceptor(),
			),
		}
		userConn, err := grpc.Dial(defaultListener.Addr().String(), opts1...)
		Expect(err).To(Succeed())
		simpleClientUser := testgrpc.NewSimpleServiceClient(userConn)
		objectClientUser := testgrpc.NewObjectServiceClient(userConn)

		aggregatorConn, err := grpc.Dial(defaultListener.Addr().String(), opts2...)
		Expect(err).To(Succeed())
		simpleClientAggregator := testgrpc.NewSimpleServiceClient(aggregatorConn)
		objectClientAggregator := testgrpc.NewObjectServiceClient(aggregatorConn)

		// ------ setup server that connects remotely to the other two

		aggregatorServer := testgrpc.NewAggregatorServer(defaultTtl, simpleClientAggregator, objectClientAggregator)

		aggregatorListener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", freeport.GetFreePort()))
		Expect(err).To(Succeed())

		server2 := grpc.NewServer(
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             15 * time.Second,
				PermitWithoutStream: true,
			}),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    15 * time.Second,
				Timeout: 5 * time.Second,
			}),
			grpc.ChainUnaryInterceptor(
				aggregatorCacher.UnaryServerInterceptor(),
			),
		)
		_ = lo.Async(func() error {
			return server2.Serve(aggregatorListener)
		})
		testgrpc.RegisterAggregatorServiceServer(server2, aggregatorServer)

		opts3 := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainUnaryInterceptor(
				testAggregatorClientCacher.UnaryClientInterceptor(),
			),
		}
		testAggregatorConn, err := grpc.Dial(aggregatorListener.Addr().String(), opts3...)
		Expect(err).To(Succeed())
		aggregatorClient := testgrpc.NewAggregatorServiceClient(testAggregatorConn)

		DeferCleanup(func() {
			userConn.Close()
			aggregatorConn.Close()
			testAggregatorConn.Close()
			server.Stop()
			server2.Stop()
		})
		return simpleServer, objectServer, aggregatorServer, simpleClientUser, objectClientUser, aggregatorClient
	},
)

func BuildCachingInterceptorSuite(
	name string,
	buildCache cacheInterceptorConstructor,
	serverAndClientConstructor func(
		cacheInterceptorConstructor,
	) (testgrpc.SimpleServiceServer,
		testgrpc.ObjectServiceServer,
		testgrpc.AggregatorServiceServer,
		testgrpc.SimpleServiceClient,
		testgrpc.ObjectServiceClient,
		testgrpc.AggregatorServiceClient),
) bool {
	return Describe(fmt.Sprintf("GRPC caching interceptor for %s", name), Ordered, Label("integration"), func() {
		var testSimpleServer testgrpc.SimpleServiceServer
		var testAggregatorServer testgrpc.AggregatorServiceServer
		var testSimpleClient testgrpc.SimpleServiceClient
		var testObjectClient testgrpc.ObjectServiceClient
		var testAggregatorClient testgrpc.AggregatorServiceClient
		var ctx context.Context
		var id1, id2 string
		BeforeAll(func() {
			ctx = context.Background()
			testSimpleServer,
				_,
				testAggregatorServer,
				// _,
				testSimpleClient,
				testObjectClient,
				testAggregatorClient = serverAndClientConstructor(buildCache)
			id1, id2 = uuid.New().String(), uuid.New().String()

		})

		var aggregate int64 = 1

		When("using the client-side caching interceptor", func() {
			Specify("the client should be able request caching using cache-control headers", func() {
				_, err := testSimpleClient.Increment(ctx, &testgrpc.IncrementRequest{
					Value: 1,
				})
				Expect(err).To(Succeed())
				ctxMetadata := caching.WithGrpcClientCaching(ctx, 5*time.Second)

				md, ok := metadata.FromOutgoingContext(ctxMetadata)
				Expect(ok).To(BeTrue())
				Expect(md.Get(caching.GrpcCacheControlHeader(caching.CacheTypeClient))).NotTo(HaveLen(0))

				value, err := testSimpleClient.GetValue(
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
						_, err := testSimpleClient.Increment(ctx, &testgrpc.IncrementRequest{
							Value: 1,
						})
						Expect(err).To(Succeed())
					}()
					aggregate++
				}
				wg.Wait()
				By("verifying the aggregated value is in the server")
				value, err = testSimpleServer.GetValue(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(aggregate)))

				By("verifying the value in the cache hasn't expired")
				value, err = testSimpleClient.GetValue(
					ctx,
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(1)))

				By("verifying we can tell the client to bypass any caching")
				value, err = testSimpleClient.GetValue(
					caching.WithBypassCache(ctx),
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(aggregate)))

				for i := 0; i < 5; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := testSimpleClient.Increment(ctx, &testgrpc.IncrementRequest{
							Value: 1,
						})
						Expect(err).To(Succeed())
					}()
					aggregate++
				}
				wg.Wait()

				By("verifying the value in the cache hasn't expired (2)")
				value, err = testSimpleClient.GetValue(
					ctx,
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(1)))

				By("letting the cache expire and getting the server's actual value")
				Eventually(func() int64 {
					return util.Must(testSimpleClient.GetValue(ctx, &emptypb.Empty{})).Value
				}, defaultTtl*2, defaultEvictionInterval).Should(Equal(int64(aggregate)))
			})

			Specify("the server should be able to force the client to cache the response", func() {
				Eventually(func() int64 {
					return util.Must(testSimpleClient.GetValue(ctx, &emptypb.Empty{})).Value
				}, defaultTtl*2, defaultEvictionInterval).Should(Equal(int64(aggregate)))

				_, err := testSimpleClient.Increment(ctx, &testgrpc.IncrementRequest{
					Value: 1,
				})
				Expect(err).To(Succeed())
				aggregate++

				value, err := testSimpleClient.GetValueWithForcedClientCaching(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(aggregate)))
				var wg sync.WaitGroup
				for i := 0; i < 10; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, err := testSimpleClient.Increment(ctx, &testgrpc.IncrementRequest{
							Value: 1,
						})
						Expect(err).To(Succeed())
					}()
					aggregate++
				}
				wg.Wait()

				By("verifying the value exists in the cache, despite the client not opting into it")
				value, err = testSimpleClient.GetValueWithForcedClientCaching(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(aggregate - 10))

				By("veryfing the client can still bypass the cache")
				value, err = testSimpleClient.GetValueWithForcedClientCaching(
					caching.WithBypassCache(ctx),
					&emptypb.Empty{},
				)
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(aggregate)))

				By("verifying the value in the cache will eventually expire")
				Eventually(func() int64 {
					return util.Must(testSimpleClient.GetValue(ctx, &emptypb.Empty{})).Value
				}, defaultTtl*2, defaultEvictionInterval).Should(Equal(int64(aggregate)))
			})

			Specify("proto messages can implement their own cache keys", func() {
				var _ caching.CacheKeyer = (*testgrpc.ObjectReference)(nil)

				_, err := testObjectClient.IncrementObject(ctx, &testgrpc.IncrementObjectRequest{
					Id: &testgrpc.ObjectReference{
						Id: id1,
					},
					Value: 1,
				})
				Expect(err).To(Succeed())

				_, err = testObjectClient.IncrementObject(ctx, &testgrpc.IncrementObjectRequest{
					Id: &testgrpc.ObjectReference{
						Id: id2,
					},
					Value: 1,
				})
				Expect(err).To(Succeed())

				By("sanity checking ObjectReference's use of cache key")
				value, err := testObjectClient.GetObjectValue(
					caching.WithGrpcClientCaching(ctx, 5*time.Second),
					&testgrpc.ObjectReference{
						Id: id1,
					})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(1)))

				_, err = testObjectClient.IncrementObject(ctx, &testgrpc.IncrementObjectRequest{
					Id: &testgrpc.ObjectReference{
						Id: id2,
					},
					Value: 1,
				})
				Expect(err).To(Succeed())

				// no cache requested for object
				value, err = testObjectClient.GetObjectValue(
					caching.WithGrpcClientCaching(ctx, 5*time.Second),
					&testgrpc.ObjectReference{
						Id: id2,
					})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(2)))
			})

			Specify("cache control headers set by client should not leak to nested RPC calls", func() {
				val, err := testAggregatorServer.GetAll(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(val.Value).To(Equal(int64(25)))

				val, err = testAggregatorClient.GetAll(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(val.Value).To(Equal(int64(25)))

				_, err = testAggregatorClient.IncrementAll(
					ctx, &testgrpc.IncrementRequest{
						Value: 1,
					})
				Expect(err).To(Succeed())
				aggregate++

				val, err = testAggregatorClient.GetAll(caching.WithGrpcClientCaching(ctx, defaultTtl), &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(val.Value).To(Equal(int64(28)))

				_, err = testSimpleClient.Increment(ctx, &testgrpc.IncrementRequest{
					Value: 2,
				})
				Expect(err).To(Succeed())
				aggregate += 2

				val, err = testSimpleClient.GetValue(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(val.Value).To(Equal(int64(aggregate)))

				val, err = testAggregatorClient.GetAll(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(val.Value).To(Equal(int64(28)))

				By("verifying the value in the cache will eventually expire")
				Eventually(func() int64 {
					return util.Must(testAggregatorClient.GetAll(ctx, &emptypb.Empty{})).Value
				}, defaultTtl*2, defaultEvictionInterval).Should(Equal(int64(30)))
			})

			Specify("cache control headers set by the server should not leak to encapsulating RPC calls", func() {
				By("veryfing force-cached apis don't force any apis calling them to also cache")
				value, err := testAggregatorClient.GetAllWithNestedCaching(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(30)))

				_, err = testSimpleClient.Increment(ctx, &testgrpc.IncrementRequest{
					Value: 1,
				})
				Expect(err).To(Succeed())

				_, err = testObjectClient.IncrementObject(ctx, &testgrpc.IncrementObjectRequest{
					Id: &testgrpc.ObjectReference{
						Id: id1,
					},
					Value: 1,
				})
				Expect(err).To(Succeed())
				_, err = testObjectClient.IncrementObject(ctx, &testgrpc.IncrementObjectRequest{
					Id: &testgrpc.ObjectReference{
						Id: id2,
					},
					Value: 1,
				})
				Expect(err).To(Succeed())

				value, err = testAggregatorClient.GetAllWithNestedCaching(ctx, &emptypb.Empty{})
				Expect(err).To(Succeed())
				Expect(value.Value).To(Equal(int64(32)))

				By("verifying that the internal call's cache will eventually expire")
				Eventually(func() int64 {
					return util.Must(testAggregatorClient.GetAllWithNestedCaching(ctx, &emptypb.Empty{})).Value
				}, defaultTtl*2, defaultEvictionInterval).Should(Equal(int64(33)))
			})
		})

		When("The caching interceptor is used with other interceptors", func() {
			Specify("ordering should not affect the caching mechanism", func() {
				// fuzzy ordering

				type s struct {
					grpc.UnaryServerInterceptor
					grpc.UnaryClientInterceptor
				}
				randomInterceptors := []s{
					{
						UnaryServerInterceptor: otelgrpc.UnaryServerInterceptor(),
						UnaryClientInterceptor: otelgrpc.UnaryClientInterceptor(),
					},
				}

				for i := 0; i < 10; i++ {
					randomInterceptors = append(randomInterceptors, s{
						UnaryServerInterceptor: func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
							resp, err := handler(ctx, req)
							clientMd, ok := metadata.FromOutgoingContext(ctx)
							if ok {
								clientMd.Append(uuid.New().String(), uuid.New().String())
								grpc.SetHeader(ctx, clientMd)
							} else {
								grpc.SetHeader(ctx, metadata.Pairs(uuid.New().String(), uuid.New().String()))
							}
							return resp, err
						},
						UnaryClientInterceptor: func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
							var requestMd metadata.MD
							if err := invoker(ctx, method, req, reply, cc,
								append(opts,
									grpc.Header(&requestMd),
								)...,
							); err != nil {
								return err
							}
							if requestMd != nil {
								requestMd.Append(uuid.New().String(), uuid.New().String())
								grpc.SetHeader(ctx, requestMd)
							}

							return nil
						},
					})
				}

				numTestcases := 5
				var mu sync.Mutex

				var wg sync.WaitGroup
				for i := 0; i < numTestcases; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						aggregate := 1
						By("setting up a cache server")
						testCacher := buildCache()
						n := rand.Intn(len(randomInterceptors))
						registerInterceptors := lo.Samples(randomInterceptors, n)
						registerInterceptors = append(registerInterceptors, s{
							UnaryServerInterceptor: testCacher.UnaryServerInterceptor(),
							UnaryClientInterceptor: testCacher.UnaryClientInterceptor(),
						})
						registerInterceptors = lo.Shuffle(registerInterceptors)
						// append the value to the end of the slice
						registerInterceptors = append(registerInterceptors, s{
							UnaryServerInterceptor: testCacher.UnaryServerInterceptor(),
							UnaryClientInterceptor: testCacher.UnaryClientInterceptor(),
						})

						// select random index
						randomIndex := rand.Intn(len(registerInterceptors))

						// shift the elements to the right from the random index to the end of the slice
						copy(registerInterceptors[randomIndex+1:], registerInterceptors[randomIndex:len(registerInterceptors)-1])

						// serve + client

						// ------ setup standalone servers
						simpleServer := testgrpc.NewSimpleServer(defaultTtl)

						defaultListener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", freeport.GetFreePort()))
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
								lo.Map(registerInterceptors, func(item s, _ int) grpc.UnaryServerInterceptor {
									return item.UnaryServerInterceptor
								})...,
							),
						)

						testgrpc.RegisterSimpleServiceServer(server, simpleServer)

						_ = lo.Async(func() error {
							return server.Serve(defaultListener)
						})
						mu.Lock()
						DeferCleanup(server.Stop)
						mu.Unlock()
						opts := []grpc.DialOption{
							grpc.WithTransportCredentials(insecure.NewCredentials()),
							grpc.WithChainUnaryInterceptor(
								lo.Map(registerInterceptors, func(item s, _ int) grpc.UnaryClientInterceptor {
									return item.UnaryClientInterceptor
								})...,
							),
						}

						userConn, err := grpc.Dial(defaultListener.Addr().String(), opts...)
						Expect(err).To(Succeed())
						mu.Lock()
						DeferCleanup(userConn.Close)
						mu.Unlock()

						Expect(err).To(Succeed())
						simpleClientUser := testgrpc.NewSimpleServiceClient(userConn)

						_, err = simpleClientUser.Increment(ctx, &testgrpc.IncrementRequest{
							Value: 1,
						})
						Expect(err).To(Succeed())
						ctxMetadata := caching.WithGrpcClientCaching(ctx, 5*time.Second)

						md, ok := metadata.FromOutgoingContext(ctxMetadata)
						Expect(ok).To(BeTrue())
						Expect(md.Get(caching.GrpcCacheControlHeader(caching.CacheTypeClient))).NotTo(HaveLen(0))

						value, err := simpleClientUser.GetValue(
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
								_, err := simpleClientUser.Increment(ctx, &testgrpc.IncrementRequest{
									Value: 1,
								})
								Expect(err).To(Succeed())
							}()
							aggregate++
						}
						wg.Wait()
						By("verifying the aggregated value is in the server")
						value, err = simpleServer.GetValue(ctx, &emptypb.Empty{})
						Expect(err).To(Succeed())
						Expect(value.Value).To(Equal(int64(aggregate)))

						By("verifying the value in the cache hasn't expired")
						value, err = simpleClientUser.GetValue(
							ctx,
							&emptypb.Empty{},
						)
						Expect(err).To(Succeed())
						Expect(value.Value).To(Equal(int64(1)))

					}()
				}
				wg.Wait()
			})
		})
	})
}
