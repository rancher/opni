package bootstrap_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/test"
	mock_ident "github.com/rancher/opni/pkg/test/mock/ident"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("In-Cluster Bootstrap", Ordered, Label("integration"), func() {
	var gatewayEndpoint, managementEndpoint string
	var managementClient managementv1.ManagementClient
	BeforeAll(func() {
		env := test.Environment{}
		Expect(env.Start()).To(Succeed())

		conf := env.GatewayConfig()
		gatewayEndpoint = strings.TrimPrefix(conf.Spec.GRPCListenAddress, "tcp://")
		managementEndpoint = strings.TrimPrefix(conf.Spec.Management.GRPCListenAddress, "tcp://")

		managementClient = env.NewManagementClient()
		DeferCleanup(env.Stop)
	})
	It("should bootstrap using the management endpoint", func() {
		bootstrapper := &bootstrap.InClusterBootstrapper{
			Capability:         wellknown.CapabilityExample,
			GatewayEndpoint:    gatewayEndpoint,
			ManagementEndpoint: managementEndpoint,
		}

		By("checking tokens")
		tokens, err := managementClient.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).ToNot(HaveOccurred())
		Expect(tokens.Items).To(HaveLen(0))

		By("bootstrapping")
		_, err = bootstrapper.Bootstrap(context.Background(), mock_ident.NewTestIdentProvider(ctrl, "foo"))
		Expect(err).NotTo(HaveOccurred())

		By("checking tokens after bootstrap")
		tokens, err = managementClient.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).ToNot(HaveOccurred())
		Expect(tokens.Items).To(HaveLen(1))

		By("finalizing")
		err = bootstrapper.Finalize(context.Background())
		Expect(err).NotTo(HaveOccurred())

		By("checking tokens after finalize")
		tokens, err = managementClient.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).ToNot(HaveOccurred())
		Expect(tokens.Items).To(HaveLen(0))
	})
	Context("error handling", func() {
		When("the gateway endpoint is missing", func() {
			It("should error", func() {
				bootstrapper := &bootstrap.InClusterBootstrapper{
					Capability:         wellknown.CapabilityExample,
					ManagementEndpoint: managementEndpoint,
				}
				_, err := bootstrapper.Bootstrap(context.Background(), mock_ident.NewTestIdentProvider(ctrl, "foo"))
				Expect(err).To(HaveOccurred())
			})
		})
		When("the management endpoint is missing", func() {
			It("should error", func() {
				bootstrapper := &bootstrap.InClusterBootstrapper{
					Capability:      wellknown.CapabilityExample,
					GatewayEndpoint: gatewayEndpoint,
				}
				_, err := bootstrapper.Bootstrap(context.Background(), mock_ident.NewTestIdentProvider(ctrl, "foo"))
				Expect(err).To(HaveOccurred())
			})
		})
		When("finalizing after an error occurs", func() {
			It("should be a no-op", func() {
				bootstrapper := &bootstrap.InClusterBootstrapper{}
				_, err := bootstrapper.Bootstrap(context.Background(), mock_ident.NewTestIdentProvider(ctrl, "foo"))
				Expect(err).To(HaveOccurred())
				err = bootstrapper.Finalize(context.Background())
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
