package stream_test

import (
	"context"
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/kralicky/totem"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"google.golang.org/grpc"

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
	var pluginImpl *mock_stream.MockStreamAPIExtension
	var pluginSet test.TestPluginSet
	var agentMode, gatewayMode meta.SchemeFunc
	var pluginLoader *plugins.PluginLoader

	var ctrl *gomock.Controller
	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		pluginImpl = mock_stream.NewMockStreamAPIExtension(ctrl)
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
		BeforeEach(func() {
			ext2mock := mock_ext.NewMockExt2Server(ctrl)
			pluginImpl.EXPECT().
				StreamServers().
				DoAndReturn(func() []stream.Server {
					return []stream.Server{
						{
							Desc: &ext.Ext2_ServiceDesc,
							Impl: &mock_ext.MockExt2ServerImpl{
								MockExt2Server: ext2mock,
							},
						},
					}
				}).
				Times(1)
			ext2mock.EXPECT().
				Foo(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, req *ext.FooRequest) (*ext.FooResponse, error) {
					return &ext.FooResponse{
						Response: fmt.Sprintf("received: %s", req.Request),
					}, nil
				}).
				Times(1)

			loadedPlugins = make(chan lo.Tuple3[types.StreamAPIExtensionPlugin, meta.PluginMeta, *grpc.ClientConn], 1)
			pluginLoader.Hook(hooks.OnLoadMC(func(p types.StreamAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
				loadedPlugins <- lo.T3(p, md, cc)
			}))
		})
		It("should load in agent mode", func(ctx SpecContext) {
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

			tc, errC := ts.Serve()
			f := future.NewFromChannel(errC)
			Expect(f.IsSet()).To(BeFalse())

			ext2Client := ext.NewExt2Client(tc)
			resp, err := ext2Client.Foo(ctx, &ext.FooRequest{
				Request: "foo",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Response).To(Equal("received: foo"))

			Expect(cc.Close()).To(Succeed())

			Expect(f.Get()).To(testutil.MatchStatusCode(grpc.ErrClientConnClosing))
		})
	})
})
