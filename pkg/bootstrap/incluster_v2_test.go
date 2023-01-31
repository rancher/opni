package bootstrap_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/test"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("In-Cluster Bootstrap V2", Ordered, func() {
	var gatewayEndpoint, managementEndpoint string
	var managementClient managementv1.ManagementClient
	BeforeAll(func() {
		env := test.Environment{
			TestBin: "../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())

		conf := env.GatewayConfig()
		gatewayEndpoint = strings.TrimPrefix(conf.Spec.GRPCListenAddress, "tcp://")
		managementEndpoint = strings.TrimPrefix(conf.Spec.Management.GRPCListenAddress, "tcp://")

		managementClient = env.NewManagementClient()
		DeferCleanup(env.Stop)
	})
	It("should bootstrap using the management endpoint", func() {
		bootstrapper := &bootstrap.InClusterBootstrapperV2{
			GatewayEndpoint:    gatewayEndpoint,
			ManagementEndpoint: managementEndpoint,
		}

		By("checking tokens")
		tokens, err := managementClient.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(tokens.Items).To(HaveLen(0))

		By("bootstrapping")
		_, err = bootstrapper.Bootstrap(context.Background(), test.NewTestIdentProvider(ctrl, "foo"))
		Expect(err).NotTo(HaveOccurred())

		By("checking tokens after bootstrap")
		tokens, err = managementClient.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(tokens.Items).To(HaveLen(1))

		By("finalizing")
		err = bootstrapper.Finalize(context.Background())
		Expect(err).NotTo(HaveOccurred())

		By("checking tokens after finalize")
		tokens, err = managementClient.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(tokens.Items).To(HaveLen(0))

		By("checking the cluster")
		cluster, err := managementClient.GetCluster(context.Background(), &corev1.Reference{
			Id: "foo",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cluster).NotTo(BeNil())
		Expect(cluster.Id).To(Equal("foo"))
		Expect(cluster.Metadata).NotTo(BeNil())
		Expect(cluster.Metadata.Capabilities).To(BeEmpty())
		Expect(cluster.Metadata.Labels).To(HaveKeyWithValue(corev1.NameLabel, "local"))
	})
	Context("error handling", func() {
		When("the gateway endpoint is missing", func() {
			It("should error", func() {
				bootstrapper := &bootstrap.InClusterBootstrapperV2{
					ManagementEndpoint: managementEndpoint,
				}
				_, err := bootstrapper.Bootstrap(context.Background(), test.NewTestIdentProvider(ctrl, "foo"))
				Expect(err).To(HaveOccurred())
			})
		})
		When("the management endpoint is missing", func() {
			It("should error", func() {
				bootstrapper := &bootstrap.InClusterBootstrapperV2{
					GatewayEndpoint: gatewayEndpoint,
				}
				_, err := bootstrapper.Bootstrap(context.Background(), test.NewTestIdentProvider(ctrl, "foo"))
				Expect(err).To(HaveOccurred())
			})
		})
		When("finalizing after an error occurs", func() {
			It("should be a no-op", func() {
				bootstrapper := &bootstrap.InClusterBootstrapperV2{}
				_, err := bootstrapper.Bootstrap(context.Background(), test.NewTestIdentProvider(ctrl, "foo"))
				Expect(err).To(HaveOccurred())
				err = bootstrapper.Finalize(context.Background())
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
