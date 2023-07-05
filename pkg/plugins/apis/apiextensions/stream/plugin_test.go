package stream_test

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/kralicky/totem"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/test"
	mock_stream "github.com/rancher/opni/pkg/test/mock/apiextensions/stream"
	mock_ext "github.com/rancher/opni/pkg/test/mock/ext"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/future"
)

var _ = Describe("Stream API Extensions Plugin", Ordered, Label("unit"), func() {
	var pluginImpl *mock_stream.MockStreamAPIExtensionWithHandlers
	var pluginSet test.TestPluginSet
	var agentMode, gatewayMode meta.SchemeFunc
	var pluginLoader *plugins.PluginLoader

	var ctrl *gomock.Controller
	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		pluginImpl = mock_stream.NewMockStreamAPIExtensionWithHandlers(ctrl)
		agentMode = func(ctx context.Context) meta.Scheme {
			agentScheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
			agentScheme.Add(stream.StreamAPIExtensionPluginID, stream.NewAgentPlugin(pluginImpl))
			return agentScheme
		}
		gatewayMode = func(ctx context.Context) meta.Scheme {
			gatewayScheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))
			gatewayScheme.Add(stream.StreamAPIExtensionPluginID, stream.NewGatewayPlugin(pluginImpl))
			return gatewayScheme
		}

		pluginSet = test.TestPluginSet{}

		pluginSet.EnablePlugin("stream_test", "testAgentPlugin", meta.ModeAgent, agentMode)
		pluginSet.EnablePlugin("stream_test", "testGatewayPlugin", meta.ModeGateway, gatewayMode)

		pluginLoader = plugins.NewPluginLoader()
	})
	JustBeforeEach(func(ctx SpecContext) {
		pluginSet.LoadPlugins(ctx, pluginLoader, meta.ModeAgent)
	})

	Context("Agent mode", func() {
		var loadedPlugins chan lo.Tuple3[types.StreamAPIExtensionPlugin, meta.PluginMeta, *grpc.ClientConn]
		var ext2MockA mock_ext.MockExt2ServerImpl
		var ext2MockB mock_ext.MockExt2ServerImpl
		BeforeEach(func() {
			loadedPlugins = make(chan lo.Tuple3[types.StreamAPIExtensionPlugin, meta.PluginMeta, *grpc.ClientConn], 1)
			pluginLoader.Hook(hooks.OnLoadMC(func(p types.StreamAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
				loadedPlugins <- lo.T3(p, md, cc)
			}))
			ext2MockA = mock_ext.MockExt2ServerImpl{
				MockExt2Server: mock_ext.NewMockExt2Server(ctrl),
			}
			ext2MockB = mock_ext.MockExt2ServerImpl{
				MockExt2Server: mock_ext.NewMockExt2Server(ctrl),
			}
			pluginImpl.EXPECT().
				StreamServers().
				DoAndReturn(func() []stream.Server {
					return []stream.Server{
						{
							Desc: &ext.Ext2_ServiceDesc,
							Impl: ext2MockA,
						},
					}
				}).
				Times(1)
		})
		When("no errors occur", func() {
			var useStreamClientCalled chan struct{}
			BeforeEach(func() {
				useStreamClientCalled = make(chan struct{})
				pluginImpl.EXPECT().
					UseStreamClient(gomock.Any()).
					DoAndReturn(func(cc grpc.ClientConnInterface) {
						ext2Client := ext.NewExt2Client(cc)
						value := uuid.NewString()
						resp, err := ext2Client.Foo(context.Background(), &ext.FooRequest{
							Request: value,
						})
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.Response).To(Equal(fmt.Sprintf("received from B: %s", value)))
						close(useStreamClientCalled)
					}).
					Times(1)
				ext2MockA.EXPECT().
					Foo(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req *ext.FooRequest) (*ext.FooResponse, error) {
						return &ext.FooResponse{
							Response: fmt.Sprintf("received from A: %s", req.Request),
						}, nil
					}).
					Times(1)
				ext2MockB.EXPECT().
					Foo(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req *ext.FooRequest) (*ext.FooResponse, error) {
						return &ext.FooResponse{
							Response: fmt.Sprintf("received from B: %s", req.Request),
						}, nil
					}).
					Times(1)
			})
			It("should handle the stream correctly", func(ctx context.Context) {
				var lp lo.Tuple3[types.StreamAPIExtensionPlugin, meta.PluginMeta, *grpc.ClientConn]
				Eventually(loadedPlugins).Should(Receive(&lp))
				p, md, cc := lp.Unpack()

				Expect(p).NotTo(BeNil())
				Expect(md).NotTo(BeNil())
				Expect(cc).NotTo(BeNil())
				streamClient := streamv1.NewStreamClient(cc)

				ctx, ca := context.WithCancel(ctx)

				stream, err := streamClient.Connect(ctx)
				Expect(err).NotTo(HaveOccurred())
				streamMd, err := stream.Header()
				Expect(err).NotTo(HaveOccurred())
				Expect(streamMd).To(HaveKeyWithValue("x-correlation-id", HaveLen(1)))

				ts, err := totem.NewServer(stream)
				Expect(err).NotTo(HaveOccurred())

				ext.RegisterExt2Server(ts, ext2MockB)

				tc, errC := ts.Serve()
				f := future.NewFromChannel(errC)
				Expect(f.IsSet()).To(BeFalse())

				_, err = streamClient.Notify(ctx, &streamv1.StreamEvent{
					Type:          streamv1.EventType_DiscoveryComplete,
					CorrelationId: streamMd["x-correlation-id"][0],
				})
				Expect(err).NotTo(HaveOccurred())

				ext2Client := ext.NewExt2Client(tc)
				resp, err := ext2Client.Foo(ctx, &ext.FooRequest{
					Request: "foo",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Response).To(Equal("received from A: foo"))

				select {
				case <-useStreamClientCalled:
				case <-time.After(1 * time.Second):
					Fail("useStreamClient was not called")
				}
				ca()
				Expect(f.Get()).To(testutil.MatchStatusCode(codes.Canceled))
			})
			When("no correlation id is given to Notify", func() {
				It("should use the old behavior and notify the active stream without a check", func(ctx context.Context) {
					var lp lo.Tuple3[types.StreamAPIExtensionPlugin, meta.PluginMeta, *grpc.ClientConn]
					Eventually(loadedPlugins).Should(Receive(&lp))
					p, md, cc := lp.Unpack()

					Expect(p).NotTo(BeNil())
					Expect(md).NotTo(BeNil())
					Expect(cc).NotTo(BeNil())
					streamClient := streamv1.NewStreamClient(cc)

					ctx, ca := context.WithCancel(ctx)

					stream, err := streamClient.Connect(ctx)
					Expect(err).NotTo(HaveOccurred())
					ts, err := totem.NewServer(stream)
					Expect(err).NotTo(HaveOccurred())

					ext.RegisterExt2Server(ts, ext2MockB)

					tc, errC := ts.Serve()
					f := future.NewFromChannel(errC)
					Expect(f.IsSet()).To(BeFalse())

					_, err = streamClient.Notify(ctx, &streamv1.StreamEvent{
						Type: streamv1.EventType_DiscoveryComplete,
					})
					Expect(err).NotTo(HaveOccurred())

					ext2Client := ext.NewExt2Client(tc)
					resp, err := ext2Client.Foo(ctx, &ext.FooRequest{
						Request: "foo",
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.Response).To(Equal("received from A: foo"))

					select {
					case <-useStreamClientCalled:
					case <-time.After(1 * time.Second):
						Fail("useStreamClient was not called")
					}
					ca()
					Expect(f.Get()).To(testutil.MatchStatusCode(codes.Canceled))
				})
			})
		})
		When("the stream is disconnected before discovery is complete", func() {
			BeforeEach(func() {
				prev := stream.X_SetDiscoveryTimeout(1 * time.Second)
				DeferCleanup(func() {
					stream.X_SetDiscoveryTimeout(prev)
				})
			})
			It("should return an error", func(ctx context.Context) {
				var lp lo.Tuple3[types.StreamAPIExtensionPlugin, meta.PluginMeta, *grpc.ClientConn]
				Eventually(loadedPlugins).Should(Receive(&lp))
				p, md, cc := lp.Unpack()

				Expect(p).NotTo(BeNil())
				Expect(md).NotTo(BeNil())
				Expect(cc).NotTo(BeNil())
				streamClient := streamv1.NewStreamClient(cc)
				ctx, ca := context.WithCancel(ctx)
				errC := make(chan error, 1)
				_, err := streamClient.Connect(ctx, grpc.OnFinish(func(err error) {
					errC <- err
				}))
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(100 * time.Millisecond)
				ca()

				Eventually(errC, 1*time.Second, 1*time.Millisecond).
					Should(Receive(testutil.MatchStatusCode(codes.Canceled)))
			})
		})
		When("the stream times out before discovery is complete", func() {
			BeforeEach(func() {
				prev := stream.X_SetDiscoveryTimeout(0)
				DeferCleanup(func() {
					stream.X_SetDiscoveryTimeout(prev)
				})
			})
			It("should return an error", func(ctx context.Context) {
				var lp lo.Tuple3[types.StreamAPIExtensionPlugin, meta.PluginMeta, *grpc.ClientConn]
				Eventually(loadedPlugins).Should(Receive(&lp))
				p, md, cc := lp.Unpack()

				Expect(p).NotTo(BeNil())
				Expect(md).NotTo(BeNil())
				Expect(cc).NotTo(BeNil())
				streamClient := streamv1.NewStreamClient(cc)
				stream, err := streamClient.Connect(ctx)
				Expect(err).NotTo(HaveOccurred())
			receive:
				select {
				case resp := <-lo.Async2(stream.Recv):
					_, err := resp.Unpack()
					if err == nil {
						goto receive
					}
					Expect(err).To(testutil.MatchStatusCode(codes.DeadlineExceeded, ContainSubstring("stream client discovery timed out")))
				case <-time.After(10 * time.Millisecond):
					Fail("stream.Recv() should have returned")
				}
			})
		})
		When("the stream encounters an error before discovery is complete", func() {
			BeforeEach(func() {
				prev := stream.X_SetDiscoveryTimeout(100 * time.Millisecond)
				DeferCleanup(func() {
					stream.X_SetDiscoveryTimeout(prev)
				})
			})
			It("should return an error", func(ctx context.Context) {
				var lp lo.Tuple3[types.StreamAPIExtensionPlugin, meta.PluginMeta, *grpc.ClientConn]
				Eventually(loadedPlugins).Should(Receive(&lp))
				p, md, cc := lp.Unpack()

				Expect(p).NotTo(BeNil())
				Expect(md).NotTo(BeNil())
				Expect(cc).NotTo(BeNil())
				streamClient := streamv1.NewStreamClient(cc)

				stream, err := streamClient.Connect(ctx)
				Expect(err).NotTo(HaveOccurred())

				ts, err := totem.NewServer(stream)
				Expect(err).NotTo(HaveOccurred())

				_, errC := ts.Serve()
				f := future.NewFromChannel(errC)
				Expect(f.IsSet()).To(BeFalse())

				// wait for longer than the discovery timeout. we are testing a separate
				// code path than the previous test case
				time.Sleep(200 * time.Millisecond)
				cc.Close()

				err = f.Get()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
