package management_test

import (
	"bytes"
	"context"
	"net/http"

	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/test"
	"github.com/kralicky/opni-monitoring/pkg/util"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server", Ordered, func() {
	var tv *testVars
	BeforeAll(setupManagementServer(&tv))
	It("should return valid cert info", func() {
		info, err := tv.client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Chain).To(HaveLen(1))
		Expect(info.Chain[0].Subject).To(Equal("CN=leaf"))
	})
	It("should serve the swagger.json endpoint", func() {
		resp, err := http.Get(tv.httpEndpoint + "/swagger.json")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body := new(bytes.Buffer)
		_, err = body.ReadFrom(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(body.String()).To(ContainSubstring(`"swagger": "2.0"`))
	})
	It("should handle configuration errors", func() {
		By("checking required options are set")
		Expect(func() {
			management.NewServer(&v1beta1.ManagementSpec{},
				management.ClusterStore(test.NewTestClusterStore(tv.ctrl)),
				management.RBACStore(test.NewTestRBACStore(tv.ctrl)))
		}).To(PanicWith("token store is required"))
		Expect(func() {
			management.NewServer(&v1beta1.ManagementSpec{},
				management.ClusterStore(test.NewTestClusterStore(tv.ctrl)),
				management.TokenStore(test.NewTestTokenStore(context.Background(), tv.ctrl)))
		}).To(PanicWith("rbac store is required"))
		Expect(func() {
			management.NewServer(&v1beta1.ManagementSpec{},
				management.RBACStore(test.NewTestRBACStore(tv.ctrl)),
				management.TokenStore(test.NewTestTokenStore(context.Background(), tv.ctrl)))
		}).To(PanicWith("cluster store is required"))

		By("checking required config fields are set")
		conf := &v1beta1.ManagementSpec{
			HTTPListenAddress: "127.0.0.1:0",
		}
		server := management.NewServer(conf,
			management.ClusterStore(test.NewTestClusterStore(tv.ctrl)),
			management.RBACStore(test.NewTestRBACStore(tv.ctrl)),
			management.TokenStore(test.NewTestTokenStore(context.Background(), tv.ctrl)),
		)
		Expect(server.ListenAndServe(context.Background())).To(MatchError("GRPCListenAddress not configured"))

		By("checking that invalid config fields cause errors")
		conf.GRPCListenAddress = "foo://bar"
		Expect(server.ListenAndServe(context.Background())).To(MatchError(util.ErrUnsupportedProtocolScheme))
	})
})
