package gateway_test

import (
	"context"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/jhump/protoreflect/grpcreflect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	mock_apiextensions "github.com/rancher/opni/pkg/test/mock/apiextensions"
	mock_ext "github.com/rancher/opni/pkg/test/mock/ext"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type unaryExtensionSrvImpl struct {
	apiextensions.UnsafeUnaryAPIExtensionServer
	*mock_apiextensions.MockUnaryAPIExtensionServer
}

type extSrvImpl struct {
	ext.UnsafeExtServer
	*mock_ext.MockExtServer
}

type ext2SrvImpl struct {
	ext.UnsafeExt2Server
	*mock_ext.MockExt2Server
}

var _ = Describe("Extensions", Ordered, Label("slow"), func() {
	var tv *testVars
	var descriptorLogic func() (*descriptorpb.ServiceDescriptorProto, error)
	shouldLoadExt1 := atomic.NewBool(true)
	shouldLoadExt2 := atomic.NewBool(false)
	JustBeforeEach(func() {
		tv = &testVars{}
		pl := plugins.NewPluginLoader()
		tv.ctrl = gomock.NewController(GinkgoT())
		extSrv := mock_ext.NewMockExtServer(tv.ctrl)
		extSrv.EXPECT().
			Foo(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *ext.FooRequest) (*ext.FooResponse, error) {
				return &ext.FooResponse{
					Response: strings.ToUpper(req.Request),
				}, nil
			}).
			AnyTimes()
		ext2Srv := mock_ext.NewMockExt2Server(tv.ctrl)
		ext2Srv.EXPECT().
			Foo(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *ext.FooRequest) (*ext.FooResponse, error) {
				return &ext.FooResponse{
					Response: strings.ToLower(req.Request),
				}, nil
			}).
			AnyTimes()
		// Setup the gateway GRPC server
		setupGatewayGRPCServer(&tv, pl)()

		// Loading the plugins after installing the hooks ensures LoadOne will block
		// until all hooks return.
		if shouldLoadExt1.Load() {
			apiextSrv := &unaryExtensionSrvImpl{
				MockUnaryAPIExtensionServer: mock_apiextensions.NewMockUnaryAPIExtensionServer(tv.ctrl),
			}
			serviceDescriptor, err := grpcreflect.LoadServiceDescriptor(&ext.Ext_ServiceDesc)
			Expect(err).NotTo(HaveOccurred())
			apiextSrv.EXPECT().
				UnaryDescriptor(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
					if descriptorLogic != nil {
						return descriptorLogic()
					}
					fqn := serviceDescriptor.GetFullyQualifiedName()
					sd := serviceDescriptor.AsServiceDescriptorProto()
					sd.Name = &fqn
					return sd, nil
				})
			cc := test.NewApiExtensionUnaryTestPlugin(apiextSrv, &ext.Ext_ServiceDesc, &extSrvImpl{
				MockExtServer: extSrv,
			})
			pl.LoadOne(context.Background(), meta.PluginMeta{
				BinaryPath: "test1",
				GoVersion:  "test1",
				Module:     "test1",
			}, cc)
		}
		if shouldLoadExt2.Load() {
			apiextSrv2 := &unaryExtensionSrvImpl{
				MockUnaryAPIExtensionServer: mock_apiextensions.NewMockUnaryAPIExtensionServer(tv.ctrl),
			}
			serviceDescriptor2, err := grpcreflect.LoadServiceDescriptor(&ext.Ext2_ServiceDesc)
			Expect(err).NotTo(HaveOccurred())
			apiextSrv2.EXPECT().
				UnaryDescriptor(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
					if descriptorLogic != nil {
						return descriptorLogic()
					}
					fqn := serviceDescriptor2.GetFullyQualifiedName()
					sd := serviceDescriptor2.AsServiceDescriptorProto()
					sd.Name = &fqn
					return sd, nil
				})
			cc2 := test.NewApiExtensionUnaryTestPlugin(apiextSrv2, &ext.Ext2_ServiceDesc, &ext2SrvImpl{
				MockExt2Server: ext2Srv,
			})
			pl.LoadOne(context.Background(), meta.PluginMeta{
				BinaryPath: "test2",
				GoVersion:  "test2",
				Module:     "test2",
			}, cc2)
		}

		pl.Complete()
	})
	It("should forward gRPC calls to the plugin", func() {
		cc, err := grpc.Dial(tv.grpcEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(tv.interceptor.unaryClientInterceptor),
			grpc.WithBlock(),
		)
		Expect(err).NotTo(HaveOccurred())
		defer cc.Close()

		extClient := ext.NewExtClient(cc)
		resp, err := extClient.Foo(context.Background(), &ext.FooRequest{
			Request: "hello",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Response).To(Equal("HELLO"))
	})
})
