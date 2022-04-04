package management_test

import (
	"bytes"
	"context"
	"net/http"

	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/util"
	"google.golang.org/protobuf/types/known/emptypb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server", Ordered, Label(test.Unit, test.Slow), func() {
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
		By("checking required config fields are set")
		conf := &v1beta1.ManagementSpec{
			HTTPListenAddress: "127.0.0.1:0",
		}
		server := management.NewServer(context.Background(), conf, tv.coreDataSource)
		Expect(server.ListenAndServe()).To(MatchError("GRPCListenAddress not configured"))

		By("checking that invalid config fields cause errors")
		conf.GRPCListenAddress = "foo://bar"
		Expect(server.ListenAndServe()).To(MatchError(util.ErrUnsupportedProtocolScheme))
	})
})
