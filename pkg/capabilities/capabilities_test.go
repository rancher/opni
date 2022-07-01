package capabilities_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Capabilities", Label("unit"), func() {
	It("should allow creating backends from grpc clients", func() {
		client := test.NewTestCapabilityBackendClient(ctrl, &test.CapabilityInfo{
			Name:              "test",
			CanInstall:        true,
			InstallerTemplate: "foo",
		})
		backend := capabilities.NewBackend(client)
		Expect(backend.InstallerTemplate()).To(Equal("foo"))
		Expect(backend.CanInstall()).To(Succeed())
		Expect(backend.Install(nil)).To(Succeed())
		Expect(backend.Uninstall(nil)).To(Succeed())

		client = test.NewTestCapabilityBackendClient(ctrl, &test.CapabilityInfo{
			Name:              "test2",
			CanInstall:        false,
			InstallerTemplate: "bar",
		})
		backend = capabilities.NewBackend(client)
		Expect(backend.InstallerTemplate()).To(Equal("bar"))
		Expect(backend.CanInstall()).NotTo(Succeed())
	})

	It("should check for installed capabilities in supported types", func() {
		resource := &testResourceWithMetadata{}
		c1 := testCapability("c1")
		c2 := testCapability("c2")
		c3 := testCapability("c3")
		resource.SetCapabilities([]testCapability{c1, c2})
		Expect(capabilities.Has(resource, c1)).To(BeTrue())
		Expect(capabilities.Has(resource, c2)).To(BeTrue())
		Expect(capabilities.Has(resource, c3)).To(BeFalse())
	})

	It("should construct capability objects for well-known types", func() {
		clusterCap := capabilities.Cluster("test")
		Expect(clusterCap.Name).To(Equal("test"))

		tokenCap := capabilities.JoinExistingCluster.For(&corev1.Reference{
			Id: "foo",
		})
		Expect(tokenCap.Type).To(BeEquivalentTo(capabilities.JoinExistingCluster))
		Expect(tokenCap.Reference.Id).To(Equal("foo"))
	})
})
