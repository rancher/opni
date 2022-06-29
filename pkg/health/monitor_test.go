package health_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Monitor", func() {
	When("creating a new monitor", func() {
		It("should have no current health status", func() {
			monitor := health.NewMonitor()
			healthStatus := monitor.GetHealthStatus("nonexistent")
			Expect(healthStatus).NotTo(BeNil())
			Expect(healthStatus.Health).To(BeNil())
			Expect(healthStatus.Status).To(BeNil())
		})
	})
	It("should listen to a health status updater", func() {
		agent1 := &test.HealthStore{}
		agent1.SetHealth(&corev1.Health{Ready: false, Conditions: []string{"foo"}})
		listener := newFastListener()
		defer listener.Close()

		agentCtx, agentCa := context.WithCancel(context.Background())
		monitorCtx, monitorCa := context.WithCancel(context.Background())
		defer monitorCa()
		go listener.HandleConnection(test.ContextWithAuthorizedID(agentCtx, "agent1"), agent1)
		monitor := health.NewMonitor()
		go monitor.Run(monitorCtx, listener)

		Eventually(func() *corev1.HealthStatus {
			return monitor.GetHealthStatus("agent1")
		}).Should(Equal(&corev1.HealthStatus{
			Health: &corev1.Health{
				Ready:      false,
				Conditions: []string{"foo"},
			},
			Status: &corev1.Status{
				Connected: true,
			},
		}))

		agent1.SetHealth(&corev1.Health{Ready: true})

		Eventually(func() *corev1.HealthStatus {
			return monitor.GetHealthStatus("agent1")
		}).Should(Equal(&corev1.HealthStatus{
			Health: &corev1.Health{
				Ready: true,
			},
			Status: &corev1.Status{
				Connected: true,
			},
		}))

		agentCa()

		Eventually(func() *corev1.HealthStatus {
			return monitor.GetHealthStatus("agent1")
		}).Should(Equal(&corev1.HealthStatus{
			Health: &corev1.Health{
				Ready: false,
			},
			Status: &corev1.Status{
				Connected: false,
			},
		}))
	})
	It("should stop if the updater's health or status channels are closed", func() {
		monitor := health.NewMonitor()
		listener := health.NewListener()
		stopped := make(chan struct{})
		go func() {
			monitor.Run(context.Background(), listener)
			close(stopped)
		}()

		Consistently(stopped).ShouldNot(BeClosed())
		listener.Close()
		Eventually(stopped).Should(BeClosed())
	})
})
