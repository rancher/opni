package web_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Agents", Ordered, Label("web"), func() {
	var mgmtClient managementv1.ManagementClient
	BeforeAll(func() {
		mgmtClient = env.NewManagementClient()
	})
	When("starting up for the first time", func() {
		It("should show an empty agents list", func() {
			b.Navigate(url)
			Eventually(".no-rows > td:nth-child(1) > span:nth-child(1)").Should(b.HaveInnerText("There are no rows to show."))
		})
	})
	var agent1Ctx, agent2Ctx context.Context
	var agent1Cancel, agent2Cancel context.CancelFunc
	When("adding agents", func() {
		It("should show the agents in the list", func() {
			agent1Ctx, agent1Cancel = context.WithCancel(env.Context())
			agent2Ctx, agent2Cancel = context.WithCancel(env.Context())

			Expect(env.BootstrapNewAgent("agent1", test.WithContext(agent1Ctx), test.WithLocalAgent())).To(Succeed())
			Expect(env.BootstrapNewAgent("agent2", test.WithContext(agent2Ctx))).To(Succeed())

			b.Navigate(url)

			Eventually("tr.main-row:nth-child(1) > td:nth-child(2) > div:nth-child(1) > span:nth-child(1)").Should(And(b.HaveClass("bg-success"), b.HaveInnerText("Ready")))
			Eventually("tr.main-row:nth-child(2) > td:nth-child(2) > div:nth-child(1) > span:nth-child(1)").Should(And(b.HaveClass("bg-success"), b.HaveInnerText("Ready")))

			Eventually("tr.main-row:nth-child(1) > td:nth-child(3) > span:nth-child(1)").Should(b.HaveInnerText("agent1"))
			Eventually("tr.main-row:nth-child(2) > td:nth-child(3) > span:nth-child(1)").Should(b.HaveInnerText("agent2"))

			Eventually("tr.main-row:nth-child(1) > td:nth-child(4) > span:nth-child(1)").Should(b.HaveInnerText("agent1"))
			Eventually("tr.main-row:nth-child(2) > td:nth-child(4) > span:nth-child(1)").Should(b.HaveInnerText("agent2"))

			Eventually("tr.main-row:nth-child(1) > td:nth-child(5) > span:nth-child(1) > i").Should(b.HaveClass("icon-checkmark"))
			Eventually("tr.main-row:nth-child(2) > td:nth-child(5) > span:nth-child(1) > i").ShouldNot(b.Exist())
		})
	})

	When("stopping agents", func() {
		It("should show the agents as disconnected", func() {
			agent1Cancel()
			agent2Cancel()

			b.Navigate(url)

			Eventually("tr.main-row:nth-child(1) > td:nth-child(2) > div:nth-child(1) > span:nth-child(1)").Should(And(b.HaveClass("bg-error"), b.HaveInnerText("Disconnected")))
			Eventually("tr.main-row:nth-child(2) > td:nth-child(2) > div:nth-child(1) > span:nth-child(1)").Should(And(b.HaveClass("bg-error"), b.HaveInnerText("Disconnected")))
		})
	})

	When("deleting agents", func() {
		It("should remove the agents from the list", func() {
			_, err := mgmtClient.DeleteCluster(context.Background(), &corev1.Reference{Id: "agent1"})
			Expect(err).ToNot(HaveOccurred())
			_, err = mgmtClient.DeleteCluster(context.Background(), &corev1.Reference{Id: "agent2"})
			Expect(err).ToNot(HaveOccurred())

			b.Navigate(url)

			Eventually(".no-rows > td:nth-child(1) > span:nth-child(1)").Should(b.HaveInnerText("There are no rows to show."))
		})
	})
})
