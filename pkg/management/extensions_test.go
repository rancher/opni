package management_test

import (
	"context"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/test"
	example "github.com/rancher/opni-monitoring/plugins/example/pkg"
)

var _ = Describe("Extensions", Ordered, func() {
	var tv *testVars
	BeforeAll(func() {
		ctx, ca := context.WithCancel(context.Background())
		cc := test.NewTestExamplePlugin(ctx)
		plugins.Load(cc)
		extensions := plugins.DispenseAll(apiextensions.ManagementAPIExtensionPluginID)
		setupManagementServer(&tv, management.APIExtensions(extensions))()
		DeferCleanup(ca)
	})
	It("should load API extensions", func() {
		extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(extensions.Items).To(HaveLen(1))
		Expect(extensions.Items[0].ServiceDesc.GetName()).To(Equal("ExampleAPIExtension"))
		Expect(extensions.Items[0].ServiceDesc.Method[0].GetName()).To(Equal("Echo"))
		Expect(extensions.Items[0].Rules).To(HaveLen(1))
		Expect(extensions.Items[0].Rules[0].Http.GetPost()).To(Equal("/echo"))
		Expect(extensions.Items[0].Rules[0].Http.GetBody()).To(Equal("*"))
	})
	It("should forward gRPC calls to the plugin", func() {
		cc, err := grpc.Dial(tv.grpcEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		Expect(err).NotTo(HaveOccurred())
		defer cc.Close()
		client := example.NewExampleAPIExtensionClient(cc)
		resp, err := client.Echo(context.Background(), &example.EchoRequest{
			Message: "hello",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Message).To(Equal("hello"))
	})
	It("should forward HTTP calls to the plugin", func() {
		resp, err := http.Post(tv.httpEndpoint+"/ExampleAPIExtension/echo",
			"application/json", strings.NewReader(`{"message": "hello"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(200))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		Expect(string(body)).To(Equal(`{"message":"hello"}`))
	})
})
