package web_test

import (
	"context"
	"fmt"
	"time"

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
			b.Navigate(webUrl + "/agents")
			Eventually(Table()).Should(HaveNoRows())
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

			time.Sleep(1 * time.Second)

			b.Navigate(webUrl + "/agents")

			Eventually(Table().Row(1)).Should(MatchCells(CheckBox(), HaveReadyBadge(), b.HaveInnerText("agent1"), b.HaveInnerText("agent1"), HaveCheckmark()))
			Eventually(Table().Row(2)).Should(MatchCells(CheckBox(), HaveReadyBadge(), b.HaveInnerText("agent2"), b.HaveInnerText("agent2"), Not(HaveCheckmark())))
		})
	})

	When("assigning friendly names to the agents", func() {
		It("should display the friendly names in the table", func() {
			for i, id := range []string{"agent1", "agent2"} {
				By("assigning a friendly name to " + id)
				cluster, err := mgmtClient.GetCluster(context.Background(), &corev1.Reference{Id: id})
				Expect(err).NotTo(HaveOccurred())
				cluster.Metadata.Labels[corev1.NameLabel] = fmt.Sprintf("test-%d", i+1)
				_, err = mgmtClient.EditCluster(context.Background(), &managementv1.EditClusterRequest{
					Cluster: cluster.Reference(),
					Labels:  cluster.Metadata.Labels,
				})
				Expect(err).NotTo(HaveOccurred())
			}

			By("refreshing the page")
			b.Navigate(webUrl + "/agents")

			Eventually(Table().Row(1)).Should(MatchCells(CheckBox(), HaveReadyBadge(), b.HaveInnerText("test-1"), b.HaveInnerText("agent1"), HaveCheckmark()))
			Eventually(Table().Row(2)).Should(MatchCells(CheckBox(), HaveReadyBadge(), b.HaveInnerText("test-2"), b.HaveInnerText("agent2"), Not(HaveCheckmark())))
		})
	})

	When("stopping agents", func() {
		It("should show the agents as disconnected", func() {
			agent1Cancel()
			agent2Cancel()

			b.Navigate(webUrl + "/agents")

			Eventually(Table().Row(1).Col(2)).Should(HaveDisconnectedBadge())
			Eventually(Table().Row(2).Col(2)).Should(HaveDisconnectedBadge())
		})
	})

	When("deleting agents", func() {
		It("should remove the agents from the list", func() {
			_, err := mgmtClient.DeleteCluster(context.Background(), &corev1.Reference{Id: "agent1"})
			Expect(err).NotTo(HaveOccurred())
			_, err = mgmtClient.DeleteCluster(context.Background(), &corev1.Reference{Id: "agent2"})
			Expect(err).NotTo(HaveOccurred())

			b.Navigate(webUrl + "/agents")

			Eventually(Table()).Should(HaveNoRows())
		})
	})
})
