package management_test

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/jhump/protoreflect/grpcreflect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/test"
	mock_apiextensions "github.com/rancher/opni-monitoring/pkg/test/mock/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/test/testdata/plugins/ext"
)

type apiExtensionSrvImpl struct {
	apiextensions.UnsafeManagementAPIExtensionServer
	*mock_apiextensions.MockManagementAPIExtensionServer
}

var _ = Describe("Extensions", Ordered, func() {
	tv := &testVars{}
	BeforeAll(func() {
		tv.ctrl = gomock.NewController(GinkgoT())
		apiextSrv := &apiExtensionSrvImpl{
			MockManagementAPIExtensionServer: mock_apiextensions.NewMockManagementAPIExtensionServer(tv.ctrl),
		}
		serviceDescriptor, err := grpcreflect.LoadServiceDescriptor(&ext.Ext_ServiceDesc)
		Expect(err).NotTo(HaveOccurred())

		apiextSrv.EXPECT().
			Descriptor(gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
				fqn := serviceDescriptor.GetFullyQualifiedName()
				sd := serviceDescriptor.AsServiceDescriptorProto()
				sd.Name = &fqn
				return sd, nil
			})
		cc := test.NewApiExtensionTestPlugin(apiextSrv, &ext.Ext_ServiceDesc, &ext.ExtServerImpl{})

		plugins.Load(cc)
		extensions := plugins.DispenseAll(apiextensions.ManagementAPIExtensionPluginID)
		setupManagementServer(&tv, management.APIExtensions(extensions))()
	})
	It("should load API extensions", func() {
		extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(extensions.Items).To(HaveLen(1))
		Expect(extensions.Items[0].ServiceDesc.GetName()).To(Equal("Ext"))
		Expect(extensions.Items[0].ServiceDesc.Method[0].GetName()).To(Equal("Foo"))
		Expect(extensions.Items[0].Rules).To(HaveLen(5))
		Expect(extensions.Items[0].Rules[0].Http.GetPost()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[0].Http.GetBody()).To(Equal("bar"))
		Expect(extensions.Items[0].Rules[1].Http.GetGet()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[2].Http.GetPut()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[2].Http.GetBody()).To(Equal("bar"))
		Expect(extensions.Items[0].Rules[3].Http.GetDelete()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[4].Http.GetPatch()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[4].Http.GetBody()).To(Equal("bar"))
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
			Bar: "hello",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Baz).To(Equal("HELLO"))
	})
	It("should forward HTTP calls to the plugin", func() {
		resp, err := http.Post(tv.httpEndpoint+"/Ext/foo",
			"application/json", strings.NewReader(`{"bar": "hello"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(200))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		Expect(string(body)).To(Equal(`{"baz":"HELLO"}`))
	})
})
