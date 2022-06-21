package management_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jhump/protoreflect/grpcreflect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/utils/pointer"

	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	mock_apiextensions "github.com/rancher/opni/pkg/test/mock/apiextensions"
	mock_ext "github.com/rancher/opni/pkg/test/mock/ext"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
)

type apiExtensionSrvImpl struct {
	apiextensions.UnsafeManagementAPIExtensionServer
	*mock_apiextensions.MockManagementAPIExtensionServer
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

		// Create the management server, which installs hooks into the plugin loader
		setupManagementServer(&tv, pl)()

		// Loading the plugins after installing the hooks ensures LoadOne will block
		// until all hooks return.
		if shouldLoadExt1.Load() {
			apiextSrv := &apiExtensionSrvImpl{
				MockManagementAPIExtensionServer: mock_apiextensions.NewMockManagementAPIExtensionServer(tv.ctrl),
			}
			serviceDescriptor, err := grpcreflect.LoadServiceDescriptor(&ext.Ext_ServiceDesc)
			Expect(err).NotTo(HaveOccurred())
			apiextSrv.EXPECT().
				Descriptor(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
					if descriptorLogic != nil {
						return descriptorLogic()
					}
					fqn := serviceDescriptor.GetFullyQualifiedName()
					sd := serviceDescriptor.AsServiceDescriptorProto()
					sd.Name = &fqn
					return sd, nil
				})
			cc := test.NewApiExtensionTestPlugin(apiextSrv, &ext.Ext_ServiceDesc, &extSrvImpl{
				MockExtServer: extSrv,
			})
			pl.LoadOne(context.Background(), meta.PluginMeta{
				BinaryPath: "test1",
				GoVersion:  "test1",
				Module:     "test1",
			}, cc)
		}

		if shouldLoadExt2.Load() {
			apiextSrv2 := &apiExtensionSrvImpl{
				MockManagementAPIExtensionServer: mock_apiextensions.NewMockManagementAPIExtensionServer(tv.ctrl),
			}
			serviceDescriptor2, err := grpcreflect.LoadServiceDescriptor(&ext.Ext2_ServiceDesc)
			Expect(err).NotTo(HaveOccurred())
			apiextSrv2.EXPECT().
				Descriptor(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
					if descriptorLogic != nil {
						return descriptorLogic()
					}
					fqn := serviceDescriptor2.GetFullyQualifiedName()
					sd := serviceDescriptor2.AsServiceDescriptorProto()
					sd.Name = &fqn
					return sd, nil
				})
			cc2 := test.NewApiExtensionTestPlugin(apiextSrv2, &ext.Ext2_ServiceDesc, &ext2SrvImpl{
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
	It("should load API extensions", func() {
		extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(extensions.Items).To(HaveLen(1))
		Expect(extensions.Items[0].ServiceDesc.GetName()).To(Equal("Ext"))
		Expect(extensions.Items[0].ServiceDesc.Method[0].GetName()).To(Equal("Foo"))
		Expect(extensions.Items[0].Rules).To(HaveLen(5))
		Expect(extensions.Items[0].Rules[0].Http.GetPost()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[0].Http.GetBody()).To(Equal("request"))
		Expect(extensions.Items[0].Rules[1].Http.GetGet()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[2].Http.GetPut()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[2].Http.GetBody()).To(Equal("request"))
		Expect(extensions.Items[0].Rules[3].Http.GetDelete()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[4].Http.GetPatch()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[4].Http.GetBody()).To(Equal("request"))
	})
	It("should forward gRPC calls to the plugin", func() {
		cc, err := grpc.Dial(tv.grpcEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		Expect(err).NotTo(HaveOccurred())
		defer cc.Close()
		client := ext.NewExtClient(cc)
		resp, err := client.Foo(context.Background(), &ext.FooRequest{
			Request: "hello",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Response).To(Equal("HELLO"))
	})
	It("should forward HTTP calls to the plugin", func() {
		tries := 10 // need to wait a bit for the server to become ready
		for {
			resp, err := http.Post(tv.httpEndpoint+"/Ext/foo",
				"application/json", strings.NewReader(`{"request": "hello"}`))
			if (err != nil || resp.StatusCode != 200) && tries > 0 {
				tries--
				time.Sleep(100 * time.Millisecond)
				continue
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(string(body)).To(Equal(`{"response":"HELLO"}`))
			break
		}
	})
	Context("error handling", func() {
		When("the plugin's Descriptor method returns an error", func() {
			BeforeEach(func() {
				descriptorLogic = func() (*descriptorpb.ServiceDescriptorProto, error) {
					return nil, fmt.Errorf("test error")
				}
				DeferCleanup(func() {
					descriptorLogic = nil
				})
			})
			It("should not load the api extension", func() {
				extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())
				Expect(extensions.Items).To(HaveLen(0))
			})
		})
	})
	When("the plugin is not serving the service returned from Descriptor", func() {
		BeforeEach(func() {
			descriptorLogic = func() (*descriptorpb.ServiceDescriptorProto, error) {
				return &descriptorpb.ServiceDescriptorProto{
					Name: pointer.String("NotExt"),
				}, nil
			}
			DeferCleanup(func() {
				descriptorLogic = nil
			})
		})
		It("should not load the api extension", func() {
			extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(extensions.Items).To(HaveLen(0))
		})
	})
	When("the service has no http rules", func() {
		BeforeEach(func() {
			shouldLoadExt1.Store(false)
			shouldLoadExt2.Store(true)
			DeferCleanup(func() {
				shouldLoadExt1.Store(true)
				shouldLoadExt2.Store(false)
			})
		})
		It("should load the extension, but not serve its api over http", func() {
			extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(extensions.Items).To(HaveLen(1))
			Expect(extensions.Items[0].ServiceDesc).NotTo(BeNil())
			Expect(extensions.Items[0].Rules).To(BeEmpty())
		})
	})
})
